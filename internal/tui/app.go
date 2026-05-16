package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/sync/errgroup"

	"github.com/ktrysmt/gh-reva/internal/api"
	"github.com/ktrysmt/gh-reva/internal/diff"
	"github.com/ktrysmt/gh-reva/internal/model"
	"github.com/ktrysmt/gh-reva/internal/theme"
)

type Model struct {
	client      api.Client
	target      *api.Target
	state       *model.AppState
	theme       *theme.Theme
	syntaxCache  *syntaxCache
	patchLinesC  patchLinesCache
	rowCache     *diffRowCache
	threadsCache *threadsViewCache
	width       int
	height      int
	err         error

	// prefetchedRef / prefetchedPath track the (ref, path) pair the
	// last prefetch Cmd targeted. Used by the Update-tail prefetch
	// trigger to avoid re-issuing the same fetch when the user
	// navigates back to a file they previously visited.
	prefetchedRef  string
	prefetchedPath string

	// Splash variants are picked once at NewModel time and held for the
	// life of the program so the loading view does not flicker through
	// designs during the few seconds of PR load. version is rendered
	// between the splash block and the spinner; empty → omitted.
	version      string
	splashLayout splashLayout
	splashArtIdx int

	// Per-pane render budgets, set by View() before delegating to the
	// pane renderers. Each pane uses these for width-aware wrapping
	// (Comments) or viewport sizing (Diff).
	paneWidthFiles     int
	paneHeightFiles    int
	paneWidthCommits   int
	paneHeightCommits  int
	paneWidthDiff      int
	paneHeightDiff     int
	paneWidthComments  int
	paneHeightComments int

	// syntaxExtensions maps file-extension suffixes (with leading dot,
	// e.g. ".j2") to chroma lexer names — sourced from reva.toml's
	// [syntax.extensions] table. Longest-suffix-match wins at lookup
	// time so ".html.j2" can shadow ".j2". Empty / nil means
	// "no override; use chroma's default extension matcher".
	syntaxExtensions map[string]string

	// commentsWidthPercent is the Comments column's share of total
	// terminal width — sourced from reva.toml's
	// [layout.comments_width_percent]. Zero / out-of-range falls back
	// to defaultCommentsWidthPercent inside splitColumnWidths.
	commentsWidthPercent int

	// editorPopupWidthPct / editorPopupHeightPct carry reva.toml's
	// [editor].popup_*_percent overrides used by buildEditorCmd when
	// the user is inside a tmux session. Zero / out-of-range falls
	// back to defaultEditorPopupPercent inside resolveEditorPopupPercent.
	editorPopupWidthPct  int
	editorPopupHeightPct int
}

func (m Model) Err() error { return m.err }

// SetDiffHeight pins the Diff viewport height. Used by the test-only
// --diff-height flag to make scroll assertions deterministic regardless of
// terminal size.
func (m *Model) SetDiffHeight(h int) { m.state.DiffViewport.Height = h }

// SetTheme installs the resolved color palette. Must be called before the
// first render. A nil theme is replaced with the builtin dark fallback so
// rendering code can rely on m.theme being non-nil.
func (m *Model) SetTheme(t *theme.Theme) {
	if t == nil {
		t, _ = theme.Resolve("")
	}
	m.theme = t
}

// SetVersion stores the version string rendered on the loading splash.
// Empty string suppresses the version line entirely. cmd/root.go passes
// the ldflag-injected version (e.g. "v0.4.2" or "dev").
func (m *Model) SetVersion(v string) { m.version = v }

// SetSyntaxExtensions installs the user's [syntax.extensions] override
// table (sourced from reva.toml). Each key is a filename suffix with
// leading dot (".j2", ".html.j2"); each value is a chroma lexer name
// or alias (yaml, jinja, html, …). Lookups happen on every diff cell
// so the map is read directly without copying. Pass nil / empty to
// clear the overrides.
func (m *Model) SetSyntaxExtensions(ext map[string]string) {
	m.syntaxExtensions = ext
}

// SetCommentsWidthPercent installs the user's
// [layout.comments_width_percent] override. Out-of-range / zero is
// tolerated — splitColumnWidths falls back to
// defaultCommentsWidthPercent when the value is outside
// [commentsWidthPercentMin, commentsWidthPercentMax]. Pass 0 to leave
// the default in effect.
func (m *Model) SetCommentsWidthPercent(p int) {
	m.commentsWidthPercent = p
}

// SetEditorPopupSize installs the user's [editor].popup_*_percent
// overrides used by the tmux display-popup branch of buildEditorCmd.
// Out-of-range / zero is tolerated per resolveEditorPopupPercent;
// callers can pass cfg.Editor.PopupWidthPercent / PopupHeightPercent
// directly without sanitizing. Has no effect outside tmux.
func (m *Model) SetEditorPopupSize(widthPct, heightPct int) {
	m.editorPopupWidthPct = widthPct
	m.editorPopupHeightPct = heightPct
}

func NewModel(client api.Client, target *api.Target) Model {
	t, _ := theme.Resolve("")
	return Model{
		client:       client,
		target:       target,
		state:        model.NewAppState(),
		theme:        t,
		syntaxCache:  &syntaxCache{},
		patchLinesC:  patchLinesCache{cache: map[string]*patchInfo{}},
		rowCache:     &diffRowCache{m: map[string][]string{}},
		threadsCache: &threadsViewCache{},
		splashLayout: chooseSplashLayout(),
		splashArtIdx: chooseSplashArt(),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(loadPRCmd(m.client, m.target), spinnerTickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.updateInner(msg)
	mm, _ := model.(Model)
	if pref := mm.maybePrefetchFileContents(); pref != nil {
		cmd = tea.Batch(cmd, pref)
		mm.prefetchedRef = mm.currentFileRef()
		mm.prefetchedPath = mm.state.SelectedFile
		return mm, cmd
	}
	return model, cmd
}

// maybePrefetchFileContents fires a background fetch of FileContents for
// the active (ref, path) when the user has navigated to a new file (or
// switched commit range) and contents aren't already cached. Returns nil
// when no fetch is needed — same-file frames, AllFilesPath, no client
// (tests) or empty path all short-circuit.
func (m Model) maybePrefetchFileContents() tea.Cmd {
	if m.client == nil || m.target == nil || m.state == nil || m.state.PR == nil {
		return nil
	}
	path := m.state.SelectedFile
	if path == "" || path == model.AllFilesPath {
		return nil
	}
	ref := m.currentFileRef()
	if ref == m.prefetchedRef && path == m.prefetchedPath {
		return nil
	}
	if m.state.FileContents != nil {
		if _, cached := m.state.FileContents[model.FileContentsKey{Ref: ref, Path: path}]; cached {
			return nil
		}
	}
	return fetchFileContentsCmd(m.client, m.target, ref, path)
}

func (m Model) updateInner(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Apply any new dimensions FIRST so the unconditional measureLayout
	// below sees the latest width / height. A single call here covers
	// every downstream branch — handleKey, handleMouse,
	// ScrollDiffToLineMsg etc. all read paneWidth* / paneHeight* off
	// the value-receiver model returned by the previous Update, so
	// once measured they stay measured until the next resize. View
	// still calls measureLayout for the very first frame (before any
	// Update has fired).
	if w, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = w.Width
		m.height = w.Height
	}
	m.measureLayout()
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case SpinnerTickMsg:
		m.state.LoadFrame++
		if m.state.LoadStage == model.LoadStageDone {
			return m, nil
		}
		return m, spinnerTickCmd()
	case PRLoadedMsg:
		m.state.PR = msg.PR
		m.state.DiffCache = msg.Diffs
		m.state.ViewerLogin = msg.ViewerLogin
		if len(msg.PR.Files) > 0 {
			// Cursor 0 is the synthetic "[*] All (N files)" row. Initial
			// landing is the All view so the splash hands off to a
			// PR-wide overview (concat diff across every file); users
			// drill into a single file with j / Shift+J / Enter. Keeping
			// FilesCursor and SelectedFile in sync at boot avoids the
			// confusing mid-state where the cursor sits on [*] while the
			// Diff/Commits/Comments columns still reflect files[0].
			m.state.SelectedFile = model.AllFilesPath
			m.state.FilesCursor = 0
		}
		m.state.LoadStage = model.LoadStageDone
		return m, nil
	case ScrollDiffToLineMsg:
		lines := m.patchLines()
		if len(lines) == 0 {
			return m, nil
		}
		bufIdx := bufferIndexForNewLine(lines, msg.NewLine)
		if bufIdx >= 0 {
			m.scrollDiffToLine(bufIdx, len(lines))
		}
		return m, nil
	case composeBodyMsg:
		cmd := m.applyComposeBody(msg)
		return m, cmd
	case composeSubmittedMsg:
		return m, m.applyComposeSubmitted(msg)
	case commentsRefreshedMsg:
		m.applyCommentsRefreshed(msg)
		return m, nil
	case fileContentsLoadedMsg:
		m.applyFileContentsLoaded(msg)
		return m, nil
	case ErrMsg:
		m.err = msg.Err
		return m, tea.Quit
	}
	return m, nil
}

// applyFileContentsLoaded stores the fetched file body under
// FileContents[(Ref, Path)] and invalidates any patchInfo cache slot
// that may now have stale (FileLines = nil → no synthetic) data.
// Errors are silent — FileContents stays unpopulated so the synthetic
// rows simply don't appear for this file (the user retains the raw
// diff). Surfacing every prefetch failure as a Notice would clobber
// the status bar on fixtures / repos that don't expose file contents,
// while user-initiated fetches (Enter on synthetic) already set a
// transient "fetching file contents…" Notice that the user can read.
func (m *Model) applyFileContentsLoaded(msg fileContentsLoadedMsg) {
	if m.state.Notice == "fetching file contents…" {
		m.state.Notice = ""
	}
	if msg.Err != nil {
		return
	}
	if m.state.FileContents == nil {
		m.state.FileContents = map[model.FileContentsKey][]string{}
	}
	m.state.FileContents[model.FileContentsKey{Ref: msg.Ref, Path: msg.Path}] = msg.Lines
	m.invalidatePatchInfoCacheForRef(msg.Ref, msg.Path)
}

func (m Model) View() string {
	if m.state.PR == nil {
		return m.loadingView(m.state.LoadFrame, m.state.LoadStage)
	}
	// Reserve the bottom 3 rows for the bordered status bar (CLAUDE.md
	// §4 #6) once the PR is loaded. The visual hint and the modal /
	// help close hint all ride the bar's middle row, so the previous
	// standalone `-- VISUAL --` banner is gone. statusBar() returns ""
	// when m.height <= statusBarRows, in which case bodyHeight stays
	// equal to m.height and the body uses the whole screen.
	bodyHeight := m.height
	if bodyHeight > statusBarRows {
		bodyHeight -= statusBarRows
	}
	statusBar := m.statusBar()
	if m.width <= 0 || bodyHeight < 8 {
		// Stacked fallback fires in two cases:
		//   1. Pre-WindowSize (m.width <= 0): smoke tests and the very
		//      first frame before bubbletea has measured the terminal.
		//   2. Tiny terminal (bodyHeight < 8): each pane box needs 4 rows
		//      of chrome (top border / title / divider / bottom border),
		//      so Files + Commits stacked vertically need >= 8 rows just
		//      for chrome. Below that, the bordered layout would emit
		//      more rows than bodyHeight allows and visibly overflow.
		//      Stacked rendering keeps every pane reachable in degenerate
		//      windows at the cost of layout fidelity.
		paneRows := []string{
			m.filesView(),
			m.commitsView(),
			m.diffView(),
		}
		if !m.state.CommentsHidden {
			paneRows = append(paneRows, m.commentsView())
		}
		body := strings.Join(paneRows, "\n\n")
		body = m.overlayModal(body)
		body = m.overlayHelp(body)
		body = m.overlayCompose(body)
		body = m.overlayConfirm(body)
		if statusBar != "" {
			return body + "\n" + statusBar
		}
		return body
	}

	// Populate per-pane render budgets on the local m so the pane
	// renderers below (filesView, diffView, …) see consistent widths.
	// Mouse handling (Update on tea.MouseMsg) re-runs measureLayout
	// before hit-testing because Bubbletea's value-receiver Update
	// gets a fresh m each tick that doesn't carry View's measurements.
	m.measureLayout()

	leftW, midW, rightW := splitColumnWidths(m.width, m.state.CommentsHidden, m.commentsWidthPercent)
	topH, bottomH := splitColumnHeights(bodyHeight)

	files := m.boxFromPaneView(m.filesView(), leftW, topH, model.PaneFiles)
	commits := m.boxFromPaneView(m.commitsView(), leftW, bottomH, model.PaneCommits)
	leftCol := lipgloss.JoinVertical(lipgloss.Left, files, commits)
	diffCol := m.boxFromPaneView(m.diffView(), midW, bodyHeight, model.PaneDiff)
	var body string
	if m.state.CommentsHidden {
		body = lipgloss.JoinHorizontal(lipgloss.Top, leftCol, diffCol)
	} else {
		commentsCol := m.boxFromPaneView(m.commentsView(), rightW, bodyHeight, model.PaneComments)
		body = lipgloss.JoinHorizontal(lipgloss.Top, leftCol, diffCol, commentsCol)
	}
	body = m.overlayModal(body)
	body = m.overlayHelp(body)
	body = m.overlayCompose(body)
	body = m.overlayConfirm(body)

	if statusBar != "" {
		return body + "\n" + statusBar
	}
	return body
}

// defaultCommentsWidthPercent is the built-in share of total terminal
// width allocated to the Comments column when the user has not set
// `layout.comments_width_percent` in reva.toml. Picked to give the
// Comments pane meaningful room on wide terminals without starving the
// Diff column on narrower ones.
const defaultCommentsWidthPercent = 35

// commentsWidthPercentRange is the honored interval for user overrides.
// Values outside this range fall back to the built-in default — the
// loader stays a thin TOML→struct adapter, so the clamp lives here
// alongside the consumer.
const (
	commentsWidthPercentMin = 10
	commentsWidthPercentMax = 70
)

// splitColumnWidths divides total terminal width across the three columns.
// `commentsPct` is the requested share for the Comments column (0..100);
// the caller is expected to pass either defaultCommentsWidthPercent or a
// validated user override. Out-of-range / zero values fall back to the
// default so callers don't need to pre-clamp.
//
// Files keeps its familiar fixed width on wide terminals (≥ 130) and
// scales proportionally below; the Diff column absorbs whatever's left
// after Files and Comments claim their share. Diff's mid-25 floor (the
// readable-Diff minimum that already lived in the 80 ≤ total < 130
// branch) is preserved even under aggressive percentage overrides on
// narrow terminals — the override is best-effort, not a guarantee.
//
// When commentsHidden is true the right column collapses to 0 and its
// width is added to the middle (Diff) column — Files / Commits keep
// their familiar widths so the layout transition is local to the right
// side of the screen.
func splitColumnWidths(total int, commentsHidden bool, commentsPct int) (left, mid, right int) {
	if commentsPct < commentsWidthPercentMin || commentsPct > commentsWidthPercentMax {
		commentsPct = defaultCommentsWidthPercent
	}
	if commentsHidden {
		// Reuse the visible-Comments left budget so Files / Commits don't
		// reflow when the toggle fires; the Diff pane absorbs everything
		// to the right of the left column.
		left, _, _ = splitColumnWidths(total, false, commentsPct)
		mid = total - left
		if mid < 1 {
			mid = 1
		}
		return
	}
	if total >= 130 {
		left = 42
		right = total * commentsPct / 100
		// Floor right so a low override doesn't squeeze Comments below
		// a readable width; the original 80–130 branch used 28 as its
		// floor and we mirror it here.
		if right < 28 {
			right = 28
		}
		mid = total - left - right
		if mid < 25 {
			// Diff floor (mirror of the narrow-terminal branch). Steal
			// from right rather than from left so the user-facing
			// Comments share is the elastic one.
			mid = 25
			right = total - left - mid
			if right < 1 {
				right = 1
			}
		}
		return
	}
	if total >= 80 {
		left = total / 4
		if left < 22 {
			left = 22
		}
		if left > 38 {
			left = 38
		}
		right = total * commentsPct / 100
		if right < 28 {
			right = 28
		}
		mid = total - left - right
		if mid < 25 {
			mid = 25
			over := left + mid + right - total
			right -= over
			if right < 22 {
				right = 22
				left = total - mid - right
			}
		}
		return
	}
	// Degenerate (<80 cols): keep something sensible.
	left = total / 4
	mid = total / 2
	right = total - left - mid
	if right < 1 {
		right = 1
	}
	return
}

// splitColumnHeights divides the body height between Files (top) and Commits
// (bottom). Top gets the larger half so file lists are easier to scan.
func splitColumnHeights(total int) (top, bottom int) {
	if total < 4 {
		return total, 0
	}
	top = (total + 1) / 2
	bottom = total - top
	return
}

func atLeast(n, floor int) int {
	if n < floor {
		return floor
	}
	return n
}

// boxFromPaneView lifts a pane renderer's "title\nbody" output into a
// bordered box with a horizontal divider under the title row. width and
// height are outer dimensions.
//
//	┌────────┐
//	│ title  │
//	├────────┤
//	│ body…  │
//	└────────┘
//
// pane identifies which pane this is so the active/inactive border color
// can be picked from the model's theme.
func (m Model) boxFromPaneView(view string, width, height int, pane model.PaneID) string {
	title, body := splitTitleBody(view)
	active := m.state.FocusedPane == pane
	border := m.theme.PaneBorderInactive
	if active {
		border = m.theme.PaneBorderActive
	}
	return renderPaneBox(title, body, width, height, border)
}

func splitTitleBody(s string) (string, string) {
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// renderPaneBox draws a bordered box. The border color is applied to every
// glyph (corners, horizontals, side bars). Title and body lines are passed
// through unchanged — pane renderers own their own coloring.
func renderPaneBox(title, body string, width, height int, border lipgloss.Color) string {
	innerW := atLeast(width-2, 1)
	contentRows := atLeast(height-4, 0)
	bar := strings.Repeat("─", innerW)
	side := fg("│", border)
	hr := fg(bar, border)

	var sb strings.Builder
	sb.WriteString(fg("┌"+bar+"┐", border) + "\n")
	sb.WriteString(side + padTrunc(title, innerW) + side + "\n")
	sb.WriteString(fg("├", border) + hr + fg("┤", border) + "\n")

	bodyLines := strings.Split(body, "\n")
	for i := 0; i < contentRows; i++ {
		line := ""
		if i < len(bodyLines) {
			line = bodyLines[i]
		}
		sb.WriteString(side + padTrunc(line, innerW) + side + "\n")
	}
	sb.WriteString(fg("└"+bar+"┘", border))
	return sb.String()
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// logoArt is the splash logo shown above the spinner during PR load.
// Sourced from logo.md at the repo root. Three glyphs encode shading:
// █ = brightest (LogoShade1), ▓ = mid (LogoShade2), ░ = dimmest (LogoShade3).
const logoArt = `          ▓▓▓▓▓▓▓▓▓▓
        ▓▓▓▓▓▓▓▓▓▓▓▓▓▓
      ▓▓▓▓▓▓▓▓▓▓▓▓▓▓░░▓▓
    ▓▓▓▓░░▓▓▓▓▓▓▓▓▓▓▓▓░░▓▓
    ▓▓░░▓▓▓▓░░▓▓▓▓░░▓▓▓▓░░
  ░░▓▓░░▓▓░░██░░▓▓▓▓░░▓▓░░░░
░░░░▓▓▓▓░░░░████░░░░▓▓░░▓▓░░░░
  ░░▓▓░░██░░██████░░██░░▓▓░░
    ▓▓░░██░░██████░░██░░▓▓
  ▓▓░░▓▓░░██████████░░▓▓░░▓▓`

// renderLogo returns the splash logo with each glyph colored from the active
// theme. The source rows in logoArt have different widths because the
// leading-space gradient encodes the dome curve; renderLogo right-pads
// every row to the widest row so per-row centering downstream preserves
// the dome's vertical axis. Same-shade runs are coalesced into one SGR
// span to bound escape overhead.
func renderLogo(th *theme.Theme) string {
	rows := strings.Split(logoArt, "\n")
	maxW := 0
	for _, r := range rows {
		if w := lipgloss.Width(r); w > maxW {
			maxW = w
		}
	}
	out := make([]string, len(rows))
	for i, row := range rows {
		var b strings.Builder
		runes := []rune(row)
		j := 0
		for j < len(runes) {
			c := runes[j]
			color := lipgloss.Color("")
			switch c {
			case '█':
				color = th.LogoShade1
			case '▓':
				color = th.LogoShade2
			case '░':
				color = th.LogoShade3
			}
			k := j + 1
			for k < len(runes) && runes[k] == c {
				k++
			}
			run := string(runes[j:k])
			if color == "" {
				b.WriteString(run)
			} else {
				b.WriteString(fg(run, color))
			}
			j = k
		}
		if pad := maxW - lipgloss.Width(row); pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
		out[i] = b.String()
	}
	return strings.Join(out, "\n")
}

// loadingView renders the splash + spinner shown until PRLoadedMsg
// arrives. The `stage` parameter is retained for backwards-compatibility
// with `loading_test.go` callers that pass model.LoadStagePR; the
// spinner label no longer changes per stage because loadPRCmd fans the
// independent reads out concurrently (CLAUDE.md §4 #7).
func (m Model) loadingView(frame int, _ model.LoadStage) string {
	glyph := spinnerFrames[frame%len(spinnerFrames)]
	spinnerLine := fmt.Sprintf("%s Loading PR...", fg(glyph, m.theme.LoadingSpinner))
	if m.width <= 0 || m.height <= 0 {
		// Pre-WindowSize fallback: keep the spinner top-left so the very first
		// frame still emits text. Skip the splash here to avoid emitting a
		// raw-art block before we know the terminal width.
		return spinnerLine
	}

	// Block = splash + blank + (version + blank if non-empty) + spinner.
	// The splash variant is chosen at NewModel time and held for the
	// program's lifetime so the loading view does not flicker between
	// designs during the few seconds of PR load.
	var splashRows []string
	switch m.splashLayout {
	case splashLayoutAscii:
		splashRows = strings.Split(m.renderRevaArt(), "\n")
	case splashLayoutDomeAscii:
		splashRows = m.composeDomeAndAscii()
	default: // splashLayoutDome
		splashRows = strings.Split(renderLogo(m.theme), "\n")
	}

	rows := make([]string, 0, len(splashRows)+4)
	rows = append(rows, splashRows...)
	rows = append(rows, "")
	if v := m.versionLineFor(m.splashLayout); v != "" {
		rows = append(rows, v)
		rows = append(rows, "")
	}
	rows = append(rows, spinnerLine)

	// Center each row by its own visible width so glyph rows of differing
	// width still align around the terminal midline.
	var sb strings.Builder
	topPad := 0
	if m.height > len(rows) {
		topPad = (m.height - len(rows)) / 2
	}
	for i := 0; i < topPad; i++ {
		sb.WriteByte('\n')
	}
	for i, r := range rows {
		visible := lipgloss.Width(r)
		leftPad := 0
		if m.width > visible {
			leftPad = (m.width - visible) / 2
		}
		if leftPad > 0 {
			sb.WriteString(strings.Repeat(" ", leftPad))
		}
		sb.WriteString(r)
		if i < len(rows)-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return SpinnerTickMsg{}
	})
}

// loadPRCmd fans the PR's independent reads (GetPR, ListCommits,
// ListFiles, ListComments, ViewerLogin) out concurrently via errgroup.
// All five are pure reads with no inter-dependencies — earlier the
// loader chained them through tea.Sequence and paid 5*RTT on every
// startup. The errgroup version bounds total time at max(per-stage),
// which is dominated on real PRs by ListCommits (whose own per-commit
// detail loop is itself parallelized — see api.commitDetailConcurrency).
//
// CommentCount is no longer derived inside ListFiles (which used to
// re-fetch comments internally and serialize the load); the assembler
// builds it from the independently-fetched comments list, mirroring
// CLAUDE.md §4 #25's "outdated comments don't count" rule.
//
// ViewerLogin runs alongside the rest. A failure there is non-fatal:
// Comments-pane Enter falls back to reply-only when ownership is
// unknown, so a network blip on viewer should not abort the load.
func loadPRCmd(c api.Client, t *api.Target) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var (
			pr       *model.PR
			commits  []*model.Commit
			files    []*model.FileEntry
			comments []*model.ReviewComment
			viewer   string
		)
		eg, egCtx := errgroup.WithContext(ctx)
		eg.Go(func() error {
			p, err := c.GetPR(egCtx, t.Owner, t.Repo, t.Number)
			if err != nil {
				return err
			}
			pr = p
			return nil
		})
		eg.Go(func() error {
			l, err := c.ListCommits(egCtx, t.Owner, t.Repo, t.Number)
			if err != nil {
				return err
			}
			commits = l
			return nil
		})
		eg.Go(func() error {
			l, err := c.ListFiles(egCtx, t.Owner, t.Repo, t.Number)
			if err != nil {
				return err
			}
			files = l
			return nil
		})
		eg.Go(func() error {
			l, err := c.ListComments(egCtx, t.Owner, t.Repo, t.Number)
			if err != nil {
				return err
			}
			comments = l
			return nil
		})
		// Viewer error is intentionally not propagated to the errgroup —
		// it's a soft dependency (Comments-pane edit gating).
		eg.Go(func() error {
			v, _ := c.ViewerLogin(egCtx)
			viewer = v
			return nil
		})
		if err := eg.Wait(); err != nil {
			return ErrMsg{Err: err}
		}

		// Derive CommentCount per file from the comments list. Outdated
		// entries (CLAUDE.md §4 #25) don't count toward the badge —
		// they're hidden in the HEAD/WholePR view that drives the badge.
		counts := map[string]int{}
		for _, cm := range comments {
			if !cm.Outdated {
				counts[cm.Path]++
			}
		}
		for _, f := range files {
			f.CommentCount = counts[f.Path]
		}

		// Diff cache assembly — these calls hit ghClient's already-
		// populated commits / prFiles caches, so no further network
		// I/O happens here in production. The fixture client serves
		// directly from its in-memory diff map.
		diffs := map[string]string{}
		// PR-wide (WholePR) per-file slices + the cross-file concat
		// served under diffKey("", AllFilesPath). Iterate PR.Files
		// order so the concat reads top-to-bottom as the user sees
		// them in the Files pane.
		var allWhole strings.Builder
		for _, f := range files {
			d, err := c.GetFileDiff(ctx, t.Owner, t.Repo, t.Number, "", f.Path)
			if err == nil && d != "" {
				diffs[diffKey("", f.Path)] = d
				allWhole.WriteString(d)
			}
		}
		if allWhole.Len() > 0 {
			diffs[diffKey("", model.AllFilesPath)] = allWhole.String()
		}
		// Per-commit per-file slices + per-commit cross-file concat
		// under diffKey(sha, AllFilesPath). Each commit's iteration
		// order follows PR.Files so the concat shows files in the
		// same order the user navigates in Files (the ChangedFiles
		// map is unordered).
		for _, com := range commits {
			var allCommit strings.Builder
			for _, f := range files {
				if _, touched := com.ChangedFiles[f.Path]; !touched {
					continue
				}
				d, err := c.GetFileDiff(ctx, t.Owner, t.Repo, t.Number, com.SHA, f.Path)
				if err == nil && d != "" {
					diffs[diffKey(com.SHA, f.Path)] = d
					allCommit.WriteString(d)
				}
			}
			if allCommit.Len() > 0 {
				diffs[diffKey(com.SHA, model.AllFilesPath)] = allCommit.String()
			}
		}

		pr.Commits = commits
		pr.Files = files
		pr.Comments = comments
		return PRLoadedMsg{PR: pr, Diffs: diffs, ViewerLogin: viewer}
	}
}

func diffKey(sha, path string) string {
	return sha + "::" + path
}

func (m Model) currentPatch() string {
	if m.state.PR == nil || m.state.SelectedFile == "" {
		return ""
	}
	sha := ""
	if m.state.SelectedRange.Kind == model.RangeSingleCommit {
		sha = m.state.SelectedRange.SHA
	}
	return m.state.DiffCache[diffKey(sha, m.state.SelectedFile)]
}

// patchInfo bundles the per-patch derived data that the renderer needs on
// every frame. Building these is O(buffer_size), so caching them keyed on
// (sha, path) keeps split-mode `j/k` repeat cost flat instead of redoing
// the walk on every redraw.
//
// lines is the augmented patch buffer: original patch rows interleaved
// with `···` synthetic rows and any expanded-context rows the user has
// revealed. specs / gaps are byproducts of diff.Expand and the
// synthetic-aware parseDiffSpecs replacement (ParseSpecsAug) — both are
// built eagerly so consumers always see consistent shapes.
type patchInfo struct {
	lines   []string
	specs   []diffLineSpec
	newNums []int // populated lazily; commentLineSet etc.
	oldNums []int // populated lazily; LEFT-side comment anchoring
	// gaps maps each synthetic row's buffer index to its GapInfo. Used
	// by the Enter handler to decide which gap to expand and by the
	// renderer to render the `··· N hidden` glyph with the correct
	// hidden count.
	gaps map[int]diff.GapInfo
	// markers caches commentLineMarkers' result for this patch keyed on
	// the threadsCache generation counter. nil = uncomputed; mismatched
	// markersGen = stale (recompute). The threads-cache gen bumps on
	// every successful threadsForView rebuild, which itself fires when
	// PR.Comments mutates (compose / refresh) or the file/range key
	// changes — so this single counter covers all invalidation triggers.
	markers    *sideMarkers
	markersGen uint64
}

// diffRowCache memoizes the per-buffer-line render output for the Diff
// pane. The hot loop in `j/k` repeat re-renders the same non-cursor rows
// every frame; with the cache, only the previous-cursor and new-cursor
// rows recompute each keystroke. Invalidated by including the patch key
// and layout dimensions in the cache key.
type diffRowCache struct {
	patchKey string
	width    int
	halfW    int
	m        map[string][]string
}

func (c *diffRowCache) get(key string) ([]string, bool) {
	v, ok := c.m[key]
	return v, ok
}

func (c *diffRowCache) put(key string, rows []string) {
	c.m[key] = rows
}

func (c *diffRowCache) reset(patchKey string, width, halfW int) {
	c.patchKey = patchKey
	c.width = width
	c.halfW = halfW
	c.m = map[string][]string{}
}

// rowCacheKey composes the cache key for a Diff buffer line. The key
// only carries the bits the renderer actually branches on — line index,
// gutter glyph (markerAnchor / markerResolved / 0), plus a
// mode tag (`s` split, `u` unified). The width dimensions are validated
// by the cache itself in invalidateRowCacheIfStale.
func (m Model) rowCacheKey(mode string, idx, halfW int, marker rune) string {
	return mode + "\x00" + strconv.Itoa(idx) + "\x00" + strconv.Itoa(halfW) + "\x00" + string(marker)
}

// invalidateRowCacheIfStale clears the row cache when the patch identity
// or layout dimensions change. Called once per Diff render before the
// per-row loop; cheap when nothing changed.
func (m Model) invalidateRowCacheIfStale() {
	if m.rowCache == nil {
		return
	}
	patchKey := ""
	if m.state != nil && m.state.PR != nil && m.state.SelectedFile != "" {
		sha := ""
		if m.state.SelectedRange.Kind == model.RangeSingleCommit {
			sha = m.state.SelectedRange.SHA
		}
		patchKey = diffKey(sha, m.state.SelectedFile)
	}
	_, halfW := m.splitLayout()
	if m.rowCache.patchKey != patchKey || m.rowCache.width != m.paneWidthDiff || m.rowCache.halfW != halfW {
		m.rowCache.reset(patchKey, m.paneWidthDiff, halfW)
	}
}

// patchLinesCache memoizes strings.Split of the current patch plus the
// derived per-line specs / new-file line numbers. Keyed by
// diffKey(sha, path); invalidated implicitly when the user changes file
// or commit (different key, miss, recompute).
type patchLinesCache struct {
	cache map[string]*patchInfo
}

func (m Model) patchInfo() *patchInfo {
	if m.state == nil || m.state.PR == nil || m.state.SelectedFile == "" {
		return nil
	}
	sha := ""
	if m.state.SelectedRange.Kind == model.RangeSingleCommit {
		sha = m.state.SelectedRange.SHA
	}
	key := diffKey(sha, m.state.SelectedFile)
	if m.patchLinesC.cache != nil {
		if v, ok := m.patchLinesC.cache[key]; ok {
			return v
		}
	}
	patch := m.state.DiffCache[key]
	if patch == "" {
		return nil
	}

	// Resolve the expand state for this (file, range). Missing key = no
	// expansion — Expand still emits BOF / inter-hunk synthetics from
	// the hunk-header arithmetic, and (if file lines are cached) the
	// EOF synthetic too.
	ek := model.ExpandKey{
		Path:      m.state.SelectedFile,
		RangeKind: m.state.SelectedRange.Kind,
		RangeSHA:  m.state.SelectedRange.SHA,
	}
	var es model.ExpandState
	if v := m.state.ExpandedContext[ek]; v != nil {
		es = *v
	}

	// File-content ref: HEAD for the WholePR view, the commit SHA for a
	// single-commit drill. The fetch is lazy — Diff renders without
	// EOF synthetic until the user's first expand triggers GetFileContents.
	ref := ""
	if m.state.PR != nil {
		ref = m.state.PR.HeadSHA
	}
	if m.state.SelectedRange.Kind == model.RangeSingleCommit {
		ref = m.state.SelectedRange.SHA
	}
	var fileLines []string
	if m.state.FileContents != nil {
		fileLines = m.state.FileContents[model.FileContentsKey{Ref: ref, Path: m.state.SelectedFile}]
	}

	res := diff.Expand(diff.ExpandInputs{
		Patch:     patch,
		FileLines: fileLines,
		Expand: diff.ExpandState{
			BOFBelow:   es.BOFBelow,
			EOFAbove:   es.EOFAbove,
			InterAbove: es.InterAbove,
			InterBelow: es.InterBelow,
		},
	})
	info := &patchInfo{
		lines: res.Lines,
		specs: convertAugSpecs(diff.ParseSpecsAug(res.Lines, res.Gaps)),
		gaps:  res.Gaps,
	}
	if m.patchLinesC.cache != nil {
		m.patchLinesC.cache[key] = info
	}
	return info
}

// patchGaps returns the synthetic-row index → GapInfo map for the
// currently-displayed patch. Used by the Diff Enter handler to identify
// the gap under the cursor and by renderers that show `··· N hidden`.
func (m Model) patchGaps() map[int]diff.GapInfo {
	pi := m.patchInfo()
	if pi == nil {
		return nil
	}
	return pi.gaps
}

// invalidatePatchInfoCache drops the cached patchInfo for the given
// (file, range) pair. Called after ExpandedContext mutates (Enter on a
// synthetic row) or FileContents arrives, so the next render rebuilds
// the augmented buffer with the new state.
func (m Model) invalidatePatchInfoCache(ek model.ExpandKey) {
	sha := ""
	if ek.RangeKind == model.RangeSingleCommit {
		sha = ek.RangeSHA
	}
	dk := diffKey(sha, ek.Path)
	if m.patchLinesC.cache != nil {
		delete(m.patchLinesC.cache, dk)
	}
	if m.rowCache != nil {
		m.rowCache.reset(m.rowCache.patchKey, m.rowCache.width, m.rowCache.halfW)
	}
	if m.threadsCache != nil {
		m.threadsCache.valid = false
	}
}

// convertAugSpecs widens diff.AugSpec into the tui-level diffLineSpec
// the renderer already understands. Synthetic rows produce Kind 's'
// with zero line numbers.
func convertAugSpecs(in []diff.AugSpec) []diffLineSpec {
	out := make([]diffLineSpec, len(in))
	for i, s := range in {
		out[i] = diffLineSpec{Kind: s.Kind, OldLn: s.OldLn, NewLn: s.NewLn}
	}
	return out
}

func (m Model) patchLines() []string {
	pi := m.patchInfo()
	if pi == nil {
		return nil
	}
	return pi.lines
}

// patchSpecs returns the cached diffLineSpec slice for the current patch.
// Always populated eagerly by patchInfo (the synthetic-aware walker can't
// be reproduced from raw lines without the gaps map, so lazy-deferral
// would just re-pass the same data).
func (m Model) patchSpecs() []diffLineSpec {
	pi := m.patchInfo()
	if pi == nil {
		return nil
	}
	return pi.specs
}

// patchNewLineNumbers returns the cached new-file line-number mapping.
// Built lazily from specs so synthetic rows skip the counter (their
// AugSpec.NewLn is 0) and the post-synthetic gap-end jump baked into
// ParseSpecsAug propagates here.
func (m Model) patchNewLineNumbers() []int {
	pi := m.patchInfo()
	if pi == nil {
		return nil
	}
	if pi.newNums == nil {
		pi.newNums = make([]int, len(pi.specs))
		for i, s := range pi.specs {
			if s.Kind == ' ' || s.Kind == '+' {
				pi.newNums[i] = s.NewLn
			}
		}
	}
	return pi.newNums
}

// patchOldLineNumbers mirrors patchNewLineNumbers for the OLD-side
// counter. Synthetic and '+' rows produce 0.
func (m Model) patchOldLineNumbers() []int {
	pi := m.patchInfo()
	if pi == nil {
		return nil
	}
	if pi.oldNums == nil {
		pi.oldNums = make([]int, len(pi.specs))
		for i, s := range pi.specs {
			if s.Kind == ' ' || s.Kind == '-' {
				pi.oldNums[i] = s.OldLn
			}
		}
	}
	return pi.oldNums
}

func (m Model) effectiveDiffViewMode() string {
	if m.state.DiffViewMode == model.DiffViewUnified {
		return "unified"
	}
	if m.width > 0 && m.width < 100 {
		return "unified"
	}
	return "split"
}
