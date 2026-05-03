package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ktrysmt/gh-rv/internal/model"
)

// hoverMaxLines caps the popup body so a pathologically long commit
// message cannot eat the screen. 12 rows fits a typical "subject + bullet
// list" body without dominating the layout.
const hoverMaxLines = 12

// hoverEligible answers whether the popup should be drawn given the
// current focus / mode. Visual mode suppresses it because the visual
// banner already carries focus + a row-wide bg highlight.
func (m Model) hoverEligible() bool {
	if !m.state.Hover.Show {
		return false
	}
	if m.state.Visual != nil {
		return false
	}
	return m.state.FocusedPane == model.PaneFiles || m.state.FocusedPane == model.PaneCommits
}

// shouldScheduleHover decides whether to fire a HoverTickMsg after a
// keystroke. Mirrors hoverEligible (Files / Commits + non-visual) but
// ignores the Show flag because we are scheduling, not drawing.
func (m Model) shouldScheduleHover() bool {
	if m.hoverDelay <= 0 {
		return false
	}
	if m.state.Visual != nil {
		return false
	}
	return m.state.FocusedPane == model.PaneFiles || m.state.FocusedPane == model.PaneCommits
}

// scheduleHoverTick snapshots the current Gen and returns a tea.Cmd that
// fires HoverTickMsg with that snapshot after hoverDelay. If the user
// presses anything in the meantime the live Gen will have moved on and
// the eventual message is discarded by the Update handler.
func (m Model) scheduleHoverTick() tea.Cmd {
	if m.hoverDelay <= 0 {
		return nil
	}
	gen := m.state.Hover.Gen
	delay := m.hoverDelay
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return HoverTickMsg{Gen: gen}
	})
}

// hoverLines returns the popup body. Files = single-line path. Commits =
// "<sha> <subject>" followed by every body line of the commit message,
// preserving blank lines so bullet-list / paragraph-style descriptions
// render verbatim.
func (m Model) hoverLines() []string {
	if m.state == nil || m.state.PR == nil {
		return nil
	}
	switch m.state.FocusedPane {
	case model.PaneFiles:
		idx := m.state.FilesCursor
		files := m.state.PR.Files
		if idx < 0 || idx >= len(files) {
			return nil
		}
		return []string{files[idx].Path}
	case model.PaneCommits:
		commits := m.visibleCommits()
		idx := m.state.CommitsCursor
		if idx < 0 || idx >= len(commits) {
			return nil
		}
		c := commits[idx]
		subject, body := splitCommitMessage(c.Message)
		out := []string{shortSHA(c.SHA) + " " + subject}
		out = append(out, body...)
		return out
	}
	return nil
}

// splitCommitMessage divides a git commit message into its subject (first
// line) and body lines. The conventional blank separator between subject
// and body is dropped; subsequent blank lines inside the body are kept so
// paragraph structure survives.
func splitCommitMessage(msg string) (subject string, body []string) {
	msg = strings.TrimRight(msg, "\n")
	idx := strings.IndexByte(msg, '\n')
	if idx < 0 {
		return msg, nil
	}
	subject = msg[:idx]
	rest := strings.TrimLeft(msg[idx+1:], "\n")
	if rest == "" {
		return subject, nil
	}
	return subject, strings.Split(rest, "\n")
}

// hoverCursorRow returns the screen row index of the focused pane's
// cursor, or -1 when the cursor is not visible (off-screen, wrong pane,
// degenerate window). The math mirrors the View() column layout:
// pane top row + 1 (top border) + 1 (title) + 1 (divider) + intra-pane
// cursor index. Files / Commits do not scroll, so a cursor beyond the
// pane's content rows is simply hidden by the box renderer; we bail in
// that case so the popup never points at empty space.
func (m Model) hoverCursorRow() int {
	if m.height <= 0 || m.width <= 0 {
		return -1
	}
	bodyHeight := m.height
	if m.state.Visual != nil {
		bodyHeight--
	}
	topH, bottomH := splitColumnHeights(bodyHeight)
	switch m.state.FocusedPane {
	case model.PaneFiles:
		row := 3 + m.state.FilesCursor
		if row >= topH-1 {
			return -1
		}
		return row
	case model.PaneCommits:
		row := topH + 3 + m.state.CommitsCursor
		if row >= topH+bottomH-1 {
			return -1
		}
		return row
	}
	return -1
}

// hoverLayout decides where to draw the popup and how many body lines to
// show. Position priority is: above the cursor (the natural reading
// direction once the user has pressed j to land on a row), then below
// (if there is no room above), then "best effort" — pick the side with
// more vertical space and shrink the popup to fit.
func (m Model) hoverLayout() (lines []string, top, left, width int, ok bool) {
	rawLines := m.hoverLines()
	if len(rawLines) == 0 {
		return nil, 0, 0, 0, false
	}
	cursorRow := m.hoverCursorRow()
	if cursorRow < 0 {
		return nil, 0, 0, 0, false
	}
	leftW, _, _ := splitColumnWidths(m.width)
	left = leftW
	width = m.width - leftW
	if width < 30 {
		return nil, 0, 0, 0, false
	}

	spaceAbove := cursorRow
	spaceBelow := m.height - cursorRow - 1
	if m.state.Visual != nil {
		spaceBelow--
	}

	want := len(rawLines)
	if want > hoverMaxLines {
		want = hoverMaxLines
	}
	contentN := want
	height := contentN + 2

	// Prefer above when it fits.
	if height <= spaceAbove {
		top = cursorRow - height
		return rawLines[:contentN], top, left, width, true
	}
	if height <= spaceBelow {
		top = cursorRow + 1
		return rawLines[:contentN], top, left, width, true
	}

	// Neither side fits the full popup — pick the larger side and
	// shrink content to fit. We still need at least one content row
	// (so spaceN-2 >= 1) for the popup to be useful.
	pickAbove := spaceAbove >= spaceBelow
	avail := spaceAbove
	if !pickAbove {
		avail = spaceBelow
	}
	contentN = avail - 2
	if contentN < 1 {
		return nil, 0, 0, 0, false
	}
	height = contentN + 2
	if pickAbove {
		top = cursorRow - height
	} else {
		top = cursorRow + 1
	}
	return rawLines[:contentN], top, left, width, true
}

// renderHoverPopupBlock builds the bordered tooltip box. width is the
// outer width including borders; inner content area is width-2.
func (m Model) renderHoverPopupBlock(lines []string, width int) string {
	if len(lines) == 0 {
		return ""
	}
	innerW := atLeast(width-2, 1)
	bar := strings.Repeat("─", innerW)
	border := m.theme.PaneBorderActive
	side := fg("│", border)
	var sb strings.Builder
	sb.WriteString(fg("┌"+bar+"┐", border) + "\n")
	for _, ln := range lines {
		sb.WriteString(side + padTrunc(ln, innerW) + side + "\n")
	}
	sb.WriteString(fg("└"+bar+"┘", border))
	return sb.String()
}

// overlayHover splices the popup over the body at the rectangle returned
// by hoverLayout. Each popup row replaces visible columns
// [left .. left+width) of the corresponding body row; the prefix is
// preserved (active pane chrome) and the suffix is dropped (the popup
// extends to the screen's right edge so nothing significant lives past
// it).
func (m Model) overlayHover(body string) string {
	if !m.hoverEligible() {
		return body
	}
	lines, top, left, width, ok := m.hoverLayout()
	if !ok {
		return body
	}
	popup := m.renderHoverPopupBlock(lines, width)
	if popup == "" {
		return body
	}
	popupRows := strings.Split(popup, "\n")
	bodyRows := strings.Split(body, "\n")
	for i, pr := range popupRows {
		row := top + i
		if row < 0 || row >= len(bodyRows) {
			continue
		}
		bodyRows[row] = spliceColumn(bodyRows[row], pr, left)
	}
	return strings.Join(bodyRows, "\n")
}

// spliceColumn returns line with everything past visible column col
// dropped, then replacement appended. The original prefix is padded with
// spaces if it was shorter than col cells. SGR codes inside the prefix
// are preserved by ansi.Truncate; padding is plain spaces so trailing
// styles do not bleed into the popup background.
func spliceColumn(line, replacement string, col int) string {
	if col <= 0 {
		return replacement
	}
	leftPart := ansi.Truncate(line, col, "")
	leftW := lipgloss.Width(leftPart)
	if leftW < col {
		leftPart += strings.Repeat(" ", col-leftW)
	}
	return leftPart + replacement
}
