package tui

import (
	"strings"
	"unicode/utf8"

	"github.com/ktrysmt/gh-rv/internal/model"
)

func paneTitle(label string, active bool, suffix string) string {
	prefix := "  "
	if active {
		prefix = "▶ "
	}
	if suffix != "" {
		return prefix + label + " " + suffix
	}
	return prefix + label
}

// styledPaneTitle is paneTitle plus theme coloring: active titles get the
// active-title color in bold; inactive titles get the inactive-title color.
// Width-aware truncation still goes through fitPaneTitle on the raw text so
// SGR codes never participate in length math.
func (m Model) styledPaneTitle(label string, active bool, suffix string) string {
	var raw string
	if w := paneInnerWidth(m, label); w > 0 {
		raw = fitPaneTitle(label, suffix, active, w)
	} else {
		raw = paneTitle(label, active, suffix)
	}
	prefix := "  "
	if active {
		prefix = "▶ "
	}
	// Re-apply color to the prefix and the rest separately. The prefix is
	// always exactly 2 cells, so a rune slice is safe.
	runes := []rune(raw)
	body := string(runes[2:])
	if active {
		return fgBold(prefix, m.theme.PaneTitleActive) + fgBold(body, m.theme.PaneTitleActive)
	}
	return prefix + fg(body, m.theme.PaneTitle)
}

// paneInnerWidth picks the right per-pane width budget for fitPaneTitle.
// Returns 0 when no budget has been computed yet (pre-first-frame), letting
// callers fall back to the un-fitted form. Matches by prefix so dynamic
// labels like "Diff: path @ sha" still pick the Diff budget.
func paneInnerWidth(m Model, label string) int {
	switch {
	case strings.HasPrefix(label, "Files"):
		return m.paneWidthFiles
	case strings.HasPrefix(label, "Commits"):
		return m.paneWidthCommits
	case strings.HasPrefix(label, "Diff"):
		return m.paneWidthDiff
	case strings.HasPrefix(label, "Comments"):
		return m.paneWidthComments
	}
	return 0
}

func cursorPrefix(isCursor bool) string {
	if isCursor {
		return "> "
	}
	return "  "
}

// styledCursor returns the row-prefix glyph (`> ` or `  `) with the cursor
// row color and bold weight applied when the row is the cursor or part of
// the visual range. Mirrors cursorMarker's logic but produces a colored
// string instead of plain text.
func (m Model) styledCursor(pane model.PaneID, idx, cursor int) string {
	if idx == cursor || m.inVisualRange(pane, idx) {
		return fgBold("> ", m.theme.CursorRow)
	}
	return "  "
}

func changeKindShort(k model.ChangeKind) string {
	switch k {
	case model.ChangeAdded:
		return "A"
	case model.ChangeModified:
		return "M"
	case model.ChangeDeleted:
		return "D"
	case model.ChangeRenamed:
		return "R"
	}
	return "?"
}

// styledStatus returns the single-letter change-kind glyph wrapped in the
// matching theme color. Used by Files and Commits panes for the per-row
// `[A]/[M]/[D]/[R]` annotations.
func (m Model) styledStatus(k model.ChangeKind) string {
	letter := changeKindShort(k)
	switch k {
	case model.ChangeAdded:
		return fg(letter, m.theme.StatusAdded)
	case model.ChangeModified:
		return fg(letter, m.theme.StatusModified)
	case model.ChangeDeleted:
		return fg(letter, m.theme.StatusDeleted)
	case model.ChangeRenamed:
		return fg(letter, m.theme.StatusRenamed)
	}
	return letter
}

func shortSHA(s string) string {
	if len(s) < 7 {
		return s
	}
	return s[:7]
}

func indent(level int) string {
	if level <= 0 {
		return ""
	}
	return strings.Repeat("  ", level)
}

// fitPaneTitle composes a pane title that fits in `width` columns while
// preserving the suffix (e.g. mode tag) at the right. When the full title
// would overflow, the label is shrunk with an ellipsis; the suffix stays
// visible so users can still see the view-mode marker on narrow terminals.
func fitPaneTitle(label, suffix string, active bool, width int) string {
	prefix := "  "
	if active {
		prefix = "▶ "
	}
	full := prefix + label
	if suffix != "" {
		full += " " + suffix
	}
	if width <= 0 || utf8.RuneCountInString(full) <= width {
		return full
	}
	if suffix != "" {
		// Try to keep the full suffix, shrink the label.
		reserve := utf8.RuneCountInString(prefix) + 1 + utf8.RuneCountInString(suffix)
		avail := width - reserve
		if avail >= 2 {
			runes := []rune(label)
			if len(runes) > avail {
				label = string(runes[:avail-1]) + "…"
			}
			return prefix + label + " " + suffix
		}
	}
	// Fall back to a hard truncate of the whole title.
	runes := []rune(full)
	if width >= 2 {
		return string(runes[:width-1]) + "…"
	}
	if width == 1 {
		return string(runes[:1])
	}
	return ""
}

// wrapText word-wraps text into lines of at most `width` columns. Words longer
// than width are hard-broken on rune boundaries. width <= 0 disables wrapping.
func wrapText(text string, width int) []string {
	if width <= 0 || utf8.RuneCountInString(text) <= width {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}
	var out []string
	var line strings.Builder
	lineLen := 0
	flush := func() {
		if line.Len() > 0 {
			out = append(out, line.String())
			line.Reset()
			lineLen = 0
		}
	}
	for _, w := range words {
		runes := []rune(w)
		for len(runes) > width {
			flush()
			out = append(out, string(runes[:width]))
			runes = runes[width:]
		}
		w = string(runes)
		wLen := len(runes)
		sep := 0
		if lineLen > 0 {
			sep = 1
		}
		if lineLen+sep+wLen <= width {
			if sep == 1 {
				line.WriteByte(' ')
			}
			line.WriteString(w)
			lineLen += sep + wLen
		} else {
			flush()
			line.WriteString(w)
			lineLen = wLen
		}
	}
	flush()
	if len(out) == 0 {
		return []string{""}
	}
	return out
}
