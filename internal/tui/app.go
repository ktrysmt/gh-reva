package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ktrysmt/gh-rv/internal/api"
	"github.com/ktrysmt/gh-rv/internal/model"
	"github.com/ktrysmt/gh-rv/internal/theme"
)

type Model struct {
	client       api.Client
	target       *api.Target
	state        *model.AppState
	theme        *theme.Theme
	syntaxCache  *syntaxCache
	patchLinesC  patchLinesCache
	rowCache     *diffRowCache
	width        int
	height       int
	err          error

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

func NewModel(client api.Client, target *api.Target) Model {
	t, _ := theme.Resolve("")
	return Model{
		client:      client,
		target:      target,
		state:       model.NewAppState(),
		theme:       t,
		syntaxCache: &syntaxCache{},
		patchLinesC: patchLinesCache{cache: map[string]*patchInfo{}},
		rowCache:    &diffRowCache{m: map[string][]string{}},
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
	case LoadStageMsg:
		m.state.LoadStage = msg.Stage
		return m, nil
	case SpinnerTickMsg:
		m.state.LoadFrame++
		if m.state.LoadStage == model.LoadStageDone {
			return m, nil
		}
		return m, spinnerTickCmd()
	case PRLoadedMsg:
		m.state.PR = msg.PR
		m.state.DiffCache = msg.Diffs
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
	// Reserve a row for the visual indicator when active so it does not
	// disappear under the column rendering.
	bodyHeight := m.height
	if m.state.Visual != nil && bodyHeight > 0 {
		bodyHeight--
	}
	if m.width <= 0 || bodyHeight <= 0 {
		// Window size not received yet — fall back to a stacked render
		// (used by smoke tests and the very first frame).
		body := strings.Join([]string{
			m.filesView(),
			m.commitsView(),
			m.diffView(),
			m.commentsView(),
		}, "\n\n")
		if m.state.Visual != nil {
			return body + "\n" + m.visualIndicator()
		}
		return body
	}

	leftW, midW, rightW := splitColumnWidths(m.width)
	topH, bottomH := splitColumnHeights(bodyHeight)

	// Each pane renders as: top border + title row + ├──┤ divider + content
	// rows + bottom border. Inner content budget is therefore outer width − 2
	// and outer height − 4.
	innerLeftW := atLeast(leftW-2, 1)
	innerMidW := atLeast(midW-2, 1)
	innerRightW := atLeast(rightW-2, 1)
	innerTopH := atLeast(topH-4, 1)
	innerBottomH := atLeast(bottomH-4, 1)
	innerBodyH := atLeast(bodyHeight-4, 1)

	m.paneWidthFiles = innerLeftW
	m.paneHeightFiles = innerTopH
	m.paneWidthCommits = innerLeftW
	m.paneHeightCommits = innerBottomH
	m.paneWidthDiff = innerMidW
	m.paneHeightDiff = innerBodyH
	m.paneWidthComments = innerRightW
	m.paneHeightComments = innerBodyH

	files := m.boxFromPaneView(m.filesView(), leftW, topH, model.PaneFiles)
	commits := m.boxFromPaneView(m.commitsView(), leftW, bottomH, model.PaneCommits)
	leftCol := lipgloss.JoinVertical(lipgloss.Left, files, commits)
	diffCol := m.boxFromPaneView(m.diffView(), midW, bodyHeight, model.PaneDiff)
	commentsCol := m.boxFromPaneView(m.commentsView(), rightW, bodyHeight, model.PaneComments)
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, diffCol, commentsCol)

	if m.state.Visual != nil {
		return body + "\n" + m.visualIndicator()
	}
	return body
}

// visualIndicator renders the "-- VISUAL --" marker shown at the bottom of
// the screen while linewise visual mode is active. CommentAnchor (orange in
// builtin-dark) doubles as the attention hue for mode banners; bold makes
// it stand out from the surrounding pane chrome.
func (m Model) visualIndicator() string {
	return fgBold("-- VISUAL --", m.theme.CommentAnchor)
}

// splitColumnWidths divides total terminal width across the three columns.
// Targets roughly 30 / 60 / remainder for left / right / middle so the
// Comments column has room to display typical comment bodies without
// aggressive wrap.
func splitColumnWidths(total int) (left, mid, right int) {
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

func (m Model) loadingView(frame int, stage model.LoadStage) string {
	glyph := spinnerFrames[frame%len(spinnerFrames)]
	return fmt.Sprintf("%s Loading PR (%s)...", fg(glyph, m.theme.LoadingSpinner), stageLabel(stage))
}

func stageLabel(s model.LoadStage) string {
	switch s {
	case model.LoadStagePR:
		return "metadata"
	case model.LoadStageCommits:
		return "commits"
	case model.LoadStageFiles:
		return "files"
	case model.LoadStageComments:
		return "comments"
	case model.LoadStageDiffs:
		return "diffs"
	default:
		return "ready"
	}
}

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return SpinnerTickMsg{}
	})
}

// loadPRCmd loads PR data in stages via tea.Sequence. Each stage emits a
// LoadStageMsg so the spinner label can update; the final stage assembles
// accumulated data and emits PRLoadedMsg. A per-launch accumulator (closed
// over by every cmd) carries data between stages.
func loadPRCmd(c api.Client, t *api.Target) tea.Cmd {
	acc := &loadAccumulator{}
	ctx := context.Background()
	return tea.Sequence(
		stageMsgCmd(model.LoadStagePR),
		func() tea.Msg {
			pr, err := c.GetPR(ctx, t.Owner, t.Repo, t.Number)
			if err != nil {
				return ErrMsg{Err: err}
			}
			acc.pr = pr
			return LoadStageMsg{Stage: model.LoadStageCommits}
		},
		func() tea.Msg {
			commits, err := c.ListCommits(ctx, t.Owner, t.Repo, t.Number)
			if err != nil {
				return ErrMsg{Err: err}
			}
			acc.commits = commits
			return LoadStageMsg{Stage: model.LoadStageFiles}
		},
		func() tea.Msg {
			files, err := c.ListFiles(ctx, t.Owner, t.Repo, t.Number)
			if err != nil {
				return ErrMsg{Err: err}
			}
			acc.files = files
			return LoadStageMsg{Stage: model.LoadStageComments}
		},
		func() tea.Msg {
			comments, err := c.ListComments(ctx, t.Owner, t.Repo, t.Number)
			if err != nil {
				return ErrMsg{Err: err}
			}
			acc.comments = comments
			return LoadStageMsg{Stage: model.LoadStageDiffs}
		},
		func() tea.Msg {
			diffs := map[string]string{}
			for _, f := range acc.files {
				d, err := c.GetFileDiff(ctx, t.Owner, t.Repo, t.Number, "", f.Path)
				if err == nil && d != "" {
					diffs[diffKey("", f.Path)] = d
				}
			}
			for _, com := range acc.commits {
				for path := range com.ChangedFiles {
					d, err := c.GetFileDiff(ctx, t.Owner, t.Repo, t.Number, com.SHA, path)
					if err == nil && d != "" {
						diffs[diffKey(com.SHA, path)] = d
					}
				}
			}
			acc.pr.Commits = acc.commits
			acc.pr.Files = acc.files
			acc.pr.Comments = acc.comments
			return PRLoadedMsg{PR: acc.pr, Diffs: diffs}
		},
	)
}

type loadAccumulator struct {
	pr       *model.PR
	commits  []*model.Commit
	files    []*model.FileEntry
	comments []*model.ReviewComment
}

func stageMsgCmd(s model.LoadStage) tea.Cmd {
	return func() tea.Msg { return LoadStageMsg{Stage: s} }
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
	lines    []string
	specs    []diffLineSpec // populated lazily; only split mode reads it
	newNums  []int          // populated lazily; commentLineSet etc.
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

func (m Model) effectiveDiffViewMode() string {
	if m.state.DiffViewMode == model.DiffViewUnified {
		return "unified"
	}
	if m.width > 0 && m.width < 100 {
		return "unified"
	}
	return "split"
}
