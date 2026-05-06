package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// commentsModalWrapMax caps the modal's wrap budget for the Comments pane.
// Files / Commits content is unwrapped, so its modal width is purely
// content-driven; Comments has no natural max width without a target, so
// we pin one and let modalLayout cap to terminal width on top.
const commentsModalWrapMax = 80

// toggleModal flips the modal for the given pane. Pressing `<space>`
// twice on the same pane closes; pressing on a different pane is not
// reachable today (only the active pane's `<space>` opens its own modal),
// but the same-pane check guards against future bindings.
func (m *Model) toggleModal(pane model.PaneID) {
	if m.state.Modal != nil && m.state.Modal.Pane == pane {
		m.state.Modal = nil
		return
	}
	m.state.Modal = &model.ModalState{Pane: pane}
}

// modalContent returns the body rows shown inside the modal plus the
// title label. Files / Commits reuse the regular pane renderers (their
// row format is width-independent); Comments re-renders with a wider
// wrap budget. Title text is the bare pane name — no `▶` accent because
// the modal is itself the focus indicator.
func (m Model) modalContent() (rows []string, title string) {
	if m.state.Modal == nil {
		return nil, ""
	}
	switch m.state.Modal.Pane {
	case model.PaneFiles:
		return m.paneBodyRows(m.filesView()), "Files"
	case model.PaneCommits:
		return m.paneBodyRows(m.commitsView()), "Commits"
	case model.PaneComments:
		// Local copy mutation: m is a value receiver, so widening
		// paneWidthComments here does not leak into the underlying body
		// rendered behind the modal.
		targetW := m.width - 10
		if targetW > commentsModalWrapMax {
			targetW = commentsModalWrapMax
		}
		if targetW < 30 {
			targetW = 30
		}
		m.paneWidthComments = targetW
		return m.paneBodyRows(m.commentsView()), "Comments"
	}
	return nil, ""
}

// paneBodyRows strips the title from a `title\nbody` pane render, leaving
// just the body lines. Used by modalContent so the modal's own chrome can
// supply the title.
func (m Model) paneBodyRows(view string) []string {
	_, body := splitTitleBody(view)
	if body == "" {
		return nil
	}
	return strings.Split(body, "\n")
}

// modalLayout sizes and positions the modal. Width is content-max + 4
// (1 col padding each side, plus the 2 border cols), capped to
// `m.width - 4`. Height is body rows + 4 (top border + title + divider +
// bottom border), capped to `m.height - 2`. The result is centered both
// axes. Returns ok=false when the terminal is too small to fit the modal
// at all (the underlying body still renders unmodified).
func (m Model) modalLayout() (rows []string, top, left, width int, title string, ok bool) {
	if m.state.Modal == nil {
		return nil, 0, 0, 0, "", false
	}
	if m.width < 12 || m.height < 6 {
		return nil, 0, 0, 0, "", false
	}
	body, label := m.modalContent()
	if len(body) == 0 {
		// Empty body still gets a "(empty)" row so the modal frame draws.
		body = []string{"(empty)"}
	}
	contentW := lipgloss.Width(label) + 1 // ` <Title>` leading space
	for _, r := range body {
		if w := lipgloss.Width(r); w > contentW {
			contentW = w
		}
	}
	// Inner width = content + 1 (leading-space pad mirrors help modal).
	width = contentW + 3
	if max := m.width - 4; width > max {
		width = max
	}
	if width < 10 {
		return nil, 0, 0, 0, "", false
	}
	maxBody := m.height - 4 - 2
	if maxBody < 1 {
		return nil, 0, 0, 0, "", false
	}
	if len(body) > maxBody {
		body = body[:maxBody]
	}
	height := len(body) + 4
	left = (m.width - width) / 2
	top = (m.height - height) / 2
	if left < 0 {
		left = 0
	}
	if top < 0 {
		top = 0
	}
	return body, top, left, width, label, true
}

// renderModalBox draws the modal as a self-contained bordered block. The
// title row carries the pane name (Files / Commits / Comments) preceded
// by a single space — same chrome convention as renderHelpModal so the
// two modals look visually identical.
func (m Model) renderModalBox(rows []string, width int, title string) string {
	innerW := atLeast(width-2, 1)
	bar := strings.Repeat("─", innerW)
	border := m.theme.PaneBorderActive
	side := fg("│", border)
	hr := fg(bar, border)
	var sb strings.Builder
	sb.WriteString(fg("┌"+bar+"┐", border) + "\n")
	sb.WriteString(side + padTrunc(fgBold(" "+title, m.theme.PaneTitleActive), innerW) + side + "\n")
	sb.WriteString(fg("├", border) + hr + fg("┤", border) + "\n")
	for _, ln := range rows {
		sb.WriteString(side + padTrunc(" "+ln, innerW) + side + "\n")
	}
	sb.WriteString(fg("└"+bar+"┘", border))
	return sb.String()
}

// overlayModal splices the modal popup over the body at the rectangle
// returned by modalLayout. Pane chrome columns past the modal stay
// intact via spliceMid, so the user can still see the surrounding panes.
func (m Model) overlayModal(body string) string {
	if m.state.Modal == nil {
		return body
	}
	rows, top, left, width, title, ok := m.modalLayout()
	if !ok {
		return body
	}
	popup := m.renderModalBox(rows, width, title)
	popupRows := strings.Split(popup, "\n")
	bodyRows := strings.Split(body, "\n")
	for i, pr := range popupRows {
		row := top + i
		if row < 0 || row >= len(bodyRows) {
			continue
		}
		bodyRows[row] = spliceMid(bodyRows[row], pr, left, width)
	}
	return strings.Join(bodyRows, "\n")
}

// spliceMid replaces a [col, col+width) slice of `line` with `replacement`
// while preserving everything before and after. Truncation honors SGR
// runs via ansi.Truncate / ansi.TruncateLeft so escape codes do not leak.
// Used by both the help overlay and the pane modal.
func spliceMid(line, replacement string, col, width int) string {
	leftPart := ""
	if col > 0 {
		leftPart = ansi.Truncate(line, col, "")
		if w := lipgloss.Width(leftPart); w < col {
			leftPart += strings.Repeat(" ", col-w)
		}
	}
	rightPart := ansi.TruncateLeft(line, col+width, "")
	return leftPart + replacement + rightPart
}
