package diff

import (
	"strconv"
	"strings"
)

// Anchor describes the GitHub side / line tuple for a single buffer line
// in a unified diff. Side is "RIGHT" for additions and context, "LEFT"
// for deletions, and empty for header / hunk rows. NewLine / OldLine are
// 0 when the row has no number on that side.
type Anchor struct {
	Kind    byte // 'h' file header, '@' hunk, '+' addition, '-' deletion, ' ' context
	NewLine int
	OldLine int
	Side    string
}

// Range is the resolved (start_line, start_side) → (line, side) pair for
// a multi-line review comment. When anchor and cursor collapse to the
// same buffer line, StartLine is 0 (and StartSide empty) so the caller
// can omit the start_* fields per GitHub's API contract.
type Range struct {
	StartLine int
	StartSide string
	Line      int
	Side      string
}

// ResolveAnchor walks the patch and returns the anchor for the given
// 0-based buffer line index. Returns ok=false when the index is out of
// range or points at a header / hunk row (those rows cannot anchor a
// review comment per the GitHub API).
func ResolveAnchor(patch string, bufferLine int) (Anchor, bool) {
	if bufferLine < 0 {
		return Anchor{}, false
	}
	specs := walkSpecs(patch)
	if bufferLine >= len(specs) {
		return Anchor{}, false
	}
	a := specs[bufferLine]
	if a.Kind == 'h' || a.Kind == '@' {
		return Anchor{}, false
	}
	return a, true
}

// ResolveRange returns the canonical Range for a multi-line review
// comment spanning the two buffer-line endpoints. Endpoints can be in
// either order; the returned Range always has the buffer-earlier
// endpoint as start. When both endpoints land on the same buffer line,
// StartLine is returned as 0 to signal a single-line comment (caller
// drops the start_* fields).
//
// Mixed-side ranges (e.g. anchor on a '-' row, cursor on a '+' row) are
// accepted: GitHub's API allows start_side != side. The user's intent
// is preserved by ordering endpoints by buffer position rather than by
// numeric line value — comparing a LEFT oldLine to a RIGHT newLine has
// no shared coordinate space, but buffer index is the canonical "which
// row in the diff comes first" answer the user actually clicked on.
// Without this normalization, anchoring on a later '+' row and
// dragging cursor up to an earlier '-' row produced a payload with
// numerically-larger start_line, which GitHub rejects with 422
// "start_line must be less than end_line".
func ResolveRange(patch string, anchor, cursor int) (Range, bool) {
	a, okA := ResolveAnchor(patch, anchor)
	b, okB := ResolveAnchor(patch, cursor)
	if !okA || !okB {
		return Range{}, false
	}
	if anchor == cursor {
		return Range{Line: anchorLine(a), Side: a.Side}, true
	}
	startSpec, endSpec := a, b
	if anchor > cursor {
		startSpec, endSpec = b, a
	}
	return Range{
		StartLine: anchorLine(startSpec),
		StartSide: startSpec.Side,
		Line:      anchorLine(endSpec),
		Side:      endSpec.Side,
	}, true
}

// anchorLine picks the file-line number that matches the row's Side. A
// '-' row uses OldLine (LEFT side), '+' / context use NewLine (RIGHT).
func anchorLine(a Anchor) int {
	if a.Side == "LEFT" {
		return a.OldLine
	}
	return a.NewLine
}

// walkSpecs runs the same single-pass classifier that the TUI renderer
// uses for split-mode line numbers, but emits Anchor instead of the
// renderer-specific diffLineSpec. The two are intentionally separate:
// the renderer trims trailing newlines and runs on every frame, while
// the anchor resolver runs once per Compose entry and prefers a
// self-contained input.
func walkSpecs(patch string) []Anchor {
	if patch == "" {
		return nil
	}
	lines := strings.Split(strings.TrimRight(patch, "\n"), "\n")
	out := make([]Anchor, len(lines))
	var oldLn, newLn int
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "---"), strings.HasPrefix(l, "+++"):
			out[i] = Anchor{Kind: 'h'}
		case strings.HasPrefix(l, "@@"):
			oldLn, newLn = parseHunkStarts(l)
			out[i] = Anchor{Kind: '@'}
		case strings.HasPrefix(l, "-"):
			out[i] = Anchor{Kind: '-', OldLine: oldLn, Side: "LEFT"}
			oldLn++
		case strings.HasPrefix(l, "+"):
			out[i] = Anchor{Kind: '+', NewLine: newLn, Side: "RIGHT"}
			newLn++
		default:
			out[i] = Anchor{Kind: ' ', OldLine: oldLn, NewLine: newLn, Side: "RIGHT"}
			oldLn++
			newLn++
		}
	}
	return out
}

func parseHunkStarts(hunk string) (int, int) {
	parts := strings.Fields(hunk)
	var oldS, newS int
	for _, p := range parts {
		switch {
		case strings.HasPrefix(p, "-"):
			oldS = parseStartTok(p[1:])
		case strings.HasPrefix(p, "+"):
			newS = parseStartTok(p[1:])
		}
	}
	return oldS, newS
}

func parseStartTok(s string) int {
	if i := strings.Index(s, ","); i > 0 {
		s = s[:i]
	}
	n, _ := strconv.Atoi(s)
	return n
}
