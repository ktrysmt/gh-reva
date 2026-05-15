package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// Search-match highlighting in split mode used to wrap the entire row with
// theme.SearchMatchBg, which painted the empty opposite-side cell (the LEFT
// half for a `+` line, the RIGHT half for a `-` line). That misled users
// into thinking the match was on the wrong column. The fix: apply
// SearchMatchBg only on the side(s) carrying the matched content.
//
// Rule (mirrors splitDiffLine's column routing):
//
//   - `+` lines  → RIGHT only (LEFT is blank)
//   - `-` lines  → LEFT only  (RIGHT is blank)
//   - context    → both sides (same content)
//   - header/@@  → both sides (same content)
//
// Unified mode collapses both columns into one cell so the row-wide bg
// still applies — only the split path is per-side.

// bgFragment returns the "48;2;R;G;B" SGR fragment for the given color.
// We grep for this substring so detection survives combined SGR like
// "\e[38;2;...;48;2;87;75;0m" that lipgloss emits when fg and bg co-occur
// on the same token.
func bgFragment(c lipgloss.Color) string {
	sample := lipgloss.NewStyle().Background(c).Render("X")
	// sample starts with "\e[48;2;R;G;Bm" — strip "\e[" and "m" delimiters
	// to keep only the parameter list.
	if !strings.HasPrefix(sample, "\x1b[") {
		return ""
	}
	end := strings.Index(sample, "m")
	if end < 0 {
		return ""
	}
	return sample[2:end]
}

func renderSplitForMatch(t *testing.T, kind byte, line string) ([]string, string, string) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	m.state.SelectedFile = "test.go"
	m.paneWidthDiff = 80
	m.state.DiffViewMode = model.DiffViewSplit
	m.width = 200
	m.state.DiffCursor.Side = model.DiffSideRight
	m.state.DiffCursor.Line = 99 // cursor elsewhere so this row is not the cursor row

	frag := bgFragment(m.theme.SearchMatchBg)
	if frag == "" {
		t.Fatalf("SearchMatchBg = %q produced no SGR fragment under TrueColor", m.theme.SearchMatchBg)
	}

	isSplit, halfW := m.splitLayout()
	if !isSplit {
		t.Fatalf("split layout not active (halfW=%d)", halfW)
	}
	spec := diffLineSpec{Kind: kind, OldLn: 1, NewLn: 1}
	rows := m.renderSplitBufferLine(line, spec, halfW, 0, 99, model.DiffSideRight, 0, 0, true /* matched */)
	if len(rows) == 0 {
		t.Fatalf("no rows emitted")
	}
	sep := fg("│", m.theme.DiffSeparator)
	return rows, frag, sep
}

func sideHasOpener(row, sep, frag string) (left, right bool) {
	idx := strings.LastIndex(row, sep)
	if idx < 0 {
		// Fall back to halfway split — separator must be present in split
		// mode; if not, the test setup is wrong, fail loudly elsewhere.
		return strings.Contains(row, frag), strings.Contains(row, frag)
	}
	leftPart := row[:idx]
	rightPart := row[idx+len(sep):]
	return strings.Contains(leftPart, frag), strings.Contains(rightPart, frag)
}

func TestRenderSplitBufferLine_SearchMatchBg_PlusLine_RightOnly(t *testing.T) {
	rows, opener, sep := renderSplitForMatch(t, '+', "+added line")
	for i, r := range rows {
		left, right := sideHasOpener(r, sep, opener)
		if left {
			t.Errorf("row %d: + line should NOT carry SearchMatchBg on LEFT (empty) cell; row=%q", i, r)
		}
		if !right {
			t.Errorf("row %d: + line should carry SearchMatchBg on RIGHT cell; row=%q", i, r)
		}
	}
}

func TestRenderSplitBufferLine_SearchMatchBg_MinusLine_LeftOnly(t *testing.T) {
	rows, opener, sep := renderSplitForMatch(t, '-', "-removed line")
	for i, r := range rows {
		left, right := sideHasOpener(r, sep, opener)
		if !left {
			t.Errorf("row %d: - line should carry SearchMatchBg on LEFT cell; row=%q", i, r)
		}
		if right {
			t.Errorf("row %d: - line should NOT carry SearchMatchBg on RIGHT (empty) cell; row=%q", i, r)
		}
	}
}

func TestRenderSplitBufferLine_SearchMatchBg_ContextLine_BothSides(t *testing.T) {
	rows, opener, sep := renderSplitForMatch(t, ' ', " context line")
	for i, r := range rows {
		left, right := sideHasOpener(r, sep, opener)
		if !left || !right {
			t.Errorf("row %d: context line should carry SearchMatchBg on BOTH sides (left=%v right=%v); row=%q", i, left, right, r)
		}
	}
}
