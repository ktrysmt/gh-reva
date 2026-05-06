package tui

import (
	"errors"
	"testing"

	"github.com/ktrysmt/gh-reva/internal/model"
)

func TestStartSubmitReview_CountsPending(t *testing.T) {
	m := newComposeModel(t, composePatch, []*model.ReviewComment{
		{ID: 1, Pending: true, Path: "foo.go"},
		{ID: 2, Pending: true, Path: "foo.go"},
		{ID: 3, Pending: false, Path: "foo.go"},
	})
	if cmd := m.startSubmitReview(); cmd != nil {
		t.Fatalf("startSubmitReview should not queue a Cmd before user picks event")
	}
	if m.state.SubmitReview == nil {
		t.Fatalf("SubmitReview must be set")
	}
	if m.state.SubmitReview.PendingCount != 2 {
		t.Fatalf("expected PendingCount=2, got %d", m.state.SubmitReview.PendingCount)
	}
	if m.state.SubmitReview.Status != model.SubmitChoosing {
		t.Fatalf("status must be Choosing initially")
	}
}

func TestApplySubmitReviewDone_RefetchesOnSuccess(t *testing.T) {
	stub := &composeStubClient{
		listResponse: []*model.ReviewComment{
			{ID: 1, Pending: false, Path: "foo.go"},
		},
	}
	m := newComposeModel(t, composePatch, []*model.ReviewComment{
		{ID: 1, Pending: true, Path: "foo.go"},
	})
	m.client = stub
	m.state.SubmitReview = &model.SubmitReviewState{Status: model.SubmitSubmitting, Event: model.SubmitComment}
	cmd := m.applySubmitReviewDone(submitReviewDoneMsg{})
	if cmd == nil {
		t.Fatalf("success should queue refetch")
	}
	if m.state.SubmitReview != nil {
		t.Fatalf("SubmitReview must be cleared on success")
	}
	msg := cmd()
	rm, ok := msg.(commentsRefreshedMsg)
	if !ok {
		t.Fatalf("expected commentsRefreshedMsg, got %T", msg)
	}
	if len(rm.comments) != 1 || rm.comments[0].Pending {
		t.Fatalf("refresh response wrong: %+v", rm)
	}
}

func TestApplySubmitReviewDone_FailureKeepsModal(t *testing.T) {
	m := newComposeModel(t, composePatch, nil)
	m.state.SubmitReview = &model.SubmitReviewState{Status: model.SubmitSubmitting, Event: model.SubmitApprove}
	cmd := m.applySubmitReviewDone(submitReviewDoneMsg{err: errors.New("HTTP 422")})
	if cmd != nil {
		t.Fatalf("failure should not queue any cmd")
	}
	if m.state.SubmitReview == nil {
		t.Fatalf("SubmitReview must persist on failure")
	}
	if m.state.SubmitReview.Status != model.SubmitFailed {
		t.Fatalf("status must be Failed")
	}
	if m.state.SubmitReview.Event != model.SubmitApprove {
		t.Fatalf("Event must be preserved for retry, got %s", m.state.SubmitReview.Event)
	}
}

func TestApplyCommentsRefreshed_FlipsPendingFlags(t *testing.T) {
	m := newComposeModel(t, composePatch, []*model.ReviewComment{
		{ID: 1, Pending: true, Path: "foo.go"},
	})
	fresh := []*model.ReviewComment{
		{ID: 1, Pending: false, Path: "foo.go"},
		{ID: 2, Pending: false, Path: "foo.go"},
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
