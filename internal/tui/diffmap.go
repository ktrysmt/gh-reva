package tui

import (
	"strconv"
	"strings"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// newLineNumbers walks a unified diff and returns, for each rendered line,
// the corresponding new-file line number — or 0 if that line has no new-file
// counterpart (header, hunk marker, removed line).
func newLineNumbers(lines []string) []int {
	if len(lines) == 0 {
		return nil
	}
	out := make([]int, len(lines))
	cur := 0
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "@@"):
			cur = parseHunkNewStart(l)
		case strings.HasPrefix(l, "---"), strings.HasPrefix(l, "+++"):
			// headers — out[i] stays 0
		case strings.HasPrefix(l, "+"):
			out[i] = cur
			cur++
		case strings.HasPrefix(l, "-"):
			// removed line — no new-file counterpart
		default:
			// context (leading space) or empty
			if cur > 0 {
				out[i] = cur
				cur++
			}
		}
	}
	return out
}

func parseHunkNewStart(hunk string) int {
	// "@@ -A,B +C,D @@" → C
	parts := strings.Fields(hunk)
	for _, p := range parts {
		if !strings.HasPrefix(p, "+") {
			continue
		}
		body := strings.TrimPrefix(p, "+")
		if i := strings.Index(body, ","); i > 0 {
			body = body[:i]
		}
		n, err := strconv.Atoi(body)
		if err == nil {
			return n
		}
	}
	return 0
}

// bufferIndexForNewLine returns the index in the rendered patch buffer that
// corresponds to the given new-file line number, or -1 when the line is not
// represented in this patch.
func bufferIndexForNewLine(lines []string, newLine int) int {
	if newLine <= 0 {
		return -1
	}
	mapping := newLineNumbers(lines)
	for i, n := range mapping {
		if n == newLine {
			return i
		}
	}
	return -1
}

func commentNewLine(c *model.ReviewComment) int {
	if c.Line > 0 {
		return c.Line
	}
	return c.OriginalLine
}
