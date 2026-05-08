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

// oldLineNumbers mirrors newLineNumbers for the OLD file: each rendered
// patch line maps to its old-file line number, or 0 when the line has
// no old-file counterpart (header, hunk marker, added line). Required by
// the side-aware anchor pipeline — LEFT-side comments carry their old
// line number in c.Line, so their buffer index has to come from this
// mapping. Without it, a comment posted on a `-` row never matches a
// buffer line and the ◆ marker / Comments column silently drops.
func oldLineNumbers(lines []string) []int {
	if len(lines) == 0 {
		return nil
	}
	out := make([]int, len(lines))
	cur := 0
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "@@"):
			cur = parseHunkOldStart(l)
		case strings.HasPrefix(l, "---"), strings.HasPrefix(l, "+++"):
			// headers — out[i] stays 0
		case strings.HasPrefix(l, "+"):
			// added line — no old-file counterpart
		case strings.HasPrefix(l, "-"):
			out[i] = cur
			cur++
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

func parseHunkOldStart(hunk string) int {
	// "@@ -A,B +C,D @@" → A
	parts := strings.Fields(hunk)
	for _, p := range parts {
		if !strings.HasPrefix(p, "-") {
			continue
		}
		body := strings.TrimPrefix(p, "-")
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

// commentBufferIndex returns the patch-buffer index where comment c
// anchors. LEFT-side comments are matched against oldNums (c.Line is an
// OLD-file line number); every other side (RIGHT or empty for legacy
// comments) is matched against newNums. Returns -1 when the anchor
// cannot be located in the visible hunks (line out of range, file not
// loaded, etc.). Caller passes both mappings so commentLineSet /
// threadsForCursor can amortize the slice walks across the whole pass.
func commentBufferIndex(c *model.ReviewComment, oldNums, newNums []int) int {
	line := commentNewLine(c)
	if line <= 0 {
		return -1
	}
	target := newNums
	if c.Side == "LEFT" {
		target = oldNums
	}
	for i, n := range target {
		if n == line {
			return i
		}
	}
	return -1
}
