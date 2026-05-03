package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ktrysmt/gh-rv/internal/model"
)

// hoverPopupRows is the rendered popup height (top border + content + bottom
// border). Kept fixed so layout math is predictable; long content is
// truncated with an ellipsis rather than wrapped to multiple rows. The
// reasoning: the popup is meant to disambiguate cut-off rows, not
// preserve every byte. If the path is also too long for the popup, the
// truncation point shifts but the user has already learned more.
const hoverPopupRows = 3

// hoverEligible answers whether the popup should be drawn given the
// current focus / mode. Visual mode suppresses it because the visual
// banner already occupies the body bottom.
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
// keystroke. The check mirrors hoverEligible (Files / Commits + non-visual)
// but ignores the Show flag because we are scheduling, not drawing.
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

// hoverContent returns the text shown in the popup for the focused pane.
// Files: full path. Commits: short SHA + subject. Other panes return ""
// so callers know to skip rendering.
func (m Model) hoverContent() string {
	if m.state == nil || m.state.PR == nil {
		return ""
	}
	switch m.state.FocusedPane {
	case model.PaneFiles:
		idx := m.state.FilesCursor
		files := m.state.PR.Files
		if idx < 0 || idx >= len(files) {
			return ""
		}
		return files[idx].Path
	case model.PaneCommits:
		commits := m.visibleCommits()
		idx := m.state.CommitsCursor
		if idx < 0 || idx >= len(commits) {
			return ""
		}
		c := commits[idx]
		return shortSHA(c.SHA) + " " + c.Message
	}
	return ""
}

// renderHoverPopup builds the bordered tooltip box. Width is the outer
// width including borders; inner content area is width-2.
func (m Model) renderHoverPopup(width int) string {
	content := m.hoverContent()
	if content == "" {
		return ""
	}
	innerW := atLeast(width-2, 1)
	bar := strings.Repeat("─", innerW)
	border := m.theme.PaneBorderActive
	side := fg("│", border)
	var sb strings.Builder
	sb.WriteString(fg("┌"+bar+"┐", border) + "\n")
	sb.WriteString(side + padTrunc(content, innerW) + side + "\n")
	sb.WriteString(fg("└"+bar+"┘", border))
	return sb.String()
}

// overlayHover replaces the bottom hoverPopupRows of body with a centered
// popup. The covered rows are pane bottom borders / blank slack — losing
// them for the popup duration is preferable to the alternative (reserving
// rows always, which shifts pane layout on every show/hide cycle).
//
// Inputs body assumed to be a single string with \n-delimited rows. Width
// is m.width (terminal width); when zero or too small the original body
// is returned unchanged.
func (m Model) overlayHover(body string) string {
	if !m.hoverEligible() {
		return body
	}
	if m.width <= 0 {
		return body
	}
	popupW := m.width - 4
	if popupW > 100 {
		popupW = 100
	}
	if popupW < 20 {
		return body
	}
	popup := m.renderHoverPopup(popupW)
	if popup == "" {
		return body
	}
	popupLines := strings.Split(popup, "\n")
	bodyLines := strings.Split(body, "\n")
	if len(bodyLines) < len(popupLines) {
		return body
	}
	start := len(bodyLines) - len(popupLines)
	for i, pl := range popupLines {
		row := start + i
		w := lipgloss.Width(pl)
		leftPad := (m.width - w) / 2
		if leftPad < 0 {
			leftPad = 0
		}
		rightPad := m.width - leftPad - w
		if rightPad < 0 {
			rightPad = 0
		}
		bodyLines[row] = strings.Repeat(" ", leftPad) + pl + strings.Repeat(" ", rightPad)
	}
	return strings.Join(bodyLines, "\n")
}
