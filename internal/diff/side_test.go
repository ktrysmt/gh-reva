package diff

import "testing"

// Patch fixture used across the side-resolver tests. Hunk header sets old
// start = 10, new start = 20, so:
//
//	idx 0: file header (---)
//	idx 1: file header (+++)
//	idx 2: hunk header (@@)
//	idx 3: context  -> oldLn 10 / newLn 20
//	idx 4: deletion -> oldLn 11 / no new
//	idx 5: addition -> no old / newLn 21
//	idx 6: context  -> oldLn 12 / newLn 22
//	idx 7: addition -> no old / newLn 23
const sidePatch = `--- a/foo.go
+++ b/foo.go
@@ -10,3 +20,4 @@ context
 unchanged-1
-removed-line
+added-line
 unchanged-2
+tail-add`

func TestResolveAnchor_ContextLine(t *testing.T) {
	a, ok := ResolveAnchor(sidePatch, 3)
	if !ok {
		t.Fatalf("context line should anchor, got !ok")
	}
	if a.Side != "RIGHT" || a.NewLine != 20 || a.OldLine != 10 {
		t.Fatalf("context: got %+v", a)
	}
}

func TestResolveAnchor_Deletion(t *testing.T) {
	a, ok := ResolveAnchor(sidePatch, 4)
	if !ok {
		t.Fatalf("deletion should anchor, got !ok")
	}
	if a.Side != "LEFT" || a.OldLine != 11 || a.NewLine != 0 {
		t.Fatalf("deletion: got %+v", a)
	}
}

func TestResolveAnchor_Addition(t *testing.T) {
	a, ok := ResolveAnchor(sidePatch, 5)
	if !ok {
		t.Fatalf("addition should anchor, got !ok")
	}
	if a.Side != "RIGHT" || a.NewLine != 21 || a.OldLine != 0 {
		t.Fatalf("addition: got %+v", a)
	}
}

func TestResolveAnchor_TailAddition(t *testing.T) {
	a, ok := ResolveAnchor(sidePatch, 7)
	if !ok {
		t.Fatalf("tail add should anchor, got !ok")
	}
	if a.Side != "RIGHT" || a.NewLine != 23 {
		t.Fatalf("tail add: got %+v", a)
	}
}

func TestResolveAnchor_FileHeaderRejected(t *testing.T) {
	for _, idx := range []int{0, 1, 2} {
		if _, ok := ResolveAnchor(sidePatch, idx); ok {
			t.Fatalf("idx %d (header/hunk) must not anchor", idx)
		}
	}
}

func TestResolveAnchor_OutOfRange(t *testing.T) {
	if _, ok := ResolveAnchor(sidePatch, -1); ok {
		t.Fatalf("negative idx must not anchor")
	}
	if _, ok := ResolveAnchor(sidePatch, 999); ok {
		t.Fatalf("over-range idx must not anchor")
	}
}

func TestResolveAnchor_EmptyPatch(t *testing.T) {
	if _, ok := ResolveAnchor("", 0); ok {
		t.Fatalf("empty patch must not anchor")
	}
}

// Multi-line range resolver: ResolveRange takes anchor + cursor buffer
// indices and returns the canonical (start_line, start_side, line, side)
// tuple. start_line is always <= line in the same-side case; mixed-side
// ranges are accepted as-is (GitHub allows differing start_side / side).
func TestResolveRange_AllAddRight(t *testing.T) {
	r, ok := ResolveRange(sidePatch, 5, 7) // both '+' rows
	if !ok {
		t.Fatalf("range should resolve")
	}
	if r.Side != "RIGHT" || r.StartSide != "RIGHT" {
		t.Fatalf("sides: got %+v", r)
	}
	if r.StartLine != 21 || r.Line != 23 {
		t.Fatalf("lines: got start=%d line=%d", r.StartLine, r.Line)
	}
}

func TestResolveRange_ReversedAnchorCursor(t *testing.T) {
	// anchor below cursor — output should still place start at the smaller
	// line / cursor at the larger so GitHub accepts it.
	r, ok := ResolveRange(sidePatch, 7, 5)
	if !ok {
		t.Fatalf("reversed range should resolve")
	}
	if r.StartLine != 21 || r.Line != 23 {
		t.Fatalf("reversed: got start=%d line=%d", r.StartLine, r.Line)
	}
}

func TestResolveRange_SingleLine(t *testing.T) {
	// anchor == cursor: collapse to single-line; StartLine should be 0
	// (caller treats StartLine==0 as "no start_line / start_side fields").
	r, ok := ResolveRange(sidePatch, 5, 5)
	if !ok {
		t.Fatalf("single-line range should resolve")
	}
	if r.StartLine != 0 || r.Line != 21 || r.Side != "RIGHT" {
		t.Fatalf("single: got %+v", r)
	}
}

func TestResolveRange_HeaderEndpoint(t *testing.T) {
	if _, ok := ResolveRange(sidePatch, 2, 5); ok {
		t.Fatalf("header endpoint must not resolve")
	}
}

// Mixed-side ranges (LEFT '-' line + RIGHT '+'/context line) must be
// normalized so the buffer-earlier endpoint becomes start. Previously
// shouldSwap returned false for cross-side pairs and the user-provided
// (anchor → cursor) order leaked through, so a user who anchored on a
// later '+' row and dragged up to an earlier '-' row produced a payload
// with start_line numerically larger than line — GitHub responds with
// 422 "start_line must be less than end_line".
func TestResolveRange_MixedSideNormalizesByBuffer(t *testing.T) {
	// idx 5 = '+added' (RIGHT, newLine 21); idx 4 = '-removed' (LEFT, oldLine 11)
	// User anchors at the '+' (idx 5) and walks cursor up to the '-' (idx 4):
	// canonical range must start at the '-' and end at the '+'.
	r, ok := ResolveRange(sidePatch, 5, 4)
	if !ok {
		t.Fatalf("mixed-side range should resolve")
	}
	if r.StartSide != "LEFT" || r.StartLine != 11 {
		t.Fatalf("start should be '-' row: got side=%s line=%d", r.StartSide, r.StartLine)
	}
	if r.Side != "RIGHT" || r.Line != 21 {
		t.Fatalf("end should be '+' row: got side=%s line=%d", r.Side, r.Line)
	}
}

func TestResolveRange_MixedSideAlreadyOrdered(t *testing.T) {
	// anchor = '-' (idx 4), cursor = '+' (idx 5): buffer-ordered already,
	// must pass through without swap.
	r, ok := ResolveRange(sidePatch, 4, 5)
	if !ok {
		t.Fatalf("mixed-side range should resolve")
	}
	if r.StartSide != "LEFT" || r.StartLine != 11 || r.Side != "RIGHT" || r.Line != 21 {
		t.Fatalf("ordered mixed-side wrong: %+v", r)
	}
}

// Empty patch must reject Range too (ResolveAnchor already does; this
// pins the contract end-to-end).
func TestResolveRange_EmptyPatch(t *testing.T) {
	if _, ok := ResolveRange("", 0, 0); ok {
		t.Fatalf("empty patch must not resolve a range")
	}
}

// Single-line on a deletion ('-') must produce LEFT/oldLine, not the
// RIGHT default. Locks down the path that previously only had a
// '+'-row single-line test.
func TestResolveRange_SingleLineDeletion(t *testing.T) {
	r, ok := ResolveRange(sidePatch, 4, 4)
	if !ok {
		t.Fatalf("single-line deletion range should resolve")
	}
	if r.Side != "LEFT" || r.Line != 11 || r.StartLine != 0 {
		t.Fatalf("deletion single: got %+v", r)
	}
}

// Range whose endpoints sit in different hunks must still resolve;
// GitHub itself rejects cross-hunk multi-line comments, but the
// resolver should not silently mangle data — it returns the
// canonical (buffer-ordered) tuple and lets the API surface the
// validation error.
func TestResolveRange_CrossHunk(t *testing.T) {
	const twoHunkPatch = `--- a/foo.go
+++ b/foo.go
@@ -10,2 +20,2 @@
 ctxA
+addA
@@ -30,2 +40,2 @@
 ctxB
+addB`
	// idx 0,1: file headers
	// idx 2: hunk1 @@ (oldStart=10, newStart=20)
	// idx 3: ctxA  -> oldLn=10/newLn=20
	// idx 4: +addA -> newLn=21
	// idx 5: hunk2 @@ (oldStart=30, newStart=40)
	// idx 6: ctxB  -> oldLn=30/newLn=40
	// idx 7: +addB -> newLn=41
	r, ok := ResolveRange(twoHunkPatch, 4, 7)
	if !ok {
		t.Fatalf("cross-hunk range should resolve")
	}
	if r.StartLine != 21 || r.Line != 41 {
		t.Fatalf("cross-hunk wrong: start=%d line=%d", r.StartLine, r.Line)
	}
}
