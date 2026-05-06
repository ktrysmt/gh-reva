package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ktrysmt/gh-reva/internal/api"
	"github.com/ktrysmt/gh-reva/internal/model"
)

// composeStubClient records what the orchestration layer POSTs and
// returns canned responses so the state machine can be driven without
// touching GitHub. Only the Pending* / Submit methods are exercised
// here; the read-only methods inherit nop behavior from the embedded
// nil interface (any call would panic, signaling unintended use).
type composeStubClient struct {
	api.Client
	thread       api.CreatePendingThreadInput
	threadCalls  int
	replyParent  string
	replyBody    string
	replyCalls   int
	submitEvent  model.SubmitEvent
	submitCalls  int
	submitErr    error
	resp         *model.ReviewComment
	threadErr    error
	replyErr     error
	listResponse []*model.ReviewComment
	listErr      error
}

func (c *composeStubClient) CreatePendingReviewThread(_ context.Context, _, _ string, _ int, in api.CreatePendingThreadInput) (*model.ReviewComment, error) {
	c.threadCalls++
	c.thread = in
	return c.resp, c.threadErr
}

func (c *composeStubClient) CreatePendingReviewThreadReply(_ context.Context, _, _ string, _ int, parentThreadID, body string) (*model.ReviewComment, error) {
	c.replyCalls++
	c.replyParent = parentThreadID
	c.replyBody = body
	return c.resp, c.replyErr
}

func (c *composeStubClient) SubmitPendingReview(_ context.Context, _, _ string, _ int, event model.SubmitEvent, _ string) error {
	c.submitCalls++
	c.submitEvent = event
	return c.submitErr
}

func (c *composeStubClient) ListComments(_ context.Context, _, _ string, _ int) ([]*model.ReviewComment, error) {
	return c.listResponse, c.listErr
}

func newComposeModel(t *testing.T, patch string, comments []*model.ReviewComment) *Model {
	t.Helper()
	m := NewModel(nil, &api.Target{Owner: "octocat", Repo: "hello", Number: 1})
	m.state.PR = &model.PR{
		Owner:    "octocat",
		Repo:     "hello",
		Number:   1,
		HeadSHA:  "head1234",
		Files:    []*model.FileEntry{{Path: "foo.go"}},
		Comments: comments,
	}
	m.state.SelectedFile = "foo.go"
	m.state.DiffCache[diffKey("", "foo.go")] = patch
	m.paneWidthComments = 50
	return &m
}

const composePatch = `--- a/foo.go
+++ b/foo.go
@@ -10,3 +20,4 @@
 unchanged
-removed
+added
+more`

func TestBuildComposeInline_Addition(t *testing.T) {
	m := newComposeModel(t, composePatch, nil)
	m.state.DiffCursor.Line = 5 // "+added"
	if !m.buildComposeInline() {
		t.Fatalf("buildComposeInline returned false")
	}
	cs := m.state.Compose
	if cs == nil || cs.Kind != model.ComposeInline {
		t.Fatalf("ComposeState wrong: %+v", cs)
	}
	if cs.Path != "foo.go" || cs.CommitSHA != "head1234" {
		t.Fatalf("anchor wrong: %+v", cs)
	}
	if cs.Side != "RIGHT" || cs.Line != 21 {
		t.Fatalf("expected RIGHT/21, got side=%s line=%d", cs.Side, cs.Line)
	}
	if cs.Status != model.ComposeEditing {
		t.Fatalf("status must be Editing initially")
	}
}

func TestBuildComposeInline_RejectsHunkHeader(t *testing.T) {
	m := newComposeModel(t, composePatch, nil)
	m.state.DiffCursor.Line = 2 // "@@" hunk
	if m.buildComposeInline() {
		t.Fatalf("buildComposeInline should reject hunk header")
	}
	if m.state.Compose != nil {
		t.Fatalf("Compose must remain nil")
	}
}

func TestBuildComposeInline_VisualRange(t *testing.T) {
	m := newComposeModel(t, composePatch, nil)
	m.state.DiffCursor.Line = 6
	m.state.Visual = &model.VisualState{OriginPane: model.PaneDiff, AnchorLine: 5}
	if !m.buildComposeInline() {
		t.Fatalf("range buildComposeInline returned false")
	}
	cs := m.state.Compose
	if cs.StartLine == nil || *cs.StartLine != 21 {
		t.Fatalf("expected StartLine=21, got %v", cs.StartLine)
	}
	if cs.Line != 22 || cs.Side != "RIGHT" || cs.StartSide != "RIGHT" {
		t.Fatalf("range fields wrong: %+v", cs)
	}
	if m.state.Visual != nil {
		t.Fatalf("visual must be cleared")
	}
}

func TestBuildComposeReply_FindsThreadID(t *testing.T) {
	root := &model.ReviewComment{ID: 100, ThreadID: "PRT_abc", Path: "foo.go", Line: 21, Side: "RIGHT", CreatedAt: time.Unix(1, 0)}
	reply := &model.ReviewComment{ID: 101, ThreadID: "PRT_abc", Path: "foo.go", Line: 21, Side: "RIGHT", InReplyTo: 100, CreatedAt: time.Unix(2, 0)}
	m := newComposeModel(t, composePatch, []*model.ReviewComment{root, reply})
	m.state.DiffCursor.Line = 5
	m.state.CommentsCursor = 1 // reply row
	if !m.buildComposeReply() {
		t.Fatalf("buildComposeReply returned false")
	}
	if m.state.Compose.ParentThreadID != "PRT_abc" {
		t.Fatalf("expected ParentThreadID=PRT_abc, got %q", m.state.Compose.ParentThreadID)
	}
	if m.state.Compose.ParentDBID != 100 {
		t.Fatalf("expected ParentDBID=100, got %d", m.state.Compose.ParentDBID)
	}
}

func TestBuildComposeReply_NoCursorThread(t *testing.T) {
	m := newComposeModel(t, composePatch, nil)
	if m.buildComposeReply() {
		t.Fatalf("reply with no thread should return false")
	}
}

func TestApplyComposeBody_EmptyBodyCancels(t *testing.T) {
	m := newComposeModel(t, composePatch, nil)
	m.state.DiffCursor.Line = 5
	m.buildComposeInline()
	cmd := m.applyComposeBody(composeBodyMsg{body: "  \n\n"})
	if cmd != nil {
		t.Fatalf("empty body must not queue submit")
	}
	if m.state.Compose != nil {
		t.Fatalf("Compose must be cleared")
	}
}

func TestApplyComposeBody_QueuesPendingPOST(t *testing.T) {
	stub := &composeStubClient{resp: &model.ReviewComment{
		ID: 7, NodeID: "PRRC_7", ThreadID: "PRT_7", Path: "foo.go", Body: "ok", Pending: true,
	}}
	m := newComposeModel(t, composePatch, nil)
	m.client = stub
	m.state.DiffCursor.Line = 5
	m.buildComposeInline()
	cmd := m.applyComposeBody(composeBodyMsg{body: "ok"})
	if cmd == nil {
		t.Fatalf("expected submit cmd")
	}
	if m.state.Compose.Status != model.ComposeSubmitting {
		t.Fatalf("status must be Submitting after queueing POST")
	}
	msg := cmd()
	sub, ok := msg.(composeSubmittedMsg)
	if !ok {
		t.Fatalf("expected composeSubmittedMsg, got %T", msg)
	}
	if sub.err != nil {
		t.Fatalf("POST returned err: %v", sub.err)
	}
	if stub.threadCalls != 1 {
		t.Fatalf("expected 1 thread POST, got %d", stub.threadCalls)
	}
	if stub.thread.Path != "foo.go" || stub.thread.Line != 21 || stub.thread.Side != "RIGHT" {
		t.Fatalf("payload wrong: %+v", stub.thread)
	}
}

func TestApplyComposeBody_ReplyRoutesByThreadID(t *testing.T) {
	stub := &composeStubClient{resp: &model.ReviewComment{
		ID: 8, NodeID: "PRRC_8", ThreadID: "PRT_abc", InReplyTo: 100, Body: "+1", Pending: true,
	}}
	root := &model.ReviewComment{ID: 100, ThreadID: "PRT_abc", Path: "foo.go", Line: 21, Side: "RIGHT", CreatedAt: time.Unix(1, 0)}
	m := newComposeModel(t, composePatch, []*model.ReviewComment{root})
	m.client = stub
	m.state.DiffCursor.Line = 5
	m.state.CommentsCursor = 0
	m.buildComposeReply()
	cmd := m.applyComposeBody(composeBodyMsg{body: "+1"})
	if cmd == nil {
		t.Fatalf("expected reply cmd")
	}
	cmd()
	if stub.replyCalls != 1 {
		t.Fatalf("expected 1 reply call, got %d", stub.replyCalls)
	}
	if stub.replyParent != "PRT_abc" || stub.replyBody != "+1" {
		t.Fatalf("reply payload wrong: parent=%q body=%q", stub.replyParent, stub.replyBody)
	}
}

func TestApplyComposeSubmitted_AppendsAndClears(t *testing.T) {
	m := newComposeModel(t, composePatch, nil)
	m.state.DiffCursor.Line = 5
	m.buildComposeInline()
	rc := &model.ReviewComment{ID: 9, Path: "foo.go", Body: "x", Pending: true}
	m.applyComposeSubmitted(composeSubmittedMsg{comment: rc})
	if m.state.Compose != nil {
		t.Fatalf("Compose must be cleared on success")
	}
	if got := len(m.state.PR.Comments); got != 1 || !m.state.PR.Comments[0].Pending {
		t.Fatalf("pending comment not appended: %+v", m.state.PR.Comments)
	}
	if m.state.PR.Files[0].CommentCount != 1 {
		t.Fatalf("CommentCount not bumped")
	}
}

func TestApplyComposeSubmitted_FailureKeepsState(t *testing.T) {
	m := newComposeModel(t, composePatch, nil)
	m.state.DiffCursor.Line = 5
	m.buildComposeInline()
	m.state.Compose.Body = "draft"
	m.state.Compose.Status = model.ComposeSubmitting
	m.applyComposeSubmitted(composeSubmittedMsg{err: errors.New("HTTP 422")})
	if m.state.Compose == nil {
		t.Fatalf("Compose must persist on failure")
	}
	if m.state.Compose.Status != model.ComposeFailed {
		t.Fatalf("status must be Failed")
	}
	if m.state.Compose.Body != "draft" || m.state.Compose.ErrMsg == "" {
		t.Fatalf("body / err not preserved: %+v", m.state.Compose)
	}
}

func TestRetryComposeSubmit_RequiresFailedState(t *testing.T) {
	m := newComposeModel(t, composePatch, nil)
	m.state.DiffCursor.Line = 5
	m.buildComposeInline()
	if cmd := m.retryComposeSubmit(); cmd != nil {
		t.Fatalf("retry should be a no-op outside Failed state")
	}
	m.state.Compose.Body = "draft"
	m.state.Compose.Status = model.ComposeFailed
	stub := &composeStubClient{resp: &model.ReviewComment{ID: 10, Body: "draft", Pending: true}}
	m.client = stub
	cmd := m.retryComposeSubmit()
	if cmd == nil {
		t.Fatalf("retry should queue submit when in Failed")
	}
	cmd()
	if stub.threadCalls != 1 {
		t.Fatalf("retry should re-issue the thread POST, got %d calls", stub.threadCalls)
	}
}
