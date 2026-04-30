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

func cursorPrefix(isCursor bool) string {
	if isCursor {
		return "> "
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
