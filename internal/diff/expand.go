package diff

import (
	"strings"
)

// SyntheticLine is the sentinel string the augmented patch buffer uses to
// represent a `···` row that stands in for a hidden context region. Real
// diff content never starts with the leading byte, so prefix-checking call
// sites (lineExistsOnSide, splitDiffLine, diffLineKind, …) can route on
// the sentinel without colliding with real diff lines.
const SyntheticLine = "\x01SYNTH"

// GapKind classifies a hidden region. BOF sits above the first hunk; Mid
// between two adjacent hunks; EOF below the last hunk.
type GapKind int

const (
	// GapKindBOF is the gap between the file start and the first hunk.
	// Detected from the first hunk's old/new start > 1.
	GapKindBOF GapKind = iota + 1
	// GapKindMid is a gap between two adjacent hunks. Detected from the
	// arithmetic gap between consecutive @@ headers.
	GapKindMid
	// GapKindEOF is the gap between the last hunk and the file end.
	// Requires the file's line count; surfaced only when FileLines is
	// non-nil.
	GapKindEOF
)

// GapID identifies a gap within a patch. For Mid gaps, Index is the
// 0-based position in the inter-hunk sequence so per-gap expand counters
// can be addressed individually.
type GapID struct {
	Kind  GapKind
	Index int
}

// GapInfo carries the metadata for one synthetic row in the augmented
// buffer. HiddenCount is the number of file lines still hidden after the
// caller's ExpandState has been applied; OldStart/OldEnd and
// NewStart/NewEnd describe the still-hidden range (inclusive, 1-based).
type GapInfo struct {
	ID          GapID
	HiddenCount int
	OldStart    int
	OldEnd      int
	NewStart    int
	NewEnd      int
}

// ExpandState records how much of each gap has already been revealed.
// BOFBelow grows toward the file start from the first hunk; EOFAbove
// grows toward the file end from the last hunk. InterAbove[i] expands
// downward from the bottom of hunk i; InterBelow[i] expands upward from
// the top of hunk i+1. Maps may be nil — callers default to 0.
type ExpandState struct {
	BOFBelow   int
	EOFAbove   int
	InterAbove map[int]int
	InterBelow map[int]int
}

// ExpandInputs is the immutable input to Expand. FileLines is the
// full NEW-side file content (1-based; index 0 = line 1). When nil, EOF
// gaps are silently suppressed (Expand cannot know the file length) and
// expanded-context emission for any gap is skipped because the source
// content isn't available.
type ExpandInputs struct {
	Patch     string
	FileLines []string
	Expand    ExpandState
}

// ExpandResult is the augmented patch buffer (synthetic + expanded
// context rows merged into the original patch) plus a map from each
// synthetic row's buffer index to its GapInfo.
type ExpandResult struct {
	Lines []string
	Gaps  map[int]GapInfo
}

// AugSpec is the per-buffer-line classification produced by ParseSpecsAug
// — the synthetic-aware companion to the existing per-line spec walker
// in the tui package. Kind 's' marks a synthetic row; 'h' file header;
// '@' hunk header; '+' / '-' add/del; ' ' context (including expanded
// context emitted by Expand).
type AugSpec struct {
	Kind  byte
	OldLn int
	NewLn int
}

type hunkParse struct {
	HeaderIdx int
	BodyEnd   int
	OldStart  int
	OldCount  int
	NewStart  int
	NewCount  int
}

// Expand rebuilds the patch buffer with synthetic gap rows and (when
// the caller has expanded context) the revealed file lines slotted in
// their canonical position. The output preserves the original patch's
// hunk header / +/- / context structure verbatim so the renderer's
// existing line-walk consumers continue to work.
//
// Synthetic rows are emitted only when FileLines is non-nil — without
// the file body the expand action would have nothing to reveal, so the
// surfaceable rows would be dead-ended cues. Production code prefetches
// FileLines on file selection so synthetic rows always show; tests that
// don't exercise expansion skip FileLines and observe a raw augmented
// buffer (just the patch, no synthetic rows).
func Expand(in ExpandInputs) ExpandResult {
	out := ExpandResult{Gaps: map[int]GapInfo{}}
	if in.Patch == "" {
		return out
	}
	emit := in.FileLines != nil
	rawLines := strings.Split(strings.TrimRight(in.Patch, "\n"), "\n")

	// Locate file headers (---/+++ before the first @@) and the hunks.
	var headerLines []string
	var hunks []hunkParse
	firstHunk := -1
	for i, l := range rawLines {
		if strings.HasPrefix(l, "@@") {
			if firstHunk < 0 {
				firstHunk = i
				headerLines = append(headerLines, rawLines[:i]...)
			}
			oldS, oldC, newS, newC := parseHunkRange(l)
			hunks = append(hunks, hunkParse{
				HeaderIdx: i,
				OldStart:  oldS, OldCount: oldC,
				NewStart: newS, NewCount: newC,
			})
		}
	}
	// Each hunk's body ends at the next hunk header (or end of buffer).
	for i := range hunks {
		end := len(rawLines)
		if i+1 < len(hunks) {
			end = hunks[i+1].HeaderIdx
		}
		hunks[i].BodyEnd = end
	}

	// File-header rows (---/+++) stay at the top, unmodified — they're
	// metadata describing the diff, not file content, so the BOF synthetic
	// goes BELOW them.
	out.Lines = append(out.Lines, headerLines...)

	// BOF gap: any hidden region between file line 1 and the first hunk's
	// OldStart. The new and old starts always agree at the first hunk
	// (delta = 0 before any change), so we read the count from OldStart.
	if emit && len(hunks) > 0 && hunks[0].OldStart > 1 {
		hidden := hunks[0].OldStart - 1
		emitBOFGap(&out, hidden, hunks[0].NewStart, in.Expand.BOFBelow, in.FileLines)
	}

	// Hunk bodies interleaved with inter-hunk gaps.
	for i := range hunks {
		out.Lines = append(out.Lines, rawLines[hunks[i].HeaderIdx:hunks[i].BodyEnd]...)
		if !emit || i+1 >= len(hunks) {
			continue
		}
		gapOldStart := hunks[i].OldStart + hunks[i].OldCount
		gapNewStart := hunks[i].NewStart + hunks[i].NewCount
		hidden := hunks[i+1].OldStart - gapOldStart
		if hidden <= 0 {
			continue
		}
		emitMidGap(&out, i, hidden, gapOldStart, gapNewStart,
			in.Expand.InterAbove[i], in.Expand.InterBelow[i], in.FileLines)
	}

	// EOF gap: any hidden region between the last hunk's end and the file
	// end. Requires file lines (we can't know the file length otherwise).
	if emit && len(hunks) > 0 && len(in.FileLines) > 0 {
		last := hunks[len(hunks)-1]
		nextOld := last.OldStart + last.OldCount
		nextNew := last.NewStart + last.NewCount
		hidden := len(in.FileLines) - (nextNew - 1)
		if hidden > 0 {
			emitEOFGap(&out, hidden, nextOld, nextNew, in.Expand.EOFAbove, in.FileLines)
		}
	}

	return out
}

// emitBOFGap emits the (optional) synthetic + expanded-context rows that
// stand in for the file-start gap. below=N reveals the N file lines
// immediately above the first hunk so the user always sees the most
// proximate context first.
func emitBOFGap(out *ExpandResult, hidden, firstHunkNewStart, below int, fileLines []string) {
	if below < 0 {
		below = 0
	}
	if below > hidden {
		below = hidden
	}
	stillHidden := hidden - below
	if stillHidden > 0 {
		idx := len(out.Lines)
		out.Lines = append(out.Lines, SyntheticLine)
		out.Gaps[idx] = GapInfo{
			ID:          GapID{Kind: GapKindBOF},
			HiddenCount: stillHidden,
			OldStart:    1, NewStart: 1,
			OldEnd: stillHidden, NewEnd: stillHidden,
		}
	}
	// BOF delta = 0 (no changes before first hunk), so old==new line nums.
	for n := firstHunkNewStart - below; n < firstHunkNewStart; n++ {
		if n >= 1 && n-1 < len(fileLines) {
			out.Lines = append(out.Lines, " "+fileLines[n-1])
		}
	}
}

// emitMidGap emits a single inter-hunk gap segment: `above` revealed
// lines after the upper hunk's end, a synthetic for any still-hidden
// middle, then `below` revealed lines before the lower hunk's start.
// When above+below covers the whole gap the synthetic is omitted and a
// contiguous context block is emitted instead.
func emitMidGap(out *ExpandResult, gapIdx, hidden, gapOldStart, gapNewStart, above, below int, fileLines []string) {
	if above < 0 {
		above = 0
	}
	if below < 0 {
		below = 0
	}
	if above+below >= hidden {
		for k := 0; k < hidden; k++ {
			n := gapNewStart + k
			if n-1 < len(fileLines) {
				out.Lines = append(out.Lines, " "+fileLines[n-1])
			}
		}
		return
	}
	for k := 0; k < above; k++ {
		n := gapNewStart + k
		if n-1 < len(fileLines) {
			out.Lines = append(out.Lines, " "+fileLines[n-1])
		}
	}
	midHidden := hidden - above - below
	midOldStart := gapOldStart + above
	midNewStart := gapNewStart + above
	idx := len(out.Lines)
	out.Lines = append(out.Lines, SyntheticLine)
	out.Gaps[idx] = GapInfo{
		ID:          GapID{Kind: GapKindMid, Index: gapIdx},
		HiddenCount: midHidden,
		OldStart:    midOldStart, OldEnd: midOldStart + midHidden - 1,
		NewStart: midNewStart, NewEnd: midNewStart + midHidden - 1,
	}
	belowStart := gapNewStart + hidden - below
	for k := 0; k < below; k++ {
		n := belowStart + k
		if n-1 < len(fileLines) {
			out.Lines = append(out.Lines, " "+fileLines[n-1])
		}
	}
}

// emitEOFGap mirrors emitBOFGap at the trailing end. above=N reveals the
// N file lines immediately below the last hunk.
func emitEOFGap(out *ExpandResult, hidden, gapOldStart, gapNewStart, above int, fileLines []string) {
	if above < 0 {
		above = 0
	}
	if above > hidden {
		above = hidden
	}
	for k := 0; k < above; k++ {
		n := gapNewStart + k
		if n-1 < len(fileLines) {
			out.Lines = append(out.Lines, " "+fileLines[n-1])
		}
	}
	stillHidden := hidden - above
	if stillHidden <= 0 {
		return
	}
	idx := len(out.Lines)
	out.Lines = append(out.Lines, SyntheticLine)
	out.Gaps[idx] = GapInfo{
		ID:          GapID{Kind: GapKindEOF},
		HiddenCount: stillHidden,
		OldStart:    gapOldStart + above,
		OldEnd:      gapOldStart + above + stillHidden - 1,
		NewStart:    gapNewStart + above,
		NewEnd:      gapNewStart + above + stillHidden - 1,
	}
}

// parseHunkRange extracts (old start, old count, new start, new count)
// from a "@@ -A,B +C,D @@" header. Missing count tokens default to 1
// (per unified-diff convention "@@ -A +C @@" implies 1 line each).
func parseHunkRange(hunk string) (oldS, oldC, newS, newC int) {
	parts := strings.Fields(hunk)
	for _, p := range parts {
		switch {
		case strings.HasPrefix(p, "-"):
			oldS, oldC = parseRangeTok(p[1:])
		case strings.HasPrefix(p, "+"):
			newS, newC = parseRangeTok(p[1:])
		}
	}
	return
}

// parseRangeTok splits "A,B" → (A, B). Missing B defaults to 1.
func parseRangeTok(s string) (int, int) {
	count := 1
	body := s
	if i := strings.Index(s, ","); i > 0 {
		body = s[:i]
		count = parseStartTok(s[i+1:])
	}
	return parseStartTok(body), count
}

// ParseSpecsAug walks an augmented patch buffer (as produced by Expand)
// and classifies each row into a kind + 1-based OLD/NEW line number
// pair. Synthetic rows produce Kind 's' with zeroed line numbers AND
// teach the walker to resume counting at the gap's OldEnd+1 / NewEnd+1
// so subsequent expanded-context rows report the correct file line
// numbers without needing an in-band line-counter marker.
func ParseSpecsAug(lines []string, gaps map[int]GapInfo) []AugSpec {
	out := make([]AugSpec, len(lines))
	var oldLn, newLn int
	for i, l := range lines {
		if l == SyntheticLine {
			out[i] = AugSpec{Kind: 's'}
			if g, ok := gaps[i]; ok {
				oldLn = g.OldEnd + 1
				newLn = g.NewEnd + 1
			}
			continue
		}
		switch {
		case strings.HasPrefix(l, "---"), strings.HasPrefix(l, "+++"):
			out[i] = AugSpec{Kind: 'h'}
		case strings.HasPrefix(l, "@@"):
			oldS, _, newS, _ := parseHunkRange(l)
			oldLn = oldS
			newLn = newS
			out[i] = AugSpec{Kind: '@'}
		case strings.HasPrefix(l, "-"):
			out[i] = AugSpec{Kind: '-', OldLn: oldLn}
			oldLn++
		case strings.HasPrefix(l, "+"):
			out[i] = AugSpec{Kind: '+', NewLn: newLn}
			newLn++
		default:
			out[i] = AugSpec{Kind: ' ', OldLn: oldLn, NewLn: newLn}
			oldLn++
			newLn++
		}
	}
	return out
}
