package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// mouseModelFixture extends commentsModelFixture with a wider 140x40 layout
// and three files / two commits so cursor-row click and Commits auto-select
// have something to bite on. View() is invoked once so paneWidth* /
// paneHeight* are populated — paneAt and the per-pane mouse handlers all
// assume a measured frame.
//
// Layout under m.width=140, m.height=40, statusBarRows=2:
//
//	Files     outer x=[0,42)   y=[0,19)   inner content y=[3,18)
//	Commits   outer x=[0,42)   y=[19,38)  inner content y=[22,37)
//	Diff      outer x=[42,83)  y=[0,38)   inner content y=[3,37)
//	Comments  outer x=[83,140) y=[0,38)   inner content y=[3,37)
//	StatusBar y=[38,40)
func mouseModelFixture(t *testing.T) Model {
	t.Helper()
	m := commentsModelFixture(t)
	m.state.PR.Files = append(m.state.PR.Files,
		&model.FileEntry{Path: "src/bar.go", Status: model.ChangeAdded},
		&model.FileEntry{Path: "src/baz.go", Status: model.ChangeModified},
	)
	m.state.PR.Commits = []*model.Commit{
		{SHA: "1111111111111111111111111111111111111111", ShortSHA: "1111111", Message: "first commit",
			ChangedFiles: map[string]model.ChangeKind{"src/foo.go": model.ChangeModified}},
		{SHA: "2222222222222222222222222222222222222222", ShortSHA: "2222222", Message: "second commit",
			ChangedFiles: map[string]model.ChangeKind{"src/foo.go": model.ChangeModified}},
	}
	m.width = 140
	m.height = 40
	m.measureLayout()
	return m
}

func mouseMsg(x, y int, button tea.MouseButton) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: button}
}

func leftClick(x, y int) tea.MouseMsg { return mouseMsg(x, y, tea.MouseButtonLeft) }
func wheelUp(x, y int) tea.MouseMsg   { return mouseMsg(x, y, tea.MouseButtonWheelUp) }
func wheelDown(x, y int) tea.MouseMsg { return mouseMsg(x, y, tea.MouseButtonWheelDown) }

func TestPaneAt_Files_ContentRow(t *testing.T) {
	m := mouseModelFixture(t)
	hit, ok := m.paneAt(10, 4) // y=4 → content row 4-3=1
	if !ok {
		t.Fatalf("expected hit inside Files content")
	}
	if hit.Pane != model.PaneFiles {
		t.Errorf("pane=%v, want Files", hit.Pane)
	}
	if hit.OnTitle {
		t.Errorf("OnTitle=true; expected content row")
	}
	if hit.ContentRow != 1 {
		t.Errorf("ContentRow=%d, want 1", hit.ContentRow)
	}
}

func TestPaneAt_Files_TitleRow(t *testing.T) {
	m := mouseModelFixture(t)
	hit, ok := m.paneAt(5, 1)
	if !ok {
		t.Fatalf("expected hit on Files title row")
	}
	if hit.Pane != model.PaneFiles || !hit.OnTitle {
		t.Errorf("hit=%+v, want Files title", hit)
	}
}

func TestPaneAt_Files_BorderRowsRejected(t *testing.T) {
	m := mouseModelFixture(t)
	for _, y := range []int{0, 2, 18} { // top border / divider / bottom border
		if _, ok := m.paneAt(5, y); ok {
			t.Errorf("y=%d: expected ok=false for border/divider", y)
		}
	}
}

func TestPaneAt_Files_SideBarRejected(t *testing.T) {
	m := mouseModelFixture(t)
	if _, ok := m.paneAt(0, 4); ok { // left side bar │
		t.Errorf("x=0 (left side bar): expected ok=false")
	}
	if _, ok := m.paneAt(41, 4); ok { // right side bar │
		t.Errorf("x=41 (right side bar): expected ok=false")
	}
}

func TestPaneAt_Commits_ContentRow(t *testing.T) {
	m := mouseModelFixture(t)
	hit, ok := m.paneAt(5, 23) // Commits top=19 → content y=22 base; y=23 → row 1
	if !ok || hit.Pane != model.PaneCommits || hit.OnTitle || hit.ContentRow != 1 {
		t.Errorf("hit=%+v ok=%v, want Commits content row 1", hit, ok)
	}
}

func TestPaneAt_Diff_ContentRow(t *testing.T) {
	m := mouseModelFixture(t)
	hit, ok := m.paneAt(60, 5) // Diff x=[42,83); y=5 → content row 2
	if !ok || hit.Pane != model.PaneDiff || hit.OnTitle || hit.ContentRow != 2 {
		t.Errorf("hit=%+v ok=%v, want Diff content row 2", hit, ok)
	}
}

func TestPaneAt_Comments_ContentRow(t *testing.T) {
	m := mouseModelFixture(t)
	hit, ok := m.paneAt(100, 6) // Comments x=[83,140); y=6 → content row 3
	if !ok || hit.Pane != model.PaneComments || hit.OnTitle || hit.ContentRow != 3 {
		t.Errorf("hit=%+v ok=%v, want Comments content row 3", hit, ok)
	}
}

func TestPaneAt_StatusBarArea_Rejected(t *testing.T) {
	m := mouseModelFixture(t)
	for _, y := range []int{38, 39} {
		if _, ok := m.paneAt(50, y); ok {
			t.Errorf("y=%d (status bar): expected ok=false", y)
		}
	}
}

func TestPaneAt_HiddenComments_Rejected(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.CommentsHidden = true
	m.measureLayout()
	// With Comments hidden, x=100 falls inside the expanded Diff column,
	// not Comments. Verify: hit must be Diff or, if x is past total width,
	// rejected.
	hit, ok := m.paneAt(100, 6)
	if !ok {
		t.Fatalf("x=100 with Comments hidden should land in Diff")
	}
	if hit.Pane != model.PaneDiff {
		t.Errorf("Comments hidden: x=100 must be Diff; got %v", hit.Pane)
	}
}

func TestPaneAt_StackedFallback_Rejected(t *testing.T) {
	m := mouseModelFixture(t)
	m.width = 0 // pre-WindowSize fallback
	m.height = 40
	if _, ok := m.paneAt(5, 5); ok {
		t.Errorf("stacked fallback (width=0): expected ok=false")
	}
}

func TestMouse_ClickFilesContent_FocusesAndCommitsFile(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.FocusedPane = model.PaneDiff
	m.state.FilesCursor = 0
	res, _ := m.handleMouse(leftClick(5, 5)) // Files content row 2 → src/baz.go
	m = res.(Model)
	if m.state.FocusedPane != model.PaneFiles {
		t.Errorf("FocusedPane=%v, want Files", m.state.FocusedPane)
	}
	if m.state.FilesCursor != 2 {
		t.Errorf("FilesCursor=%d, want 2", m.state.FilesCursor)
	}
	// A click is a deliberate one-shot gesture (unlike j/k repeat — #19),
	// so the Diff column must follow: SelectedFile updates to the row.
	if m.state.SelectedFile != "src/baz.go" {
		t.Errorf("SelectedFile=%q, want src/baz.go (click commits)", m.state.SelectedFile)
	}
}

func TestMouse_ClickFilesTreeDirRow_FoldsToggle(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.FilesTreeMode = true
	rows := m.filesTreeRows()
	// Find the first directory row.
	var dirIdx = -1
	for i, r := range rows {
		if r.Kind == model.FilesRowDir {
			dirIdx = i
			break
		}
	}
	if dirIdx < 0 {
		t.Skip("fixture has no directory row in tree mode")
	}
	dirPath := rows[dirIdx].Path
	if m.state.FoldedDirs[dirPath] {
		t.Fatalf("pre: dir %q already folded", dirPath)
	}
	prevSelected := m.state.SelectedFile
	res, _ := m.handleMouse(leftClick(5, 3+dirIdx))
	m = res.(Model)
	if !m.state.FoldedDirs[dirPath] {
		t.Errorf("click on dir row must fold it; got FoldedDirs[%q]=%v", dirPath, m.state.FoldedDirs[dirPath])
	}
	if m.state.SelectedFile != prevSelected {
		t.Errorf("dir click must NOT change SelectedFile; got %q want %q", m.state.SelectedFile, prevSelected)
	}
}

func TestMouse_ClickFilesTitle_FocusesOnly(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.FocusedPane = model.PaneDiff
	m.state.FilesCursor = 1
	res, _ := m.handleMouse(leftClick(5, 1)) // title row
	m = res.(Model)
	if m.state.FocusedPane != model.PaneFiles {
		t.Errorf("FocusedPane=%v, want Files (title click focuses)", m.state.FocusedPane)
	}
	if m.state.FilesCursor != 1 {
		t.Errorf("FilesCursor changed via title click: got %d, want 1", m.state.FilesCursor)
	}
}

func TestMouse_ClickCommitsContent_AutoSelects(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.FocusedPane = model.PaneFiles
	// Content row 0 is the synthetic "All commits" row; row 1 picks commit[0].
	res, _ := m.handleMouse(leftClick(5, 23))
	m = res.(Model)
	if m.state.FocusedPane != model.PaneCommits {
		t.Errorf("FocusedPane=%v, want Commits", m.state.FocusedPane)
	}
	if m.state.CommitsCursor != 1 {
		t.Errorf("CommitsCursor=%d, want 1", m.state.CommitsCursor)
	}
	if m.state.SelectedRange.Kind != model.RangeSingleCommit ||
		m.state.SelectedRange.SHA != m.state.PR.Commits[0].SHA {
		t.Errorf("expected SelectedRange to follow click; got %+v", m.state.SelectedRange)
	}
}

func TestMouse_ClickDiffContent_MovesCursor(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.FocusedPane = model.PaneFiles
	m.state.DiffViewport.Top = 0
	// Under m.width=140 the Diff pane gets paneWidthDiff=39, so split
	// halfW = (39-21)/2 = 9. The hunk header (`@@ -1,3 +1,5 @@`,
	// 16 chars) wraps to 2 display rows on both sides, and `+addedLine*`
	// (11 chars) wraps to 2 rows on its RIGHT side. Buffer / display
	// mapping under this layout:
	//   display 0..1 → buffer 0 (hunk header, 2 wraps)
	//   display 2    → buffer 1 (` line1`, both sides 1 row)
	//   display 3..4 → buffer 2 (`+addedLine2`, RIGHT wraps to 2 rows)
	//   display 5..6 → buffer 3 (`+addedLine3`)
	//   display 7    → buffer 4 (` line4`)
	// Pick content row 7 (y=3+7=10) and assert buffer line 4 to lock the
	// wrap-aware path explicitly.
	res, _ := m.handleMouse(leftClick(60, 10))
	m = res.(Model)
	if m.state.FocusedPane != model.PaneDiff {
		t.Errorf("FocusedPane=%v, want Diff", m.state.FocusedPane)
	}
	if m.state.DiffCursor.Line != 4 {
		t.Errorf("DiffCursor.Line=%d, want 4 (wrap-aware mapping)", m.state.DiffCursor.Line)
	}
}

// TestMouse_ClickDiffLeftCell_SetsLeftSide pins that a click landing in
// the LEFT half of the split-mode Diff pane parks DiffCursor.Side on
// LEFT. Inner col 17 is inside the leftCell area (oldLn=cols 4..7,
// space=8, leftCell starts at col 9, divider at col 10+halfW=19).
func TestMouse_ClickDiffLeftCell_SetsLeftSide(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.DiffViewport.Top = 0
	m.state.DiffCursor.Side = model.DiffSideRight

	// x=60 → relX=18 → ContentCol=17, well inside LEFT half.
	res, _ := m.handleMouse(leftClick(60, 5))
	m = res.(Model)
	if m.state.DiffCursor.Side != model.DiffSideLeft {
		t.Errorf("click in LEFT half must set Side=LEFT; got %q", m.state.DiffCursor.Side)
	}
}

// TestMouse_ClickDiffRightCell_SetsRightSide pins the symmetric case.
// x=80 lands well past the inner divider, in the RIGHT half.
func TestMouse_ClickDiffRightCell_SetsRightSide(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.DiffViewport.Top = 0
	m.state.DiffCursor.Side = model.DiffSideLeft

	res, _ := m.handleMouse(leftClick(80, 5))
	m = res.(Model)
	if m.state.DiffCursor.Side != model.DiffSideRight {
		t.Errorf("click in RIGHT half must set Side=RIGHT; got %q", m.state.DiffCursor.Side)
	}
}

// TestMouse_ClickDiffContent_NoWrap_OneToOne pins the simpler case: when
// the column is wide enough that no buffer line wraps, content-row N
// resolves directly to buffer line viewport.Top+N.
func TestMouse_ClickDiffContent_NoWrap_OneToOne(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.DiffViewport.Top = 0
	// Force unified mode and a wide pane so no wrapping kicks in.
	m.state.DiffViewMode = model.DiffViewUnified
	m.paneWidthDiff = 200
	res, _ := m.handleMouse(leftClick(60, 6)) // content row 3
	m = res.(Model)
	if m.state.DiffCursor.Line != 3 {
		t.Errorf("no-wrap mapping: DiffCursor.Line=%d, want 3", m.state.DiffCursor.Line)
	}
}

func TestMouse_ClickCommentsContent_MovesCursor(t *testing.T) {
	m := mouseModelFixture(t)
	// Park Diff cursor on a ◆ row so Comments has a thread to render.
	m.state.DiffCursor.Line = 2
	m.state.FocusedPane = model.PaneFiles
	// First Comments content row carries the thread root header → cursor 0.
	res, _ := m.handleMouse(leftClick(100, 3))
	m = res.(Model)
	if m.state.FocusedPane != model.PaneComments {
		t.Errorf("FocusedPane=%v, want Comments", m.state.FocusedPane)
	}
	if m.state.CommentsCursor != 0 {
		t.Errorf("CommentsCursor=%d, want 0", m.state.CommentsCursor)
	}
}

func TestMouse_WheelDownDiff_AdvancesCursor(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.FocusedPane = model.PaneFiles // wheel must not change focus
	before := m.state.DiffCursor.Line
	res, _ := m.handleMouse(wheelDown(60, 10))
	m = res.(Model)
	if m.state.FocusedPane != model.PaneFiles {
		t.Errorf("wheel changed focus: %v", m.state.FocusedPane)
	}
	if m.state.DiffCursor.Line != before+1 {
		t.Errorf("wheel down: DiffCursor.Line=%d, want %d", m.state.DiffCursor.Line, before+1)
	}
}

func TestMouse_WheelUpFiles_DecrementsCursor(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.FilesCursor = 2
	m.state.FocusedPane = model.PaneDiff
	res, _ := m.handleMouse(wheelUp(5, 5))
	m = res.(Model)
	if m.state.FocusedPane != model.PaneDiff {
		t.Errorf("wheel changed focus: %v", m.state.FocusedPane)
	}
	if m.state.FilesCursor != 1 {
		t.Errorf("FilesCursor=%d, want 1", m.state.FilesCursor)
	}
	if m.state.SelectedFile != "src/foo.go" {
		t.Errorf("Files wheel must not auto-select; got %q", m.state.SelectedFile)
	}
}

func TestMouse_WheelDownCommits_AdvancesAndAutoSelects(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.CommitsCursor = 0
	m.state.FocusedPane = model.PaneDiff
	res, _ := m.handleMouse(wheelDown(5, 23))
	m = res.(Model)
	if m.state.CommitsCursor != 1 {
		t.Errorf("CommitsCursor=%d, want 1", m.state.CommitsCursor)
	}
	if m.state.SelectedRange.Kind != model.RangeSingleCommit {
		t.Errorf("Commits wheel must auto-select; got %+v", m.state.SelectedRange)
	}
}

func TestMouse_AbsorbedDuringHelp(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.HelpOpen = true
	m.state.FocusedPane = model.PaneFiles
	m.state.FilesCursor = 0
	res, _ := m.handleMouse(leftClick(60, 5)) // would otherwise focus Diff
	m = res.(Model)
	if m.state.FocusedPane != model.PaneFiles {
		t.Errorf("Help open: mouse must not change focus; got %v", m.state.FocusedPane)
	}
}

func TestMouse_AbsorbedDuringCompose(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.Compose = &model.ComposeState{Status: model.ComposeEditing, Body: "wip"}
	m.state.FocusedPane = model.PaneFiles
	res, _ := m.handleMouse(leftClick(60, 5))
	m = res.(Model)
	if m.state.FocusedPane != model.PaneFiles {
		t.Errorf("Compose active: mouse must not change focus; got %v", m.state.FocusedPane)
	}
}

func TestMouse_AbsorbedDuringPendingConfirm(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.PendingConfirm = &model.PendingConfirm{}
	m.state.FocusedPane = model.PaneFiles
	res, _ := m.handleMouse(leftClick(60, 5))
	m = res.(Model)
	if m.state.FocusedPane != model.PaneFiles {
		t.Errorf("PendingConfirm active: mouse must not change focus")
	}
}

func TestMouse_AbsorbedDuringModal(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.Modal = &model.ModalState{Pane: model.PaneFiles, Origin: model.PaneFiles}
	m.state.FocusedPane = model.PaneFiles
	res, _ := m.handleMouse(leftClick(60, 5))
	m = res.(Model)
	if m.state.FocusedPane != model.PaneFiles {
		t.Errorf("Modal open: mouse must not change focus")
	}
}

func TestMouse_AbsorbedDuringSearchEditing(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.Search = &model.SearchState{Status: model.SearchEditing, Query: "foo"}
	m.state.FocusedPane = model.PaneFiles
	res, _ := m.handleMouse(leftClick(60, 5))
	m = res.(Model)
	if m.state.FocusedPane != model.PaneFiles {
		t.Errorf("Search editing: mouse must not change focus")
	}
}

func TestMouse_LoadingPhase_Rejected(t *testing.T) {
	m := mouseModelFixture(t)
	m.state.PR = nil // back to loading
	res, _ := m.handleMouse(leftClick(60, 5))
	if res == nil {
		t.Fatalf("handleMouse returned nil model")
	}
}
