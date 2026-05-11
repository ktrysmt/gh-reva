package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// Visual-range rows must not paint a row-wide background. Earlier behavior
// (bgRow(_, VisualRangeBg) on the whole row) leaked the highlight onto the
// opposite split lane — the user never selected that side because h/l is
// locked in visual mode and j/k auto-skips opposite-side rows, so the bg
// signaled "this side is selected too" which is misleading. Range
// membership is now indicated by the `> ` glyph alone (mirroring how
// Files / Commits / Comments render visual selection).

// bgOpener returns the SGR prefix emitted by lipgloss for the given bg
// color under the current color profile. Empty when the profile strips bg
// (Ascii) — callers should run under TrueColor for a non-empty opener.
func bgOpener(c lipgloss.Color) string {
	sample := lipgloss.NewStyle().Background(c).Render("X")
	i := strings.Index(sample, "X")
	if i <= 0 {
		return ""
	}
	return sample[:i]
}

func TestRenderSplitBufferLine_VisualRangeHasNoRowBackground(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	m.state.SelectedFile = "test.go"
	m.paneWidthDiff = 50
	m.state.DiffViewMode = model.DiffViewSplit
	m.width = 200
	m.state.DiffCursor.Side = model.DiffSideRight
	m.state.DiffCursor.Line = 2
	m.state.Visual = &model.VisualState{OriginPane: model.PaneDiff, AnchorLine: 0}

	opener := bgOpener(m.theme.VisualRangeBg)
	if opener == "" {
		t.Fatalf("theme.VisualRangeBg = %q produced no SGR opener under TrueColor; cannot detect bg-leak", m.theme.VisualRangeBg)
	}

	isSplit, halfW := m.splitLayout()
	if !isSplit {
		t.Fatalf("split layout not active; halfW=%d", halfW)
	}
	spec := diffLineSpec{Kind: ' ', OldLn: 1, NewLn: 1}
	// idx=1 is inside the visual range [0..2] but not the cursor row,
	// so the rendered row would previously be wrapped in VisualRangeBg.
	rows := m.renderSplitBufferLine(" hello world", spec, halfW, 1, 2, model.DiffSideRight, 0, 0, false)
	if len(rows) == 0 {
		t.Fatalf("no rows emitted")
	}
	for i, r := range rows {
		if strings.Contains(r, opener) {
			t.Errorf("split visual-range row %d contains VisualRangeBg SGR %q; bg must be removed. row=%q", i, opener, r)
		}
	}
}

func TestRenderUnifiedBufferLine_VisualRangeHasNoRowBackground(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	m.state.SelectedFile = "test.go"
	m.paneWidthDiff = 50
	m.state.DiffViewMode = model.DiffViewUnified
	m.state.DiffCursor.Line = 2
	m.state.Visual = &model.VisualState{OriginPane: model.PaneDiff, AnchorLine: 0}

	opener := bgOpener(m.theme.VisualRangeBg)
	if opener == "" {
		t.Fatalf("theme.VisualRangeBg = %q produced no SGR opener under TrueColor; cannot detect bg-leak", m.theme.VisualRangeBg)
	}

	rows := m.renderUnifiedBufferLine(" hello world", 1, 2, 0, false)
	if len(rows) == 0 {
		t.Fatalf("no rows emitted")
	}
	for i, r := range rows {
		if strings.Contains(r, opener) {
			t.Errorf("unified visual-range row %d contains VisualRangeBg SGR %q; bg must be removed. row=%q", i, opener, r)
		}
	}
}
