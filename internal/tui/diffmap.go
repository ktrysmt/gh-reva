package tui

import (
	"strconv"
	"strings"

	"github.com/ktrysmt/gh-reva/internal/diff"
	"github.com/ktrysmt/gh-reva/internal/model"
)

// newLineNumbers walks a (raw, non-augmented) unified diff and returns,
// for each rendered line, the corresponding new-file line number — or 0
// if that line has no new-file counterpart (header, hunk marker, removed
// line). Synthetic rows are NOT handled here; augmented buffers go
// through Model.patchNewLineNumbers, which derives the mapping from
// pre-built (synthetic-aware) specs.
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

// commentRangeStartBufferIndex returns the patch-buffer index where the
// upper edge of a multi-line range comment lives. Falls back to
// OriginalStartLine when StartLine is 0 (mirrors commentNewLine /
// commentBufferIndex semantics for outdated anchors). Returns -1 when
// the row has no range or the start anchor cannot be resolved.
func commentRangeStartBufferIndex(c *model.ReviewComment, oldNums, newNums []int) int {
	startLine := c.StartLine
	if startLine <= 0 {
		startLine = c.OriginalStartLine
	}
	if startLine <= 0 {
		return -1
	}
	target := newNums
	if c.StartSide == "LEFT" {
		target = oldNums
	}
	for i, n := range target {
		if n == startLine {
			return i
		}
	}
	return -1
}

// Marker glyphs drawn in the Diff gutter. Single-line / range-end uses
// markerAnchor (or markerResolved when the thread is resolved); range
// start uses markerStart; intermediate buffer rows use markerMiddle.
// `└` is intentionally absent — the anchor diamond doubles as the
// bottom edge of the range, and the resolved checkmark mirrors that
// role.
const (
	markerAnchor   = '◆'
	markerResolved = '✓'
	markerStart    = '┌'
	markerMiddle   = '│'
)

// markerRank orders the glyphs by visual precedence so that overlapping
// threads collapse to a single character per buffer row. Higher value =
// wins. A zero / unknown rune ranks lowest so any real glyph beats "no
// marker".
//
// Unresolved ◆ outranks resolved ✓: when both an unresolved and a
// resolved thread share a buffer row, the unresolved one demands more
// attention so the ◆ stays visible. Range markers (┌, │) rank below
// either anchor — a single-line anchor on the same row as another
// thread's range middle should win the gutter slot.
func markerRank(r rune) int {
	switch r {
	case markerAnchor:
		return 4
	case markerResolved:
		return 3
	case markerStart:
		return 2
	case markerMiddle:
		return 1
	default:
		return 0
	}
}

// lineExistsOnSide reports whether buffer line `l` has a counterpart in
// the given Side column. `+` rows belong only to RIGHT, `-` rows only to
// LEFT, and everything else (context, header, hunk, synthetic `···`) is
// treated as existing on both sides so j/k auto-skip never strands the
// user on a row they cannot leave.
func lineExistsOnSide(line string, side model.DiffSide) bool {
	switch {
	case line == diff.SyntheticLine:
		return true
	case strings.HasPrefix(line, "---"), strings.HasPrefix(line, "+++"):
		return true
	case strings.HasPrefix(line, "+"):
		return side == model.DiffSideRight
	case strings.HasPrefix(line, "-"):
		return side == model.DiffSideLeft
	default:
		return true
	}
}

// nextSideLine finds the next buffer index reachable by j/k under the
// auto-skip contract: starting one step in `dir` from `from`, advance
// until a line that exists on `side` is found. Returns -1 when no such
// line exists in that direction (cursor stays put).
func nextSideLine(lines []string, from int, side model.DiffSide, dir int) int {
	if dir == 0 {
		return -1
	}
	for i := from + dir; i >= 0 && i < len(lines); i += dir {
		if lineExistsOnSide(lines[i], side) {
			return i
		}
	}
	return -1
}

// firstSideLine returns the first buffer index belonging to `side`.
// Used by gg. Returns 0 as a safe fallback when no row exists on the
// side (an empty patch can't reach this branch in practice — handler
// short-circuits on totalLines == 0).
func firstSideLine(lines []string, side model.DiffSide) int {
	for i, l := range lines {
		if lineExistsOnSide(l, side) {
			return i
		}
	}
	return 0
}

// lastSideLine mirrors firstSideLine for G. Returns the last buffer
// index whose row exists on `side`, or len(lines)-1 as a fallback.
func lastSideLine(lines []string, side model.DiffSide) int {
	for i := len(lines) - 1; i >= 0; i-- {
		if lineExistsOnSide(lines[i], side) {
			return i
		}
	}
	return len(lines) - 1
}

// nearestSideLine finds the closest buffer index to `from` (inclusive)
// that exists on `side`. Search radiates outward, preferring the
// upward direction first so an h-from-`+` lands on the just-removed
// row above rather than skipping past unrelated context below.
// Returns `from` unchanged when it already exists on `side`; -1 when
// no row exists on `side` at all.
func nearestSideLine(lines []string, from int, side model.DiffSide) int {
	if from < 0 || from >= len(lines) {
		return -1
	}
	if lineExistsOnSide(lines[from], side) {
		return from
	}
	for d := 1; d < len(lines); d++ {
		up := from - d
		if up >= 0 && lineExistsOnSide(lines[up], side) {
			return up
		}
		down := from + d
		if down < len(lines) && lineExistsOnSide(lines[down], side) {
			return down
		}
		if up < 0 && down >= len(lines) {
			break
		}
	}
	return -1
}

// sideMarkers carries the per-column gutter-glyph maps. Left holds
// glyphs that belong in the LEFT (before) column's gutter; Right holds
// the RIGHT (after) column's. Buffer indices in Left always correspond
// to rows that exist on the LEFT side (`-` and context); Right indices
// to RIGHT-side rows (`+` and context).
type sideMarkers struct {
	Left  map[int]rune
	Right map[int]rune
}

// commentLineMarkers returns the per-column gutter glyphs for the
// current Diff view. The ◆ / ┌ / │ run for each thread is painted
// entirely on the column the thread's anchor lives on:
//
//   - Single-line root → markerAnchor on the anchor Side at the root's
//     buffer index.
//   - Multi-line root → markerStart at the start-edge, markerAnchor at
//     the end-edge, markerMiddle on every buffer index in between, all
//     on the END's Side. Mixed-side ranges follow the END's column so
//     the run stays contiguous in one lane (painting half of a range
//     in LEFT and half in RIGHT would visually conflate two different
//     review columns).
//
// Replies are ignored — replies inherit the thread's anchor on GitHub
// and never widen the visible span.
//
// Overlap precedence (markerAnchor > markerStart > markerMiddle, see
// markerRank) is computed per side, so a LEFT ◆ and a RIGHT ◆ at the
// same buffer index coexist without colliding.
func (m Model) commentLineMarkers() sideMarkers {
	// Force threadsForView to refresh `gen` first, then read the cache
	// hit on patchInfo. Without the threads call here, the gen counter
	// would lag behind a stale-cache invalidation that nothing else
	// has triggered yet. threadsForView is itself memoized so the call
	// is essentially free on a hot render path.
	threads := m.threadsForView()
	pi := m.patchInfo()
	gen := uint64(0)
	if m.threadsCache != nil {
		gen = m.threadsCache.gen
	}
	if pi != nil && pi.markers != nil && pi.markersGen == gen {
		return *pi.markers
	}
	out := sideMarkers{Left: map[int]rune{}, Right: map[int]rune{}}
	newMap := m.patchNewLineNumbers()
	oldMap := m.patchOldLineNumbers()
	if len(newMap) == 0 && len(oldMap) == 0 {
		if pi != nil {
			pi.markers = &out
			pi.markersGen = gen
		}
		return out
	}
	if len(threads) == 0 {
		if pi != nil {
			pi.markers = &out
			pi.markersGen = gen
		}
		return out
	}
	put := func(side string, idx int, r rune) {
		if idx < 0 {
			return
		}
		target := out.Right
		if side == "LEFT" {
			target = out.Left
		}
		if cur, ok := target[idx]; ok && markerRank(cur) >= markerRank(r) {
			return
		}
		target[idx] = r
	}
	for _, t := range threads {
		root := t.Root
		end := commentBufferIndex(root, oldMap, newMap)
		if end < 0 {
			continue
		}
		side := root.Side
		if side == "" {
			side = "RIGHT"
		}
		// Anchor glyph swaps to the resolved checkmark when the thread
		// has been marked resolved on GitHub. Start (┌) and middle (│)
		// glyphs are unchanged — the range shape still reads as a
		// range; only the bottom edge signals "concern addressed".
		anchor := markerAnchor
		if root.Resolved {
			anchor = markerResolved
		}
		start := commentRangeStartBufferIndex(root, oldMap, newMap)
		if start < 0 || start >= end {
			// Single-line, missing start, or unresolvable / inverted
			// range — fall back to the legacy single-anchor glyph.
			put(side, end, anchor)
			continue
		}
		put(side, start, markerStart)
		for i := start + 1; i < end; i++ {
			put(side, i, markerMiddle)
		}
		put(side, end, anchor)
	}
	if pi != nil {
		pi.markers = &out
		pi.markersGen = gen
	}
	return out
}
