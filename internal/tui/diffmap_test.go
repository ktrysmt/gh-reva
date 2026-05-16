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

// Range comments anchor a single ◆ at the end row; the upper edge of
// the range is conveyed by the `R<start>-<end>` text in the Comments
// column header, not by gutter glyphs. ┌/│ used to occupy the middle
// rows but were dropped — they could not coexist cleanly with anchors
// from neighbouring threads (markerRank forced ◆ to win the slot,
// hiding the range shape).
func TestCommentLineMarkers_SameSideRange(t *testing.T) {
	m := markersFixture(t)
	root := rcAt(1, 4, "RIGHT")
	root.StartLine = 2
	root.StartSide = "RIGHT"
	m.state.PR.Comments = []*model.ReviewComment{root}

	got := m.commentLineMarkers()
	want := map[int]rune{4: '◆'}
	if !equalMarkers(got.Right, want) {
		t.Fatalf("same-side range must only mark the end anchor: got %v want %v", got.Right, want)
	}
	if len(got.Left) != 0 {
		t.Fatalf("Left map must stay empty for RIGHT-only range: got %v", got.Left)
	}
}

func TestCommentLineMarkers_MixedSideRange(t *testing.T) {
	m := markersMixedSideFixture(t)
	// Range LEFT(oldLine 2) → RIGHT(newLine 2). End is RIGHT → ◆ lives in
	// the Right map at the `+` buffer row; no start glyph anywhere.
	root := rcAt(1, 2, "RIGHT")
	root.StartLine = 2
	root.StartSide = "LEFT"
	m.state.PR.Comments = []*model.ReviewComment{root}

	got := m.commentLineMarkers()
	want := map[int]rune{3: '◆'}
	if !equalMarkers(got.Right, want) {
		t.Fatalf("mixed-side range must only mark end anchor on RIGHT: got Right=%v want %v", got.Right, want)
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
	// Reply with bogus StartLine must NOT add a stray anchor on a
	// different buffer index.
	reply.StartLine = 1
	reply.StartSide = "RIGHT"
	m.state.PR.Comments = []*model.ReviewComment{root, reply}

	got := m.commentLineMarkers()
	want := map[int]rune{4: '◆'}
	if !equalMarkers(got.Right, want) {
		t.Fatalf("replies must not produce extra markers: got %v want %v", got.Right, want)
	}
}

// Two ranges whose anchors land on different buffer rows leave both ◆
// glyphs intact — no inter-thread interference now that range bodies
// no longer occupy intermediate rows.
func TestCommentLineMarkers_TwoRangesDifferentAnchors(t *testing.T) {
	m := markersFixture(t)
	a := rcAt(1, 4, "RIGHT")
	a.StartLine = 2
	a.StartSide = "RIGHT"
	b := rcAt(2, 5, "RIGHT")
	b.StartLine = 3
	b.StartSide = "RIGHT"
	m.state.PR.Comments = []*model.ReviewComment{a, b}

	got := m.commentLineMarkers()
	want := map[int]rune{4: '◆', 5: '◆'}
	if !equalMarkers(got.Right, want) {
		t.Fatalf("two ranges with distinct anchors Right: got %v want %v", got.Right, want)
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

// Resolved multi-line range marks only the end-anchor with ✓; the
// upper edge of the range is conveyed via the Comments header tag.
func TestCommentLineMarkers_ResolvedRange(t *testing.T) {
	m := markersFixture(t)
	root := rcAt(1, 4, "RIGHT")
	root.StartLine = 2
	root.StartSide = "RIGHT"
	root.Resolved = true
	m.state.PR.Comments = []*model.ReviewComment{root}

	got := m.commentLineMarkers()
	want := map[int]rune{4: '✓'}
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
