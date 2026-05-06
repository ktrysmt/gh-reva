package tui

import (
	"testing"

	"github.com/ktrysmt/gh-reva/internal/model"
)

func TestApplyCommentsRefreshed_FlipsPendingFlags(t *testing.T) {
	m := newComposeModel(t, composePatch, []*model.ReviewComment{
		{ID: 1, NodeID: "PRRC_1", Pending: true, Path: "foo.go"},
	})
	fresh := []*model.ReviewComment{
		{ID: 1, NodeID: "PRRC_1", Pending: false, Path: "foo.go"},
		{ID: 2, NodeID: "PRRC_2", Pending: false, Path: "foo.go"},
	}
	m.applyCommentsRefreshed(commentsRefreshedMsg{comments: fresh})
	if got := len(m.state.PR.Comments); got != 2 {
		t.Fatalf("expected 2 comments after refresh, got %d", got)
	}
	for _, c := range m.state.PR.Comments {
		if c.Pending {
			t.Fatalf("refreshed comment should not be pending: %+v", c)
		}
	}
	if m.state.PR.Files[0].CommentCount != 2 {
		t.Fatalf("CommentCount must reflect refreshed list, got %d", m.state.PR.Files[0].CommentCount)
	}
}

// applyCommentsRefreshed must NOT erase a locally-known Pending comment
// just because the refresh response is missing it. GitHub's
// reviewThreads endpoint has eventual-consistency lag relative to
// addPullRequestReviewThread — a refresh fired immediately after a
// successful POST can return the pre-POST snapshot, and a naive
// REPLACE would silently drop the user's just-posted draft from the UI
// until the next refresh. Reported as: "post してもrefreshされていない
// のか画面に反映されない；再起動すると反映される".
func TestApplyCommentsRefreshed_PreservesPendingMissingFromResponse(t *testing.T) {
	optimistic := &model.ReviewComment{
		ID: 99, NodeID: "PRRC_local", Pending: true, Path: "foo.go", Body: "draft",
	}
	m := newComposeModel(t, composePatch, []*model.ReviewComment{
		{ID: 1, NodeID: "PRRC_1", Pending: false, Path: "foo.go"},
		optimistic,
	})
	// Refresh returns the pre-POST snapshot — only the public comment.
	stale := []*model.ReviewComment{
		{ID: 1, NodeID: "PRRC_1", Pending: false, Path: "foo.go"},
	}
	m.applyCommentsRefreshed(commentsRefreshedMsg{comments: stale})
	if got := len(m.state.PR.Comments); got != 2 {
		t.Fatalf("expected 2 comments (refresh + preserved pending), got %d", got)
	}
	var sawOptimistic bool
	for _, c := range m.state.PR.Comments {
		if c.NodeID == "PRRC_local" {
			sawOptimistic = true
			if !c.Pending {
				t.Errorf("preserved comment must keep Pending=true, got %+v", c)
			}
			if c.Body != "draft" {
				t.Errorf("preserved comment must keep Body, got %q", c.Body)
			}
		}
	}
	if !sawOptimistic {
		t.Fatalf("optimistic pending comment must be preserved when refresh omits it; got %+v", m.state.PR.Comments)
	}
	// CommentCount must include the preserved pending entry.
	if m.state.PR.Files[0].CommentCount != 2 {
		t.Fatalf("CommentCount must include preserved pending, got %d", m.state.PR.Files[0].CommentCount)
	}
}

// Sanity check: when the refresh response DOES include the optimistic
// comment (typical happy path), no duplicate is produced.
func TestApplyCommentsRefreshed_NoDuplicateOnHappyPath(t *testing.T) {
	m := newComposeModel(t, composePatch, []*model.ReviewComment{
		{ID: 99, NodeID: "PRRC_local", Pending: true, Path: "foo.go"},
	})
	fresh := []*model.ReviewComment{
		{ID: 99, NodeID: "PRRC_local", Pending: true, Path: "foo.go"},
	}
	m.applyCommentsRefreshed(commentsRefreshedMsg{comments: fresh})
	if got := len(m.state.PR.Comments); got != 1 {
		t.Fatalf("happy path must not duplicate, got %d", got)
	}
}
