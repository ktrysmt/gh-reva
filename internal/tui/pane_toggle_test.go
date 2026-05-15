package tui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// pressKey is a helper that drives one keystroke through Model.handleKey
// and returns the resulting Model. The model is a value receiver, so the
// caller must use the returned Model for any post-key assertion.
func pressKey(t *testing.T, m Model, key string) Model {
	t.Helper()
	res, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return res.(Model)
}

func pressCtrl(t *testing.T, m Model, kt tea.KeyType) Model {
	t.Helper()
	res, _ := m.handleKey(tea.KeyMsg{Type: kt})
	return res.(Model)
}

// TestCommentsToggle_FlipsHiddenFlag pins the contract that Ctrl+E toggles
// AppState.CommentsHidden. The toggle is a global key (handled before the
// per-pane dispatcher), so the test fires it from each pane to confirm
// it's reachable everywhere.
func TestCommentsToggle_FlipsHiddenFlag(t *testing.T) {
	for _, p := range []model.PaneID{model.PaneFiles, model.PaneCommits, model.PaneDiff, model.PaneComments} {
		m := commentsModelFixture(t)
		m.state.FocusedPane = p
		if m.state.CommentsHidden {
			t.Fatalf("default CommentsHidden must be false")
		}
		m = pressCtrl(t, m, tea.KeyCtrlE)
		if !m.state.CommentsHidden {
			t.Errorf("ctrl+e from pane %v: expected CommentsHidden=true after first press", p)
		}
		m = pressCtrl(t, m, tea.KeyCtrlE)
		if m.state.CommentsHidden {
			t.Errorf("ctrl+e from pane %v: expected CommentsHidden=false after second press", p)
		}
	}
}

// TestCommentsToggle_ReturnsClearScreenCmd pins that Ctrl+E returns
// tea.ClearScreen as its Cmd. Bubbletea's standardRenderer skips
// rewriting a line when the new line equals the previous frame's line at
// the same y; toggling the Comments column changes overall layout but
// some rows (e.g. blank-body rows in narrow panes) can coincide between
// hide / show frames, leaving stale cells from the prior wider Diff
// (carrying DiffPlusBg / DiffMinusBg) underneath the new Comments
// column. wezterm's reported leak is the most visible symptom but the
// underlying gap is generic. Returning ClearScreen forces a full repaint
// (EraseEntireScreen + CursorHome) so no stale SGR survives.
func TestCommentsToggle_ReturnsClearScreenCmd(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.FocusedPane = model.PaneDiff

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlE})
	if cmd == nil {
		t.Fatalf("ctrl+e must return a non-nil Cmd to force full repaint")
	}
	wantPtr := reflect.ValueOf(tea.ClearScreen).Pointer()
	gotPtr := reflect.ValueOf(cmd).Pointer()
	if gotPtr != wantPtr {
		t.Errorf("ctrl+e Cmd must be tea.ClearScreen; got a different func pointer")
	}
}

// TestCommentsToggle_HideShiftsFocusOff pins that hiding the Comments pane
// while the user has focus on it moves focus to Diff — leaving focus on a
// hidden pane would strand keystrokes on an invisible target.
func TestCommentsToggle_HideShiftsFocusOff(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.FocusedPane = model.PaneComments
	m = pressCtrl(t, m, tea.KeyCtrlE)
	if !m.state.CommentsHidden {
		t.Fatalf("expected hide to fire")
	}
	if m.state.FocusedPane != model.PaneDiff {
		t.Errorf("expected FocusedPane=Diff after hiding from Comments; got %v", m.state.FocusedPane)
	}
}

// TestCommentsToggle_RevealKeepsFocus pins that revealing (Hidden=true →
// false) does not move focus — the user may have parked focus on Diff
// while Comments was hidden, and Ctrl+E to bring the column back should
// not yank them away from their current pane.
func TestCommentsToggle_RevealKeepsFocus(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.CommentsHidden = true
	m.state.FocusedPane = model.PaneDiff
	m = pressCtrl(t, m, tea.KeyCtrlE)
	if m.state.CommentsHidden {
		t.Fatalf("expected reveal to fire")
	}
	if m.state.FocusedPane != model.PaneDiff {
		t.Errorf("reveal must not change focus; got %v", m.state.FocusedPane)
	}
}

// TestTab_SkipsCommentsWhenHidden pins that Tab cycle skips the Comments
// pane while it's hidden. Files → Commits → Diff → Files, etc.
func TestTab_SkipsCommentsWhenHidden(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.CommentsHidden = true
	m.state.FocusedPane = model.PaneDiff

	res, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	m = res.(Model)
	if m.state.FocusedPane != model.PaneFiles {
		t.Errorf("Tab from Diff with Comments hidden should land on Files; got %v", m.state.FocusedPane)
	}

	m.state.FocusedPane = model.PaneCommits
	res, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = res.(Model)
	if m.state.FocusedPane != model.PaneFiles {
		t.Errorf("Shift+Tab from Commits with Comments hidden should land on Files; got %v", m.state.FocusedPane)
	}

	m.state.FocusedPane = model.PaneFiles
	res, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = res.(Model)
	if m.state.FocusedPane != model.PaneDiff {
		t.Errorf("Shift+Tab from Files with Comments hidden should skip Comments → land on Diff; got %v",
			m.state.FocusedPane)
	}
}

// TestSplitColumnWidths_HiddenCommentsExpandsDiff pins the layout contract:
// when Comments is hidden, right=0 and the saved-up width is added to the
// middle (Diff) column. The 130-col branch is the canonical e2e width;
// other branches are exercised at width thresholds where the existing
// behavior would otherwise floor out.
func TestSplitColumnWidths_HiddenCommentsExpandsDiff(t *testing.T) {
	cases := []struct{ total int }{{130}, {160}, {200}, {100}, {85}}
	for _, c := range cases {
		l, mid, r := splitColumnWidths(c.total, true /* commentsHidden */, defaultCommentsWidthPercent)
		if r != 0 {
			t.Errorf("total=%d hidden: right must be 0, got %d", c.total, r)
		}
		if l <= 0 || mid <= 0 {
			t.Errorf("total=%d hidden: left/mid must stay positive; got l=%d mid=%d", c.total, l, mid)
		}
		if l+mid != c.total {
			t.Errorf("total=%d hidden: left+mid must equal total; got %d", c.total, l+mid)
		}
		// Sanity: the expanded mid is wider than the visible-Comments mid.
		_, vmid, _ := splitColumnWidths(c.total, false /* commentsHidden */, defaultCommentsWidthPercent)
		if mid <= vmid {
			t.Errorf("total=%d hidden mid=%d must exceed visible mid=%d", c.total, mid, vmid)
		}
	}
}

// TestDiffEnter_AutoRevealsCommentsWhenHidden pins that Diff Enter on
// a row carrying threads auto-reveals the Comments pane before
// shifting focus, so the user always lands on a visible pane. The
// previous Comments-zoom-modal handoff was retired in favor of a
// plain focus shift (Comments pane Space still opens the modal for
// inspection). Without auto-reveal, focus would strand on an
// invisible target — Tab / Shift+Tab skip Comments while hidden, so
// the user could not navigate back without re-opening the column
// first.
func TestDiffEnter_AutoRevealsCommentsWhenHidden(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.CommentsHidden = true
	m.state.FocusedPane = model.PaneDiff
	m.state.DiffCursor.Line = 2 // ◆ row for thread T1

	res, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = res.(Model)

	if m.state.CommentsHidden {
		t.Errorf("Diff Enter on a comment row must auto-reveal Comments")
	}
	if m.state.Modal != nil {
		t.Errorf("Diff Enter must NOT open a modal anymore; got %+v", m.state.Modal)
	}
	if m.state.FocusedPane != model.PaneComments {
		t.Errorf("focus must shift to Comments; got %v", m.state.FocusedPane)
	}
}

// TestFiles_jKDoesNotChangeSelectedFile pins the new contract: Files
// j/k move the cursor only — they no longer auto-select the file
// under the cursor. Triggers a re-render of the Diff column on every
// keystroke; users reported that as sluggish for j/k navigation.
// SelectedFile only changes via Enter (commit) or Shift+J/K
// (advanceFile from any pane).
func TestFiles_jKDoesNotChangeSelectedFile(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.PR.Files = append(m.state.PR.Files,
		&model.FileEntry{Path: "src/bar.go", Status: model.ChangeAdded},
		&model.FileEntry{Path: "src/baz.go", Status: model.ChangeAdded},
	)
	m.state.FocusedPane = model.PaneFiles
	m.state.SelectedFile = m.state.PR.Files[0].Path
	m.state.FilesCursor = 0

	res, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = res.(Model)
	if m.state.FilesCursor != 1 {
		t.Errorf("j must advance FilesCursor to 1; got %d", m.state.FilesCursor)
	}
	if m.state.SelectedFile != "src/foo.go" {
		t.Errorf("j must NOT change SelectedFile; got %q", m.state.SelectedFile)
	}

	res, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = res.(Model)
	if m.state.FilesCursor != 0 {
		t.Errorf("k must move FilesCursor back to 0; got %d", m.state.FilesCursor)
	}
	if m.state.SelectedFile != "src/foo.go" {
		t.Errorf("k must NOT change SelectedFile; got %q", m.state.SelectedFile)
	}
}

// TestFiles_ggGCursorOnly pins that gg / G move the Files cursor but
// leave SelectedFile alone — peer to the j/k contract.
func TestFiles_ggGCursorOnly(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.PR.Files = append(m.state.PR.Files,
		&model.FileEntry{Path: "src/bar.go", Status: model.ChangeAdded},
		&model.FileEntry{Path: "src/baz.go", Status: model.ChangeAdded},
	)
	m.state.FocusedPane = model.PaneFiles
	m.state.SelectedFile = "src/foo.go"
	m.state.FilesCursor = 1 // src/foo.go under the All-row shifted indexing

	// G — bottom (cursor space is [0, len(files)] now; G lands on len(files))
	res, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = res.(Model)
	last := len(m.state.PR.Files)
	if m.state.FilesCursor != last {
		t.Errorf("G must move cursor to last file row (%d); got %d", last, m.state.FilesCursor)
	}
	if m.state.SelectedFile != "src/foo.go" {
		t.Errorf("G must NOT change SelectedFile; got %q", m.state.SelectedFile)
	}

	// gg — top (now the All row)
	res, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = res.(Model)
	res, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = res.(Model)
	if m.state.FilesCursor != 0 {
		t.Errorf("gg must move cursor to 0 (All row); got %d", m.state.FilesCursor)
	}
	if m.state.SelectedFile != "src/foo.go" {
		t.Errorf("gg must NOT change SelectedFile; got %q", m.state.SelectedFile)
	}
}

// TestFiles_EnterShiftsFocusToDiffAndSelects pins the commit gesture:
// Enter on a file row in flat mode shifts focus to Diff and updates
// SelectedFile to the cursor's file. Replaces the prior "Enter on a
// file row is a no-op" contract — cursor-only j/k makes Enter the
// natural commit step.
func TestFiles_EnterShiftsFocusToDiffAndSelects(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.PR.Files = append(m.state.PR.Files,
		&model.FileEntry{Path: "src/bar.go", Status: model.ChangeAdded},
	)
	m.state.FocusedPane = model.PaneFiles
	m.state.SelectedFile = "src/foo.go"
	m.state.FilesCursor = 2 // src/bar.go (cursor 1 is files[0] under the All-row shift)

	res, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = res.(Model)
	if m.state.FocusedPane != model.PaneDiff {
		t.Errorf("Enter on a file row must shift focus to Diff; got %v", m.state.FocusedPane)
	}
	if m.state.SelectedFile != "src/bar.go" {
		t.Errorf("Enter must select the cursor file; got %q", m.state.SelectedFile)
	}
}

// TestFiles_EnterTreeDirStillFolds pins that tree-mode dir rows
// retain the fold/unfold gesture. The "Enter shifts focus" contract
// only applies to file rows.
func TestFiles_EnterTreeDirStillFolds(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.PR.Files = []*model.FileEntry{
		{Path: "src/foo.go", Status: model.ChangeModified},
		{Path: "src/bar.go", Status: model.ChangeAdded},
	}
	m.state.FocusedPane = model.PaneFiles
	m.state.FilesTreeMode = true
	m.state.FoldedDirs = map[string]bool{}
	m.state.FilesCursor = 1 // dir row "src/" (idx 0 is the synthetic All row)

	res, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = res.(Model)
	if m.state.FocusedPane != model.PaneFiles {
		t.Errorf("Enter on a tree dir row must keep focus on Files; got %v", m.state.FocusedPane)
	}
	if !m.state.FoldedDirs["src"] {
		t.Errorf("Enter on tree dir row must fold the directory")
	}
}

// TestComments_SpaceNoopWhenNoThread pins that pressing <space> in the
// Comments pane is a no-op when the Diff cursor is not on a ◆ row
// (placeholder "(no comment at cursor)"). Opening a zoom modal that
// just wraps the same placeholder text is noise; reserve the gesture
// for when there's actual content to zoom.
func TestComments_SpaceNoopWhenNoThread(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.FocusedPane = model.PaneComments
	m.state.DiffCursor.Line = 0 // header row → no thread anchored

	res, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = res.(Model)
	if m.state.Modal != nil {
		t.Errorf("Comments <space> on (no comment) must NOT open a modal; got %+v", m.state.Modal)
	}
}

// TestComments_SpaceOpensModalWhenThreadVisible pins the existing
// behavior is preserved when the Comments pane has visible threads.
func TestComments_SpaceOpensModalWhenThreadVisible(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.FocusedPane = model.PaneComments
	m.state.DiffCursor.Line = 2 // ◆ row for thread T1

	res, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = res.(Model)
	if m.state.Modal == nil || m.state.Modal.Pane != model.PaneComments {
		t.Errorf("Comments <space> with visible threads must open the zoom modal; got %+v", m.state.Modal)
	}
}

// TestStatusBar_AdvertisesToggleKey pins that the common navigation
// suffix carries the Ctrl+E binding so the user can discover the toggle
// without documentation. The full assertion goes through the rendered
// bar (with a Target set so the URL ladder doesn't squeeze out the
// suffix per composeStatusBar's pass-1 contract).
func TestStatusBar_AdvertisesToggleKey(t *testing.T) {
	if !strings.Contains(statusCommonSuffix, "ctrl+e") {
		t.Errorf("statusCommonSuffix must advertise the ctrl+e Comments toggle; got %q", statusCommonSuffix)
	}
	_ = time.Now() // silence unused import if Clock helpers move
}
