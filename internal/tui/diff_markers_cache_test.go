package tui

import (
	"testing"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// commentLineMarkers is called once per diffView render and walks every
// thread. Caching it on patchInfo keyed on the threads-cache generation
// counter keeps repeat j/k presses (which re-render constantly) flat.
func TestCommentLineMarkers_CachedAcrossCallsForSameThreads(t *testing.T) {
	m := newComposeModel(t, composePatch, []*model.ReviewComment{
		{ID: 1, NodeID: "PRRC_1", Path: "foo.go", Line: 20, Side: "RIGHT", Body: "x"},
	})
	a := m.commentLineMarkers()
	b := m.commentLineMarkers()
	// Same map identity → cached. Maps in Go are reference types so
	// equality checks compare references; if the cache rebuilt, the
	// maps would be different references.
	if &a == &b {
		// stack-local addresses can collide — fall back to pointer to
		// the underlying map header via a small probe.
	}
	// Insert a unique key into a's map and observe via b. If b is the
	// same cached value (returned by reference), the insertion would be
	// visible — but commentLineMarkers returns a struct of two maps,
	// and struct returns are copied. So we test identity differently:
	// add to a.Right then re-fetch and verify the key is still there.
	a.Right[12345] = '◆'
	c := m.commentLineMarkers()
	if _, ok := c.Right[12345]; !ok {
		t.Fatalf("commentLineMarkers must reuse the same backing maps across calls; got fresh map on third call")
	}
	// Cleanup so other tests aren't poisoned.
	delete(c.Right, 12345)
}

// Mutating PR.Comments must invalidate both threadsCache (already
// covered) AND the cached markers — otherwise the gutter glyphs lag
// behind the just-posted comment.
func TestCommentLineMarkers_InvalidatedAfterCommentPosted(t *testing.T) {
	m := newComposeModel(t, composePatch, []*model.ReviewComment{
		{ID: 1, NodeID: "PRRC_1", Path: "foo.go", Line: 20, Side: "RIGHT", Body: "first"},
	})
	before := m.commentLineMarkers()
	// composePatch buffer index for new line 21 is 5 (the second `+more`
	// row at index 5). For index 20 → line 4 (the first `+added`).
	// We don't pin the exact index, just verify a NEW marker shows up.
	beforeCount := len(before.Right)

	// Set Compose so applyComposeSubmitted appends the new comment.
	m.state.Compose = &model.ComposeState{Kind: model.ComposeInline, Status: model.ComposeSubmitting}
	rc := &model.ReviewComment{ID: 2, NodeID: "PRRC_2", Path: "foo.go", Line: 21, Side: "RIGHT", Body: "second"}
	m.applyComposeSubmitted(composeSubmittedMsg{comment: rc})

	after := m.commentLineMarkers()
	if len(after.Right) <= beforeCount {
		t.Fatalf("markers cache not invalidated after applyComposeSubmitted: before=%d after=%d", beforeCount, len(after.Right))
	}
}
