package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// sideTestModel builds a Model with a fixture patch carrying a mix of
// `-`, `+`, and context lines so j/k auto-skip / h/l switching has
// distinguishable rows to land on.
//
// Buffer layout:
//
//	0: @@ -1,3 +1,4 @@
//	1:  ctx1                 (LEFT + RIGHT)
//	2: -removed_a            (LEFT only)
//	3: -removed_b            (LEFT only)
//	4: +added_x              (RIGHT only)
//	5: +added_y              (RIGHT only)
//	6:  ctx2                 (LEFT + RIGHT)
func sideTestModel(t *testing.T) Model {
	t.Helper()
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
		"-removed_b",
		"+added_x",
		"+added_y",
		" ctx2",
	}, "\n")
	m.paneWidthDiff = 60
	m.state.FocusedPane = model.PaneDiff
	return m
}

// pressDiffKey drives a Diff-pane keystroke through handleKey (so the
// pre-dispatch hooks like Notice clearing and h/l routing fire) and
// returns the resulting Model. Unlike the package-shared pressKey it
// also asserts FocusedPane=Diff so cross-pane regressions surface.
func pressDiffKey(t *testing.T, m Model, key string) Model {
	t.Helper()
	if m.state.FocusedPane != model.PaneDiff {
		t.Fatalf("pressDiffKey: FocusedPane must be Diff, got %v", m.state.FocusedPane)
	}
	out, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return out.(Model)
}

func TestDiffJ_RightSideSkipsMinusLines(t *testing.T) {
	m := sideTestModel(t)
	m.state.DiffCursor.Line = 1 // ctx1 (exists on both)
	m.state.DiffCursor.Side = model.DiffSideRight

	// j once: should skip the `-` rows (indices 2, 3) and land on
	// `+added_x` at index 4.
	m = pressDiffKey(t, m, "j")
	if m.state.DiffCursor.Line != 4 {
		t.Errorf("j on RIGHT must skip `-` rows; cursor=%d, want 4", m.state.DiffCursor.Line)
	}
}

func TestDiffJ_LeftSideSkipsPlusLines(t *testing.T) {
	m := sideTestModel(t)
	m.state.DiffCursor.Line = 3 // last `-` row
	m.state.DiffCursor.Side = model.DiffSideLeft

	// j: skip indices 4, 5 (`+`) and land on ctx2 at index 6.
	m = pressDiffKey(t, m, "j")
	if m.state.DiffCursor.Line != 6 {
		t.Errorf("j on LEFT must skip `+` rows; cursor=%d, want 6", m.state.DiffCursor.Line)
	}
}

func TestDiffK_RightSideSkipsMinusLines(t *testing.T) {
	m := sideTestModel(t)
	m.state.DiffCursor.Line = 4 // `+added_x`
	m.state.DiffCursor.Side = model.DiffSideRight

	m = pressDiffKey(t, m, "k")
	if m.state.DiffCursor.Line != 1 {
		t.Errorf("k on RIGHT must skip `-` rows back to ctx1; cursor=%d, want 1", m.state.DiffCursor.Line)
	}
}

func TestDiffJ_StaysWhenNoFurtherSideRow(t *testing.T) {
	m := sideTestModel(t)
	m.state.DiffCursor.Line = 6 // ctx2
	m.state.DiffCursor.Side = model.DiffSideRight

	// No more rows below; cursor must stay at 6 (existing j-at-end behavior).
	m = pressDiffKey(t, m, "j")
	if m.state.DiffCursor.Line != 6 {
		t.Errorf("j at last side-row must be a no-op; cursor=%d, want 6", m.state.DiffCursor.Line)
	}
}

func TestDiffL_SwitchesToRight(t *testing.T) {
	m := sideTestModel(t)
	m.state.DiffCursor.Line = 1
	m.state.DiffCursor.Side = model.DiffSideLeft

	m = pressDiffKey(t, m, "l")
	if m.state.DiffCursor.Side != model.DiffSideRight {
		t.Errorf("l must switch to RIGHT; got %q", m.state.DiffCursor.Side)
	}
	if m.state.DiffCursor.Line != 1 {
		t.Errorf("l on a both-sides row must keep the line; got %d, want 1", m.state.DiffCursor.Line)
	}
}

func TestDiffH_SwitchesToLeft(t *testing.T) {
	m := sideTestModel(t)
	m.state.DiffCursor.Line = 1
	m.state.DiffCursor.Side = model.DiffSideRight

	m = pressDiffKey(t, m, "h")
	if m.state.DiffCursor.Side != model.DiffSideLeft {
		t.Errorf("h must switch to LEFT; got %q", m.state.DiffCursor.Side)
	}
}

func TestDiffH_FromPlusLineRepositionsToNearestLeftRow(t *testing.T) {
	m := sideTestModel(t)
	// Cursor on `+added_x` (index 4) — does not exist on LEFT.
	m.state.DiffCursor.Line = 4
	m.state.DiffCursor.Side = model.DiffSideRight

	m = pressDiffKey(t, m, "h")
	if m.state.DiffCursor.Side != model.DiffSideLeft {
		t.Errorf("h must switch to LEFT; got %q", m.state.DiffCursor.Side)
	}
	// Nearest LEFT-existing row is index 3 (last `-` above) or index 6
	// (ctx2 below). Either is acceptable; we pin "search backward
	// first" so the user lands on adjacent context / removal that the
	// `+` was logically replacing.
	if m.state.DiffCursor.Line != 3 {
		t.Errorf("h from `+` row must reposition cursor to nearest LEFT row above; got %d, want 3", m.state.DiffCursor.Line)
	}
}

func TestDiffH_NoOpInUnifiedMode(t *testing.T) {
	m := sideTestModel(t)
	m.state.DiffViewMode = model.DiffViewUnified
	m.state.DiffCursor.Line = 1
	m.state.DiffCursor.Side = model.DiffSideRight

	m = pressDiffKey(t, m, "h")
	if m.state.DiffCursor.Side != model.DiffSideRight {
		t.Errorf("h in unified mode must NOT change Side; got %q", m.state.DiffCursor.Side)
	}
	if m.state.Notice == "" {
		t.Errorf("h in unified mode should surface a Notice explaining the no-op; got empty")
	}
}

func TestDiffH_NoOpInVisualMode(t *testing.T) {
	m := sideTestModel(t)
	m.state.DiffCursor.Line = 1
	m.state.DiffCursor.Side = model.DiffSideRight
	m.state.Visual = &model.VisualState{
		OriginPane: model.PaneDiff,
		AnchorLine: 1,
	}

	// Use visual handler, mirroring real dispatch from handleKey.
	out, _ := m.handleKeyVisual(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	got := out.(Model)
	if got.state.DiffCursor.Side != model.DiffSideRight {
		t.Errorf("h in visual mode must NOT change Side; got %q", got.state.DiffCursor.Side)
	}
	if got.state.Notice == "" {
		t.Errorf("h in visual mode should surface a Notice (\"side locked in visual\"); got empty")
	}
}
