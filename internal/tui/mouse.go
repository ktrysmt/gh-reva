package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// measureLayout populates the per-pane render budgets (paneWidth* /
// paneHeight*) from the current terminal size. Called by View() before
// the pane renderers run, and by handleMouse() before hit-testing — the
// value-receiver Update that delivers tea.MouseMsg gets a fresh Model
// that does not carry View's in-flight measurements.
//
// Pre-WindowSize (m.width<=0) and stacked fallback (bodyHeight<8) leave
// the fields untouched so the existing zero-width fallbacks downstream
// stay unchanged.
func (m *Model) measureLayout() {
	if m.width <= 0 {
		return
	}
	bodyHeight := m.height
	if bodyHeight > statusBarRows {
		bodyHeight -= statusBarRows
	}
	if bodyHeight < 8 {
		return
	}
	leftW, midW, rightW := splitColumnWidths(m.width, m.state.CommentsHidden, m.commentsWidthPercent)
	topH, bottomH := splitColumnHeights(bodyHeight)
	m.paneWidthFiles = atLeast(leftW-2, 1)
	m.paneHeightFiles = atLeast(topH-4, 1)
	m.paneWidthCommits = atLeast(leftW-2, 1)
	m.paneHeightCommits = atLeast(bottomH-4, 1)
	m.paneWidthDiff = atLeast(midW-2, 1)
	m.paneHeightDiff = atLeast(bodyHeight-4, 1)
	m.paneWidthComments = atLeast(rightW-2, 1)
	m.paneHeightComments = atLeast(bodyHeight-4, 1)
}

// mouseHit describes a successful (x, y) → pane resolution.
//
//   - Pane:       which pane the cursor landed in.
//   - OnTitle:    true when the click is on the title row; the click
//     should focus the pane and nothing else.
//   - ContentRow: 0-based index inside the inner content area (valid
//     only when OnTitle == false; -1 otherwise).
//   - ContentCol: 0-based column inside the inner content area.
type mouseHit struct {
	Pane       model.PaneID
	OnTitle    bool
	ContentRow int
	ContentCol int
}

// paneAt resolves a terminal coordinate to a pane and (where applicable)
// an inner content row. ok=false is returned for:
//
//   - Pre-WindowSize (m.width<=0) or stacked fallback (bodyHeight<8).
//   - Loading phase (m.state.PR == nil) — there are no clickable rows.
//   - Status-bar rows.
//   - Top / bottom borders, dividers, and side bars.
//   - Hidden Comments column when x falls past the right edge.
//
// Pane outer rect: row 0 = top border, row 1 = title, row 2 = divider,
// rows [3, h-1) = content, row h-1 = bottom border. Side bars sit at
// relX==0 and relX==w-1.
func (m Model) paneAt(x, y int) (mouseHit, bool) {
	if m.state == nil || m.state.PR == nil {
		return mouseHit{}, false
	}
	if m.width <= 0 {
		return mouseHit{}, false
	}
	bodyHeight := m.height
	if bodyHeight > statusBarRows {
		bodyHeight -= statusBarRows
	}
	if bodyHeight < 8 {
		return mouseHit{}, false
	}
	if y < 0 || y >= bodyHeight {
		return mouseHit{}, false
	}
	if x < 0 || x >= m.width {
		return mouseHit{}, false
	}
	leftW, midW, rightW := splitColumnWidths(m.width, m.state.CommentsHidden, m.commentsWidthPercent)
	topH, _ := splitColumnHeights(bodyHeight)

	var (
		paneX, paneW int
		paneY, paneH int
		pane         model.PaneID
	)
	switch {
	case x < leftW:
		paneX, paneW = 0, leftW
		if y < topH {
			pane, paneY, paneH = model.PaneFiles, 0, topH
		} else {
			pane, paneY, paneH = model.PaneCommits, topH, bodyHeight-topH
		}
	case x < leftW+midW:
		paneX, paneW = leftW, midW
		pane, paneY, paneH = model.PaneDiff, 0, bodyHeight
	case !m.state.CommentsHidden && x < leftW+midW+rightW:
		paneX, paneW = leftW+midW, rightW
		pane, paneY, paneH = model.PaneComments, 0, bodyHeight
	default:
		return mouseHit{}, false
	}

	relY := y - paneY
	if relY <= 0 || relY >= paneH-1 {
		return mouseHit{}, false
	}
	relX := x - paneX
	if relX <= 0 || relX >= paneW-1 {
		return mouseHit{}, false
	}
	if relY == 1 {
		return mouseHit{Pane: pane, OnTitle: true, ContentRow: -1}, true
	}
	if relY == 2 {
		return mouseHit{}, false
	}
	return mouseHit{
		Pane:       pane,
		ContentRow: relY - 3,
		ContentCol: relX - 1,
	}, true
}

// handleMouse routes a tea.MouseMsg. Absorbed entirely while a modal /
// help / compose / pending-confirm / search-editing layer is up: those
// layers own input until dismissed, and a stray click or wheel must
// not move state behind them. Loading phase is also absorbed (nothing
// clickable yet). Only Press events are honored — Release / Motion are
// no-ops.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.state == nil || m.state.PR == nil {
		return m, nil
	}
	if m.state.Compose != nil ||
		m.state.PendingConfirm != nil ||
		m.state.HelpOpen ||
		m.state.Modal != nil ||
		(m.state.Search != nil && m.state.Search.Status == model.SearchEditing) {
		return m, nil
	}
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	hit, ok := m.paneAt(msg.X, msg.Y)
	if !ok {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonLeft:
		return m.handleMouseClick(hit)
	case tea.MouseButtonWheelUp:
		return m.handleMouseWheel(hit, -1)
	case tea.MouseButtonWheelDown:
		return m.handleMouseWheel(hit, +1)
	}
	return m, nil
}

// handleMouseClick focuses the clicked pane and (for content-row clicks)
// dispatches to the per-pane click handler. Title clicks focus only.
func (m Model) handleMouseClick(hit mouseHit) (tea.Model, tea.Cmd) {
	m.state.FocusedPane = hit.Pane
	if hit.OnTitle {
		return m, nil
	}
	switch hit.Pane {
	case model.PaneFiles:
		m.mouseClickFiles(hit.ContentRow)
	case model.PaneCommits:
		m.mouseClickCommits(hit.ContentRow)
	case model.PaneDiff:
		m.mouseClickDiff(hit.ContentRow, hit.ContentCol)
	case model.PaneComments:
		m.mouseClickComments(hit.ContentRow)
	}
	return m, nil
}

// mouseClickFiles commits the clicked file: cursor moves AND SelectedFile
// updates so Diff re-renders. j/k stay cursor-only (#19) because the
// per-keystroke Diff rebuild is sluggish under repeat — but a click is
// a deliberate one-shot gesture, so the user expects the Diff column
// to follow immediately. Tree-mode dir rows still fold / unfold and
// leave SelectedFile alone. Focus stays on Files (set by the caller)
// so the user can keep clicking through files without re-targeting.
func (m *Model) mouseClickFiles(row int) {
	if m.state.PR == nil {
		return
	}
	// hit.ContentRow is relative to the visible top of the pane; the pane
	// may be scrolled (FilesTop > 0), so add the offset to recover the
	// absolute row index. FilesTop reflects the last render — exactly the
	// frame the user clicked on.
	row += m.state.FilesTop
	rows := m.filesTreeRows()
	if row < 0 || row >= len(rows) {
		return
	}
	m.state.FilesCursor = row
	r := rows[row]
	switch r.Kind {
	case model.FilesRowAll:
		m.selectAllFiles()
	case model.FilesRowFile:
		m.selectFile(r.Path)
	case model.FilesRowDir:
		if m.state.FoldedDirs[r.Path] {
			delete(m.state.FoldedDirs, r.Path)
		} else {
			m.state.FoldedDirs[r.Path] = true
		}
	}
}

func (m *Model) mouseClickCommits(row int) {
	// Add the viewport offset so a click on a scrolled Commits column
	// resolves to the absolute row (mirrors mouseClickFiles / the
	// Comments-pane CommentsTop offset).
	row += m.state.CommitsTop
	commits := m.visibleCommits()
	if row < 0 || row > len(commits) {
		return
	}
	m.state.CommitsCursor = row
	m.autoSelectCommit(commits)
}

func (m *Model) mouseClickDiff(row, col int) {
	// Content row 0 is the pinned sticky header (#diffStickyHeader), not a
	// scrollable diff row — shift the click down into the scrollable area
	// and ignore clicks on the sticky row itself.
	row -= m.diffStickyRows()
	if row < 0 {
		return
	}
	idx := m.bufferLineAtDiffDisplayRow(row)
	if idx < 0 {
		return
	}
	m.state.DiffCursor.Line = idx
	if side, ok := m.diffSideAtCol(col); ok {
		// Park cursor on the clicked column. Re-snap to the nearest
		// row that exists on the new Side when the click lands on a
		// `+` row but the user clicked the LEFT half (or vice versa)
		// — the user's column gesture wins, the row repositions
		// rather than producing a Side+row state j/k could never
		// produce naturally.
		m.switchSide(side, m.patchLines())
	}
	m.scrollDiffIntoView(len(m.patchLines()))
}

// diffSideAtCol reports which physical column the click landed in.
// Splits on the inner divider position. Returns ok=false in unified
// mode (no Side concept) so the caller leaves DiffCursor.Side alone.
func (m Model) diffSideAtCol(col int) (model.DiffSide, bool) {
	isSplit, halfW := m.splitLayout()
	if !isSplit {
		return "", false
	}
	// Inner column of the divider: leadup = Lcursor 2 + Lmarker 2 +
	// oldLn 4 + sp 1 + leftCell halfW + sp 1 = halfW + 10. The │
	// itself sits at col halfW+10; everything strictly less is LEFT,
	// everything greater is RIGHT, the divider itself defaults to
	// the cursor's existing Side (passed-through caller).
	divider := halfW + 10
	if col < divider {
		return model.DiffSideLeft, true
	}
	if col > divider {
		return model.DiffSideRight, true
	}
	return "", false
}

func (m *Model) mouseClickComments(row int) {
	idx := m.commentIndexAtDisplayRow(row)
	if idx < 0 {
		return
	}
	// Click is a deliberate "make this comment current" gesture. Park
	// CommentsTop on the clicked comment's header row so the new current
	// (= viewport-top) comment is exactly the one the user pointed at;
	// clamp to maxTop so a click near the end doesn't strand the
	// viewport past the last row. The derived cursor then matches idx
	// (or the nearest comment if the clamp ate the requested top).
	rows, headerAt := m.commentsLayout()
	if idx >= len(headerAt) {
		return
	}
	maxTop := commentsMaxTopFor(len(rows), m.commentsViewportHeight())
	top := headerAt[idx]
	if top > maxTop {
		top = maxTop
	}
	m.state.CommentsTop = top
	prev := m.state.CommentsCursor
	m.state.CommentsCursor = m.commentsCursorFromTop(top)
	if m.state.CommentsCursor != prev {
		m.syncDiffToCursorComment()
	}
}

// handleMouseWheel scrolls the pane under the cursor. Wheel does NOT
// change focus — hovering and scrolling without committing focus is
// the expected idiom across editors.
func (m Model) handleMouseWheel(hit mouseHit, dir int) (tea.Model, tea.Cmd) {
	switch hit.Pane {
	case model.PaneFiles:
		m.mouseWheelFiles(dir)
	case model.PaneCommits:
		m.mouseWheelCommits(dir)
	case model.PaneDiff:
		m.mouseWheelDiff(dir)
	case model.PaneComments:
		m.mouseWheelComments(dir)
	}
	return m, nil
}

func (m *Model) mouseWheelFiles(dir int) {
	if m.state.PR == nil {
		return
	}
	rows := m.filesTreeRows()
	next := m.state.FilesCursor + dir
	if next < 0 || next >= len(rows) {
		return
	}
	m.state.FilesCursor = next
}

func (m *Model) mouseWheelCommits(dir int) {
	commits := m.visibleCommits()
	next := m.state.CommitsCursor + dir
	if next < 0 || next > len(commits) {
		return
	}
	m.state.CommitsCursor = next
	m.autoSelectCommit(commits)
}

func (m *Model) mouseWheelDiff(dir int) {
	totalLines := len(m.patchLines())
	if totalLines == 0 {
		return
	}
	next := m.state.DiffCursor.Line + dir
	if next < 0 {
		next = 0
	}
	if next >= totalLines {
		next = totalLines - 1
	}
	m.state.DiffCursor.Line = next
	m.scrollDiffIntoView(totalLines)
}

func (m *Model) mouseWheelComments(dir int) {
	m.scrollCommentsBy(dir)
}

// bufferLineAtDiffDisplayRow maps a Diff content row (0-indexed from the
// top of the inner content area) to the buffer line currently rendered
// there. Walks display-row counts from DiffViewport.Top onward — same
// math as displayRowsForLine — so wrapped lines still land on the
// correct buffer index. Returns -1 when the row falls past content.
func (m Model) bufferLineAtDiffDisplayRow(displayRow int) int {
	if displayRow < 0 {
		return -1
	}
	lines := m.patchLines()
	if len(lines) == 0 {
		return -1
	}
	top := m.state.DiffViewport.Top
	if top < 0 {
		top = 0
	}
	if top >= len(lines) {
		return -1
	}
	if m.paneWidthDiff <= 0 {
		idx := top + displayRow
		if idx >= len(lines) {
			return -1
		}
		return idx
	}
	isSplit, halfW := m.splitLayout()
	contentW := m.paneWidthDiff - 4
	if contentW <= 0 {
		contentW = 1
	}
	accum := 0
	for i := top; i < len(lines); i++ {
		n := displayRowsForLine(lines[i], isSplit, halfW, contentW)
		if displayRow < accum+n {
			return i
		}
		accum += n
	}
	return -1
}

// commentIndexAtDisplayRow maps a Comments content row to the flat-
// comment index rendered there. Body wraps belong to their owning
// comment; blank separator rows (between threads, between root and
// reply) return -1 so the cursor is not stranded on a non-comment row.
//
// Walks the same shape commentsView builds: header + body wraps per
// renderCommentRow, with blank rows interleaved. The walk is O(N×wraps)
// per click which is fine — mouse events are infrequent and the slice
// is bounded by the threads anchored at the current Diff cursor.
func (m Model) commentIndexAtDisplayRow(displayRow int) int {
	if displayRow < 0 {
		return -1
	}
	// displayRow is a pane-relative content row; commentsView slices its
	// rows from CommentsTop, so the layout row the user clicked on is
	// (displayRow + CommentsTop). Without this offset, clicks on a
	// scrolled-down Comments column would resolve to comments above the
	// visible window.
	layoutRow := displayRow + m.state.CommentsTop
	threads := m.threadsForCursor()
	if len(threads) == 0 {
		return -1
	}
	idx, row := 0, 0
	for ti, t := range threads {
		if ti > 0 {
			if layoutRow == row {
				return -1
			}
			row++
		}
		rootRows := m.renderCommentRow(t.Root, 0, idx)
		for j := 0; j < len(rootRows); j++ {
			if layoutRow == row {
				return idx
			}
			row++
		}
		idx++
		for _, r := range t.Replies {
			if layoutRow == row {
				return -1
			}
			row++
			replyRows := m.renderCommentRow(r, 1, idx)
			for j := 0; j < len(replyRows); j++ {
				if layoutRow == row {
					return idx
				}
				row++
			}
			idx++
		}
	}
	return -1
}
