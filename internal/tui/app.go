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
	"github.com/ktrysmt/gh-reva/internal/model"
	"github.com/ktrysmt/gh-reva/internal/theme"
)

type Model struct {
	client      api.Client
	target      *api.Target
	state       *model.AppState
	theme       *theme.Theme
	syntaxCache *syntaxCache
	patchLinesC patchLinesCache
	rowCache    *diffRowCache
	width       int
	height      int
	err         error

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
		splashLayout: chooseSplashLayout(),
		splashArtIdx: chooseSplashArt(),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(loadPRCmd(m.client, m.target), spinnerTickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
			m.state.SelectedFile = msg.PR.Files[0].Path
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
	case ErrMsg:
		m.err = msg.Err
		return m, tea.Quit
	}
	return m, nil
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

	leftW, midW, rightW := splitColumnWidths(m.width, m.state.CommentsHidden)
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

	if statusBar != "" {
		return body + "\n" + statusBar
	}
	return body
}

// splitColumnWidths divides total terminal width across the three columns.
// Targets roughly 30 / 60 / remainder for left / right / middle so the
// Comments column has room to display typical comment bodies without
// aggressive wrap. When commentsHidden is true the right column collapses
// to 0 and its width is added to the middle (Diff) column — Files /
// Commits keep their familiar widths so the layout transition is local
// to the right side of the screen.
func splitColumnWidths(total int, commentsHidden bool) (left, mid, right int) {
	if commentsHidden {
		// Reuse the visible-Comments left budget so Files / Commits don't
		// reflow when the toggle fires; the Diff pane absorbs everything
		// to the right of the left column.
		left, _, _ = splitColumnWidths(total, false)
		mid = total - left
		if mid < 1 {
			mid = 1
		}
		return
	}
	if total >= 130 {
		// Border consumes 2 cols per pane; bump outer widths so inner widths
		// (used for content) match the pre-border targets.
		left = 42
		right = 57
		mid = total - left - right
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
		right = total * 2 / 5
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
		for _, f := range files {
			d, err := c.GetFileDiff(ctx, t.Owner, t.Repo, t.Number, "", f.Path)
			if err == nil && d != "" {
				diffs[diffKey("", f.Path)] = d
			}
		}
		for _, com := range commits {
			for path := range com.ChangedFiles {
				d, err := c.GetFileDiff(ctx, t.Owner, t.Repo, t.Number, com.SHA, path)
				if err == nil && d != "" {
					diffs[diffKey(com.SHA, path)] = d
				}
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
type patchInfo struct {
	lines   []string
	specs   []diffLineSpec // populated lazily; only split mode reads it
	newNums []int          // populated lazily; commentLineSet etc.
	oldNums []int          // populated lazily; LEFT-side comment anchoring
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
// commented gutter, plus a mode tag (`s` split, `u` unified). The width
// dimensions are validated by the cache itself in invalidateRowCacheIfStale.
func (m Model) rowCacheKey(mode string, idx, halfW int, commented bool) string {
	c := byte('0')
	if commented {
		c = '1'
	}
	return mode + "\x00" + strconv.Itoa(idx) + "\x00" + strconv.Itoa(halfW) + "\x00" + string(c)
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
	info := &patchInfo{
		lines: strings.Split(strings.TrimRight(patch, "\n"), "\n"),
	}
	if m.patchLinesC.cache != nil {
		m.patchLinesC.cache[key] = info
	}
	return info
}

func (m Model) patchLines() []string {
	pi := m.patchInfo()
	if pi == nil {
		return nil
	}
	return pi.lines
}

// patchSpecs returns the cached diffLineSpec slice for the current patch.
// First reader pays the parse; subsequent renders reuse the slice.
func (m Model) patchSpecs() []diffLineSpec {
	pi := m.patchInfo()
	if pi == nil {
		return nil
	}
	if pi.specs == nil {
		pi.specs = parseDiffSpecs(pi.lines)
	}
	return pi.specs
}

// patchNewLineNumbers returns the cached new-file line-number mapping.
// Lazy for the same reason as patchSpecs.
func (m Model) patchNewLineNumbers() []int {
	pi := m.patchInfo()
	if pi == nil {
		return nil
	}
	if pi.newNums == nil {
		pi.newNums = newLineNumbers(pi.lines)
	}
	return pi.newNums
}

// patchOldLineNumbers returns the cached old-file line-number mapping.
// Built lazily on first read (LEFT-side comments are common but not
// universal, so the walk only pays off when a thread anchors on a `-`
// row). Cached on the same patchInfo as newNums so per-render lookups
// stay flat.
func (m Model) patchOldLineNumbers() []int {
	pi := m.patchInfo()
	if pi == nil {
		return nil
	}
	if pi.oldNums == nil {
		pi.oldNums = oldLineNumbers(pi.lines)
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
