package tui

import "github.com/charmbracelet/lipgloss"

// fg wraps s in an SGR foreground color. Returns s unchanged when c is the
// zero value (empty string), so callers can pass an unset theme field
// without conditional logic.
func fg(s string, c lipgloss.Color) string {
	if c == "" {
		return s
	}
	return lipgloss.NewStyle().Foreground(c).Render(s)
}

// fgBold combines fg with bold. Used for active pane titles and the
// cursor-row glyph.
func fgBold(s string, c lipgloss.Color) string {
	if c == "" {
		return lipgloss.NewStyle().Bold(true).Render(s)
	}
	return lipgloss.NewStyle().Foreground(c).Bold(true).Render(s)
}

// bgRow paints the line's background. Used for the visual-mode range row
// highlight. Returns s unchanged when c is empty.
func bgRow(s string, c lipgloss.Color) string {
	if c == "" {
		return s
	}
	return lipgloss.NewStyle().Background(c).Render(s)
}
