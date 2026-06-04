package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// commitsManyFixture builds a Model with `n` commits under the All-files
// view (so visibleCommits is unfiltered) and a small Commits viewport so
// the rendered rows overflow and force scrolling. The Commits pane cursor
// space is [0, n]: index 0 is the synthetic "All commits" row, 1..n map to
// commits[i-1].
func commitsManyFixture(t *testing.T, n int) Model {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	var commits []*model.Commit
	for i := 0; i < n; i++ {
		commits = append(commits, &model.Commit{
			SHA:      fmt.Sprintf("%040d", i),
			ShortSHA: fmt.Sprintf("sha%04d", i),
			Message:  fmt.Sprintf("commit message %d", i),
		})
	}
	m.state.PR = &model.PR{
		Owner: "o", Repo: "r", Number: 1,
		Commits: commits,
		Files:   []*model.FileEntry{{Path: "a.go", Status: model.ChangeModified}},
	}
	m.state.SelectedFile = model.AllFilesPath
	m.state.FocusedPane = model.PaneCommits
	m.paneWidthCommits = 40
	m.paneHeightCommits = 6 // small viewport so scroll is forced
	return m
}

// filesManyFixture builds a Model with `n` files and a small Files
// viewport. Files cursor space is [0, n]: index 0 is the synthetic All row.
func filesManyFixture(t *testing.T, n int) Model {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	var files []*model.FileEntry
	for i := 0; i < n; i++ {
		files = append(files, &model.FileEntry{
			Path:   fmt.Sprintf("src/file%03d.go", i),
			Status: model.ChangeModified,
		})
	}
	m.state.PR = &model.PR{Owner: "o", Repo: "r", Number: 1, Files: files}
	m.state.SelectedFile = model.AllFilesPath
	m.state.FocusedPane = model.PaneFiles
	m.paneWidthFiles = 40
	m.paneHeightFiles = 6
	return m
}

// TestCommits_NoScrollWhenContentFits pins that a short commit list never
// scrolls — Top stays 0 so the existing small-fixture layout is unchanged.
func TestCommits_NoScrollWhenContentFits(t *testing.T) {
	m := commitsManyFixture(t, 3) // 3 commits + All row = 4 rows < height 6
	m.state.CommitsCursor = 3
	_ = m.commitsView()
	if m.state.CommitsTop != 0 {
		t.Errorf("short list must not scroll; CommitsTop=%d want 0", m.state.CommitsTop)
	}
}

// TestCommits_CursorStaysVisibleOnScrollDown pins that pressing j past the
// viewport edge scrolls the viewport so the cursor row stays inside
// [Top, Top+H).
func TestCommits_CursorStaysVisibleOnScrollDown(t *testing.T) {
	m := commitsManyFixture(t, 40)
	for i := 0; i < 20; i++ {
		m = pressKey(t, m, "j")
	}
	_ = m.commitsView()
	h := m.paneHeightCommits
	cur := m.state.CommitsCursor
	top := m.state.CommitsTop
	if cur != 20 {
		t.Fatalf("setup: cursor should be 20 after 20×j; got %d", cur)
	}
	if !(top <= cur && cur < top+h) {
		t.Errorf("cursor %d must be visible in [%d,%d); CommitsTop=%d", cur, top, top+h, top)
	}
}

// TestCommits_RenderShowsCursorRow pins the user-facing payoff: the cursor
// commit's message is actually present in the rendered (sliced) output.
func TestCommits_RenderShowsCursorRow(t *testing.T) {
	m := commitsManyFixture(t, 40)
	for i := 0; i < 25; i++ {
		m = pressKey(t, m, "j")
	}
	got := m.commitsView()
	// Cursor at 25 → commits[24] = "commit message 24".
	if !strings.Contains(got, "commit message 24") {
		t.Errorf("scrolled Commits view must show the cursor commit; got:\n%s", got)
	}
	// The first commit must have scrolled off-screen.
	if strings.Contains(got, "commit message 0 ") {
		t.Errorf("first commit must be scrolled off; got:\n%s", got)
	}
}

// TestCommits_GScrollsToBottom pins that G lands the cursor on the last row
// and the viewport scrolls so it's visible.
func TestCommits_GScrollsToBottom(t *testing.T) {
	m := commitsManyFixture(t, 40)
	m = pressKey(t, m, "G")
	_ = m.commitsView()
	cur := m.state.CommitsCursor
	h := m.paneHeightCommits
	top := m.state.CommitsTop
	if cur != 40 {
		t.Fatalf("G must move cursor to last row (40); got %d", cur)
	}
	if !(top <= cur && cur < top+h) {
		t.Errorf("after G cursor %d must be visible; CommitsTop=%d height=%d", cur, top, h)
	}
}

// TestCommits_ScrollUpKeepsCursorVisible pins the symmetric inverse — after
// scrolling down then k back up, the cursor stays visible.
func TestCommits_ScrollUpKeepsCursorVisible(t *testing.T) {
	m := commitsManyFixture(t, 40)
	for i := 0; i < 30; i++ {
		m = pressKey(t, m, "j")
	}
	_ = m.commitsView()
	for i := 0; i < 25; i++ {
		m = pressKey(t, m, "k")
	}
	_ = m.commitsView()
	cur := m.state.CommitsCursor
	h := m.paneHeightCommits
	top := m.state.CommitsTop
	if !(top <= cur && cur < top+h) {
		t.Errorf("after scroll up cursor %d must be visible; CommitsTop=%d height=%d", cur, top, h)
	}
}

// TestCommits_WheelScrolls pins that a mouse wheel over the Commits pane
// moves the cursor and the viewport follows.
func TestCommits_WheelScrolls(t *testing.T) {
	m := commitsManyFixture(t, 40)
	m.width = 120
	m.height = 30
	for i := 0; i < 20; i++ {
		m.mouseWheelCommits(1)
	}
	_ = m.commitsView()
	cur := m.state.CommitsCursor
	h := m.paneHeightCommits
	top := m.state.CommitsTop
	if cur != 20 {
		t.Fatalf("20 wheel-down ticks must move cursor to 20; got %d", cur)
	}
	if !(top <= cur && cur < top+h) {
		t.Errorf("wheel-scrolled cursor %d must be visible; CommitsTop=%d", cur, top)
	}
}

// TestFiles_CursorStaysVisibleOnScrollDown mirrors the Commits test for the
// Files pane.
func TestFiles_CursorStaysVisibleOnScrollDown(t *testing.T) {
	m := filesManyFixture(t, 40)
	for i := 0; i < 22; i++ {
		m = pressKey(t, m, "j")
	}
	got := m.filesView()
	h := m.paneHeightFiles
	cur := m.state.FilesCursor
	top := m.state.FilesTop
	if cur != 22 {
		t.Fatalf("setup: FilesCursor should be 22; got %d", cur)
	}
	if !(top <= cur && cur < top+h) {
		t.Errorf("cursor %d must be visible in [%d,%d); FilesTop=%d", cur, top, top+h, top)
	}
	// Cursor at 22 → files[21] = "src/file021.go".
	if !strings.Contains(got, "file021.go") {
		t.Errorf("scrolled Files view must show the cursor file; got:\n%s", got)
	}
}

// TestFiles_NoScrollWhenContentFits pins that a short file list never
// scrolls.
func TestFiles_NoScrollWhenContentFits(t *testing.T) {
	m := filesManyFixture(t, 3)
	m.state.FilesCursor = 3
	_ = m.filesView()
	if m.state.FilesTop != 0 {
		t.Errorf("short list must not scroll; FilesTop=%d want 0", m.state.FilesTop)
	}
}

// TestFiles_ClickUsesViewportOffset pins that a click on a scrolled Files
// column resolves to the absolute row, not the viewport-relative one.
func TestFiles_ClickUsesViewportOffset(t *testing.T) {
	m := filesManyFixture(t, 40)
	for i := 0; i < 22; i++ {
		m = pressKey(t, m, "j")
	}
	_ = m.filesView()
	top := m.state.FilesTop
	if top == 0 {
		t.Fatalf("setup: FilesTop should be > 0 after scrolling")
	}
	// Click content row 0 — must map to absolute row `top`, selecting the
	// file at that absolute index (top maps to files[top-1] when top>=1).
	m.mouseClickFiles(0)
	if m.state.FilesCursor != top {
		t.Errorf("click on visible row 0 must resolve to absolute row %d; got FilesCursor=%d", top, m.state.FilesCursor)
	}
}
