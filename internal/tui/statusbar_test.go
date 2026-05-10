package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// TestDiffHint_ShowsSideTagAndHLBinding pins that the Diff status hint
// surfaces the cursor's column ([A] / [B]) and the new h/l keybinding
// alongside the existing keymap. The user-visible "which lane am I in"
// signal is the most-asked question after the per-column UX landed —
// the bar must answer it without the user opening Help.
func TestDiffHint_ShowsSideTagAndHLBinding(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	m.state.PR = &model.PR{Owner: "o", Repo: "r", Number: 1}
	m.state.FocusedPane = model.PaneDiff

	m.state.DiffCursor.Side = model.DiffSideRight
	got := m.diffHint()
	if !strings.HasPrefix(got, "[A]") {
		t.Errorf("RIGHT cursor must show `[A]` prefix; got %q", got)
	}
	if !strings.Contains(got, "h/l:side") {
		t.Errorf("Diff hint must surface h/l binding; got %q", got)
	}

	m.state.DiffCursor.Side = model.DiffSideLeft
	got = m.diffHint()
	if !strings.HasPrefix(got, "[B]") {
		t.Errorf("LEFT cursor must show `[B]` prefix; got %q", got)
	}
}
