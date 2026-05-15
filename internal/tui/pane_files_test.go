package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
)

func makeFilesModel(t *testing.T, files []*model.FileEntry, commits []*model.Commit) Model {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	m.state.PR = &model.PR{Files: files, Commits: commits}
	if len(files) > 0 {
		// Mirror the loader: cursor 1 is files[0] (cursor 0 = All row).
		m.state.SelectedFile = files[0].Path
		m.state.FilesCursor = 1
	}
	return m
}

func sampleFiles() []*model.FileEntry {
	return []*model.FileEntry{
		{Path: "a.go", Status: model.ChangeModified},
		{Path: "b.go", Status: model.ChangeAdded},
		{Path: "c.go", Status: model.ChangeDeleted},
	}
}

// The Files pane prepends a synthetic "All (N files)" row at cursor
// index 0, symmetric to the Commits pane's "All commits (N)" row.
func TestFilesView_AllRowAtIndexZero(t *testing.T) {
	m := makeFilesModel(t, sampleFiles(), nil)
	out := m.filesView()
	lines := strings.Split(out, "\n")
	// Skip title (line 0). Line 1 is the first content row.
	if len(lines) < 2 {
		t.Fatalf("filesView should render title + rows; got: %q", out)
	}
	if !strings.Contains(lines[1], "All (3 files)") {
		t.Fatalf("first content row must read 'All (3 files)'; got: %q", lines[1])
	}
}

// Enter on the All row commits "no single file" by setting SelectedFile
// to model.AllFilesPath and shifts focus to Diff.
func TestFilesEnter_OnAllRow_SelectsAllFiles(t *testing.T) {
	m := makeFilesModel(t, sampleFiles(), nil)
	m.state.FilesCursor = 0
	mm, _ := m.handleKeyFiles(tea.KeyMsg{Type: tea.KeyEnter})
	got := mm.(Model)
	if got.state.SelectedFile != model.AllFilesPath {
		t.Fatalf("Enter on All row should set SelectedFile=AllFilesPath; got %q", got.state.SelectedFile)
	}
	if got.state.FocusedPane != model.PaneDiff {
		t.Fatalf("Enter on All row should shift focus to Diff; got %v", got.state.FocusedPane)
	}
}

// Enter on a real file row at cursor 1 commits files[0]; cursor n+1
// commits files[n]. The cursor index shifts by one against PR.Files[].
func TestFilesEnter_OnFileRow_UsesShiftedIndex(t *testing.T) {
	m := makeFilesModel(t, sampleFiles(), nil)
	m.state.FilesCursor = 2 // cursor 2 → files[1]
	mm, _ := m.handleKeyFiles(tea.KeyMsg{Type: tea.KeyEnter})
	got := mm.(Model)
	if got.state.SelectedFile != "b.go" {
		t.Fatalf("Enter at cursor 2 should select files[1]=b.go; got %q", got.state.SelectedFile)
	}
}

// j/k bounds: cursor span is [0, len(files)] inclusive. j past last
// file is a no-op; k from 0 is a no-op.
func TestFilesJK_BoundsIncludeAllRow(t *testing.T) {
	m := makeFilesModel(t, sampleFiles(), nil)
	m.state.FilesCursor = 0
	mm, _ := m.handleKeyFiles(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if mm.(Model).state.FilesCursor != 0 {
		t.Fatalf("k from cursor 0 should stay at 0; got %d", mm.(Model).state.FilesCursor)
	}
	m.state.FilesCursor = len(sampleFiles()) // last file row
	mm, _ = m.handleKeyFiles(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if mm.(Model).state.FilesCursor != len(sampleFiles()) {
		t.Fatalf("j at last file should not advance past; got %d", mm.(Model).state.FilesCursor)
	}
}

// G jumps to the last file (cursor = len(files)), NOT past the end.
func TestFilesG_LandsOnLastFile(t *testing.T) {
	m := makeFilesModel(t, sampleFiles(), nil)
	m.state.FilesCursor = 0
	mm, _ := m.handleKeyFiles(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	if mm.(Model).state.FilesCursor != len(sampleFiles()) {
		t.Fatalf("G should jump to cursor=len(files)=%d (last file); got %d",
			len(sampleFiles()), mm.(Model).state.FilesCursor)
	}
}

// On AllFilesPath, visibleCommits returns the full commit list with no
// per-file filtering — the user wants to browse the entire PR history.
func TestVisibleCommits_AllFilesPath_NoFilter(t *testing.T) {
	commits := []*model.Commit{
		{SHA: "aaaaaaaa", ShortSHA: "aaaaaaa", Message: "first",
			ChangedFiles: map[string]model.ChangeKind{"a.go": model.ChangeModified}},
		{SHA: "bbbbbbbb", ShortSHA: "bbbbbbb", Message: "second",
			ChangedFiles: map[string]model.ChangeKind{"b.go": model.ChangeAdded}},
	}
	m := makeFilesModel(t, sampleFiles(), commits)
	m.state.SelectedFile = model.AllFilesPath
	got := m.visibleCommits()
	if len(got) != len(commits) {
		t.Fatalf("AllFilesPath must bypass file filter; got %d commits, want %d", len(got), len(commits))
	}
}

// allCommitsRow on AllFilesPath drops the [status] annotation and the
// "(M of N)" suffix — those refer to a single file.
func TestAllCommitsRow_OnAllFilesPath_NoAnnotation(t *testing.T) {
	commits := []*model.Commit{
		{SHA: "aaaaaaaa", ShortSHA: "aaaaaaa", Message: "first",
			ChangedFiles: map[string]model.ChangeKind{"a.go": model.ChangeModified}},
		{SHA: "bbbbbbbb", ShortSHA: "bbbbbbb", Message: "second",
			ChangedFiles: map[string]model.ChangeKind{"b.go": model.ChangeAdded}},
	}
	m := makeFilesModel(t, sampleFiles(), commits)
	m.state.SelectedFile = model.AllFilesPath
	row := m.allCommitsRow(m.visibleCommits())
	if !strings.Contains(row, "All commits (2)") {
		t.Fatalf("row should read 'All commits (2)' (no M-of-N); got: %q", row)
	}
	if strings.Contains(row, " of ") {
		t.Fatalf("row must not contain 'of' when SelectedFile is AllFilesPath; got: %q", row)
	}
}

// commitsView on AllFilesPath omits the per-row [A/M/D/R] annotation.
func TestCommitsView_OnAllFilesPath_NoPerRowAnnotation(t *testing.T) {
	commits := []*model.Commit{
		{SHA: "aaaaaaaa", ShortSHA: "aaaaaaa", Message: "first",
			ChangedFiles: map[string]model.ChangeKind{"a.go": model.ChangeModified}},
	}
	m := makeFilesModel(t, sampleFiles(), commits)
	m.state.SelectedFile = model.AllFilesPath
	out := m.commitsView()
	if strings.Contains(out, "[M]") || strings.Contains(out, "[A]") ||
		strings.Contains(out, "[D]") || strings.Contains(out, "[R]") {
		t.Fatalf("AllFilesPath must suppress [A/M/D/R] annotation; got:\n%s", out)
	}
}

// patchInfo returns a non-nil patch when SelectedFile is AllFilesPath,
// drawing from the pre-built diffKey("", AllFilesPath) entry.
func TestPatchInfo_AllFilesPath_ReturnsConcatPatch(t *testing.T) {
	m := makeFilesModel(t, sampleFiles(), nil)
	m.state.SelectedFile = model.AllFilesPath
	m.state.DiffCache[diffKey("", model.AllFilesPath)] = "--- a/x\n+++ b/x\n@@ -1 +1 @@\n-old\n+new\n"
	pi := m.patchInfo()
	if pi == nil {
		t.Fatalf("patchInfo must serve AllFilesPath when DiffCache has the entry; got nil")
	}
	if len(pi.lines) == 0 {
		t.Fatalf("patch lines should be populated; got 0 lines")
	}
}

// Yank in Files visual mode skips index 0 (the All row) — it is not a
// real path. Mirrors the Commits-pane visual yank behavior for the
// All-commits virtual row.
func TestYank_FilesVisual_SkipsAllRow(t *testing.T) {
	m := makeFilesModel(t, sampleFiles(), nil)
	m.state.FocusedPane = model.PaneFiles
	m.state.FilesCursor = 2
	m.state.Visual = &model.VisualState{OriginPane: model.PaneFiles, Anchor: 0, Linewise: true}
	got := m.yankString()
	if strings.Contains(got, "All (") {
		t.Fatalf("yank must not include the All row literal; got: %q", got)
	}
	// Anchor 0 → cursor 2 covers idx 0 (All), 1 (a.go), 2 (b.go). Skip All → a.go + b.go.
	if !strings.Contains(got, "a.go") || !strings.Contains(got, "b.go") {
		t.Fatalf("yank should contain a.go and b.go; got: %q", got)
	}
	if strings.Contains(got, "c.go") {
		t.Fatalf("yank must not reach files[2]=c.go (cursor was 2 → b.go after shift); got: %q", got)
	}
}
