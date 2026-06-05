package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-reva/internal/diff"
	"github.com/ktrysmt/gh-reva/internal/model"
)

// expandPatch covers OLD/NEW lines 3 and 12 in foo.go (one-line hunks),
// so the augmented buffer has a BOF synthetic (2 hidden) and a Mid
// synthetic (8 hidden) once Expand has run. With file lines provided,
// an EOF synthetic also appears.
const expandPatch = `--- a/foo.go
+++ b/foo.go
@@ -3,1 +3,1 @@
-old3
+new3
@@ -12,1 +12,1 @@
-old12
+new12`

func newExpandModel(t *testing.T) *Model {
	t.Helper()
	m := newComposeModel(t, expandPatch, nil)
	// File lines: 15 entries so the EOF gap is visible (lines 13..15
	// hidden until expanded). NEW-side content for L3 / L12 mirrors the
	// patch's `+` rows for realism, though Expand only uses the lines
	// for expanded-context emission (not for the +/- rows).
	lines := make([]string, 15)
	for i := range lines {
		switch i + 1 {
		case 3:
			lines[i] = "new3"
		case 12:
			lines[i] = "new12"
		default:
			lines[i] = "L" + itoa15(i+1)
		}
	}
	m.state.FileContents[model.FileContentsKey{Ref: "head1234", Path: "foo.go"}] = lines
	return m
}

func itoa15(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func TestPatchLines_IncludesSyntheticGapsByDefault(t *testing.T) {
	m := newExpandModel(t)
	lines := m.patchLines()
	if len(lines) == 0 {
		t.Fatalf("patchLines empty")
	}
	synth := 0
	for _, l := range lines {
		if l == diff.SyntheticLine {
			synth++
		}
	}
	// Expect BOF + Mid + EOF = 3 synthetic rows.
	if synth != 3 {
		t.Fatalf("synthetic count: got %d want 3 (lines=%q)", synth, lines)
	}
}

func TestPatchLines_GapsMapPopulated(t *testing.T) {
	m := newExpandModel(t)
	gaps := m.patchGaps()
	if len(gaps) != 3 {
		t.Fatalf("gaps map size: got %d want 3 (gaps=%+v)", len(gaps), gaps)
	}
	// Each gap's buffer index must point at a synthetic row.
	lines := m.patchLines()
	for idx := range gaps {
		if idx < 0 || idx >= len(lines) || lines[idx] != diff.SyntheticLine {
			t.Fatalf("gap idx %d does not point at a synthetic row (got %q)", idx, lines[idx])
		}
	}
}

func TestPatchLines_ExpansionStateAppliedAndCached(t *testing.T) {
	m := newExpandModel(t)
	key := model.ExpandKey{Path: "foo.go", RangeKind: model.RangeWholePR}
	m.state.ExpandedContext[key] = &model.ExpandState{BOFBelow: 1}
	m.invalidatePatchInfoCache(key)
	lines := m.patchLines()
	// File header at idx 0,1; idx 2 should be SyntheticLine (1 line still
	// hidden); idx 3 should be the expanded-context for NEW line 2.
	if lines[2] != diff.SyntheticLine {
		t.Fatalf("idx 2 should be synthetic, got %q", lines[2])
	}
	if lines[3] != " L2" {
		t.Fatalf("idx 3 should be expanded ctx ` L2`, got %q", lines[3])
	}
}

func TestPatchLines_CacheReusedWhenExpandUnchanged(t *testing.T) {
	m := newExpandModel(t)
	first := m.patchInfo()
	second := m.patchInfo()
	if first != second {
		t.Fatalf("patchInfo cache identity changed between calls without state mutation")
	}
}

func TestPatchSpecs_SyntheticKindS(t *testing.T) {
	m := newExpandModel(t)
	lines := m.patchLines()
	specs := m.patchSpecs()
	if len(lines) != len(specs) {
		t.Fatalf("lines/specs length mismatch: %d vs %d", len(lines), len(specs))
	}
	for i, l := range lines {
		if l == diff.SyntheticLine {
			if specs[i].Kind != 's' {
				t.Fatalf("synthetic row at idx %d should have spec Kind 's', got %q", i, specs[i].Kind)
			}
		}
	}
}

// commentLineMarkers must not place anchors on synthetic rows nor produce
// stale buffer indices that exceed the augmented buffer.
func TestCommentLineMarkers_DoesNotMarkSyntheticRows(t *testing.T) {
	m := newExpandModel(t)
	// Attach a comment at NEW line 3 (the `+new3` row at idx ?).
	m.state.PR.Comments = []*model.ReviewComment{
		{ID: 1, NodeID: "PRRC_1", ThreadID: "PRT_1", Path: "foo.go", Line: 3, Side: "RIGHT", Body: "x"},
	}
	m.threadsCache = &threadsViewCache{}
	markers := m.commentLineMarkers()
	lines := m.patchLines()
	for idx := range markers.Right {
		if idx < 0 || idx >= len(lines) {
			t.Fatalf("marker at out-of-range idx %d (len=%d)", idx, len(lines))
		}
		if lines[idx] == diff.SyntheticLine {
			t.Fatalf("marker on synthetic row at idx %d", idx)
		}
	}
}

// Verify that running Expand on the same inputs is stable (no flicker
// across consecutive patchInfo calls when nothing changes).
func TestPatchLines_StableAcrossCalls(t *testing.T) {
	m := newExpandModel(t)
	a := strings.Join(m.patchLines(), "\n")
	b := strings.Join(m.patchLines(), "\n")
	if a != b {
		t.Fatalf("patchLines unstable across calls")
	}
}

// Synthetic rows render as a single, full-width row in both split and
// unified mode. The body shows the hidden count + "enter: expand" hint.
func TestRenderSynthRow_ContainsHiddenCountAndHint(t *testing.T) {
	m := newExpandModel(t)
	m.paneWidthDiff = 80
	m.paneHeightDiff = 20
	gaps := m.patchGaps()
	var synthIdx int
	var hidden int
	for idx, g := range gaps {
		if g.ID.Kind == diff.GapKindBOF {
			synthIdx = idx
			hidden = g.HiddenCount
			break
		}
	}
	if hidden == 0 {
		t.Fatalf("test setup: no BOF gap in expand model")
	}
	m.state.DiffCursor.Line = synthIdx
	out := m.diffView()
	if !strings.Contains(out, "···") {
		t.Fatalf("synthetic row missing `···` glyph: %q", out)
	}
	wantCount := fmtInt(hidden)
	if !strings.Contains(out, wantCount+" lines folded") {
		t.Fatalf("synthetic row missing folded count %q: %q", wantCount, out)
	}
	if !strings.Contains(out, "Enter") {
		t.Fatalf("synthetic row missing Enter hint: %q", out)
	}
}

// buildComposeInline must reject synthetic rows — they have no
// underlying file line to anchor a review comment to.
func TestBuildComposeInline_RejectsSyntheticRow(t *testing.T) {
	m := newExpandModel(t)
	m.state.FocusedPane = model.PaneDiff
	gaps := m.patchGaps()
	var synthIdx int = -1
	for idx := range gaps {
		synthIdx = idx
		break
	}
	if synthIdx < 0 {
		t.Fatalf("no synthetic in expand model")
	}
	m.state.DiffCursor.Line = synthIdx
	if m.buildComposeInline() {
		t.Fatalf("buildComposeInline should return false on synthetic row")
	}
}

// Expanded context rows MUST anchor correctly. After expanding the BOF
// gap, the revealed context lines carry real OLD/NEW line numbers; a
// new comment on one of them anchors at that line with the cursor's Side.
func TestBuildComposeInline_OnExpandedContext_AnchorsCorrectly(t *testing.T) {
	m := newExpandModel(t)
	m.state.FocusedPane = model.PaneDiff
	ek := model.ExpandKey{Path: "foo.go", RangeKind: model.RangeWholePR}
	m.state.ExpandedContext[ek] = &model.ExpandState{BOFBelow: 1}
	m.invalidatePatchInfoCache(ek)
	lines := m.patchLines()
	// Idx 3 is the expanded BOF ctx " L2" with NewLn=2.
	if lines[3] != " L2" {
		t.Fatalf("setup: idx 3 not ` L2`: %q", lines[3])
	}
	m.state.DiffCursor.Line = 3
	m.state.DiffCursor.Side = model.DiffSideRight
	if !m.buildComposeInline() {
		t.Fatalf("buildComposeInline returned false")
	}
	cs := m.state.Compose
	if cs.Line != 2 || cs.Side != "RIGHT" {
		t.Fatalf("expanded ctx anchor: got line=%d side=%s, want 2/RIGHT", cs.Line, cs.Side)
	}
}

// Enter on a BOF synthetic with FileContents already cached applies the
// expansion synchronously: ExpandedContext.BOFBelow grows by 20 (capped
// at the gap size), the patchInfo cache is invalidated, and the next
// render emits expanded context rows. No tea.Cmd is queued because the
// fetch has already happened.
func TestHandleKeyDiff_EnterOnBOFSynthetic_AppliesExpand(t *testing.T) {
	m := newExpandModel(t)
	m.state.FocusedPane = model.PaneDiff
	m.paneWidthDiff = 80
	m.paneHeightDiff = 20
	gaps := m.patchGaps()
	var bofIdx int = -1
	for idx, g := range gaps {
		if g.ID.Kind == diff.GapKindBOF {
			bofIdx = idx
			break
		}
	}
	if bofIdx < 0 {
		t.Fatalf("no BOF gap in expand model")
	}
	m.state.DiffCursor.Line = bofIdx
	model2, cmd := m.handleKeyDiff(tea.KeyMsg{Type: tea.KeyEnter})
	_ = cmd
	mm, ok := model2.(Model)
	if !ok {
		t.Fatalf("handleKeyDiff returned %T", model2)
	}
	es := mm.state.ExpandedContext[model.ExpandKey{Path: "foo.go", RangeKind: model.RangeWholePR}]
	if es == nil {
		t.Fatalf("ExpandedContext not populated")
	}
	// BOF gap had hidden=2; BOFBelow grew to min(20, 2) = 2 after cap in
	// diff.Expand, but the stored counter just grows by 20 (cap applies
	// only at rebuild). Either is acceptable so long as the next render
	// fully reveals the BOF gap.
	if es.BOFBelow < 1 {
		t.Fatalf("BOFBelow not grown: %+v", es)
	}
	// Re-fetch patchInfo; BOF synthetic should be gone.
	for _, g := range mm.patchGaps() {
		if g.ID.Kind == diff.GapKindBOF {
			t.Fatalf("BOF synthetic still present: %+v", g)
		}
	}
}

func TestHandleKeyDiff_EnterOnMidSynthetic_AppliesSymmetric(t *testing.T) {
	m := newExpandModel(t)
	m.state.FocusedPane = model.PaneDiff
	m.paneWidthDiff = 80
	m.paneHeightDiff = 20
	gaps := m.patchGaps()
	var midIdx int = -1
	for idx, g := range gaps {
		if g.ID.Kind == diff.GapKindMid {
			midIdx = idx
			break
		}
	}
	if midIdx < 0 {
		t.Fatalf("no Mid gap in expand model")
	}
	m.state.DiffCursor.Line = midIdx
	model2, _ := m.handleKeyDiff(tea.KeyMsg{Type: tea.KeyEnter})
	mm := model2.(Model)
	es := mm.state.ExpandedContext[model.ExpandKey{Path: "foo.go", RangeKind: model.RangeWholePR}]
	if es == nil {
		t.Fatalf("ExpandedContext not populated")
	}
	if es.InterAbove[0] != 10 || es.InterBelow[0] != 10 {
		t.Fatalf("Mid expand not 10/10: %+v", es)
	}
}

func TestHandleKeyDiff_EnterOnEOFSynthetic_AppliesAbove(t *testing.T) {
	m := newExpandModel(t)
	m.state.FocusedPane = model.PaneDiff
	m.paneWidthDiff = 80
	m.paneHeightDiff = 20
	gaps := m.patchGaps()
	var eofIdx int = -1
	for idx, g := range gaps {
		if g.ID.Kind == diff.GapKindEOF {
			eofIdx = idx
			break
		}
	}
	if eofIdx < 0 {
		t.Fatalf("no EOF gap in expand model")
	}
	m.state.DiffCursor.Line = eofIdx
	model2, _ := m.handleKeyDiff(tea.KeyMsg{Type: tea.KeyEnter})
	mm := model2.(Model)
	es := mm.state.ExpandedContext[model.ExpandKey{Path: "foo.go", RangeKind: model.RangeWholePR}]
	if es == nil || es.EOFAbove < 1 {
		t.Fatalf("EOFAbove not grown: %+v", es)
	}
}

func fmtInt(n int) string {
	if n == 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	return out
}

// findSynthIdx returns the first synthetic buffer index in m. Helper for
// tests that don't care which gap kind they pick.
func findSynthIdx(t *testing.T, m *Model) int {
	t.Helper()
	gaps := m.patchGaps()
	for idx := range gaps {
		return idx
	}
	t.Fatalf("no synthetic row in expand model")
	return -1
}

// Synthetic rows in split mode paint the `···` body on BOTH halves with
// the `│` divider intact, mirroring the regular split row geometry. A
// single full-width body strands the cursor on the left column and reads
// as "cursor jumped" when DiffCursor.Side=RIGHT.
func TestRenderSynthRow_SplitMode_BodyOnBothHalves(t *testing.T) {
	m := newExpandModel(t)
	m.paneWidthDiff = 80
	m.paneHeightDiff = 20
	m.state.DiffViewMode = model.DiffViewSplit
	m.width = 200
	isSplit, halfW := m.splitLayout()
	if !isSplit {
		t.Fatalf("split layout not engaged; halfW=%d", halfW)
	}
	idx := findSynthIdx(t, m)
	rows := m.renderSynthBufferLine(idx, -1, m.patchGaps()[idx])
	if len(rows) == 0 {
		t.Fatalf("no rows emitted")
	}
	row := stripSGR(rows[0])
	if got := strings.Count(row, "···"); got != 2 {
		t.Errorf("split synth row must contain `···` twice (left + right cell); got %d in %q", got, row)
	}
	sepCol := halfW + 10
	if r := runeAtCol(row, sepCol); r != '│' {
		t.Errorf("split synth separator at col %d = %q, want │; row=%q", sepCol, r, row)
	}
}

func TestRenderSynthRow_SplitMode_CursorOnRightSide(t *testing.T) {
	m := newExpandModel(t)
	m.paneWidthDiff = 80
	m.paneHeightDiff = 20
	m.state.DiffViewMode = model.DiffViewSplit
	m.width = 200
	idx := findSynthIdx(t, m)
	m.state.DiffCursor.Line = idx
	m.state.DiffCursor.Side = model.DiffSideRight
	isSplit, halfW := m.splitLayout()
	if !isSplit {
		t.Fatalf("split layout not engaged; halfW=%d", halfW)
	}
	rows := m.renderSynthBufferLine(idx, idx, m.patchGaps()[idx])
	row := stripSGR(rows[0])
	if r := runeAtCol(row, 0); r == '>' {
		t.Errorf("Lcursor must NOT show '>' when Side=RIGHT on synth row; got %q", row)
	}
	rcursorCol := 14 + halfW
	if r := runeAtCol(row, rcursorCol); r != '>' {
		t.Errorf("Rcursor at col %d must show '>' when Side=RIGHT on synth row; got %q in %q", rcursorCol, r, row)
	}
}

func TestRenderSynthRow_SplitMode_CursorOnLeftSide(t *testing.T) {
	m := newExpandModel(t)
	m.paneWidthDiff = 80
	m.paneHeightDiff = 20
	m.state.DiffViewMode = model.DiffViewSplit
	m.width = 200
	idx := findSynthIdx(t, m)
	m.state.DiffCursor.Line = idx
	m.state.DiffCursor.Side = model.DiffSideLeft
	isSplit, halfW := m.splitLayout()
	if !isSplit {
		t.Fatalf("split layout not engaged; halfW=%d", halfW)
	}
	rows := m.renderSynthBufferLine(idx, idx, m.patchGaps()[idx])
	row := stripSGR(rows[0])
	if r := runeAtCol(row, 0); r != '>' {
		t.Errorf("Lcursor must show '>' at col 0 when Side=LEFT on synth row; got %q", row)
	}
	rcursorCol := 14 + halfW
	if r := runeAtCol(row, rcursorCol); r == '>' {
		t.Errorf("Rcursor at col %d must NOT show '>' when Side=LEFT on synth row; got %q in %q", rcursorCol, r, row)
	}
}

// Unified mode keeps the single full-width row (only one `···` per
// display row) since there's no second column to mirror.
func TestRenderSynthRow_UnifiedMode_SingleFullWidthRow(t *testing.T) {
	m := newExpandModel(t)
	m.paneWidthDiff = 80
	m.paneHeightDiff = 20
	m.state.DiffViewMode = model.DiffViewUnified
	idx := findSynthIdx(t, m)
	rows := m.renderSynthBufferLine(idx, -1, m.patchGaps()[idx])
	if len(rows) == 0 {
		t.Fatalf("no rows emitted")
	}
	row := stripSGR(rows[0])
	if got := strings.Count(row, "···"); got != 1 {
		t.Errorf("unified synth row must contain `···` exactly once; got %d in %q", got, row)
	}
	if strings.Contains(row, "│") {
		t.Errorf("unified synth row must not draw `│` divider; got %q", row)
	}
}
