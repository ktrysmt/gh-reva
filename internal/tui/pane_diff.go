package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ktrysmt/gh-reva/internal/diff"
	"github.com/ktrysmt/gh-reva/internal/model"
)

func (m Model) handleKeyDiff(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	lines := m.patchLines()
	totalLines := len(lines)
	if totalLines == 0 {
		// Avoid division/clamp wrap when there is no diff to navigate.
		totalLines = 1
	}
	pageSize := m.diffViewportHeight()
	half := pageSize / 2
	key := msg.String()
	// Resolve the `g`-prefix sequence (vim semantics). See handlePendingG
	// for the shared two-key state machine.
	if handled := m.handlePendingG(key, func() {
		m.state.DiffCursor.Line = firstSideLine(lines, m.state.DiffCursor.Side)
		m.scrollDiffIntoView(totalLines)
	}); handled {
		return m, nil
	}
	switch key {
	case "h":
		// h/l are no-op in unified mode (column has no meaning) and in
		// visual mode (Side pinned at anchor). Both surface a Notice so
		// the user understands why the keystroke didn't move them.
		// Visual handling lives in handleKeyVisual; this branch only
		// fires when Visual is nil because handleKeyVisual absorbs
		// every key first.
		if m.effectiveDiffViewMode() != "split" {
			m.state.Notice = "h/l: split mode only"
			return m, nil
		}
		m.switchSide(model.DiffSideLeft, lines)
		m.scrollDiffIntoView(totalLines)
		return m, nil
	case "l":
		if m.effectiveDiffViewMode() != "split" {
			m.state.Notice = "h/l: split mode only"
			return m, nil
		}
		m.switchSide(model.DiffSideRight, lines)
		m.scrollDiffIntoView(totalLines)
		return m, nil
	case "j", "down":
		next := nextSideLine(lines, m.state.DiffCursor.Line, m.state.DiffCursor.Side, +1)
		if next >= 0 {
			m.state.DiffCursor.Line = next
		}
	case "k", "up":
		next := nextSideLine(lines, m.state.DiffCursor.Line, m.state.DiffCursor.Side, -1)
		if next >= 0 {
			m.state.DiffCursor.Line = next
		}
	case "G":
		m.state.DiffCursor.Line = lastSideLine(lines, m.state.DiffCursor.Side)
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
		// Synthetic `···` rows have their own Enter contract — expand
		// the hidden gap by 20 lines (10/10 for inter-hunk, 20 toward
		// the edge for BOF / EOF). Routed before compose / focus
		// handoff so the synthetic short-circuits both. The branch is
		// the only Enter path that mutates ExpandedContext.
		cursor := m.state.DiffCursor.Line
		if cursor >= 0 && cursor < len(lines) && lines[cursor] == diff.SyntheticLine {
			return m, m.handleEnterOnSynthetic(cursor)
		}
		// On a row that already carries one or more anchored review
		// threads, Enter shifts focus to the Comments pane so the user
		// can read and act on the existing comments via the per-pane
		// keymap (Enter = edit own / r = reply / Space = zoom modal).
		// The Comments column auto-reveals if Ctrl+E had it hidden so
		// focus never lands on an invisible pane. On a row WITHOUT
		// existing comments, Enter falls through to the inline-compose
		// confirm prompt. Header / hunk rows still no-op via
		// buildComposeInline.
		if len(m.threadsForCursor()) > 0 {
			m.focusCommentsAtCursor()
			return m, nil
		}
		return m, m.startComposeInline()
	}
	m.scrollDiffIntoView(totalLines)
	return m, nil
}

// switchSide flips DiffCursor.Side and, when the new Side does not
// host the cursor's current row, repositions the cursor to the nearest
// row that does (preferring upward — see nearestSideLine). Idempotent
// when the requested Side already matches.
func (m *Model) switchSide(target model.DiffSide, lines []string) {
	if m.state.DiffCursor.Side == target {
		return
	}
	m.state.DiffCursor.Side = target
	if len(lines) == 0 {
		return
	}
	cur := m.state.DiffCursor.Line
	if cur >= 0 && cur < len(lines) && lineExistsOnSide(lines[cur], target) {
		return
	}
	if next := nearestSideLine(lines, cur, target); next >= 0 {
		m.state.DiffCursor.Line = next
	}
}

// focusCommentsAtCursor shifts focus to the Comments pane so the user
// can read and act on the threads anchored at the current Diff cursor.
// CommentsCursor resets to 0 so the user lands on the first thread of
// the row. If the Comments column is hidden (Ctrl+E), un-hide it
// first — Tab / Shift+Tab skip Comments while hidden, so leaving
// focus on an invisible pane would strand the user. The user can
// re-hide with Ctrl+E once they're done; a Comments-pane Space opens
// the zoom modal for a wider read.
func (m *Model) focusCommentsAtCursor() {
	m.state.CommentsHidden = false
	m.state.FocusedPane = model.PaneComments
	m.state.CommentsCursor = 0
	m.state.CommentsTop = 0
}

func (m Model) diffView() string {
	label := "Diff"
	if m.state.SelectedFile != "" {
		// The synthetic All view shows a cross-file concat — render a
		// human-readable label instead of leaking the AllFilesPath
		// sentinel (which contains NUL bytes the terminal would strip
		// or display as "ALL_FILES" anyway).
		shown := m.state.SelectedFile
		if shown == model.AllFilesPath {
			shown = fmt.Sprintf("All files (%d)", len(m.state.PR.Files))
		}
		label = fmt.Sprintf("Diff: %s", shown)
		if m.state.SelectedRange.Kind == model.RangeSingleCommit {
			label = fmt.Sprintf("Diff: %s @ %s", shown, shortSHA(m.state.SelectedRange.SHA))
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
	markers := m.commentLineMarkers()
	matched := m.searchMatchLines()
	isSplit, halfW := m.splitLayout()
	var specs []diffLineSpec
	if isSplit {
		specs = m.patchSpecs()
	}
	cursorLine := m.state.DiffCursor.Line
	cursorSide := m.state.DiffCursor.Side
	gaps := m.patchGaps()
	var out []string
	for i := top; i < end && len(out) < height; i++ {
		var rows []string
		if lines[i] == diff.SyntheticLine {
			rows = m.renderSynthBufferLine(i, cursorLine, gaps[i])
		} else if isSplit {
			rows = m.renderSplitBufferLine(lines[i], specs[i], halfW, i, cursorLine, cursorSide, markers.Left[i], markers.Right[i], matched[i])
		} else {
			// Unified mode collapses both columns into one cell, so the
			// per-side ◆ split is meaningless — fold the two maps and
			// pass whichever rank-wins. markerRank's precedence runs
			// here so a buffer carrying ◆ on one side and │ on the
			// other still shows ◆ in the lone gutter.
			rows = m.renderUnifiedBufferLine(lines[i], i, cursorLine, foldMarker(markers.Left[i], markers.Right[i]), matched[i])
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

// renderSynthBufferLine paints the `··· N lines hidden  (enter: expand)`
// hint for a hidden gap.
//
// Unified mode: a single full-width row with `> ` at col 0 (cursor /
// visual rows) and the body filling the remainder.
//
// Split mode: mirror the standard split geometry (#8 / #14d) — the body
// is painted on BOTH the left and right cells, `│` divides them, and
// `> ` follows DiffCursor.Side so the cursor visually stays on the same
// column as adjacent diff rows. Per-side line-number and marker columns
// are blank (synthetic rows aren't comment-anchorable; see #10). The
// shared label degrades through 5 width tiers so narrow halves still
// surface the hidden count.
func (m Model) renderSynthBufferLine(idx, cursorLine int, gap diff.GapInfo) []string {
	inVisual := m.inVisualRange(model.PaneDiff, idx)
	cursorActive := idx == cursorLine || inVisual
	isSplit, halfW := m.splitLayout()
	if !isSplit {
		cursor := "  "
		if cursorActive {
			cursor = fgBold("> ", m.theme.CursorRow)
		}
		label := fmt.Sprintf("··· %d lines hidden  (enter: expand)", gap.HiddenCount)
		body := fg(label, m.theme.DiffHunkHeader)
		return []string{padTrunc(cursor+body, m.paneWidthDiff)}
	}

	label := synthLabel(gap.HiddenCount, halfW)
	body := fg(label, m.theme.DiffHunkHeader)
	leftCell := padTrunc(body, halfW)
	rightCell := padTrunc(body, halfW)

	lCursor, rCursor := "  ", "  "
	if cursorActive {
		glyph := fgBold("> ", m.theme.CursorRow)
		if m.state.DiffCursor.Side == model.DiffSideLeft {
			lCursor = glyph
		} else {
			rCursor = glyph
		}
	}
	sep := fg("│", m.theme.DiffSeparator)
	row := padTrunc(lCursor+"  "+"    "+" "+leftCell+" "+sep+" "+"  "+rCursor+"    "+" "+rightCell, m.paneWidthDiff)
	return []string{row}
}

// synthLabel returns the longest `··· N …` variant that fits cellW
// display columns. The five tiers degrade gracefully: full hint → short
// hint → no hint → minimal → just `···` so even halfW=8 (the split
// engage threshold) shows the marker.
func synthLabel(hidden, cellW int) string {
	candidates := []string{
		fmt.Sprintf("··· %d lines hidden  (enter: expand)", hidden),
		fmt.Sprintf("··· %d lines hidden (enter)", hidden),
		fmt.Sprintf("··· %d lines hidden", hidden),
		fmt.Sprintf("··· %d hidden", hidden),
		"···",
	}
	for _, s := range candidates {
		if lipgloss.Width(s) <= cellW {
			return s
		}
	}
	return "···"
}

// renderUnifiedBufferLine returns the display rows for one buffer line in
// unified mode. First row is `<cursor 2><marker 2><content>` where content is
// the wrap-cell head. Continuation rows are `<4 blanks><wrap-cell tail>`,
// where the tail's leading blank aligns past the diff marker (`+`/`-`/space)
// — so total continuation indent is 5 cols (cursor 2 + marker 2 + 1).
//
// `marker` is the gutter glyph for this buffer line (markerAnchor or
// markerResolved, see commentLineMarkers). Zero value = no glyph
// (blank gutter). The glyph appears on the FIRST display row only;
// continuation rows always blank the gutter.
func (m Model) renderUnifiedBufferLine(line string, idx, cursorLine int, marker rune, matched bool) []string {
	isCursor := idx == cursorLine
	inVisual := m.inVisualRange(model.PaneDiff, idx)
	// Match-bg rows skip the cache because the match set varies with the
	// query keystroke-by-keystroke; treating them like cursor / visual
	// rows keeps the cache consistent without growing its key.
	cacheKey := ""
	if !isCursor && !inVisual && !matched && m.rowCache != nil {
		cacheKey = m.rowCacheKey("u", idx, 0, marker)
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
	// colorDiffCell's isRight param was designed for split mode (where the
	// opposite side of a +/- change is empty and must stay unstyled). In
	// unified there is only one cell per row, so route '+' through the
	// right-side branch (DiffPlusBg) and '-' through the left-side branch
	// (DiffMinusBg) so both kinds receive bg + syntax. Header / hunk /
	// context kinds ignore this flag.
	isRight := kind == '+'
	out := make([]string, 0, len(cells))
	for j, cell := range cells {
		colored := m.colorDiffCell(cell, kind, isRight, matched)
		var prefix string
		if j == 0 {
			cursor := m.cursorMarker(model.PaneDiff, idx, cursorLine)
			if cursor == "> " {
				cursor = fgBold(cursor, m.theme.CursorRow)
			}
			gutter := "  "
			if marker != 0 {
				gutter = fg(string(marker)+" ", m.markerColor(marker))
			}
			prefix = cursor + gutter
		} else {
			cursorCol := "  "
			if inVisual {
				cursorCol = fgBold("> ", m.theme.CursorRow)
			}
			prefix = cursorCol + "  "
		}
		row := padTrunc(prefix+colored, m.paneWidthDiff)
		out = append(out, row)
	}
	if cacheKey != "" {
		m.rowCache.put(cacheKey, out)
	}
	return out
}

// renderSplitBufferLine returns the display rows for one buffer line in split
// mode. First row carries Lcursor / Lmarker / oldLn / leftCell / │ / Rmarker /
// Rcursor / newLn / rightCell. Continuation rows blank cursor / marker /
// line-number columns, re-draw │ at the same column, and prefix each half-cell
// with 1 blank to align past the diff marker.
//
// Hot path under j/k repeat: split mode does ~2× the per-row work of
// unified (two cells, two line-number gutters, separator). To keep
// `j`-hold responsive we cache the final []string output keyed on the
// inputs that actually affect rendering. The cursor and visual rows are
// not cached (they change every keystroke); everything else is, so 28/30
// visible rows hit the cache on each redraw.
//
// `cursorSide` decides which physical column carries the `> ` glyph
// (Lcursor when LEFT, Rcursor when RIGHT). `leftMarker` / `rightMarker`
// are the per-side gutter glyphs (markerAnchor or markerResolved); 0
// leaves the corresponding gutter blank.
func (m Model) renderSplitBufferLine(line string, spec diffLineSpec, halfW, idx, cursorLine int, cursorSide model.DiffSide, leftMarker, rightMarker rune, matched bool) []string {
	isCursor := idx == cursorLine
	inVisual := m.inVisualRange(model.PaneDiff, idx)
	// Match-bg rows skip the cache for the same reason as the unified
	// path (#renderUnifiedBufferLine): per-keystroke match-set drift.
	cacheKey := ""
	if !isCursor && !inVisual && !matched && m.rowCache != nil {
		// cursorSide intentionally NOT in the key: it only changes the
		// `> ` glyph in the cursor / visual cells, both of which take
		// the no-cache path above. Including it would invalidate every
		// non-cursor row on each h/l press and accumulate dead entries
		// until the user flipped Side back. rightMarker and leftMarker
		// both stay because they DO render on every row.
		cacheKey = m.rowCacheKey("s", idx, halfW, leftMarker) + "\x00" + string(rightMarker)
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
	cursorActive := isCursor || inVisual
	for j := 0; j < n; j++ {
		left := blank
		if j < len(leftCells) {
			left = leftCells[j]
		}
		right := blank
		if j < len(rightCells) {
			right = rightCells[j]
		}
		left = m.colorDiffCell(left, spec.Kind, false, matched)
		right = m.colorDiffCell(right, spec.Kind, true, matched)

		var lCursor, lGutter, rCursor, rGutter, oldLn, newLn string
		if j == 0 {
			lCursor = "  "
			rCursor = "  "
			if cursorActive {
				glyph := fgBold("> ", m.theme.CursorRow)
				if cursorSide == model.DiffSideLeft {
					lCursor = glyph
				} else {
					rCursor = glyph
				}
			}
			lGutter = renderGutter(leftMarker, m.markerColor(leftMarker))
			rGutter = renderGutter(rightMarker, m.markerColor(rightMarker))
			oldLn = fg(lnFmt(spec.OldLn, kindHasOld(spec.Kind)), m.theme.DiffLineNumber)
			newLn = fg(lnFmt(spec.NewLn, kindHasNew(spec.Kind)), m.theme.DiffLineNumber)
		} else {
			lCursor = "  "
			rCursor = "  "
			if inVisual {
				glyph := fgBold("> ", m.theme.CursorRow)
				if cursorSide == model.DiffSideLeft {
					lCursor = glyph
				} else {
					rCursor = glyph
				}
			}
			lGutter = "  "
			rGutter = "  "
			oldLn = "    "
			newLn = "    "
		}
		row := padTrunc(lCursor+lGutter+oldLn+" "+left+" "+sep+" "+rGutter+rCursor+newLn+" "+right, m.paneWidthDiff)
		out = append(out, row)
	}
	if cacheKey != "" {
		m.rowCache.put(cacheKey, out)
	}
	return out
}

// renderGutter returns the 2-col gutter glyph for the FIRST display row
// of a buffer line. Empty marker → 2 blanks; otherwise the glyph plus a
// trailing blank, colored with the comment-anchor accent.
func renderGutter(marker rune, color lipgloss.Color) string {
	if marker == 0 {
		return "  "
	}
	return fg(string(marker)+" ", color)
}

// markerColor returns the gutter glyph color for a given marker rune.
// markerResolved (✓) uses theme.CommentResolved (green semantic) so it
// reads as "concern addressed" at a glance, distinct from the unresolved
// markerAnchor (◆) / range markers that share theme.CommentAnchor.
func (m Model) markerColor(marker rune) lipgloss.Color {
	if marker == markerResolved {
		return m.theme.CommentResolved
	}
	return m.theme.CommentAnchor
}

// foldMarker collapses a (Left, Right) marker pair into a single glyph
// for unified mode. Higher rank wins so a row carrying ◆ on one side
// and │ on the other still draws ◆ in the lone unified gutter. Both
// zero → 0.
func foldMarker(left, right rune) rune {
	if markerRank(left) >= markerRank(right) {
		return left
	}
	return right
}

// diffLineKind classifies a unified-diff buffer line into the same byte tags
// used by parseDiffSpecs ('h' file header, '@' hunk, '+'/'-' add/del, ' '
// context, 's' synthetic `···` row). Order matters — synthetic and
// `---`/`+++` must be tested before `+`/`-`.
func diffLineKind(line string) byte {
	switch {
	case line == diff.SyntheticLine:
		return 's'
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
//
// When matched=true (active search match on this buffer line), the cell's
// bg is replaced with theme.SearchMatchBg on the side(s) that carry the
// content for `kind`. Empty opposite-side cells stay untouched so the
// highlight only paints where the actual matched text lives — fixing the
// bug where the bg leaked onto the empty LEFT lane of a `+` row (and the
// empty RIGHT lane of a `-` row). bgRow wrapping the whole row used to
// drive the highlight, but lipgloss's outer Background() does not
// re-apply after internal \e[0m resets, so most of the row's bg was
// silently stripped; baking the bg into each chroma token via
// styledDiffCell sidesteps that.
func (m Model) colorDiffCell(cell string, kind byte, isRight, matched bool) string {
	matchBg := lipgloss.Color("")
	if matched {
		matchBg = m.theme.SearchMatchBg
	}
	switch kind {
	case 'h':
		if matched {
			return lipgloss.NewStyle().Foreground(m.theme.DiffFileHeader).Background(matchBg).Render(cell)
		}
		return fg(cell, m.theme.DiffFileHeader)
	case '@':
		if matched {
			return lipgloss.NewStyle().Foreground(m.theme.DiffHunkHeader).Background(matchBg).Render(cell)
		}
		return fg(cell, m.theme.DiffHunkHeader)
	case '+':
		if isRight {
			bg := m.theme.DiffPlusBg
			if matched {
				bg = matchBg
			}
			return m.styledDiffCell(cell, bg)
		}
		return cell
	case '-':
		if !isRight {
			bg := m.theme.DiffMinusBg
			if matched {
				bg = matchBg
			}
			return m.styledDiffCell(cell, bg)
		}
		return cell
	default:
		return m.styledDiffCell(cell, matchBg)
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
//	<Lcursor 2><Lmarker 2><lnL 4>< 1><leftCell halfW>< 1><│ 1>< 1><Rmarker 2><Rcursor 2><lnR 4>< 1><rightCell halfW>
//
// fixed overhead = 2+2 + (4+1) + 3 + 2 + 2 + (4+1) = 21,
// so halfW = (paneWidthDiff - 21) / 2. Per-side cursor / marker columns
// are required for h/l Side switching: a single cursor column can't
// indicate which physical lane the user is parked in.
func (m Model) splitLayout() (bool, int) {
	if m.effectiveDiffViewMode() != "split" {
		return false, 0
	}
	if m.paneWidthDiff <= 0 {
		return false, 0
	}
	avail := m.paneWidthDiff - 21
	if avail < 16 {
		return false, 0
	}
	return true, avail / 2
}

// splitDiffLine routes a unified-diff line to its old / new column. Headers
// (---, +++, @@) and context lines appear identically on both sides.
// Synthetic rows render via the special-case path in renderSplitBufferLine
// and never reach this router, so the sentinel falls through to the
// "both sides" default.
func splitDiffLine(line string) (string, string) {
	if line == diff.SyntheticLine {
		return line, line
	}
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
// patch on every iteration. Synthetic rows always count as a single
// display row — they never wrap because the renderer truncates the
// hint label to the pane width.
func displayRowsForLine(line string, isSplit bool, halfW, contentW int) int {
	if line == diff.SyntheticLine {
		return 1
	}
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
