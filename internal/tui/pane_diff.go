package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ktrysmt/gh-rv/internal/model"
)

func (m Model) handleKeyDiff(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	totalLines := len(m.patchLines())
	if totalLines == 0 {
		// Avoid division/clamp wrap when there is no diff to navigate.
		totalLines = 1
	}
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
	title := m.styledPaneTitle(label, active, suffix)
	lines := m.patchLines()
	if len(lines) == 0 {
		return title + "\n(no diff)"
	}
	m.invalidateRowCacheIfStale()
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
		specs = m.patchSpecs()
	}
	cursorLine := m.state.DiffCursor.Line
	var out []string
	for i := top; i < end && len(out) < height; i++ {
		var rows []string
		if isSplit {
			rows = m.renderSplitBufferLine(lines[i], specs[i], halfW, i, cursorLine, commented[i])
		} else {
			rows = m.renderUnifiedBufferLine(lines[i], i, cursorLine, commented[i])
		}
		for _, r := range rows {
			if len(out) >= height {
				break
			}
			out = append(out, r)
		}
	}
	return title + "\n" + strings.Join(out, "\n")
}

// renderUnifiedBufferLine returns the display rows for one buffer line in
// unified mode. First row is `<cursor 2><marker 2><content>` where content is
// the wrap-cell head. Continuation rows are `<4 blanks><wrap-cell tail>`,
// where the tail's leading blank aligns past the diff marker (`+`/`-`/space)
// — so total continuation indent is 5 cols (cursor 2 + marker 2 + 1).
func (m Model) renderUnifiedBufferLine(line string, idx, cursorLine int, commented bool) []string {
	isCursor := idx == cursorLine
	inVisual := m.inVisualRange(model.PaneDiff, idx)
	cacheKey := ""
	if !isCursor && !inVisual && m.rowCache != nil {
		cacheKey = m.rowCacheKey("u", idx, 0, commented)
		if v, ok := m.rowCache.get(cacheKey); ok {
			return v
		}
	}
	contentW := m.paneWidthDiff - 4
	if contentW <= 0 {
		contentW = 1
	}
	expanded := expandTabs(line, 4)
	cells := wrapCell(expanded, contentW)
	kind := diffLineKind(line)
	out := make([]string, 0, len(cells))
	for j, cell := range cells {
		colored := m.colorDiffCell(cell, kind, false)
		var prefix string
		if j == 0 {
			cursor := m.cursorMarker(model.PaneDiff, idx, cursorLine)
			if cursor == "> " {
				cursor = fgBold(cursor, m.theme.CursorRow)
			}
			marker := "  "
			if commented {
				marker = fg("◆ ", m.theme.CommentAnchor)
			}
			prefix = cursor + marker
		} else if inVisual {
			prefix = fgBold("> ", m.theme.CursorRow) + "  "
		} else {
			prefix = "    "
		}
		row := padTrunc(prefix+colored, m.paneWidthDiff)
		if inVisual {
			row = bgRow(row, m.theme.VisualRangeBg)
		}
		out = append(out, row)
	}
	if cacheKey != "" {
		m.rowCache.put(cacheKey, out)
	}
	return out
}

// renderSplitBufferLine returns the display rows for one buffer line in split
// mode. First row carries cursor / ◆ / line numbers / both half-cells with the
// │ separator. Continuation rows blank cursor / marker / line-number columns,
// re-draw │ at the same column, and prefix each half-cell with 1 blank to
// align past the diff marker.
//
// Hot path under j/k repeat: split mode does ~2× the per-row work of
// unified (two cells, two line-number gutters, separator). To keep
// `j`-hold responsive we cache the final []string output keyed on the
// inputs that actually affect rendering. The cursor and visual rows are
// not cached (they change every keystroke); everything else is, so 28/30
// visible rows hit the cache on each redraw.
func (m Model) renderSplitBufferLine(line string, spec diffLineSpec, halfW, idx, cursorLine int, commented bool) []string {
	isCursor := idx == cursorLine
	inVisual := m.inVisualRange(model.PaneDiff, idx)
	cacheKey := ""
	if !isCursor && !inVisual && m.rowCache != nil {
		cacheKey = m.rowCacheKey("s", idx, halfW, commented)
		if v, ok := m.rowCache.get(cacheKey); ok {
			return v
		}
	}
	expanded := expandTabs(line, 4)
	oldSide, newSide := splitDiffLine(expanded)
	leftCells := wrapCell(oldSide, halfW)
	rightCells := wrapCell(newSide, halfW)
	n := len(leftCells)
	if len(rightCells) > n {
		n = len(rightCells)
	}
	blank := strings.Repeat(" ", halfW)
	sep := fg("│", m.theme.DiffSeparator)
	out := make([]string, 0, n)
	for j := 0; j < n; j++ {
		left := blank
		if j < len(leftCells) {
			left = leftCells[j]
		}
		right := blank
		if j < len(rightCells) {
			right = rightCells[j]
		}
		left = m.colorDiffCell(left, spec.Kind, false)
		right = m.colorDiffCell(right, spec.Kind, true)

		var cursor, marker, oldLn, newLn string
		if j == 0 {
			cursor = m.cursorMarker(model.PaneDiff, idx, cursorLine)
			if cursor == "> " {
				cursor = fgBold(cursor, m.theme.CursorRow)
			}
			marker = "  "
			if commented {
				marker = fg("◆ ", m.theme.CommentAnchor)
			}
			oldLn = fg(lnFmt(spec.OldLn, kindHasOld(spec.Kind)), m.theme.DiffLineNumber)
			newLn = fg(lnFmt(spec.NewLn, kindHasNew(spec.Kind)), m.theme.DiffLineNumber)
		} else {
			if inVisual {
				cursor = fgBold("> ", m.theme.CursorRow)
			} else {
				cursor = "  "
			}
			marker = "  "
			oldLn = "    "
			newLn = "    "
		}
		row := padTrunc(cursor+marker+oldLn+" "+left+" "+sep+" "+newLn+" "+right, m.paneWidthDiff)
		if inVisual {
			row = bgRow(row, m.theme.VisualRangeBg)
		}
		out = append(out, row)
	}
	if cacheKey != "" {
		m.rowCache.put(cacheKey, out)
	}
	return out
}

// diffLineKind classifies a unified-diff buffer line into the same byte tags
// used by parseDiffSpecs ('h' file header, '@' hunk, '+'/'-' add/del, ' '
// context). Order matters — `---`/`+++` must be tested before `+`/`-`.
func diffLineKind(line string) byte {
	switch {
	case strings.HasPrefix(line, "---"), strings.HasPrefix(line, "+++"):
		return 'h'
	case strings.HasPrefix(line, "@@"):
		return '@'
	case strings.HasPrefix(line, "+"):
		return '+'
	case strings.HasPrefix(line, "-"):
		return '-'
	default:
		return ' '
	}
}

// colorDiffCell paints a pre-padded diff cell. Header and hunk-header rows
// keep a flat foreground color (they aren't source code). Added, deleted,
// and context rows all run through styledDiffCell for per-token chroma
// foreground; +/- rows additionally carry a row-wide near-black bg to mark
// the change extent. Context rows pass bg="" so the terminal default bg
// is used. The rowCache + syntaxCache pair amortizes tokenization to a
// one-shot cost per (lexer, bg, cell) tuple, so the visual gain does not
// regress j/k repeat latency.
//
// In split mode, the side opposite to a +/- change is blank and returned
// untouched so empty cells do not pick up SGR sequences.
func (m Model) colorDiffCell(cell string, kind byte, isRight bool) string {
	switch kind {
	case 'h':
		return fg(cell, m.theme.DiffFileHeader)
	case '@':
		return fg(cell, m.theme.DiffHunkHeader)
	case '+':
		if isRight {
			return m.styledDiffCell(cell, m.theme.DiffPlusBg)
		}
		return cell
	case '-':
		if !isRight {
			return m.styledDiffCell(cell, m.theme.DiffMinusBg)
		}
		return cell
	default:
		return m.styledDiffCell(cell, "")
	}
}

// wrapCell splits content into one or more `cellW`-wide rows. The first row
// holds up to cellW runes of content; continuation rows hold a single leading
// blank (to align past the diff marker) plus up to cellW-1 runes. Every
// returned row is exactly cellW runes wide (right-padded with spaces).
func wrapCell(content string, cellW int) []string {
	if cellW <= 0 {
		return []string{""}
	}
	runes := []rune(content)
	if len(runes) <= cellW {
		return []string{padTrunc(content, cellW)}
	}
	out := []string{padTrunc(string(runes[:cellW]), cellW)}
	contW := cellW - 1
	if contW < 1 {
		contW = 1
	}
	for pos := cellW; pos < len(runes); pos += contW {
		end := pos + contW
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, " "+padTrunc(string(runes[pos:end]), contW))
	}
	return out
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

// padTrunc right-pads or truncates a string to exactly `width` visible
// cells, ignoring SGR escape sequences. Truncation goes through
// ansi.Truncate so SGR codes are preserved (and a final reset is emitted
// when needed) instead of being sliced mid-sequence. Padding always uses
// plain spaces.
func padTrunc(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := lipgloss.Width(s)
	if w == width {
		return s
	}
	if w > width {
		return ansi.Truncate(s, width, "")
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
func parseDiffSpecs(lines []string) []diffLineSpec {
	if len(lines) == 0 {
		return nil
	}
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
	mapping := m.patchNewLineNumbers()
	if len(mapping) == 0 {
		return nil
	}
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

// scrollDiffIntoView clamps DiffViewport.Top so the cursor's first display
// row stays inside the visible window. Wrap-aware: walks the buffer counting
// display rows so a cursor on a multi-row wrapped line is not pushed offscreen
// just because earlier lines also wrapped.
//
// Hot path under j/k repeat: the previous implementation called
// displayRowsBetween every loop iteration (each rebuild splits the patch +
// re-counts wrapped rows). We now compute the initial remaining count
// once and subtract per-line rows as `top` advances, so the loop stays
// O(viewport) regardless of cursor distance.
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
	if top < 0 {
		top = 0
	}
	if top >= totalLines {
		top = totalLines - 1
	}
	if top >= cursor {
		m.state.DiffViewport.Top = top
		return
	}
	if m.paneWidthDiff <= 0 {
		// No layout known yet; fall back to 1:1 buffer-row mapping.
		if cursor-top+1 > height {
			top = cursor - height + 1
		}
		m.state.DiffViewport.Top = top
		return
	}
	lines := m.patchLines()
	if len(lines) == 0 {
		m.state.DiffViewport.Top = top
		return
	}
	isSplit, halfW := m.splitLayout()
	contentW := m.paneWidthDiff - 4
	if contentW <= 0 {
		contentW = 1
	}
	// Sum [top, cursor+1) once.
	remaining := 0
	hi := cursor + 1
	if hi > len(lines) {
		hi = len(lines)
	}
	for i := top; i < hi; i++ {
		remaining += displayRowsForLine(lines[i], isSplit, halfW, contentW)
	}
	for top < cursor && remaining > height {
		remaining -= displayRowsForLine(lines[top], isSplit, halfW, contentW)
		top++
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
	if top >= totalLines {
		top = totalLines - 1
	}
	m.state.DiffViewport.Top = top
}

// displayRowsBetween returns the total number of display rows that buffer
// lines [lo, hi) consume in the current Diff render mode. Used by viewport
// math to handle wrapped lines correctly. When the pane width is not yet
// known (pre-first-frame), falls back to 1 row per buffer line so callers
// behave like the legacy 1:1 mapping.
func (m Model) displayRowsBetween(lo, hi int) int {
	if hi <= lo {
		return 0
	}
	if m.paneWidthDiff <= 0 {
		return hi - lo
	}
	lines := m.patchLines()
	if len(lines) == 0 {
		return 0
	}
	if hi > len(lines) {
		hi = len(lines)
	}
	if lo < 0 {
		lo = 0
	}
	isSplit, halfW := m.splitLayout()
	contentW := m.paneWidthDiff - 4
	if contentW <= 0 {
		contentW = 1
	}
	total := 0
	for i := lo; i < hi; i++ {
		total += displayRowsForLine(lines[i], isSplit, halfW, contentW)
	}
	return total
}

// displayRowsForLine reports the display-row count for a single buffer
// line under the current view mode. Pulled out so scrollDiffIntoView can
// decrement `remaining` one line at a time without re-walking the whole
// patch on every iteration.
func displayRowsForLine(line string, isSplit bool, halfW, contentW int) int {
	expanded := expandTabs(line, 4)
	if isSplit {
		oldSide, newSide := splitDiffLine(expanded)
		l := len(wrapCell(oldSide, halfW))
		r := len(wrapCell(newSide, halfW))
		if r > l {
			l = r
		}
		return l
	}
	return len(wrapCell(expanded, contentW))
}
