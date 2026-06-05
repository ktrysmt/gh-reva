package tui

import (
	"strings"
	"testing"

	"github.com/ktrysmt/gh-reva/internal/diff"
	"github.com/ktrysmt/gh-reva/internal/model"
)

// fileHeaderLabel collapses the redundant `--- a/X` / `+++ b/X` pair into
// one label + status byte. Modified files duplicate the path on both
// rows; the collapse keeps a single path. Added / deleted / renamed
// retain their distinguishing shape.
func TestFileHeaderLabel(t *testing.T) {
	cases := []struct {
		name           string
		oldLn, newLn   string
		wantLabel      string
		wantStatus     byte
	}{
		{"modified", "--- a/internal/model/state.go", "+++ b/internal/model/state.go", "internal/model/state.go", 'M'},
		{"added", "--- /dev/null", "+++ b/src/new.go", "src/new.go", 'A'},
		{"deleted", "--- a/src/gone.go", "+++ /dev/null", "src/gone.go", 'D'},
		{"renamed", "--- a/old.go", "+++ b/new.go", "old.go → new.go", 'R'},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			label, st := fileHeaderLabel(c.oldLn, c.newLn)
			if label != c.wantLabel || st != c.wantStatus {
				t.Fatalf("fileHeaderLabel(%q,%q) = (%q,%c); want (%q,%c)",
					c.oldLn, c.newLn, label, st, c.wantLabel, c.wantStatus)
			}
		})
	}
}

// hunkSignature drops the noisy numeric range (duplicated by the gutter
// line numbers) and keeps the trailing function / type signature.
func TestHunkSignature(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"@@ -100,14 +100,18 @@ type DiffViewport struct {", "type DiffViewport struct {"},
		{"@@ -1 +1 @@", ""},
		{"@@ -1,2 +1,2 @@ func Foo()", "func Foo()"},
	}
	for _, c := range cases {
		if got := hunkSignature(c.in); got != c.want {
			t.Errorf("hunkSignature(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

// nearestHunkSignature walks backward from the viewport top to the
// enclosing hunk so the sticky header can show which hunk is on screen.
func TestNearestHunkSignature(t *testing.T) {
	lines := []string{
		"--- a/x.go",
		"+++ b/x.go",
		"@@ -1,3 +1,3 @@ func A()",
		" a",
		"-b",
		"+B",
		"@@ -10,3 +10,3 @@ func B()",
		" c",
		" d",
	}
	if got := nearestHunkSignature(lines, 4); got != "func A()" {
		t.Errorf("inside hunk A: got %q want func A()", got)
	}
	if got := nearestHunkSignature(lines, 8); got != "func B()" {
		t.Errorf("inside hunk B: got %q want func B()", got)
	}
	if got := nearestHunkSignature(lines, 1); got != "" {
		t.Errorf("above any hunk: got %q want empty", got)
	}
}

// fileBarBody renders the collapsed one-line file separator. It carries
// the path once, a bracketed status tag, and fills the remaining width
// with the bar glyph.
func TestFileBarBody(t *testing.T) {
	body := fileBarBody("src/main.go", 'M', 40)
	if !strings.Contains(body, "src/main.go") {
		t.Errorf("bar missing path: %q", body)
	}
	if !strings.Contains(body, "[M]") {
		t.Errorf("bar missing status tag: %q", body)
	}
	if !strings.HasPrefix(body, "─── ") {
		t.Errorf("bar must start with leading rule: %q", body)
	}
	if w := []rune(body); len(w) < 40 {
		// width measured in runes here is sufficient for ASCII path.
		t.Errorf("bar not filled to width 40: %q (len=%d)", body, len(w))
	}
}

// displayRowsForLine must treat the collapsed `+++` row as zero-height
// (folded into the `---` bar) and the `---` / `@@` rows as exactly one
// row so the wrap-aware reverse mapping (mouse / scroll) stays consistent.
func TestDisplayRowsForLine_Headers(t *testing.T) {
	if n := displayRowsForLine("+++ b/x.go", false, 0, 80); n != 0 {
		t.Errorf("+++ row must be 0 display rows; got %d", n)
	}
	if n := displayRowsForLine("--- a/x.go", false, 0, 80); n != 1 {
		t.Errorf("--- row must be 1 display row; got %d", n)
	}
	if n := displayRowsForLine("@@ -1,2 +1,2 @@ x", false, 0, 80); n != 1 {
		t.Errorf("@@ row must be 1 display row; got %d", n)
	}
}

// lineExistsOnSide must make the collapsed `+++` row non-navigable so
// j/k auto-skip never parks the cursor on an invisible row, while the
// `---` bar row stays a valid cursor target on both sides.
func TestLineExistsOnSide_CollapsedPlusPlusPlus(t *testing.T) {
	if lineExistsOnSide("+++ b/x.go", model.DiffSideLeft) || lineExistsOnSide("+++ b/x.go", model.DiffSideRight) {
		t.Errorf("+++ row must be non-navigable on both sides")
	}
	if !lineExistsOnSide("--- a/x.go", model.DiffSideLeft) || !lineExistsOnSide("--- a/x.go", model.DiffSideRight) {
		t.Errorf("--- bar row must be navigable on both sides")
	}
}

// renderFileBar / renderHunkRule / the collapsed `+++` path: the file
// header becomes one bar row, the hunk header a thin rule, and `+++`
// renders nothing (it is folded into the bar).
func TestRenderHeaderRows(t *testing.T) {
	m := newRenderTestModel(t, 60)

	bar := m.renderFileBar("src/main.go", 'M', 0, -1)
	if len(bar) != 1 {
		t.Fatalf("file bar must be a single row; got %d", len(bar))
	}
	if got := stripSGR(bar[0]); !strings.Contains(got, "src/main.go") || !strings.Contains(got, "[M]") {
		t.Errorf("file bar content unexpected: %q", got)
	}

	rule := m.renderHunkRule(0, -1)
	if len(rule) != 1 {
		t.Fatalf("hunk rule must be a single row; got %d", len(rule))
	}
	if got := stripSGR(rule[0]); strings.Contains(got, "@@") || strings.Contains(got, "100") {
		t.Errorf("hunk rule must drop the numeric @@ range: %q", got)
	}
	if got := stripSGR(rule[0]); !strings.Contains(got, "─") {
		t.Errorf("hunk rule must be a horizontal rule: %q", got)
	}
}

// diffStickyHeader pins the current file path and enclosing hunk
// signature at the top of the Diff viewport.
func TestDiffStickyHeader(t *testing.T) {
	m := newRenderTestModel(t, 60)
	m.state.SelectedFile = "internal/model/state.go"
	lines := []string{
		"--- a/internal/model/state.go",
		"+++ b/internal/model/state.go",
		"@@ -100,14 +100,18 @@ type DiffViewport struct {",
		" Height int",
		" Top int",
	}
	got := stripSGR(m.diffStickyHeader(3, lines))
	if !strings.Contains(got, "internal/model/state.go") {
		t.Errorf("sticky must show file path: %q", got)
	}
	if !strings.Contains(got, "type DiffViewport struct {") {
		t.Errorf("sticky must show hunk signature: %q", got)
	}
}

// diffStickyHeader in the Files "All" view (concatenated cross-file
// diff) must follow the scroll position so the pinned header always
// names the file the top visible line belongs to — the key affordance
// for knowing "which file am I looking at" while browsing All.
func TestDiffStickyHeader_AllViewFollowsScroll(t *testing.T) {
	m := newRenderTestModel(t, 60)
	m.state.PR = &model.PR{HeadSHA: "headsha"}
	m.state.SelectedFile = model.AllFilesPath
	m.state.SelectedRange = model.CommitRange{Kind: model.RangeWholePR}

	goDiff := "--- a/src/greeting.go\n+++ b/src/greeting.go\n@@ -1,1 +1,2 @@ func A()\n package src\n+func Hello() {}\n"
	pyDiff := "--- a/app/main.py\n+++ b/app/main.py\n@@ -1,1 +1,2 @@ def hi\n import os\n+def hi(): pass\n"
	m.state.DiffCache[diffKey("", model.AllFilesPath)] = goDiff + pyDiff

	lines := m.patchLines()
	// Find a buffer index inside the .go section and one inside the .py
	// section by scanning the per-line file map.
	pi := m.patchInfo()
	if pi == nil || pi.filePaths == nil {
		t.Fatal("expected per-line file paths in All view")
	}
	var goIdx, pyIdx = -1, -1
	for i, p := range pi.filePaths {
		if p == "src/greeting.go" && goIdx < 0 {
			goIdx = i
		}
		if p == "app/main.py" {
			pyIdx = i
		}
	}
	if goIdx < 0 || pyIdx < 0 {
		t.Fatalf("could not locate both file sections (go=%d py=%d) in %v", goIdx, pyIdx, lines)
	}
	if got := stripSGR(m.diffStickyHeader(goIdx, lines)); !strings.Contains(got, "src/greeting.go") {
		t.Errorf("top in .go section: sticky=%q want src/greeting.go", got)
	}
	if got := stripSGR(m.diffStickyHeader(pyIdx, lines)); !strings.Contains(got, "app/main.py") {
		t.Errorf("top in .py section: sticky=%q want app/main.py", got)
	}
}

var _ = diff.SyntheticLine
