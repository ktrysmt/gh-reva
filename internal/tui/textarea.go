package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// handleKeyTextarea is the keystroke router used while a Compose
// session is active. It absorbs all input until the user saves with
// Ctrl+S or cancels with Esc / Ctrl+C, so navigation behind the modal
// stays frozen — same contract as the help / zoom modals.
//
// Save:      Ctrl+S → emit composeBodyMsg{body: ComposeState.Body}.
//            Update transitions to Submitting and POSTs via GraphQL.
// Cancel:    Esc, Ctrl+C → ComposeState = nil.
//            (Submitting is uninterruptible by design — the request
//            is in flight and the UI just waits for the response.)
// Retry:     Ctrl+S in Failed state re-issues the POST without
//            re-typing the body.
// Backspace: drop one rune from the tail (Editing state only).
// Enter:     append "\n" (Editing state only).
// Tab:       append "\t" (Editing state only).
// Otherwise: append msg.Runes (Editing state only).
func (m Model) handleKeyTextarea(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cs := m.state.Compose
	if cs == nil {
		return m, nil
	}
	if cs.Status == model.ComposeSubmitting {
		// In-flight request: ignore everything except hard-cancel via
		// Ctrl+C / Esc. Hard-cancel just clears Compose so the response
		// (when it arrives) finds Compose==nil and is dropped on the
		// floor by applyComposeSubmitted.
		switch msg.String() {
		case "esc", "ctrl+c":
			m.state.Compose = nil
		}
		return m, nil
	}
	switch msg.String() {
	case "esc", "ctrl+c":
		m.state.Compose = nil
		return m, nil
	case "ctrl+s":
		if cs.Status == model.ComposeFailed {
			return m, m.retryComposeSubmit()
		}
		body := cs.Body
		return m, func() tea.Msg { return composeBodyMsg{body: body} }
	case "enter":
		cs.Body += "\n"
		return m, nil
	case "backspace":
		if cs.Body == "" {
			return m, nil
		}
		runes := []rune(cs.Body)
		cs.Body = string(runes[:len(runes)-1])
		return m, nil
	case "tab":
		// Tab inserts a tab character rather than moving focus —
		// matching Markdown editing convention. Code-block tabs are
		// rare in PR comments but we keep the semantics consistent.
		cs.Body += "\t"
		return m, nil
	}
	if len(msg.Runes) > 0 {
		cs.Body += string(msg.Runes)
	}
	return m, nil
}

// overlayCompose splices the textarea modal over the body when a
// Compose session has UseTextarea=true OR when the editor path is
// past the Editing stage (Submitting/Failed). The $EDITOR Editing
// state owns the terminal via tea.ExecProcess and has no overlay —
// bubbletea is suspended during that interval and resumes only when
// the editor exits.
func (m Model) overlayCompose(body string) string {
	cs := m.state.Compose
	if cs == nil {
		return body
	}
	if cs.Status == model.ComposeEditing && !cs.UseTextarea {
		return body
	}
	if m.width <= 0 || m.height <= 0 {
		return body
	}
	innerW := composeModalWidth(m.width)
	rows := composeModalRows(cs, innerW, m.theme.CommentDate, m.theme.ErrorText)
	popup := composeModalBox(rows, innerW, m.theme.PaneBorderActive)
	popupRows := strings.Split(popup, "\n")

	outerW := innerW + 2 // borders
	left := (m.width - outerW) / 2
	if left < 0 {
		left = 0
	}
	top := (m.height - len(popupRows)) / 2
	if top < 0 {
		top = 0
	}

	bodyRows := strings.Split(body, "\n")
	for i, pr := range popupRows {
		r := top + i
		if r < 0 || r >= len(bodyRows) {
			continue
		}
		bodyRows[r] = spliceMid(bodyRows[r], pr, left, outerW)
	}
	return strings.Join(bodyRows, "\n")
}

// composeModalRows assembles the rendered rows. Each source row
// (title / body / status / hint) wraps independently so a long single
// body line breaks across multiple display rows without leaking into
// the hint row. Colour is applied AFTER wrapping so wrapText measures
// plain runes.
func composeModalRows(cs *model.ComposeState, innerW int, dateColor, errColor lipgloss.Color) []string {
	contentW := innerW - 2 // 1-col padding inside borders on each side
	if contentW < 1 {
		contentW = 1
	}
	rows := []string{fgBold(composeModalTitle(cs), dateColor), ""}

	body := cs.Body
	if cs.Status == model.ComposeEditing {
		body += "█" // fake caret while typing
	}
	if body == "" {
		rows = append(rows, "(empty)")
	} else {
		for _, src := range strings.Split(body, "\n") {
			if src == "" {
				rows = append(rows, "")
				continue
			}
			rows = append(rows, wrapText(src, contentW)...)
		}
	}
	rows = append(rows, "")
	switch cs.Status {
	case model.ComposeSubmitting:
		rows = append(rows, fg("posting to GitHub…", dateColor))
	case model.ComposeFailed:
		errMsg := "failed: " + cs.ErrMsg
		for _, r := range wrapText(errMsg, contentW) {
			rows = append(rows, fg(r, errColor))
		}
		rows = append(rows, fg("ctrl+s:retry  esc:cancel", dateColor))
	default:
		rows = append(rows, fg("ctrl+s:save  esc:cancel", dateColor))
	}
	return rows
}

func composeModalTitle(cs *model.ComposeState) string {
	switch cs.Kind {
	case model.ComposeReply:
		return "Reply"
	case model.ComposeEdit:
		return "Edit comment"
	default:
		return "New comment"
	}
}

// composeModalWidth picks an inner width capped at 60 cols so even on
// a wide terminal the textarea reads comfortably; falls back to half
// the screen on narrower windows.
func composeModalWidth(termW int) int {
	w := termW - 8
	if w > 60 {
		w = 60
	}
	if w < 20 {
		w = 20
	}
	return w
}

// composeModalBox draws the bordered overlay. Rows are pre-wrapped to
// (innerW - 2) by composeModalRows; the box adds 1-col padding inside
// the borders.
func composeModalBox(rows []string, innerW int, border lipgloss.Color) string {
	contentW := innerW - 2
	if contentW < 1 {
		contentW = 1
	}
	bar := strings.Repeat("─", innerW)
	side := fg("│", border)
	var sb strings.Builder
	sb.WriteString(fg("┌"+bar+"┐", border) + "\n")
	for _, r := range rows {
		sb.WriteString(side + " " + padTrunc(r, contentW) + " " + side + "\n")
	}
	sb.WriteString(fg("└"+bar+"┘", border))
	return sb.String()
}
