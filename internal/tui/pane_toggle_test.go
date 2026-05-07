package tui

import (
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
		l, mid, r := splitColumnWidths(c.total, true /* commentsHidden */)
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
		_, vmid, _ := splitColumnWidths(c.total, false /* commentsHidden */)
		if mid <= vmid {
			t.Errorf("total=%d hidden mid=%d must exceed visible mid=%d", c.total, mid, vmid)
		}
	}
}

// TestDiffEnter_AutoRevealsCommentsWhenHidden pins that Diff Enter on a
// row that carries threads auto-reveals the Comments pane before opening
// the modal. Without auto-reveal the user would see a modal anchored to
// a hidden pane on close — confusing and inconsistent with the rest of
// the modal-close-restores-focus contract.
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
	if m.state.Modal == nil || m.state.Modal.Pane != model.PaneComments {
		t.Errorf("expected Comments zoom modal after Diff Enter handoff; got %+v", m.state.Modal)
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
