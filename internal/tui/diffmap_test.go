package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// markersFixture builds a Model with a synthetic patch on src/foo.go.
//
// Patch buffer layout:
//
//	0: @@ -1,3 +1,5 @@
//	1:  line1                (oldLine 1, newLine 1)
//	2: +addedLine2           (newLine 2)
//	3: +addedLine3           (newLine 3)
//	4: +addedLine4           (newLine 4)
//	5:  line5                (oldLine 3, newLine 5)
//
// Caller appends comments via m.state.PR.Comments.
func markersFixture(t *testing.T) Model {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	m.state.PR = &model.PR{
		Owner: "o", Repo: "r", Number: 1,
		Files: []*model.FileEntry{{Path: "src/foo.go", Status: model.ChangeModified}},
	}
	m.state.SelectedFile = "src/foo.go"
	m.state.DiffCache[diffKey("", "src/foo.go")] = strings.Join([]string{
		"@@ -1,3 +1,5 @@",
		" line1",
		"+addedLine2",
		"+addedLine3",
		"+addedLine4",
		" line5",
	}, "\n")
	return m
}

// markersMixedSideFixture builds a patch with both `-` and `+` rows, used
// to pin contiguous-buffer-span behaviour for ranges that cross sides.
//
//	0: @@ -1,3 +1,3 @@
//	1:  line1                (oldLine 1, newLine 1)
//	2: -removed_line2        (oldLine 2)
//	3: +added_line2          (newLine 2)
//	4:  line3                (oldLine 3, newLine 3)
func markersMixedSideFixture(t *testing.T) Model {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	m.state.PR = &model.PR{
		Owner: "o", Repo: "r", Number: 1,
		Files: []*model.FileEntry{{Path: "src/foo.go", Status: model.ChangeModified}},
	}
	m.state.SelectedFile = "src/foo.go"
	m.state.DiffCache[diffKey("", "src/foo.go")] = strings.Join([]string{
		"@@ -1,3 +1,3 @@",
		" line1",
		"-removed_line2",
		"+added_line2",
		" line3",
	}, "\n")
	return m
}

// rcAt is a constructor helper for ReviewComment with the minimum fields the
// renderer's marker computation looks at.
func rcAt(id int64, line int, side string) *model.ReviewComment {
	return &model.ReviewComment{
		ID: id, Path: "src/foo.go", CommitID: "abcdef",
		Line: line, Side: side, User: "u",
		CreatedAt: time.Date(2024, 1, 1, 0, 0, int(id), 0, time.Local),
		Body:      "x",
	}
}

func TestCommentLineMarkers_SingleLine(t *testing.T) {
	m := markersFixture(t)
	// Single-line root anchored on newLine 3 (buffer index 3).
	m.state.PR.Comments = []*model.ReviewComment{rcAt(1, 3, "RIGHT")}

	got := m.commentLineMarkers()
	if !equalMarkers(got.Right, map[int]rune{3: '◆'}) {
		t.Fatalf("RIGHT markers: got %v want {3:◆}", got.Right)
	}
	if len(got.Left) != 0 {
		t.Fatalf("LEFT markers must be empty for a RIGHT-only thread: got %v", got.Left)
	}
}

func TestCommentLineMarkers_LeftSideSingleLine(t *testing.T) {
	m := markersMixedSideFixture(t)
	// LEFT-side comment on oldLine 2 → buffer index 2 (the `-` row).
	m.state.PR.Comments = []*model.ReviewComment{rcAt(1, 2, "LEFT")}

	got := m.commentLineMarkers()
	if !equalMarkers(got.Left, map[int]rune{2: '◆'}) {
		t.Fatalf("LEFT-side ◆ must land in Left map at the `-` row: got Left=%v", got.Left)
	}
	if len(got.Right) != 0 {
		t.Fatalf("Right map must stay empty when only a LEFT thread exists: got %v", got.Right)
	}
}

func TestCommentLineMarkers_BothSidesAtDifferentRows(t *testing.T) {
	m := markersMixedSideFixture(t)
	// LEFT comment on `-` row (buffer 2) and RIGHT on `+` row (buffer 3).
	left := rcAt(1, 2, "LEFT")
	right := rcAt(2, 2, "RIGHT")
	m.state.PR.Comments = []*model.ReviewComment{left, right}

	got := m.commentLineMarkers()
	if !equalMarkers(got.Left, map[int]rune{2: '◆'}) {
		t.Errorf("Left map must have ◆ at the `-` buffer row: got %v", got.Left)
	}
	if !equalMarkers(got.Right, map[int]rune{3: '◆'}) {
		t.Errorf("Right map must have ◆ at the `+` buffer row: got %v", got.Right)
	}
}

func TestCommentLineMarkers_SameSideRange(t *testing.T) {
	m := markersFixture(t)
	// Range RIGHT 2 → 4 maps to buffer indices 2,3,4.
	root := rcAt(1, 4, "RIGHT")
	root.StartLine = 2
	root.StartSide = "RIGHT"
	m.state.PR.Comments = []*model.ReviewComment{root}

	got := m.commentLineMarkers()
	want := map[int]rune{2: '┌', 3: '│', 4: '◆'}
	if !equalMarkers(got.Right, want) {
		t.Fatalf("same-side range Right markers: got %v want %v", got.Right, want)
	}
	if len(got.Left) != 0 {
		t.Fatalf("Left map must stay empty for RIGHT-only range: got %v", got.Left)
	}
}

func TestCommentLineMarkers_TwoRowRange(t *testing.T) {
	m := markersFixture(t)
	// Range RIGHT 2 → 3 maps to buffer indices 2,3 — no middle row.
	root := rcAt(1, 3, "RIGHT")
	root.StartLine = 2
	root.StartSide = "RIGHT"
	m.state.PR.Comments = []*model.ReviewComment{root}

	got := m.commentLineMarkers()
	want := map[int]rune{2: '┌', 3: '◆'}
	if !equalMarkers(got.Right, want) {
		t.Fatalf("two-row range Right markers: got %v want %v", got.Right, want)
	}
}

func TestCommentLineMarkers_MixedSideRange(t *testing.T) {
	m := markersMixedSideFixture(t)
	// Range LEFT(oldLine 2) → RIGHT(newLine 2) — buffer indices 2,3.
	// The end's Side decides which column carries the visible run; the
	// start glyph rides the SAME column so the rendered range stays in
	// one column instead of being split across both. Painting across
	// both sides would visually conflate two unrelated review lanes.
	root := rcAt(1, 2, "RIGHT")
	root.StartLine = 2
	root.StartSide = "LEFT"
	m.state.PR.Comments = []*model.ReviewComment{root}

	got := m.commentLineMarkers()
	want := map[int]rune{2: '┌', 3: '◆'}
	if !equalMarkers(got.Right, want) {
		t.Fatalf("mixed-side range follows end Side (RIGHT): got Right=%v want %v", got.Right, want)
	}
	if len(got.Left) != 0 {
		t.Fatalf("Left map must stay empty when range end is RIGHT: got %v", got.Left)
	}
}

func TestCommentLineMarkers_RepliesIgnored(t *testing.T) {
	m := markersFixture(t)
	root := rcAt(1, 4, "RIGHT")
	root.StartLine = 2
	root.StartSide = "RIGHT"
	reply := rcAt(2, 4, "RIGHT")
	reply.InReplyTo = 1
	// Reply with bogus StartLine must NOT widen the marker span.
	reply.StartLine = 1
	reply.StartSide = "RIGHT"
	m.state.PR.Comments = []*model.ReviewComment{root, reply}

	got := m.commentLineMarkers()
	want := map[int]rune{2: '┌', 3: '│', 4: '◆'}
	if !equalMarkers(got.Right, want) {
		t.Fatalf("replies should not extend range Right: got %v want %v", got.Right, want)
	}
}

func TestCommentLineMarkers_OverlapPrecedence(t *testing.T) {
	m := markersFixture(t)
	// Thread A: range 2..4 → ┌ at 2, │ at 3, ◆ at 4.
	a := rcAt(1, 4, "RIGHT")
	a.StartLine = 2
	a.StartSide = "RIGHT"
	// Thread B: range 4..5 → ┌ at 4, ◆ at 5.
	// Buffer 4: A says ◆, B says ┌. Expected: ◆ wins.
	b := rcAt(2, 5, "RIGHT")
	b.StartLine = 4
	b.StartSide = "RIGHT"
	// Thread C: single-line on buffer 3 → ◆.
	// Buffer 3: A says │, C says ◆. Expected: ◆ wins.
	c := rcAt(3, 3, "RIGHT")
	m.state.PR.Comments = []*model.ReviewComment{a, b, c}

	got := m.commentLineMarkers()
	want := map[int]rune{2: '┌', 3: '◆', 4: '◆', 5: '◆'}
	if !equalMarkers(got.Right, want) {
		t.Fatalf("overlap precedence Right: got %v want %v", got.Right, want)
	}
}

// A resolved single-line thread renders as a checkmark (✓) instead of
// the diamond (◆). The thread is still anchored — gutter is occupied —
// but the visual signals "concern addressed".
func TestCommentLineMarkers_ResolvedSingleLine(t *testing.T) {
	m := markersFixture(t)
	root := rcAt(1, 3, "RIGHT")
	root.Resolved = true
	m.state.PR.Comments = []*model.ReviewComment{root}

	got := m.commentLineMarkers()
	if !equalMarkers(got.Right, map[int]rune{3: '✓'}) {
		t.Fatalf("resolved single-line Right markers: got %v want {3:✓}", got.Right)
	}
}

// Resolved multi-line range keeps the range start (┌) and middle (│)
// glyphs intact; only the end-anchor swaps from ◆ to ✓ so the range
// shape stays visible alongside the resolved signal.
func TestCommentLineMarkers_ResolvedRange(t *testing.T) {
	m := markersFixture(t)
	root := rcAt(1, 4, "RIGHT")
	root.StartLine = 2
	root.StartSide = "RIGHT"
	root.Resolved = true
	m.state.PR.Comments = []*model.ReviewComment{root}

	got := m.commentLineMarkers()
	want := map[int]rune{2: '┌', 3: '│', 4: '✓'}
	if !equalMarkers(got.Right, want) {
		t.Fatalf("resolved range Right markers: got %v want %v", got.Right, want)
	}
}

// Overlap: an unresolved ◆ and a resolved ✓ on the same buffer row →
// ◆ wins (unresolved concerns demand more attention).
func TestCommentLineMarkers_UnresolvedBeatsResolved(t *testing.T) {
	m := markersFixture(t)
	unresolved := rcAt(1, 3, "RIGHT")
	resolved := rcAt(2, 3, "RIGHT")
	resolved.Resolved = true
	m.state.PR.Comments = []*model.ReviewComment{unresolved, resolved}

	got := m.commentLineMarkers()
	if !equalMarkers(got.Right, map[int]rune{3: '◆'}) {
		t.Fatalf("overlap (unresolved + resolved): unresolved ◆ must win; got %v", got.Right)
	}
}

func TestCommentLineMarkers_RangeStartCollidesWithMiddle(t *testing.T) {
	m := markersFixture(t)
	// Thread A: range 2..4 (┌2 │3 ◆4)
	// Thread B: range 3..5 (┌3 │4 ◆5)
	// Buffer 3: A=│ vs B=┌ → ┌ wins (border > middle).
	// Buffer 4: A=◆ vs B=│ → ◆ wins.
	a := rcAt(1, 4, "RIGHT")
	a.StartLine = 2
	a.StartSide = "RIGHT"
	b := rcAt(2, 5, "RIGHT")
	b.StartLine = 3
	b.StartSide = "RIGHT"
	m.state.PR.Comments = []*model.ReviewComment{a, b}

	got := m.commentLineMarkers()
	want := map[int]rune{2: '┌', 3: '┌', 4: '◆', 5: '◆'}
	if !equalMarkers(got.Right, want) {
		t.Fatalf("border-vs-middle precedence Right: got %v want %v", got.Right, want)
	}
}

func equalMarkers(a, b map[int]rune) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
