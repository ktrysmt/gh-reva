package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// Range gutter rendering must keep the visual line `│` continuous across
// continuation rows of wrapped buffer lines. Without this, narrow terminals
// that wrap a `┌` / `│` row visibly drop the gutter line on subsequent
// display rows — the range looks fragmented.
//
// Anchor (`◆`) lines intentionally stop at the first display row: the
// diamond doubles as the bottom edge of the range (see CLAUDE.md §4 #10).

func newRenderTestModel(t *testing.T, paneWidth int) Model {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	m.state.SelectedFile = "test.go"
	m.paneWidthDiff = paneWidth
	return m
}

// longContextLine is a context line wide enough to wrap at narrow paneWidthDiff.
const longContextLine = " package src with a very long trailing comment that must wrap on a narrow pane"

func TestRenderUnifiedBufferLine_RangeStartContinuesGutter(t *testing.T) {
	m := newRenderTestModel(t, 30)
	m.state.DiffViewMode = model.DiffViewUnified

	rows := m.renderUnifiedBufferLine(longContextLine, 0, -1, markerStart, false)
	if len(rows) < 2 {
		t.Fatalf("expected wrap; rows=%d (want >=2)", len(rows))
	}
	first := stripSGR(rows[0])
	cont := stripSGR(rows[1])
	if !strings.Contains(first, string(markerStart)) {
		t.Errorf("first row must show ┌ in gutter; got %q", first)
	}
	if !strings.Contains(cont, string(markerMiddle)) {
		t.Errorf("range start: continuation row must extend gutter with │; got %q", cont)
	}
}

func TestRenderUnifiedBufferLine_RangeMiddleContinuesGutter(t *testing.T) {
	m := newRenderTestModel(t, 30)
	m.state.DiffViewMode = model.DiffViewUnified

	rows := m.renderUnifiedBufferLine(longContextLine, 0, -1, markerMiddle, false)
	if len(rows) < 2 {
		t.Fatalf("expected wrap; rows=%d", len(rows))
	}
	first := stripSGR(rows[0])
	cont := stripSGR(rows[1])
	if !strings.Contains(first, string(markerMiddle)) {
		t.Errorf("first row must show │; got %q", first)
	}
	if !strings.Contains(cont, string(markerMiddle)) {
		t.Errorf("range middle: continuation row must keep │; got %q", cont)
	}
}

func TestRenderUnifiedBufferLine_AnchorDoesNotContinueGutter(t *testing.T) {
	m := newRenderTestModel(t, 30)
	m.state.DiffViewMode = model.DiffViewUnified

	rows := m.renderUnifiedBufferLine(longContextLine, 0, -1, markerAnchor, false)
	if len(rows) < 2 {
		t.Fatalf("expected wrap; rows=%d", len(rows))
	}
	first := stripSGR(rows[0])
	cont := stripSGR(rows[1])
	if !strings.Contains(first, string(markerAnchor)) {
		t.Errorf("first row must show ◆; got %q", first)
	}
	if strings.Contains(cont, string(markerMiddle)) || strings.Contains(cont, string(markerAnchor)) {
		t.Errorf("anchor: continuation must not draw │ or ◆ (anchor doubles as bottom edge); got %q", cont)
	}
}

func TestRenderUnifiedBufferLine_NoMarkerStaysBlank(t *testing.T) {
	m := newRenderTestModel(t, 30)
	m.state.DiffViewMode = model.DiffViewUnified

	rows := m.renderUnifiedBufferLine(longContextLine, 0, -1, 0, false)
	if len(rows) < 2 {
		t.Fatalf("expected wrap; rows=%d", len(rows))
	}
	cont := stripSGR(rows[1])
	if strings.Contains(cont, string(markerMiddle)) {
		t.Errorf("no marker: continuation must not draw │; got %q", cont)
	}
}

func TestRenderSplitBufferLine_RangeStartContinuesGutter(t *testing.T) {
	m := newRenderTestModel(t, 40)
	m.state.DiffViewMode = model.DiffViewSplit
	m.width = 200 // force split via effectiveDiffViewMode

	isSplit, halfW := m.splitLayout()
	if !isSplit {
		t.Fatalf("split layout not active; halfW=%d", halfW)
	}
	spec := diffLineSpec{Kind: ' ', OldLn: 1, NewLn: 1}
	rows := m.renderSplitBufferLine(longContextLine, spec, halfW, 0, -1, model.DiffSideRight, markerStart, 0, false)
	if len(rows) < 2 {
		t.Fatalf("expected wrap; rows=%d", len(rows))
	}
	first := stripSGR(rows[0])
	cont := stripSGR(rows[1])
	gutterFirst := gutterRune(first)
	gutterCont := gutterRune(cont)
	if gutterFirst != markerStart {
		t.Errorf("split first row Left gutter must show ┌; got %q (full=%q)", gutterFirst, first)
	}
	if gutterCont != markerMiddle {
		t.Errorf("split range start: Left continuation gutter must show │; got %q (full=%q)", gutterCont, cont)
	}
}

func TestRenderSplitBufferLine_AnchorDoesNotContinueGutter(t *testing.T) {
	m := newRenderTestModel(t, 40)
	m.state.DiffViewMode = model.DiffViewSplit
	m.width = 200

	isSplit, halfW := m.splitLayout()
	if !isSplit {
		t.Fatalf("split layout not active; halfW=%d", halfW)
	}
	spec := diffLineSpec{Kind: ' ', OldLn: 1, NewLn: 1}
	rows := m.renderSplitBufferLine(longContextLine, spec, halfW, 0, -1, model.DiffSideRight, markerAnchor, 0, false)
	if len(rows) < 2 {
		t.Fatalf("expected wrap; rows=%d", len(rows))
	}
	cont := stripSGR(rows[1])
	gutterCont := gutterRune(cont)
	if gutterCont == markerMiddle || gutterCont == markerAnchor {
		t.Errorf("split anchor: Left continuation gutter must be blank; got %q (full=%q)", gutterCont, cont)
	}
}

// gutterRune returns the rune at column 2 of a rendered Diff row (the gutter
// slot, immediately after the 2-col cursor area). Returns ' ' when the row
// is shorter than expected. Rune-aware so `┌` (3 UTF-8 bytes) is read whole.
//
// Reads col 2 = LEFT-side gutter under the new split layout
// (`Lcursor 2 | Lmarker 2 | oldLn 4 | …`). RIGHT-side gutter sits
// further right; tests that need it use rightGutterRune.
func gutterRune(row string) rune {
	return runeAtCol(row, 2)
}

// runeAtCol is the column-precise reader used by layout tests. Iterates
// runes in `row`, advancing the visual column counter by the rune's
// display width (1 for ASCII / line-drawing, 2 for CJK / wide), and
// returns the rune that starts at `col`. Returns ' ' for over-runs.
func runeAtCol(row string, col int) rune {
	c := 0
	for _, r := range row {
		if c == col {
			return r
		}
		c++
	}
	return ' '
}

// TestSplitLayout_OverheadIs21 pins the per-row overhead of the split
// layout. Adding/removing columns silently breaks every test that
// computes halfW from paneWidthDiff, so the constant lives in one assert
// here. New layout: <Lcursor 2><Lmarker 2><oldLn 4><sp><leftCell halfW>
// <sp>│<sp><Rmarker 2><Rcursor 2><newLn 4><sp><rightCell halfW>.
// Fixed overhead = 2+2+4+1+1+1+1+2+2+4+1 = 21.
func TestSplitLayout_OverheadIs21(t *testing.T) {
	m := newRenderTestModel(t, 21+2*8) // exactly halfW=8
	m.state.DiffViewMode = model.DiffViewSplit
	m.width = 200
	isSplit, halfW := m.splitLayout()
	if !isSplit {
		t.Fatalf("split must engage at the overhead+min-halfW boundary; halfW=%d", halfW)
	}
	if halfW != 8 {
		t.Errorf("halfW at boundary = %d, want 8 (paneWidthDiff=37, overhead=21)", halfW)
	}
}

func TestRenderSplitBufferLine_CursorOnRightSide(t *testing.T) {
	m := newRenderTestModel(t, 50)
	m.state.DiffViewMode = model.DiffViewSplit
	m.width = 200
	m.state.DiffCursor.Side = model.DiffSideRight

	isSplit, halfW := m.splitLayout()
	if !isSplit {
		t.Fatalf("split layout not active; halfW=%d", halfW)
	}
	spec := diffLineSpec{Kind: ' ', OldLn: 1, NewLn: 1}
	rows := m.renderSplitBufferLine(" line1", spec, halfW, 0, 0, model.DiffSideRight, 0, 0, false)
	if len(rows) == 0 {
		t.Fatalf("no rows emitted")
	}
	row := stripSGR(rows[0])
	// Lcursor lives at col 0-1; must be blank when cursor.Side=RIGHT.
	if r := runeAtCol(row, 0); r == '>' {
		t.Errorf("Lcursor must NOT show '>' when cursor.Side=RIGHT; got col0=%q in %q", r, row)
	}
	// Rcursor sits at col 14+halfW; must show '>' when cursor.Side=RIGHT.
	rcursorCol := 14 + halfW
	if r := runeAtCol(row, rcursorCol); r != '>' {
		t.Errorf("Rcursor must show '>' at col %d when cursor.Side=RIGHT; got %q in %q", rcursorCol, r, row)
	}
}

func TestRenderSplitBufferLine_CursorOnLeftSide(t *testing.T) {
	m := newRenderTestModel(t, 50)
	m.state.DiffViewMode = model.DiffViewSplit
	m.width = 200
	m.state.DiffCursor.Side = model.DiffSideLeft

	isSplit, halfW := m.splitLayout()
	if !isSplit {
		t.Fatalf("split layout not active; halfW=%d", halfW)
	}
	spec := diffLineSpec{Kind: ' ', OldLn: 1, NewLn: 1}
	rows := m.renderSplitBufferLine(" line1", spec, halfW, 0, 0, model.DiffSideLeft, 0, 0, false)
	if len(rows) == 0 {
		t.Fatalf("no rows emitted")
	}
	row := stripSGR(rows[0])
	if r := runeAtCol(row, 0); r != '>' {
		t.Errorf("Lcursor must show '>' at col 0 when cursor.Side=LEFT; got %q in %q", r, row)
	}
	rcursorCol := 14 + halfW
	if r := runeAtCol(row, rcursorCol); r == '>' {
		t.Errorf("Rcursor must NOT show '>' when cursor.Side=LEFT; got col%d=%q in %q", rcursorCol, r, row)
	}
}

func TestRenderSplitBufferLine_PerSideMarkers(t *testing.T) {
	m := newRenderTestModel(t, 50)
	m.state.DiffViewMode = model.DiffViewSplit
	m.width = 200
	m.state.DiffCursor.Side = model.DiffSideRight

	isSplit, halfW := m.splitLayout()
	if !isSplit {
		t.Fatalf("split layout not active; halfW=%d", halfW)
	}
	spec := diffLineSpec{Kind: ' ', OldLn: 1, NewLn: 1}
	rows := m.renderSplitBufferLine(" line1", spec, halfW, 0, -1, model.DiffSideRight, markerAnchor, markerAnchor, false)
	row := stripSGR(rows[0])
	// Lmarker at col 2; Rmarker at col 12+halfW.
	if r := runeAtCol(row, 2); r != markerAnchor {
		t.Errorf("Lmarker at col 2 = %q, want ◆; row=%q", r, row)
	}
	rmarkerCol := 12 + halfW
	if r := runeAtCol(row, rmarkerCol); r != markerAnchor {
		t.Errorf("Rmarker at col %d = %q, want ◆; row=%q", rmarkerCol, r, row)
	}
}

func TestRenderSplitBufferLine_LeftOnlyMarkerLeavesRightBlank(t *testing.T) {
	m := newRenderTestModel(t, 50)
	m.state.DiffViewMode = model.DiffViewSplit
	m.width = 200

	isSplit, halfW := m.splitLayout()
	if !isSplit {
		t.Fatalf("split layout not active; halfW=%d", halfW)
	}
	spec := diffLineSpec{Kind: ' ', OldLn: 1, NewLn: 1}
	rows := m.renderSplitBufferLine(" line1", spec, halfW, 0, -1, model.DiffSideRight, markerAnchor, 0, false)
	row := stripSGR(rows[0])
	if r := runeAtCol(row, 2); r != markerAnchor {
		t.Errorf("Lmarker col 2 = %q, want ◆", r)
	}
	rmarkerCol := 12 + halfW
	if r := runeAtCol(row, rmarkerCol); r == markerAnchor {
		t.Errorf("Rmarker col %d must be blank when only LEFT marker is set; got %q", rmarkerCol, r)
	}
}
