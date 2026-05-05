package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// helpEntry is one row in the keymap reference. Keys are pre-formatted so
// the table column-aligns with simple padding.
type helpEntry struct {
	keys, desc string
}

type helpSection struct {
	title   string
	entries []helpEntry
}

// helpSections is the canonical keymap reference shown by the modal. Keep
// this list and the actual key handlers in lockstep — when a binding moves,
// update the entry here too. Section ordering follows the typical user
// flow: Global first, then panes in screen order, Visual last.
var helpSections = []helpSection{
	{
		title: "Global",
		entries: []helpEntry{
			{"?", "Toggle help"},
			{"q / Ctrl+C", "Quit"},
			{"Tab", "Next pane"},
			{"Shift+Tab", "Previous pane"},
			{"J / K", "Next / previous file"},
			{"v", "Enter visual mode"},
		},
	},
	{
		title: "Files",
		entries: []helpEntry{
			{"j / k", "Move cursor (auto-selects file)"},
			{"t", "Toggle tree mode"},
			{"Space", "Toggle hover popup"},
			{"Enter", "Fold / unfold dir (tree only)"},
		},
	},
	{
		title: "Commits",
		entries: []helpEntry{
			{"j / k", "Move cursor (auto-selects commit)"},
			{"Space", "Toggle hover popup"},
		},
	},
	{
		title: "Diff",
		entries: []helpEntry{
			{"j / k", "Line down / up"},
			{"g / G", "Top / bottom"},
			{"Ctrl+D / Ctrl+U", "Half page down / up"},
			{"Ctrl+F / Ctrl+B", "Full page down / up"},
			{"H / M / L", "Viewport top / middle / bottom"},
			{"Space", "Toggle split / unified"},
		},
	},
	{
		title: "Comments",
		entries: []helpEntry{
			{"j / k", "Move cursor (auto-scrolls Diff)"},
		},
	},
	{
		title: "Visual",
		entries: []helpEntry{
			{"y", "Yank and exit"},
			{"Esc / Ctrl+C", "Cancel without yanking"},
		},
	},
}

// helpModalDefaultWidth is the modal's outer width on a comfortably-wide
// terminal. Sized so the longest key column ("Ctrl+D / Ctrl+U") plus the
// longest description fits with a 2-col gap on each side.
const helpModalDefaultWidth = 60

// helpModalLines builds the body rows for the modal. The first column
// (keys) is padded to the widest keys cell across all sections so every
// section aligns on the same description column.
func helpModalLines() []string {
	keyW := 0
	for _, sec := range helpSections {
		for _, e := range sec.entries {
			if w := lipgloss.Width(e.keys); w > keyW {
				keyW = w
			}
		}
	}
	var rows []string
	for i, sec := range helpSections {
		if i > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, sec.title)
		for _, e := range sec.entries {
			pad := keyW - lipgloss.Width(e.keys)
			if pad < 0 {
				pad = 0
			}
			rows = append(rows, "  "+e.keys+strings.Repeat(" ", pad)+"  "+e.desc)
		}
	}
	return rows
}

// helpModalLayout decides where to draw the modal. Width is clamped to
// `m.width - 4` on narrow terminals; height is the natural content rows
// plus chrome (top border + title + divider + bottom border = 4) and is
// clamped to `m.height - 2`. The result is centered both horizontally and
// vertically.
func (m Model) helpModalLayout() (rows []string, top, left, width int, ok bool) {
	if m.width < 10 || m.height < 6 {
		return nil, 0, 0, 0, false
	}
	body := helpModalLines()
	contentW := 0
	for _, r := range body {
		if w := lipgloss.Width(r); w > contentW {
			contentW = w
		}
	}
	width = helpModalDefaultWidth
	if width < contentW+4 {
		width = contentW + 4
	}
	if width > m.width-4 {
		width = m.width - 4
	}
	if width < 10 {
		return nil, 0, 0, 0, false
	}

	// Chrome rows: top border + title + divider + bottom border = 4.
	maxBody := m.height - 4 - 2
	if maxBody < 1 {
		return nil, 0, 0, 0, false
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
	return body, top, left, width, true
}

// renderHelpModal renders the modal as a self-contained bordered block.
// The title row carries `Help`, with a horizontal divider beneath it —
// same chrome as the pane boxes so the visual idiom is consistent.
func (m Model) renderHelpModal(rows []string, width int) string {
	innerW := atLeast(width-2, 1)
	bar := strings.Repeat("─", innerW)
	border := m.theme.PaneBorderActive
	side := fg("│", border)
	hr := fg(bar, border)
	var sb strings.Builder
	sb.WriteString(fg("┌"+bar+"┐", border) + "\n")
	sb.WriteString(side + padTrunc(fgBold(" Help", m.theme.PaneTitleActive), innerW) + side + "\n")
	sb.WriteString(fg("├", border) + hr + fg("┤", border) + "\n")
	for _, ln := range rows {
		sb.WriteString(side + padTrunc(" "+ln, innerW) + side + "\n")
	}
	sb.WriteString(fg("└"+bar+"┘", border))
	return sb.String()
}

// overlayHelp splices the Help modal over the body at the rectangle
// returned by helpModalLayout. Same splicing semantics as overlayHover —
// row prefix and suffix on the underlying body are preserved so pane
// chrome columns past the modal remain intact.
func (m Model) overlayHelp(body string) string {
	if !m.state.HelpOpen {
		return body
	}
	rows, top, left, width, ok := m.helpModalLayout()
	if !ok {
		return body
	}
	popup := m.renderHelpModal(rows, width)
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
