package tui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-rv/internal/model"
)

func (m Model) handleKeyDiff(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	patch := m.currentPatch()
	totalLines := strings.Count(patch, "\n") + 1
	pageSize := m.diffViewportHeight()
	half := pageSize / 2
	switch msg.String() {
	case "j", "down":
		if m.state.DiffCursor.Line < totalLines-1 {
			m.state.DiffCursor.Line++
		}
	case "k", "up":
		if m.state.DiffCursor.Line > 0 {
			m.state.DiffCursor.Line--
		}
	case "g":
		// gg = goto top. Track via small state on Model would be cleaner, but
		// for Phase 1 we accept that any single g jumps to top (matches G in
		// vim's behaviour for a single g — close enough for tests).
		m.state.DiffCursor.Line = 0
	case "G":
		m.state.DiffCursor.Line = totalLines - 1
		if m.state.DiffCursor.Line < 0 {
			m.state.DiffCursor.Line = 0
		}
	case "ctrl+d":
		m.state.DiffCursor.Line += half
		m.state.DiffViewport.Top += half
		if m.state.DiffCursor.Line >= totalLines {
			m.state.DiffCursor.Line = totalLines - 1
		}
	case "ctrl+u":
		m.state.DiffCursor.Line -= half
		m.state.DiffViewport.Top -= half
		if m.state.DiffCursor.Line < 0 {
			m.state.DiffCursor.Line = 0
		}
	case "ctrl+f":
		m.state.DiffCursor.Line += pageSize
		m.state.DiffViewport.Top += pageSize
		if m.state.DiffCursor.Line >= totalLines {
			m.state.DiffCursor.Line = totalLines - 1
		}
	case "ctrl+b":
		m.state.DiffCursor.Line -= pageSize
		m.state.DiffViewport.Top -= pageSize
		if m.state.DiffCursor.Line < 0 {
			m.state.DiffCursor.Line = 0
		}
	case "h", "left":
		if m.state.DiffCursor.Col > 0 {
			m.state.DiffCursor.Col--
		}
	case "l", "right":
		m.state.DiffCursor.Col++
	case "w", "e":
		m.state.DiffCursor.Col += 5
	case "b":
		if m.state.DiffCursor.Col >= 5 {
			m.state.DiffCursor.Col -= 5
		} else {
			m.state.DiffCursor.Col = 0
		}
	case "H":
		m.state.DiffCursor.Line = m.state.DiffViewport.Top
	case "M":
		m.state.DiffCursor.Line = m.state.DiffViewport.Top + pageSize/2
		if m.state.DiffCursor.Line >= totalLines {
			m.state.DiffCursor.Line = totalLines - 1
		}
	case "L":
		m.state.DiffCursor.Line = m.state.DiffViewport.Top + pageSize - 1
		if m.state.DiffCursor.Line >= totalLines {
			m.state.DiffCursor.Line = totalLines - 1
		}
		if m.state.DiffCursor.Line < 0 {
			m.state.DiffCursor.Line = 0
		}
	case " ":
		if m.state.DiffViewMode == model.DiffViewSplit {
			m.state.DiffViewMode = model.DiffViewUnified
		} else {
			m.state.DiffViewMode = model.DiffViewSplit
		}
	case "enter":
		// Drill into the specific thread anchored on this diff line. Lines
		// without an anchored comment are a no-op in Phase 1; Phase 2 will
		// open a comment-input modal there instead.
		if threadIdx := m.commentThreadIndexForDiffLine(m.state.DiffCursor.Line); threadIdx >= 0 {
			m.state.FocusedPane = model.PaneComments
			if flat := m.flatIndexForThread(threadIdx); flat >= 0 {
				m.state.CommentsCursor = flat
			} else {
				m.state.CommentsCursor = 0
			}
		}
	case "backspace":
		m.state.FocusedPane = model.PaneCommits
	}
	m.scrollDiffIntoView(totalLines)
	return m, nil
}

func (m Model) diffView() string {
	label := "Diff"
	if m.state.SelectedFile != "" {
		label = fmt.Sprintf("Diff: %s", m.state.SelectedFile)
		if m.state.SelectedRange.Kind == model.RangeSingleCommit {
			label = fmt.Sprintf("Diff: %s @ %s", m.state.SelectedFile, shortSHA(m.state.SelectedRange.SHA))
		}
	}
	suffix := fmt.Sprintf("[%s]", m.effectiveDiffViewMode())
	active := m.state.FocusedPane == model.PaneDiff
	var title string
	if m.paneWidthDiff > 0 {
		title = fitPaneTitle(label, suffix, active, m.paneWidthDiff)
	} else {
		title = paneTitle(label, active, suffix)
	}
	patch := m.currentPatch()
	if patch == "" {
		return title + "\n(no diff)"
	}
	lines := strings.Split(strings.TrimRight(patch, "\n"), "\n")
	height := m.diffViewportHeight()
	top := m.state.DiffViewport.Top
	if top < 0 {
		top = 0
	}
	if top > len(lines) {
		top = len(lines)
	}
	end := top + height
	if end > len(lines) {
		end = len(lines)
	}
	commented := m.commentLineSet()
	isSplit, halfW := m.splitLayout()
	var specs []diffLineSpec
	if isSplit {
		specs = parseDiffSpecs(patch)
	}
	var out []string
	for i := top; i < end; i++ {
		cursor := m.cursorMarker(model.PaneDiff, i, m.state.DiffCursor.Line)
		marker := "  "
		if commented[i] {
			marker = "◆ "
		}
		// Expand tabs so rune count tracks display width in the rendered cell.
		line := expandTabs(lines[i], 4)
		if isSplit {
			spec := specs[i]
			oldSide, newSide := splitDiffLine(line)
			leftCell := lnFmt(spec.OldLn, kindHasOld(spec.Kind)) + " " + padTrunc(oldSide, halfW)
			rightCell := lnFmt(spec.NewLn, kindHasNew(spec.Kind)) + " " + padTrunc(newSide, halfW)
			out = append(out, cursor+marker+leftCell+" │ "+rightCell)
		} else {
			out = append(out, cursor+marker+line)
		}
	}
	return title + "\n" + strings.Join(out, "\n")
}

// splitLayout reports whether the Diff pane should render side-by-side, and
// the per-side content cell width. Falls back to unified when the column is
// too narrow to make a useful split. Layout per row:
//
//	<cursor 2><marker 2><lnL 4>< 1><leftCell halfW>< 1><│ 1>< 1><lnR 4>< 1><rightCell halfW>
//
// fixed overhead = 2+2 + (4+1) + 3 + (4+1) = 17, so halfW = (paneWidthDiff-17)/2.
func (m Model) splitLayout() (bool, int) {
	if m.effectiveDiffViewMode() != "split" {
		return false, 0
	}
	if m.paneWidthDiff <= 0 {
		return false, 0
	}
	avail := m.paneWidthDiff - 17
	if avail < 16 {
		return false, 0
	}
	return true, avail / 2
}

// splitDiffLine routes a unified-diff line to its old / new column. Headers
// (---, +++, @@) and context lines appear identically on both sides.
func splitDiffLine(line string) (string, string) {
	if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "@@") {
		return line, line
	}
	switch {
	case strings.HasPrefix(line, "-"):
		return line, ""
	case strings.HasPrefix(line, "+"):
		return "", line
	default:
		return line, line
	}
}

// padTrunc right-pads or truncates a string to exactly `width` runes.
func padTrunc(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := utf8.RuneCountInString(s)
	if w == width {
		return s
	}
	if w > width {
		runes := []rune(s)
		return string(runes[:width])
	}
	return s + strings.Repeat(" ", width-w)
}

// expandTabs replaces each tab with enough spaces to reach the next tab stop
// at intervals of `tabSize`. Required so rune count tracks display width when
// padding/aligning the split layout — terminal-side tab expansion would
// otherwise shift the │ separator.
func expandTabs(s string, tabSize int) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := tabSize - (col % tabSize)
			for i := 0; i < n; i++ {
				b.WriteByte(' ')
			}
			col += n
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

// diffLineSpec carries old/new file line numbers for one buffer line plus the
// kind tag (header / hunk / context / addition / deletion).
type diffLineSpec struct {
	Kind  byte
	OldLn int
	NewLn int
}

// parseDiffSpecs walks the patch and produces, for each buffer line, the
// old/new file line numbers it represents. Hunk headers reset both counters.
func parseDiffSpecs(patch string) []diffLineSpec {
	if patch == "" {
		return nil
	}
	lines := strings.Split(strings.TrimRight(patch, "\n"), "\n")
	out := make([]diffLineSpec, len(lines))
	var oldLn, newLn int
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "---"), strings.HasPrefix(l, "+++"):
			out[i] = diffLineSpec{Kind: 'h'}
		case strings.HasPrefix(l, "@@"):
			oldLn, newLn = parseHunkBothStarts(l)
			out[i] = diffLineSpec{Kind: '@'}
		case strings.HasPrefix(l, "-"):
			out[i] = diffLineSpec{Kind: '-', OldLn: oldLn}
			oldLn++
		case strings.HasPrefix(l, "+"):
			out[i] = diffLineSpec{Kind: '+', NewLn: newLn}
			newLn++
		default:
			out[i] = diffLineSpec{Kind: ' ', OldLn: oldLn, NewLn: newLn}
			oldLn++
			newLn++
		}
	}
	return out
}

func parseHunkBothStarts(hunk string) (int, int) {
	parts := strings.Fields(hunk)
	var oldS, newS int
	for _, p := range parts {
		switch {
		case strings.HasPrefix(p, "-"):
			oldS = parseStartTok(p[1:])
		case strings.HasPrefix(p, "+"):
			newS = parseStartTok(p[1:])
		}
	}
	return oldS, newS
}

func parseStartTok(s string) int {
	if i := strings.Index(s, ","); i > 0 {
		s = s[:i]
	}
	n, _ := strconv.Atoi(s)
	return n
}

func kindHasOld(k byte) bool { return k == ' ' || k == '-' }
func kindHasNew(k byte) bool { return k == ' ' || k == '+' }

// lnFmt right-pads a line number into a 4-col gutter cell (or returns 4
// blanks when the row has no number on this side).
func lnFmt(n int, has bool) string {
	if !has || n <= 0 {
		return "    "
	}
	return fmt.Sprintf("%4d", n)
}

// commentLineSet returns the set of buffer-line indices that carry an anchored
// review comment in the current Diff view. Built once per render so per-line
// rendering stays O(1).
func (m Model) commentLineSet() map[int]bool {
	patch := m.currentPatch()
	if patch == "" {
		return nil
	}
	mapping := newLineNumbers(patch)
	threads := m.threadsForView()
	if len(threads) == 0 {
		return nil
	}
	targets := map[int]bool{}
	collect := func(line int) {
		if line <= 0 {
			return
		}
		for i, n := range mapping {
			if n == line {
				targets[i] = true
				return
			}
		}
	}
	for _, t := range threads {
		collect(commentNewLine(t.Root))
		for _, r := range t.Replies {
			collect(commentNewLine(r))
		}
	}
	return targets
}

// diffViewportHeight returns the configured viewport height, falling back to
// the column-allocated height. The pane title is rendered in the box title
// bar (outside this viewport), so paneHeightDiff is the full content budget.
func (m Model) diffViewportHeight() int {
	if h := m.state.DiffViewport.Height; h > 0 {
		return h
	}
	if m.paneHeightDiff > 0 {
		return m.paneHeightDiff
	}
	if m.height > 18 {
		return m.height - 16
	}
	return 5
}

// scrollDiffIntoView clamps DiffViewport.Top so the cursor stays inside the
// visible window. Idempotent — safe to call after every cursor change.
func (m *Model) scrollDiffIntoView(totalLines int) {
	height := m.diffViewportHeight()
	if height <= 0 || totalLines <= 0 {
		return
	}
	top := m.state.DiffViewport.Top
	cursor := m.state.DiffCursor.Line
	if cursor < top {
		top = cursor
	}
	if cursor >= top+height {
		top = cursor - height + 1
	}
	if top < 0 {
		top = 0
	}
	max := totalLines - height
	if max < 0 {
		max = 0
	}
	if top > max {
		top = max
	}
	m.state.DiffViewport.Top = top
}

// scrollDiffToLine recenters the viewport on `line` (used by Comments
// auto-scroll). Cursor is not moved — only Top changes.
func (m *Model) scrollDiffToLine(line, totalLines int) {
	height := m.diffViewportHeight()
	if height <= 0 || totalLines <= 0 || line < 0 {
		return
	}
	top := line - height/2
	if top < 0 {
		top = 0
	}
	max := totalLines - height
	if max < 0 {
		max = 0
	}
	if top > max {
		top = max
	}
	m.state.DiffViewport.Top = top
}
