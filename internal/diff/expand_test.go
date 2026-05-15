package diff

import (
	"strings"
	"testing"
)

// Synthetic rows live in the augmented patch buffer at the position
// where the gap they cover sits in the file. Real diff content never
// starts with the sentinel byte; Expand callers (parseDiffSpecsAug /
// the TUI renderer) hit the sentinel and look up gap metadata in the
// returned Gaps map.

// twoHunkPatch covers OLD/NEW lines 3 and 12 (one-line hunks each), so
// BOF gap = lines 1..2 (2 lines), inter-hunk gap = lines 4..11
// (8 lines), and (when file lines are provided) EOF gap = lines 13..N.
const twoHunkPatch = `--- a/foo.go
+++ b/foo.go
@@ -3,1 +3,1 @@
-old3
+new3
@@ -12,1 +12,1 @@
-old12
+new12`

func newFileLines(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = ""
		switch i + 1 {
		case 3:
			out[i] = "new3" // mirrors patch's '+' line at NEW=3
		case 12:
			out[i] = "new12"
		default:
			out[i] = "L" + itoa(i+1)
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := []byte{}
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

func TestExpand_NoFileLines_NoSyntheticEmitted(t *testing.T) {
	// Without FileLines, Expand returns the raw patch unchanged — no
	// synthetic injection so existing test fixtures that don't exercise
	// expansion keep their buffer indices stable. Production prefetches
	// FileLines on file selection so the synthetic always shows live.
	res := Expand(ExpandInputs{Patch: twoHunkPatch})
	for _, l := range res.Lines {
		if l == SyntheticLine {
			t.Fatalf("synthetic emitted without FileLines: %q", res.Lines)
		}
	}
	if len(res.Gaps) != 0 {
		t.Fatalf("gaps map non-empty without FileLines: %+v", res.Gaps)
	}
}

func TestExpand_WithFileLines_BOFAndInterEmitted(t *testing.T) {
	// With FileLines, all gap kinds become surface-able. Verify BOF and
	// inter-hunk metadata so a regression in either branch shows up
	// distinct from the EOF assertion below.
	res := Expand(ExpandInputs{Patch: twoHunkPatch, FileLines: newFileLines(15)})
	var bof, mid *GapInfo
	for idx := range res.Gaps {
		g := res.Gaps[idx]
		switch g.ID.Kind {
		case GapKindBOF:
			gg := g
			bof = &gg
		case GapKindMid:
			gg := g
			mid = &gg
		}
	}
	if bof == nil {
		t.Fatalf("missing BOF gap")
	}
	if bof.HiddenCount != 2 || bof.OldStart != 1 || bof.OldEnd != 2 || bof.NewStart != 1 || bof.NewEnd != 2 {
		t.Fatalf("BOF gap fields: %+v", *bof)
	}
	if mid == nil {
		t.Fatalf("missing inter-hunk gap")
	}
	if mid.ID.Index != 0 || mid.HiddenCount != 8 {
		t.Fatalf("inter-hunk gap fields: %+v", *mid)
	}
	if mid.OldStart != 4 || mid.OldEnd != 11 || mid.NewStart != 4 || mid.NewEnd != 11 {
		t.Fatalf("inter-hunk line range: %+v", *mid)
	}
}

func TestExpand_NoExpansion_WithFileLines_EOFAppears(t *testing.T) {
	// File has 15 NEW lines → EOF gap = lines 13..15 (3 lines).
	res := Expand(ExpandInputs{Patch: twoHunkPatch, FileLines: newFileLines(15)})
	gotEOF := 0
	for _, g := range res.Gaps {
		if g.ID.Kind == GapKindEOF {
			gotEOF++
			if g.HiddenCount != 3 || g.NewStart != 13 || g.NewEnd != 15 {
				t.Fatalf("EOF gap fields: %+v", g)
			}
		}
	}
	if gotEOF != 1 {
		t.Fatalf("EOF synthetic count: got %d want 1", gotEOF)
	}
}

func TestExpand_NoExpansion_FileEndsAtLastHunk_NoEOF(t *testing.T) {
	// File has exactly 12 NEW lines — last hunk ends at NEW line 12, so
	// no hidden EOF region; the EOF synthetic must NOT appear.
	res := Expand(ExpandInputs{Patch: twoHunkPatch, FileLines: newFileLines(12)})
	for _, g := range res.Gaps {
		if g.ID.Kind == GapKindEOF {
			t.Fatalf("unexpected EOF synthetic: %+v", g)
		}
	}
}

func TestExpand_BOFGap_ExpandBelow(t *testing.T) {
	// Reveal 1 line immediately above the first hunk (NEW line 2).
	state := ExpandState{BOFBelow: 1}
	res := Expand(ExpandInputs{
		Patch:     twoHunkPatch,
		FileLines: newFileLines(15),
		Expand:    state,
	})
	// File headers stay at idx 0, 1. BOF synthetic + expanded ctx slot in
	// between the file headers and the first @@.
	if res.Lines[0] != "--- a/foo.go" {
		t.Fatalf("idx 0: got %q", res.Lines[0])
	}
	if res.Lines[1] != "+++ b/foo.go" {
		t.Fatalf("idx 1: got %q", res.Lines[1])
	}
	if res.Lines[2] != SyntheticLine {
		t.Fatalf("idx 2 should be synthetic, got %q", res.Lines[2])
	}
	if res.Lines[3] != " L2" {
		t.Fatalf("idx 3 (expanded BOF ctx): got %q want %q", res.Lines[3], " L2")
	}
	if !strings.HasPrefix(res.Lines[4], "@@") {
		t.Fatalf("idx 4 should be first hunk @@, got %q", res.Lines[4])
	}
	g, ok := res.Gaps[2]
	if !ok {
		t.Fatalf("no gap at idx 2 (gaps=%v)", res.Gaps)
	}
	if g.ID.Kind != GapKindBOF || g.HiddenCount != 1 || g.OldEnd != 1 || g.NewEnd != 1 {
		t.Fatalf("BOF gap after expand: %+v", g)
	}
}

func TestExpand_BOFGap_FullyExpanded_NoSynthetic(t *testing.T) {
	// Hidden BOF region is 2 lines; expand 2 → fully revealed, synthetic gone.
	state := ExpandState{BOFBelow: 2}
	res := Expand(ExpandInputs{
		Patch:     twoHunkPatch,
		FileLines: newFileLines(15),
		Expand:    state,
	})
	for _, g := range res.Gaps {
		if g.ID.Kind == GapKindBOF {
			t.Fatalf("BOF synthetic should be gone, got %+v", g)
		}
	}
	// File headers at idx 0,1. Expanded ctx for lines 1, 2 fills idx 2, 3.
	if res.Lines[2] != " L1" || res.Lines[3] != " L2" {
		t.Fatalf("BOF expanded ctx: got %q %q", res.Lines[2], res.Lines[3])
	}
}

func TestExpand_InterHunkSymmetric(t *testing.T) {
	// Inter-hunk gap (NEW 4..11, 8 lines). Above=2 Below=2 → 4 ctx shown,
	// 4 hidden in middle.
	state := ExpandState{
		InterAbove: map[int]int{0: 2},
		InterBelow: map[int]int{0: 2},
	}
	res := Expand(ExpandInputs{
		Patch:     twoHunkPatch,
		FileLines: newFileLines(15),
		Expand:    state,
	})
	// Find the second @@ in res.Lines.
	hunk2 := -1
	for i, l := range res.Lines {
		if strings.HasPrefix(l, "@@") && i > 0 {
			// second hunk (skip first @@)
			if hunk2 < 0 {
				// First match is hunk 0, we want hunk 1.
				// Use a tighter check below.
			}
		}
	}
	// Simpler: count @@ occurrences linearly.
	hunkIdxs := []int{}
	for i, l := range res.Lines {
		if strings.HasPrefix(l, "@@") {
			hunkIdxs = append(hunkIdxs, i)
		}
	}
	if len(hunkIdxs) != 2 {
		t.Fatalf("expected 2 hunk headers, got %d", len(hunkIdxs))
	}
	// Between hunk 0's last body row and hunk 1's @@: 2 ctx + 1 synth + 2 ctx = 5.
	between := res.Lines[hunkIdxs[0]+3 : hunkIdxs[1]] // hunk0 has 2 body rows (- and +)
	if len(between) != 5 {
		t.Fatalf("inter-hunk segment length: got %d want 5, segment=%q", len(between), between)
	}
	wantBetween := []string{" L4", " L5", SyntheticLine, " L10", " L11"}
	for i, w := range wantBetween {
		if between[i] != w {
			t.Fatalf("between[%d]: got %q want %q", i, between[i], w)
		}
	}
	// Synthetic's HiddenCount should be 4 (lines 6,7,8,9).
	synthIdx := hunkIdxs[0] + 3 + 2
	g, ok := res.Gaps[synthIdx]
	if !ok {
		t.Fatalf("synthetic at idx %d not in gaps map", synthIdx)
	}
	if g.HiddenCount != 4 || g.OldStart != 6 || g.OldEnd != 9 || g.NewStart != 6 || g.NewEnd != 9 {
		t.Fatalf("inter-hunk synth after expand: %+v", g)
	}
}

func TestExpand_InterHunkFullyClosed(t *testing.T) {
	// Above=4 Below=4 for 8-line gap → fully closed.
	state := ExpandState{
		InterAbove: map[int]int{0: 4},
		InterBelow: map[int]int{0: 4},
	}
	res := Expand(ExpandInputs{
		Patch:     twoHunkPatch,
		FileLines: newFileLines(15),
		Expand:    state,
	})
	for _, g := range res.Gaps {
		if g.ID.Kind == GapKindMid {
			t.Fatalf("inter-hunk synthetic should be gone, got %+v", g)
		}
	}
	// Look for the 8 expanded ctx lines.
	count := 0
	for _, l := range res.Lines {
		if strings.HasPrefix(l, " L") {
			count++
		}
	}
	if count != 8 {
		t.Fatalf("expanded ctx count: got %d want 8 (lines=%q)", count, res.Lines)
	}
}

func TestExpand_EOFExpand(t *testing.T) {
	// EOF gap NEW 13..15 (3 lines). Above=2 → reveal 13,14, hide 15.
	state := ExpandState{EOFAbove: 2}
	res := Expand(ExpandInputs{
		Patch:     twoHunkPatch,
		FileLines: newFileLines(15),
		Expand:    state,
	})
	// Last 3 lines should be " L13" " L14" SyntheticLine.
	n := len(res.Lines)
	got := res.Lines[n-3:]
	want := []string{" L13", " L14", SyntheticLine}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("EOF tail[%d]: got %q want %q (tail=%q)", i, got[i], w, got)
		}
	}
	g, ok := res.Gaps[n-1]
	if !ok {
		t.Fatalf("EOF synthetic missing at idx %d", n-1)
	}
	if g.ID.Kind != GapKindEOF || g.HiddenCount != 1 || g.NewStart != 15 || g.NewEnd != 15 {
		t.Fatalf("EOF gap after expand: %+v", g)
	}
}

func TestExpand_OverExpansionCaps(t *testing.T) {
	// Above=10 Below=10 for an 8-line gap: combined > total, fully closed,
	// no synthetic, exactly 8 ctx lines emitted (no duplicates from overlap).
	state := ExpandState{
		InterAbove: map[int]int{0: 10},
		InterBelow: map[int]int{0: 10},
	}
	res := Expand(ExpandInputs{
		Patch:     twoHunkPatch,
		FileLines: newFileLines(15),
		Expand:    state,
	})
	for _, g := range res.Gaps {
		if g.ID.Kind == GapKindMid {
			t.Fatalf("inter-hunk synthetic should be gone, got %+v", g)
		}
	}
	count := 0
	for _, l := range res.Lines {
		if strings.HasPrefix(l, " L") {
			count++
		}
	}
	if count != 8 {
		t.Fatalf("over-expand ctx count: got %d want 8", count)
	}
}

func TestExpand_PatchStartingAtLine1_NoBOF(t *testing.T) {
	const p = `--- a/foo.go
+++ b/foo.go
@@ -1,1 +1,1 @@
-old1
+new1`
	res := Expand(ExpandInputs{Patch: p, FileLines: newFileLines(1)})
	for _, g := range res.Gaps {
		if g.ID.Kind == GapKindBOF {
			t.Fatalf("BOF synthetic must not appear: %+v", g)
		}
	}
}

func TestExpand_EmptyPatch(t *testing.T) {
	res := Expand(ExpandInputs{Patch: ""})
	if len(res.Lines) != 0 || len(res.Gaps) != 0 {
		t.Fatalf("empty patch: lines=%q gaps=%v", res.Lines, res.Gaps)
	}
}

// Expanded context lines must use the OLD-file line number when computing
// their oldLn (i.e., file content at NEW line N corresponds to OLD line
// N - delta where delta is cumulative new-old shift). For an inter-hunk
// gap following the first one-line replace (no shift since old/new counts
// match), delta stays 0 — verify by computing specs on the augmented buffer.
func TestExpand_ExpandedContextLineNumbers(t *testing.T) {
	state := ExpandState{
		InterAbove: map[int]int{0: 1},
		InterBelow: map[int]int{0: 1},
	}
	res := Expand(ExpandInputs{
		Patch:     twoHunkPatch,
		FileLines: newFileLines(15),
		Expand:    state,
	})
	// Walk the augmented buffer using parseDiffSpecsAug (the
	// expansion-aware re-parser). The first expanded ctx in the inter-hunk
	// gap must report OldLn=4 / NewLn=4; the below one must report
	// OldLn=11 / NewLn=11.
	specs := ParseSpecsAug(res.Lines, res.Gaps)
	if len(specs) != len(res.Lines) {
		t.Fatalf("specs length: got %d want %d", len(specs), len(res.Lines))
	}
	// Find the first " L4" and the " L11" entries in res.Lines.
	for i, l := range res.Lines {
		switch l {
		case " L4":
			if specs[i].OldLn != 4 || specs[i].NewLn != 4 {
				t.Fatalf("specs[%d] (L4): got Old=%d New=%d", i, specs[i].OldLn, specs[i].NewLn)
			}
		case " L11":
			if specs[i].OldLn != 11 || specs[i].NewLn != 11 {
				t.Fatalf("specs[%d] (L11): got Old=%d New=%d", i, specs[i].OldLn, specs[i].NewLn)
			}
		}
	}
}
