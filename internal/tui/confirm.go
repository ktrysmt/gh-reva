package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// confirmModalDefaultWidth is the modal's outer width on a comfortably-wide
// terminal. Sized so the typical "<path>:<line> <SIDE>" subject fits on one
// row without horizontal padding looking lonely; narrower terminals clamp
// to `m.width - 4` in confirmModalLayout.
const confirmModalDefaultWidth = 50

// confirmKeymapHint is the [y]es / [n]o footer rendered at the bottom of
// every confirm modal. Three spaces between options keep the two hot keys
// visually distinct without spreading them across the full inner width.
const confirmKeymapHint = "[y]es   [n]o"

// confirmModalTitle returns the action verb shown in the modal's title row.
// One verb per ComposeKind so the user can tell at a glance whether they
// are about to start a fresh thread, post a reply, or rewrite an existing
// comment.
func confirmModalTitle(kind model.ComposeKind) string {
	switch kind {
	case model.ComposeReply:
		return "Post reply?"
	case model.ComposeEdit:
		return "Edit comment?"
	default:
		return "Start new comment?"
	}
}

// confirmModalSubject returns the target description rendered as the
// modal body. The shape is kind-specific:
//
//   - Inline (single line): `<path>:<line> <SIDE>`
//   - Inline (range, same side): `<path>:<start>-<line> <SIDE>`
//   - Inline (range, mixed sides): `<path>:<start> <STARTSIDE> → <line> <SIDE>`
//   - Reply: `<path>:<line> by <root.User>` (lookup via ParentThreadID)
//   - Edit:  `<path>:<line> <SIDE>` (lookup via NodeID)
//
// Reply / Edit fall back to the empty string if the lookup fails (PR not
// loaded, comment evicted by a refresh between confirm-build and render);
// the modal still renders, just with an empty subject row.
func (m Model) confirmModalSubject(cs *model.ComposeState) string {
	if cs == nil {
		return ""
	}
	switch cs.Kind {
	case model.ComposeInline:
		return inlineConfirmSubject(cs)
	case model.ComposeReply:
		return m.replyConfirmSubject(cs)
	case model.ComposeEdit:
		return m.editConfirmSubject(cs)
	}
	return ""
}

func inlineConfirmSubject(cs *model.ComposeState) string {
	side := cs.Side
	if side == "" {
		side = "RIGHT"
	}
	if cs.StartLine != nil {
		startSide := cs.StartSide
		if startSide == "" {
			startSide = side
		}
		if startSide == side {
			return fmt.Sprintf("%s:%d-%d %s", cs.Path, *cs.StartLine, cs.Line, side)
		}
		return fmt.Sprintf("%s:%d %s → %d %s", cs.Path, *cs.StartLine, startSide, cs.Line, side)
	}
	return fmt.Sprintf("%s:%d %s", cs.Path, cs.Line, side)
}

func (m Model) replyConfirmSubject(cs *model.ComposeState) string {
	if m.state == nil || m.state.PR == nil {
		return ""
	}
	for _, c := range m.state.PR.Comments {
		if c == nil || c.ThreadID != cs.ParentThreadID {
			continue
		}
		if c.InReplyTo != 0 {
			continue
		}
		return fmt.Sprintf("%s:%d by %s", c.Path, c.Line, c.User)
	}
	return ""
}

func (m Model) editConfirmSubject(cs *model.ComposeState) string {
	if m.state == nil || m.state.PR == nil {
		return ""
	}
	for _, c := range m.state.PR.Comments {
		if c == nil || c.NodeID != cs.EditCommentNodeID {
			continue
		}
		side := c.Side
		if side == "" {
			side = "RIGHT"
		}
		return fmt.Sprintf("%s:%d %s", c.Path, c.Line, side)
	}
	return ""
}

// confirmModalLayout sizes and positions the confirm modal. Width target
// is the longest of (title, subject, footer) plus chrome, floored at
// confirmModalDefaultWidth and capped to `m.width - 4`. Subject is
// truncated with `…` when it overflows the resulting inner width so the
// modal never spills.
func (m Model) confirmModalLayout() (rows []string, top, left, width int, title string, ok bool) {
	pc := m.state.PendingConfirm
	if pc == nil || pc.Compose == nil {
		return nil, 0, 0, 0, "", false
	}
	if m.width < 14 || m.height < 6 {
		return nil, 0, 0, 0, "", false
	}
	title = confirmModalTitle(pc.Compose.Kind)
	subject := m.confirmModalSubject(pc.Compose)

	contentW := lipgloss.Width(" " + title)
	for _, r := range []string{subject, confirmKeymapHint} {
		if w := lipgloss.Width(r); w > contentW {
			contentW = w
		}
	}
	width = contentW + 3 // 1-col leading pad + 2 borders
	if width < confirmModalDefaultWidth {
		width = confirmModalDefaultWidth
	}
	if max := m.width - 4; width > max {
		width = max
	}
	if width < 14 {
		return nil, 0, 0, 0, "", false
	}
	innerRoom := width - 3 // borders + 1-col leading pad
	if innerRoom > 1 && lipgloss.Width(subject) > innerRoom {
		subject = ansi.Truncate(subject, innerRoom-1, "") + "…"
	}
	rows = []string{subject, "", confirmKeymapHint}
	height := len(rows) + 4
	if max := m.height - 2; height > max {
		// Drop the spacer row first so the prompt still fits in tight
		// vertical real estate; subject + footer are non-negotiable.
		rows = []string{subject, confirmKeymapHint}
		height = len(rows) + 4
		if height > max {
			return nil, 0, 0, 0, "", false
		}
	}
	left = (m.width - width) / 2
	top = (m.height - height) / 2
	if left < 0 {
		left = 0
	}
	if top < 0 {
		top = 0
	}
	return rows, top, left, width, title, true
}

// renderConfirmModal renders the bordered confirm modal. Same chrome
// convention as renderModalBox / renderHelpModal so the visual idiom
// stays consistent across every overlay surface.
func (m Model) renderConfirmModal(rows []string, width int, title string) string {
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

// overlayConfirm splices the confirm modal over the body at the rectangle
// returned by confirmModalLayout. Always rendered after every other
// overlay (zoom modal, help, compose textarea) so the y/n prompt stays
// on top — the user must resolve the prompt before any underlying state
// is reachable again.
func (m Model) overlayConfirm(body string) string {
	if m.state == nil || m.state.PendingConfirm == nil {
		return body
	}
	rows, top, left, width, title, ok := m.confirmModalLayout()
	if !ok {
		return body
	}
	popup := m.renderConfirmModal(rows, width, title)
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
