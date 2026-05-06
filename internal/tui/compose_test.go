package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-reva/internal/api"
	"github.com/ktrysmt/gh-reva/internal/model"
)

// composeStubClient records what the orchestration layer POSTs and
// returns canned responses so the state machine can be driven without
// touching GitHub. Only the Pending* methods are exercised here; the
// read-only methods inherit nop behavior from the embedded nil
// interface (any call would panic, signaling unintended use).
type composeStubClient struct {
	api.Client
	thread       api.CreatePendingThreadInput
	threadCalls  int
	replyParent  string
	replyBody    string
	replyCalls   int
	editNodeID   string
	editBody     string
	editCalls    int
	editErr      error
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

func (c *composeStubClient) UpdateReviewComment(_ context.Context, _, _ string, _ int, commentNodeID, body string) (*model.ReviewComment, error) {
	c.editCalls++
	c.editNodeID = commentNodeID
	c.editBody = body
	return c.resp, c.editErr
}

func (c *composeStubClient) ViewerLogin(_ context.Context) (string, error) {
	return "you", nil
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

// Visual range Enter must NOT clear Visual at build time — the user is
// shown a confirm prompt first, and the highlighted range needs to stay
// on screen while they decide. The Visual clear happens only when
// confirmComposeStart fires (i.e. the user pressed `y`).
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
	if m.state.Visual == nil {
		t.Fatalf("visual must remain set so the highlight stays visible during the confirm prompt")
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
	stub := &composeStubClient{listResponse: []*model.ReviewComment{
		{ID: 9, Path: "foo.go", Body: "x", Pending: true},
	}}
	m := newComposeModel(t, composePatch, nil)
	m.client = stub
	m.state.DiffCursor.Line = 5
	m.buildComposeInline()
	rc := &model.ReviewComment{ID: 9, Path: "foo.go", Body: "x", Pending: true}
	cmd := m.applyComposeSubmitted(composeSubmittedMsg{comment: rc})
	if m.state.Compose != nil {
		t.Fatalf("Compose must be cleared on success")
	}
	if got := len(m.state.PR.Comments); got != 1 || !m.state.PR.Comments[0].Pending {
		t.Fatalf("pending comment not appended: %+v", m.state.PR.Comments)
	}
	if m.state.PR.Files[0].CommentCount != 1 {
		t.Fatalf("CommentCount not bumped")
	}
	if cmd == nil {
		t.Fatalf("expected refresh cmd to be queued after successful POST")
	}
	if _, ok := cmd().(commentsRefreshedMsg); !ok {
		t.Fatalf("expected commentsRefreshedMsg from queued cmd")
	}
}

func TestApplyComposeSubmitted_FailureKeepsState(t *testing.T) {
	m := newComposeModel(t, composePatch, nil)
	m.state.DiffCursor.Line = 5
	m.buildComposeInline()
	m.state.Compose.Body = "draft"
	m.state.Compose.Status = model.ComposeSubmitting
	cmd := m.applyComposeSubmitted(composeSubmittedMsg{err: errors.New("HTTP 422")})
	if cmd != nil {
		t.Fatalf("failure must not queue refresh")
	}
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

func TestShellSingleQuote(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain.md", "'plain.md'"},
		{"/tmp/with space.md", "'/tmp/with space.md'"},
		{"don't.md", `'don'\''t.md'`},
		{"", "''"},
	}
	for _, c := range cases {
		if got := shellSingleQuote(c.in); got != c.want {
			t.Fatalf("shellSingleQuote(%q): got %q want %q", c.in, got, c.want)
		}
	}
}

// Edit gate: cursor on a foreign user's comment must NOT open Compose.
// The TUI relies on this to surface the "cannot edit others' comments"
// hint instead of POSTing into a 403.
func TestBuildComposeEdit_RejectsForeignAuthor(t *testing.T) {
	foreign := &model.ReviewComment{
		ID: 5, NodeID: "PRRC_5", User: "alice", Path: "foo.go", Line: 21, Side: "RIGHT",
		Body: "old", CreatedAt: time.Unix(1, 0),
	}
	m := newComposeModel(t, composePatch, []*model.ReviewComment{foreign})
	m.state.ViewerLogin = "you"
	m.state.DiffCursor.Line = 5
	m.state.CommentsCursor = 0
	if m.buildComposeEdit() {
		t.Fatalf("buildComposeEdit must refuse foreign authors")
	}
	if m.state.Compose != nil {
		t.Fatalf("Compose must remain nil")
	}
}

// Edit happy path: own comment, cursor on it, viewer login known.
// Compose is populated with the existing body so the editor opens on
// the previous text rather than a blank buffer.
func TestBuildComposeEdit_OwnAuthorPreloadsBody(t *testing.T) {
	own := &model.ReviewComment{
		ID: 5, NodeID: "PRRC_5", User: "you", Path: "foo.go", Line: 21, Side: "RIGHT",
		Body: "draft body", CreatedAt: time.Unix(1, 0),
	}
	m := newComposeModel(t, composePatch, []*model.ReviewComment{own})
	m.state.ViewerLogin = "you"
	m.state.DiffCursor.Line = 5
	m.state.CommentsCursor = 0
	if !m.buildComposeEdit() {
		t.Fatalf("buildComposeEdit returned false on own comment")
	}
	cs := m.state.Compose
	if cs.Kind != model.ComposeEdit {
		t.Fatalf("Kind: got %v want ComposeEdit", cs.Kind)
	}
	if cs.EditCommentNodeID != "PRRC_5" {
		t.Fatalf("NodeID: got %q want PRRC_5", cs.EditCommentNodeID)
	}
	if cs.Body != "draft body" {
		t.Fatalf("Body must preload original: got %q", cs.Body)
	}
}

// Edit POST routes via UpdateReviewComment; success applies the body
// in-place on the existing comment (no append, no count bump).
func TestApplyComposeBody_EditRoutesByNodeID(t *testing.T) {
	own := &model.ReviewComment{
		ID: 5, NodeID: "PRRC_5", User: "you", Path: "foo.go", Line: 21, Side: "RIGHT",
		Body: "old", CreatedAt: time.Unix(1, 0),
	}
	stub := &composeStubClient{resp: &model.ReviewComment{
		ID: 5, NodeID: "PRRC_5", User: "you", Body: "new", Pending: false,
	}}
	m := newComposeModel(t, composePatch, []*model.ReviewComment{own})
	m.client = stub
	m.state.ViewerLogin = "you"
	m.state.DiffCursor.Line = 5
	m.state.CommentsCursor = 0
	if !m.buildComposeEdit() {
		t.Fatalf("buildComposeEdit returned false")
	}
	cmd := m.applyComposeBody(composeBodyMsg{body: "new"})
	if cmd == nil {
		t.Fatalf("expected submit cmd")
	}
	cmd()
	if stub.editCalls != 1 {
		t.Fatalf("expected 1 update call, got %d", stub.editCalls)
	}
	if stub.editNodeID != "PRRC_5" || stub.editBody != "new" {
		t.Fatalf("update payload wrong: nodeID=%q body=%q", stub.editNodeID, stub.editBody)
	}
}

// Edit success applies the new body onto the existing comment in-place
// — no duplicate appended, no CommentCount bump.
func TestApplyComposeSubmitted_EditReplacesByNodeID(t *testing.T) {
	own := &model.ReviewComment{
		ID: 5, NodeID: "PRRC_5", User: "you", Path: "foo.go", Body: "old",
	}
	m := newComposeModel(t, composePatch, []*model.ReviewComment{own})
	m.state.PR.Files[0].CommentCount = 1
	m.state.Compose = &model.ComposeState{
		Kind: model.ComposeEdit, Status: model.ComposeSubmitting,
		EditCommentNodeID: "PRRC_5", Body: "new",
	}
	rc := &model.ReviewComment{ID: 5, NodeID: "PRRC_5", User: "you", Body: "new", Pending: false}
	m.applyComposeSubmitted(composeSubmittedMsg{comment: rc})
	if got := len(m.state.PR.Comments); got != 1 {
		t.Fatalf("edit must not duplicate, got %d", got)
	}
	if m.state.PR.Comments[0].Body != "new" {
		t.Fatalf("body must be updated: %q", m.state.PR.Comments[0].Body)
	}
	if m.state.PR.Files[0].CommentCount != 1 {
		t.Fatalf("edit must not bump CommentCount, got %d", m.state.PR.Files[0].CommentCount)
	}
}

// Files modal Enter (flat mode) closes the modal and shifts focus to
// Diff so the user can immediately scroll the patch they just picked.
func TestHandleKey_FilesFlatModalEnterShiftsToDiff(t *testing.T) {
	mv := newComposeModel(t, composePatch, nil)
	mv.state.ViewerLogin = "you"
	mv.state.FocusedPane = model.PaneFiles
	mv.state.Modal = &model.ModalState{Pane: model.PaneFiles}
	updated, _ := mv.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.state.Modal != nil {
		t.Fatalf("Modal must close on Files-flat Enter, got %+v", got.state.Modal)
	}
	if got.state.FocusedPane != model.PaneDiff {
		t.Fatalf("focus must shift to Diff, got %v", got.state.FocusedPane)
	}
}

// Commits modal Enter likewise hands off to Diff.
func TestHandleKey_CommitsModalEnterShiftsToDiff(t *testing.T) {
	mv := newComposeModel(t, composePatch, nil)
	mv.state.ViewerLogin = "you"
	mv.state.FocusedPane = model.PaneCommits
	mv.state.Modal = &model.ModalState{Pane: model.PaneCommits}
	updated, _ := mv.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.state.Modal != nil {
		t.Fatalf("Modal must close on Commits Enter")
	}
	if got.state.FocusedPane != model.PaneDiff {
		t.Fatalf("focus must shift to Diff, got %v", got.state.FocusedPane)
	}
}

// Diff Enter on a row that already has anchored comments hands off to
// the Comments zoom modal instead of opening Compose directly. Lets
// users see the existing comments before deciding whether to add a new
// thread (`n`-equivalent through r/Enter inside the modal) or edit /
// reply.
func TestHandleKeyDiff_EnterOnCommentedRowOpensModal(t *testing.T) {
	root := &model.ReviewComment{
		ID: 11, NodeID: "PRRC_11", ThreadID: "PRT_a", User: "alice",
		Path: "foo.go", Line: 21, Side: "RIGHT", CreatedAt: time.Unix(1, 0),
	}
	mv := newComposeModel(t, composePatch, []*model.ReviewComment{root})
	mv.state.ViewerLogin = "you"
	mv.state.DiffCursor.Line = 5 // anchored to line 21 in composePatch
	mv.paneHeightDiff = 10       // viewport height for cursor clamp
	updated, _ := mv.handleKeyDiff(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.state.Modal == nil || got.state.Modal.Pane != model.PaneComments {
		t.Fatalf("expected Comments modal, got %+v", got.state.Modal)
	}
	if got.state.FocusedPane != model.PaneComments {
		t.Fatalf("focus must shift to Comments, got %v", got.state.FocusedPane)
	}
	if got.state.Compose != nil {
		t.Fatalf("Compose must NOT open on commented-row Enter")
	}
	if got.state.CommentsCursor != 0 {
		t.Fatalf("CommentsCursor must reset to 0, got %d", got.state.CommentsCursor)
	}
}

// Diff Enter on a row with NO existing comments queues the confirm
// prompt (PendingConfirm) for inline compose. The actual compose state
// is held inside PendingConfirm — Compose is not populated until the
// user presses `y`. Modal is left untouched.
func TestHandleKeyDiff_EnterOnUncommentedRowQueuesConfirm(t *testing.T) {
	mv := newComposeModel(t, composePatch, nil)
	mv.state.ViewerLogin = "you"
	mv.state.DiffCursor.Line = 5
	mv.paneHeightDiff = 10
	_, cmd := mv.handleKeyDiff(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("Enter must NOT launch the editor before the user confirms")
	}
	if mv.state.Modal != nil {
		t.Fatalf("Modal must remain nil for uncommented Enter")
	}
	if mv.state.Compose != nil {
		t.Fatalf("Compose must stay nil until the user confirms with y")
	}
	pc := mv.state.PendingConfirm
	if pc == nil || pc.Kind != model.ComposeInline {
		t.Fatalf("PendingConfirm must be ComposeInline, got %+v", pc)
	}
	if pc.Compose == nil || pc.Compose.Kind != model.ComposeInline {
		t.Fatalf("PendingConfirm.Compose must carry the built ComposeState")
	}
}

// Comments Enter on a foreign user's comment must surface a status-bar
// notice instead of opening Compose. The notice steers the user to `r`
// for reply.
func TestHandleKeyComments_EnterOnForeignSetsNotice(t *testing.T) {
	foreign := &model.ReviewComment{
		ID: 5, NodeID: "PRRC_5", User: "alice", Path: "foo.go", Line: 21, Side: "RIGHT",
		Body: "from alice", CreatedAt: time.Unix(1, 0),
	}
	mv := newComposeModel(t, composePatch, []*model.ReviewComment{foreign})
	mv.state.ViewerLogin = "you"
	mv.state.DiffCursor.Line = 5
	mv.state.CommentsCursor = 0
	updated, cmd := mv.handleKeyComments(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("Enter on foreign comment must NOT queue a Compose cmd")
	}
	got := updated.(Model)
	if got.state.Notice == "" {
		t.Fatalf("Notice must be set on foreign-user Enter")
	}
	if got.state.Compose != nil {
		t.Fatalf("Compose must remain nil")
	}
}

// Comments r on any thread (own or foreign) must queue the reply
// confirm prompt — the keymap split moved reply from Enter to r, and
// confirm gating defers the editor launch until the user presses `y`.
func TestHandleKeyComments_RQueuesReplyConfirm(t *testing.T) {
	root := &model.ReviewComment{
		ID: 100, NodeID: "PRRC_100", ThreadID: "PRT_abc", User: "alice",
		Path: "foo.go", Line: 21, Side: "RIGHT", CreatedAt: time.Unix(1, 0),
	}
	mv := newComposeModel(t, composePatch, []*model.ReviewComment{root})
	mv.state.ViewerLogin = "you"
	mv.state.DiffCursor.Line = 5
	mv.state.CommentsCursor = 0
	updated, cmd := mv.handleKeyComments(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd != nil {
		t.Fatalf("r must NOT launch editing before the user confirms")
	}
	got := updated.(Model)
	if got.state.Compose != nil {
		t.Fatalf("Compose must stay nil until y; got %+v", got.state.Compose)
	}
	pc := got.state.PendingConfirm
	if pc == nil || pc.Kind != model.ComposeReply {
		t.Fatalf("PendingConfirm must be ComposeReply, got %+v", pc)
	}
}

// Confirm gate (Diff Enter / Comments Enter / Comments r) — the editor
// launch is held until the user presses `y`. `n`, `Esc`, `q`, `Ctrl+C`
// cancel; other keystrokes are absorbed so navigation stays frozen
// while the prompt is up.

func TestStartComposeReply_QueuesPendingConfirm(t *testing.T) {
	root := &model.ReviewComment{
		ID: 100, NodeID: "PRRC_100", ThreadID: "PRT_abc", User: "alice",
		Path: "foo.go", Line: 21, Side: "RIGHT", CreatedAt: time.Unix(1, 0),
	}
	m := newComposeModel(t, composePatch, []*model.ReviewComment{root})
	m.state.ViewerLogin = "you"
	m.state.DiffCursor.Line = 5
	m.state.CommentsCursor = 0
	if cmd := m.startComposeReply(); cmd != nil {
		t.Fatalf("startComposeReply must NOT return an editor cmd before confirm")
	}
	if m.state.Compose != nil {
		t.Fatalf("Compose must stay nil until confirm")
	}
	pc := m.state.PendingConfirm
	if pc == nil || pc.Kind != model.ComposeReply {
		t.Fatalf("PendingConfirm must be ComposeReply, got %+v", pc)
	}
}

func TestStartComposeEdit_QueuesPendingConfirm(t *testing.T) {
	own := &model.ReviewComment{
		ID: 5, NodeID: "PRRC_5", User: "you", Path: "foo.go", Line: 21, Side: "RIGHT",
		Body: "draft", CreatedAt: time.Unix(1, 0),
	}
	m := newComposeModel(t, composePatch, []*model.ReviewComment{own})
	m.state.ViewerLogin = "you"
	m.state.DiffCursor.Line = 5
	m.state.CommentsCursor = 0
	if cmd := m.startComposeEdit(); cmd != nil {
		t.Fatalf("startComposeEdit must NOT return an editor cmd before confirm")
	}
	if m.state.Compose != nil {
		t.Fatalf("Compose must stay nil until confirm")
	}
	pc := m.state.PendingConfirm
	if pc == nil || pc.Kind != model.ComposeEdit {
		t.Fatalf("PendingConfirm must be ComposeEdit, got %+v", pc)
	}
}

// y consumes the prompt: PendingConfirm clears, Compose receives the
// previously-built state, the editor cmd is returned (textarea fallback
// in this test since EDITOR is unset).
func TestHandleKey_PendingConfirmYStartsEditing(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")
	mv := newComposeModel(t, composePatch, nil)
	mv.state.ViewerLogin = "you"
	mv.state.DiffCursor.Line = 5
	mv.paneHeightDiff = 10
	if _, _ = mv.handleKeyDiff(tea.KeyMsg{Type: tea.KeyEnter}); mv.state.PendingConfirm == nil {
		t.Fatalf("precondition: PendingConfirm must be set after Diff Enter")
	}
	updated, _ := mv.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	got := updated.(Model)
	if got.state.PendingConfirm != nil {
		t.Fatalf("PendingConfirm must clear on y")
	}
	if got.state.Compose == nil || got.state.Compose.Kind != model.ComposeInline {
		t.Fatalf("Compose must be installed on y, got %+v", got.state.Compose)
	}
	if !got.state.Compose.UseTextarea {
		t.Fatalf("textarea fallback must be set when EDITOR is unset")
	}
}

// y on an inline-range confirm must clear Visual at the moment of
// commit. Until y, the highlighted range stays visible.
func TestHandleKey_PendingConfirmYClearsVisualOnInlineRange(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")
	mv := newComposeModel(t, composePatch, nil)
	mv.state.ViewerLogin = "you"
	mv.state.DiffCursor.Line = 6
	mv.state.Visual = &model.VisualState{OriginPane: model.PaneDiff, AnchorLine: 5}
	mv.paneHeightDiff = 10
	mv.startComposeInline()
	if mv.state.Visual == nil {
		t.Fatalf("precondition: Visual must remain set during the confirm prompt")
	}
	updated, _ := mv.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	got := updated.(Model)
	if got.state.Visual != nil {
		t.Fatalf("y must clear Visual when committing an inline-range compose")
	}
}

// n / Esc / q / Ctrl+C cancel the confirm without launching the editor.
// Compose stays nil, PendingConfirm clears.
func TestHandleKey_PendingConfirmNCancels(t *testing.T) {
	mv := newComposeModel(t, composePatch, nil)
	mv.state.ViewerLogin = "you"
	mv.state.DiffCursor.Line = 5
	mv.paneHeightDiff = 10
	mv.startComposeInline()
	if mv.state.PendingConfirm == nil {
		t.Fatalf("precondition: PendingConfirm must be set")
	}
	updated, cmd := mv.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatalf("n must not launch editing")
	}
	if got.state.PendingConfirm != nil {
		t.Fatalf("PendingConfirm must clear on n")
	}
	if got.state.Compose != nil {
		t.Fatalf("Compose must stay nil on n")
	}
}

func TestHandleKey_PendingConfirmEscCancels(t *testing.T) {
	mv := newComposeModel(t, composePatch, nil)
	mv.state.ViewerLogin = "you"
	mv.state.DiffCursor.Line = 5
	mv.paneHeightDiff = 10
	mv.startComposeInline()
	updated, _ := mv.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(Model)
	if got.state.PendingConfirm != nil {
		t.Fatalf("PendingConfirm must clear on Esc")
	}
	if got.state.Compose != nil {
		t.Fatalf("Compose must stay nil on Esc")
	}
}

func TestHandleKey_PendingConfirmQCancelsWithoutQuitting(t *testing.T) {
	mv := newComposeModel(t, composePatch, nil)
	mv.state.ViewerLogin = "you"
	mv.state.DiffCursor.Line = 5
	mv.paneHeightDiff = 10
	mv.startComposeInline()
	updated, cmd := mv.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatalf("q during confirm must NOT quit the program; got cmd %v", cmd)
	}
	if got.state.PendingConfirm != nil {
		t.Fatalf("PendingConfirm must clear on q")
	}
}

// Other keystrokes are absorbed: navigation must not move while the
// confirm prompt is up. Diff cursor stays at the original row, focus
// stays where it was.
func TestHandleKey_PendingConfirmAbsorbsOtherKeys(t *testing.T) {
	mv := newComposeModel(t, composePatch, nil)
	mv.state.ViewerLogin = "you"
	mv.state.DiffCursor.Line = 5
	mv.state.FocusedPane = model.PaneDiff
	mv.paneHeightDiff = 10
	mv.startComposeInline()
	if mv.state.PendingConfirm == nil {
		t.Fatalf("precondition: PendingConfirm set")
	}
	updated, _ := mv.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	got := updated.(Model)
	if got.state.PendingConfirm == nil {
		t.Fatalf("j must NOT cancel the confirm prompt")
	}
	if got.state.DiffCursor.Line != 5 {
		t.Fatalf("DiffCursor must not move while confirm prompt is up, got %d", got.state.DiffCursor.Line)
	}
	if got.state.FocusedPane != model.PaneDiff {
		t.Fatalf("FocusedPane must not change, got %v", got.state.FocusedPane)
	}
}

// The status bar replaces its per-pane keymap with a confirm prompt
// while PendingConfirm is set. Each compose kind has its own label.
func TestStatusBarContent_ShowsConfirmPrompt(t *testing.T) {
	cases := []struct {
		kind model.ComposeKind
		want string
	}{
		{model.ComposeInline, "start new comment? [y]es [n]o"},
		{model.ComposeReply, "post reply? [y]es [n]o"},
		{model.ComposeEdit, "edit comment? [y]es [n]o"},
	}
	for _, c := range cases {
		mv := newComposeModel(t, composePatch, nil)
		mv.state.PendingConfirm = &model.PendingConfirm{
			Kind:    c.kind,
			Compose: &model.ComposeState{Kind: c.kind, Status: model.ComposeEditing},
		}
		ctx, suffix := mv.statusBarContent()
		if ctx != c.want {
			t.Fatalf("kind=%v: got %q want %q", c.kind, ctx, c.want)
		}
		if suffix != "" {
			t.Fatalf("kind=%v: confirm prompt must drop the common suffix, got %q", c.kind, suffix)
		}
	}
}

// Modal opened via Diff Enter on a commented row must record Origin =
// Diff so that the close gesture (space, q, Esc, Ctrl+C) returns focus
// to Diff. Without this, focus would linger on Comments after the
// modal closes — non-obvious because the user came from Diff.
func TestOpenCommentsModalAtCursor_RecordsOriginAsDiff(t *testing.T) {
	root := &model.ReviewComment{
		ID: 11, NodeID: "PRRC_11", ThreadID: "PRT_a", User: "alice",
		Path: "foo.go", Line: 21, Side: "RIGHT", CreatedAt: time.Unix(1, 0),
	}
	mv := newComposeModel(t, composePatch, []*model.ReviewComment{root})
	mv.state.FocusedPane = model.PaneDiff
	mv.state.DiffCursor.Line = 5
	mv.openCommentsModalAtCursor()
	if mv.state.Modal == nil || mv.state.Modal.Origin != model.PaneDiff {
		t.Fatalf("Origin must be PaneDiff (where the user opened from), got %+v", mv.state.Modal)
	}
}

// Toggling the modal via space records Origin = current focused pane,
// so the close gesture returns the user to where they were.
func TestToggleModal_RecordsCurrentFocusAsOrigin(t *testing.T) {
	mv := newComposeModel(t, composePatch, nil)
	mv.state.FocusedPane = model.PaneFiles
	mv.toggleModal(model.PaneFiles)
	if mv.state.Modal == nil || mv.state.Modal.Origin != model.PaneFiles {
		t.Fatalf("Files toggle: Origin must be PaneFiles, got %+v", mv.state.Modal)
	}
	mv.state.Modal = nil
	mv.state.FocusedPane = model.PaneComments
	mv.toggleModal(model.PaneComments)
	if mv.state.Modal == nil || mv.state.Modal.Origin != model.PaneComments {
		t.Fatalf("Comments toggle: Origin must be PaneComments, got %+v", mv.state.Modal)
	}
}

// Closing via space (toggleModal again) restores focus to Origin.
// When Comments modal was opened from Diff (handoff via Enter), the
// close gesture must return focus to Diff — not leave it on Comments
// where openCommentsModalAtCursor parked it for in-modal navigation.
func TestSpaceClose_RestoresFocusToDiffWhenOpenedFromDiff(t *testing.T) {
	root := &model.ReviewComment{
		ID: 11, NodeID: "PRRC_11", ThreadID: "PRT_a", User: "alice",
		Path: "foo.go", Line: 21, Side: "RIGHT", CreatedAt: time.Unix(1, 0),
	}
	mv := newComposeModel(t, composePatch, []*model.ReviewComment{root})
	mv.state.FocusedPane = model.PaneDiff
	mv.state.DiffCursor.Line = 5
	mv.openCommentsModalAtCursor()
	if mv.state.FocusedPane != model.PaneComments {
		t.Fatalf("precondition: focus must shift to Comments while modal is open")
	}
	updated, _ := mv.handleKeyComments(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	got := updated.(Model)
	if got.state.Modal != nil {
		t.Fatalf("space must close the Comments modal")
	}
	if got.state.FocusedPane != model.PaneDiff {
		t.Fatalf("focus must return to Diff (the opener), got %v", got.state.FocusedPane)
	}
}

// Symmetric case: opened via space from Comments, closed via space —
// focus stays on Comments.
func TestSpaceClose_StaysOnCommentsWhenOpenedFromComments(t *testing.T) {
	root := &model.ReviewComment{
		ID: 11, NodeID: "PRRC_11", ThreadID: "PRT_a", User: "alice",
		Path: "foo.go", Line: 21, Side: "RIGHT", CreatedAt: time.Unix(1, 0),
	}
	mv := newComposeModel(t, composePatch, []*model.ReviewComment{root})
	mv.state.FocusedPane = model.PaneComments
	mv.state.DiffCursor.Line = 5
	mv.toggleModal(model.PaneComments)
	if mv.state.Modal == nil || mv.state.FocusedPane != model.PaneComments {
		t.Fatalf("precondition: Comments modal open, focus on Comments")
	}
	updated, _ := mv.handleKeyComments(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	got := updated.(Model)
	if got.state.Modal != nil {
		t.Fatalf("space must close the modal")
	}
	if got.state.FocusedPane != model.PaneComments {
		t.Fatalf("focus must stay on Comments (the opener), got %v", got.state.FocusedPane)
	}
}

// q close: modal opened from Diff → q returns focus to Diff (without
// quitting the app).
func TestQClose_RestoresFocusToOrigin(t *testing.T) {
	root := &model.ReviewComment{
		ID: 11, NodeID: "PRRC_11", ThreadID: "PRT_a", User: "alice",
		Path: "foo.go", Line: 21, Side: "RIGHT", CreatedAt: time.Unix(1, 0),
	}
	mv := newComposeModel(t, composePatch, []*model.ReviewComment{root})
	mv.state.FocusedPane = model.PaneDiff
	mv.state.DiffCursor.Line = 5
	mv.openCommentsModalAtCursor()
	updated, cmd := mv.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatalf("q must not quit while modal is open; got cmd %v", cmd)
	}
	if got.state.Modal != nil {
		t.Fatalf("q must close the modal")
	}
	if got.state.FocusedPane != model.PaneDiff {
		t.Fatalf("focus must return to Diff, got %v", got.state.FocusedPane)
	}
}

func TestEscClose_RestoresFocusToOrigin(t *testing.T) {
	root := &model.ReviewComment{
		ID: 11, NodeID: "PRRC_11", ThreadID: "PRT_a", User: "alice",
		Path: "foo.go", Line: 21, Side: "RIGHT", CreatedAt: time.Unix(1, 0),
	}
	mv := newComposeModel(t, composePatch, []*model.ReviewComment{root})
	mv.state.FocusedPane = model.PaneDiff
	mv.state.DiffCursor.Line = 5
	mv.openCommentsModalAtCursor()
	updated, _ := mv.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(Model)
	if got.state.Modal != nil {
		t.Fatalf("Esc must close the modal")
	}
	if got.state.FocusedPane != model.PaneDiff {
		t.Fatalf("focus must return to Diff on Esc, got %v", got.state.FocusedPane)
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
