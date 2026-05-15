package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// Diff yank strips the leading column from `+` / `-` / context rows so a
// mixed-range paste keeps consistent indentation. Header (`---` / `+++`)
// and hunk (`@@`) rows are metadata, not source, and stay verbatim;
// synthetic `···` rows are skipped (existing contract).
func TestYank_Diff_StripsPlusMinusAndContextPrefix(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	m.state.PR = &model.PR{
		Owner: "o", Repo: "r", Number: 1,
		Files: []*model.FileEntry{{Path: "x.go", Status: model.ChangeModified}},
	}
	m.state.SelectedFile = "x.go"
	m.state.DiffCache[diffKey("", "x.go")] = strings.Join([]string{
		"@@ -1,3 +1,4 @@",
		" ctx1",
		"-removed_a",
		"+added_x",
		" ctx2",
	}, "\n")
	m.state.FocusedPane = model.PaneDiff
	m.state.DiffCursor.Line = 4
	m.state.Visual = &model.VisualState{OriginPane: model.PaneDiff, AnchorLine: 0}

	got := m.yankString()
	want := strings.Join([]string{
		"@@ -1,3 +1,4 @@",
		"ctx1",
		"removed_a",
		"added_x",
		"ctx2",
	}, "\n")
	if got != want {
		t.Fatalf("Diff yank mismatch.\nwant:\n%q\n got:\n%q", want, got)
	}
}

// Indentation past the diff prefix slot is preserved verbatim — only the
// single leading column ('+'/'-'/' ') is dropped.
func TestYank_Diff_PreservesIndentationPastPrefix(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	m.state.PR = &model.PR{
		Owner: "o", Repo: "r", Number: 1,
		Files: []*model.FileEntry{{Path: "x.go", Status: model.ChangeModified}},
	}
	m.state.SelectedFile = "x.go"
	m.state.DiffCache[diffKey("", "x.go")] = strings.Join([]string{
		"@@ -1,2 +1,2 @@",
		"-    old()",
		"+    new()",
	}, "\n")
	m.state.FocusedPane = model.PaneDiff
	m.state.DiffCursor.Line = 2
	m.state.Visual = &model.VisualState{OriginPane: model.PaneDiff, AnchorLine: 1}

	got := m.yankString()
	want := "    old()\n    new()"
	if got != want {
		t.Fatalf("Diff yank should preserve indentation past prefix slot.\nwant:\n%q\n got:\n%q", want, got)
	}
}

// Single-line yank (no visual range) on a `+` row also drops the prefix.
func TestYank_Diff_SingleLine_StripsPrefix(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	m.state.PR = &model.PR{
		Owner: "o", Repo: "r", Number: 1,
		Files: []*model.FileEntry{{Path: "x.go", Status: model.ChangeModified}},
	}
	m.state.SelectedFile = "x.go"
	m.state.DiffCache[diffKey("", "x.go")] = strings.Join([]string{
		"@@ -1 +1 @@",
		"-old",
		"+new",
	}, "\n")
	m.state.FocusedPane = model.PaneDiff
	m.state.DiffCursor.Line = 2 // "+new"

	if got, want := m.yankString(), "new"; got != want {
		t.Fatalf("single-line Diff yank should strip leading `+`; want %q got %q", want, got)
	}
}
