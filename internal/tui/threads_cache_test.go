package tui

import (
	"testing"
	"time"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// threadsForView is called multiple times per Diff render
// (commentLineMarkers, threadsForCursor, commentsView, flatComments).
// Subsequent calls within the same (file, range, comments) snapshot
// must return the identity-equal cached slice — rebuilding the thread
// tree per call is a perf regression.
func TestThreadsForView_CachedAcrossCalls(t *testing.T) {
	rc := func(id int64, body, side string) *model.ReviewComment {
		return &model.ReviewComment{
			ID: id, NodeID: "PRRC_" + body, Path: "foo.go",
			Line: 20, Side: side, Body: body,
			CreatedAt: time.Date(2025, 1, int(id), 0, 0, 0, 0, time.UTC),
		}
	}
	m := newComposeModel(t, composePatch, []*model.ReviewComment{
		rc(1, "first", "RIGHT"),
		rc(2, "second", "RIGHT"),
	})
	a := m.threadsForView()
	b := m.threadsForView()
	if len(a) != 2 || len(b) != 2 {
		t.Fatalf("len a=%d b=%d, want 2 each", len(a), len(b))
	}
	// Same backing slice header — proves the second call hit the cache.
	if &a[0] != &b[0] {
		t.Fatalf("threadsForView did not reuse the cached slice across calls")
	}
}

// Mutating PR.Comments via applyComposeSubmitted must invalidate the
// cache so the new comment becomes visible on the next read.
func TestThreadsForView_InvalidatedOnComposeSubmitted(t *testing.T) {
	m := newComposeModel(t, composePatch, []*model.ReviewComment{
		{ID: 1, NodeID: "PRRC_1", Path: "foo.go", Line: 20, Side: "RIGHT", Body: "first"},
	})
	// Prime the cache.
	m.state.Compose = &model.ComposeState{Kind: model.ComposeInline, Status: model.ComposeSubmitting}
	before := m.threadsForView()
	if len(before) != 1 {
		t.Fatalf("len before=%d, want 1", len(before))
	}
	rc := &model.ReviewComment{ID: 2, NodeID: "PRRC_2", Path: "foo.go", Line: 21, Side: "RIGHT", Body: "second"}
	m.applyComposeSubmitted(composeSubmittedMsg{comment: rc})
	after := m.threadsForView()
	if len(after) != 2 {
		t.Fatalf("cache not invalidated: len after=%d, want 2", len(after))
	}
}

// applyCommentsRefreshed must also invalidate.
func TestThreadsForView_InvalidatedOnCommentsRefreshed(t *testing.T) {
	m := newComposeModel(t, composePatch, []*model.ReviewComment{
		{ID: 1, NodeID: "PRRC_1", Path: "foo.go", Line: 20, Side: "RIGHT", Body: "old"},
	})
	_ = m.threadsForView() // prime
	fresh := []*model.ReviewComment{
		{ID: 1, NodeID: "PRRC_1", Path: "foo.go", Line: 20, Side: "RIGHT", Body: "old"},
		{ID: 2, NodeID: "PRRC_2", Path: "foo.go", Line: 21, Side: "RIGHT", Body: "new"},
	}
	m.applyCommentsRefreshed(commentsRefreshedMsg{comments: fresh})
	after := m.threadsForView()
	if len(after) != 2 {
		t.Fatalf("cache not invalidated by refresh: len after=%d, want 2", len(after))
	}
}

// SelectedFile change must yield a fresh result, not the cached one
// from the previous file.
func TestThreadsForView_RecomputesOnFileChange(t *testing.T) {
	m := newComposeModel(t, composePatch, []*model.ReviewComment{
		{ID: 1, NodeID: "PRRC_1", Path: "foo.go", Line: 20, Side: "RIGHT", Body: "foo-c"},
		{ID: 2, NodeID: "PRRC_2", Path: "bar.go", Line: 5, Side: "RIGHT", Body: "bar-c"},
	})
	a := m.threadsForView()
	if len(a) != 1 || a[0].Root.Path != "foo.go" {
		t.Fatalf("first read on foo.go: got %+v", a)
	}
	m.state.SelectedFile = "bar.go"
	b := m.threadsForView()
	if len(b) != 1 || b[0].Root.Path != "bar.go" {
		t.Fatalf("after file change: want bar.go thread, got %+v", b)
	}
}
