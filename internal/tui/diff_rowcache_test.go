package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// rowCache for split mode must NOT key on cursorSide. cursorSide only
// affects the rendered cursor cell, and cursor rows skip the cache via
// the isCursor / inVisual guard. Including cursorSide in the key forces
// a duplicate entry for every non-cursor row each time the user presses
// h or l — pure cache thrash with no correctness benefit.
func TestRenderSplitBufferLine_CacheReusedAcrossCursorSide(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := newRenderTestModel(t, 50)
	m.state.DiffViewMode = model.DiffViewSplit
	m.width = 200
	// Prime rowCache identity: invalidateRowCacheIfStale uses
	// (patchKey, width, halfW). We don't render via diffView() here, so
	// reset manually to the correct width / halfW so put/get use the
	// same map.
	_, halfW := m.splitLayout()
	m.rowCache.reset("", m.paneWidthDiff, halfW)

	spec := diffLineSpec{Kind: ' ', OldLn: 1, NewLn: 1}
	// idx=5 is NOT the cursor (cursorLine=0), so the cache path runs.
	idx, cursorLine := 5, 0

	// First render: cursorSide = RIGHT.
	_ = m.renderSplitBufferLine(" line", spec, halfW, idx, cursorLine, model.DiffSideRight, 0, 0, false)
	sizeAfterFirst := len(m.rowCache.m)
	if sizeAfterFirst != 1 {
		t.Fatalf("expected 1 cache entry after first render; got %d", sizeAfterFirst)
	}

	// Second render: same idx, same row content, same markers — only
	// cursorSide flipped. Should hit the cache (no new entry).
	_ = m.renderSplitBufferLine(" line", spec, halfW, idx, cursorLine, model.DiffSideLeft, 0, 0, false)
	sizeAfterSecond := len(m.rowCache.m)
	if sizeAfterSecond != 1 {
		t.Errorf("rowCache duplicated entry across cursorSide flip; size went 1 -> %d. cursorSide is unused for non-cursor rows and must not be part of the cache key.", sizeAfterSecond)
	}
}
