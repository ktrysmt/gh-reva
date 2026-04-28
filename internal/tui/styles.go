package tui

import (
	"strings"

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
