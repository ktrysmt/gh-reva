package tui

import (
	"context"
	"errors"
	"strings"
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
	m.state.DiffCursor.Line = 6 // "+added"
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

func TestBuildComposeInline_ContextLineFollowsCursorSide(t *testing.T) {
	// Context buffer line at index 3 in composePatch is ` unchanged`,
	// with oldLine=10 / newLine=20. Cursor.Side decides which file-line
	// number and which Side string the compose payload carries.
	t.Run("RIGHT", func(t *testing.T) {
		m := newComposeModel(t, composePatch, nil)
		m.state.DiffCursor.Line = 4
		m.state.DiffCursor.Side = model.DiffSideRight
		if !m.buildComposeInline() {
			t.Fatalf("buildComposeInline returned false")
		}
		cs := m.state.Compose
		if cs.Side != "RIGHT" || cs.Line != 20 {
			t.Fatalf("RIGHT context anchor: got side=%s line=%d, want RIGHT/20", cs.Side, cs.Line)
		}
	})
	t.Run("LEFT", func(t *testing.T) {
		m := newComposeModel(t, composePatch, nil)
		m.state.DiffCursor.Line = 4
		m.state.DiffCursor.Side = model.DiffSideLeft
		if !m.buildComposeInline() {
			t.Fatalf("buildComposeInline returned false")
		}
		cs := m.state.Compose
		if cs.Side != "LEFT" || cs.Line != 10 {
			t.Fatalf("LEFT context anchor: got side=%s line=%d, want LEFT/10", cs.Side, cs.Line)
		}
	})
}

func TestBuildComposeInline_PlusLineIgnoresLeftCursor(t *testing.T) {
	// `+added` (buffer 5) only exists on RIGHT — even if the cursor
	// somehow held Side=LEFT (it cannot under j/k auto-skip, but the
	// test pins the safety contract), the compose anchor must still
	// resolve to RIGHT because that is where the line lives.
	m := newComposeModel(t, composePatch, nil)
	m.state.DiffCursor.Line = 6
	m.state.DiffCursor.Side = model.DiffSideLeft
	if !m.buildComposeInline() {
		t.Fatalf("buildComposeInline returned false")
	}
	cs := m.state.Compose
	if cs.Side != "RIGHT" {
		t.Fatalf("`+` anchor must stay RIGHT regardless of cursor.Side; got %s", cs.Side)
	}
}

func TestBuildComposeInline_RejectsHunkHeader(t *testing.T) {
	m := newComposeModel(t, composePatch, nil)
	m.state.DiffCursor.Line = 3 // "@@" hunk
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
	m.state.DiffCursor.Line = 7
	m.state.Visual = &model.VisualState{OriginPane: model.PaneDiff, AnchorLine: 6}
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
	m.state.DiffCursor.Line = 6
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
	m.state.DiffCursor.Line = 6
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
	m.state.DiffCursor.Line = 6
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
	m.state.DiffCursor.Line = 6
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
	m.state.DiffCursor.Line = 6
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
	m.state.DiffCursor.Line = 6
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

// A successful submit auto-reveals the Comments column if the user had
// hidden it via Ctrl+E. Without auto-reveal the freshly-posted draft
// would not be visible — the user posted from Diff while Comments was
// hidden, and Tab / Shift+Tab skip Comments while hidden, so they
// would have to remember the toggle gesture before they could see
// what they just wrote.
func TestApplyComposeSubmitted_RevealsHiddenCommentsOnSuccess(t *testing.T) {
	m := newComposeModel(t, composePatch, nil)
	m.state.CommentsHidden = true
	m.state.DiffCursor.Line = 6
	m.buildComposeInline()
	rc := &model.ReviewComment{
		ID: 99, Body: "draft", Path: "foo.go", Line: 21,
		Pending: true, CreatedAt: time.Now(),
	}
	m.applyComposeSubmitted(composeSubmittedMsg{comment: rc})
	if m.state.CommentsHidden {
		t.Fatalf("Comments column must auto-reveal after a successful submit")
	}
}

// Submission failure leaves CommentsHidden alone — we don't want to
// override the user's deliberate toggle on every transient API hiccup;
// they'll see the column on the eventual successful retry.
func TestApplyComposeSubmitted_KeepsHiddenOnFailure(t *testing.T) {
	m := newComposeModel(t, composePatch, nil)
	m.state.CommentsHidden = true
	m.state.DiffCursor.Line = 6
	m.buildComposeInline()
	m.state.Compose.Body = "draft"
	m.applyComposeSubmitted(composeSubmittedMsg{err: errors.New("HTTP 500")})
	if !m.state.CommentsHidden {
		t.Fatalf("CommentsHidden must stay true on failure (user toggle wins)")
	}
}

// vim / nvim launches should auto-enter Insert mode so a fresh comment
// can be typed without an extra `i` keystroke. Detection runs against
// the first whitespace-separated token of $EDITOR (so flags like
// `nvim +Glog` still match) after stripping any leading directory and
// `.exe` suffix. Non-vim editors return an empty flag and the launch
// command is unchanged.
func TestStartInsertFlag(t *testing.T) {
	cases := []struct {
		editor string
		want   string
	}{
		{"vim", " +startinsert"},
		{"nvim", " +startinsert"},
		{"vi", " +startinsert"},
		{"gvim", " +startinsert"},
		{"mvim", " +startinsert"},
		{"/usr/bin/vim", " +startinsert"},
		{"/usr/local/bin/nvim", " +startinsert"},
		{"vim -p", " +startinsert"},
		{"nvim +Glog", " +startinsert"},
		{"nvim.exe", " +startinsert"},
		{"code --wait", ""},
		{"emacs", ""},
		{"nano", ""},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := startInsertFlag(c.editor); got != c.want {
			t.Errorf("startInsertFlag(%q): got %q want %q", c.editor, got, c.want)
		}
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
	m.state.DiffCursor.Line = 6
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
	m.state.DiffCursor.Line = 6
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
	m.state.DiffCursor.Line = 6
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

// Diff Enter on a row that already has anchored comments shifts focus
// to the Comments pane instead of opening Compose. The user inspects
// the existing threads via the Comments-pane keymap (Enter = edit own /
// `r` = reply); pressing `<space>` from there opens the zoom modal for
// a wider read. The previous behavior — auto-opening the Comments zoom
// modal — was retired once Ctrl+E gave the column a stable visibility
// gesture; the modal added a layer of UI without earning its keystroke.
func TestHandleKeyDiff_EnterOnCommentedRowFocusesComments(t *testing.T) {
	root := &model.ReviewComment{
		ID: 11, NodeID: "PRRC_11", ThreadID: "PRT_a", User: "alice",
		Path: "foo.go", Line: 21, Side: "RIGHT", CreatedAt: time.Unix(1, 0),
	}
	mv := newComposeModel(t, composePatch, []*model.ReviewComment{root})
	mv.state.ViewerLogin = "you"
	mv.state.DiffCursor.Line = 6 // anchored to line 21 in composePatch
	mv.paneHeightDiff = 10       // viewport height for cursor clamp
	updated, _ := mv.handleKeyDiff(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.state.Modal != nil {
		t.Fatalf("Modal must NOT open on commented-row Enter; got %+v", got.state.Modal)
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

// Diff Enter on a commented row while the Comments column is hidden
// (Ctrl+E) auto-reveals the column before shifting focus, so the user
// always lands on a visible pane. Without auto-reveal the focus would
// strand on an invisible target — Tab / Shift+Tab skip Comments while
// hidden, so the user could not navigate back without first re-opening.
func TestHandleKeyDiff_EnterOnCommentedRowAutoRevealsHiddenColumn(t *testing.T) {
	root := &model.ReviewComment{
		ID: 11, NodeID: "PRRC_11", ThreadID: "PRT_a", User: "alice",
		Path: "foo.go", Line: 21, Side: "RIGHT", CreatedAt: time.Unix(1, 0),
	}
	mv := newComposeModel(t, composePatch, []*model.ReviewComment{root})
	mv.state.ViewerLogin = "you"
	mv.state.CommentsHidden = true
	mv.state.DiffCursor.Line = 6
	mv.paneHeightDiff = 10
	updated, _ := mv.handleKeyDiff(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.state.CommentsHidden {
		t.Fatalf("Diff Enter on a commented row must auto-reveal the Comments column")
	}
	if got.state.FocusedPane != model.PaneComments {
		t.Fatalf("focus must shift to Comments, got %v", got.state.FocusedPane)
	}
	if got.state.Modal != nil {
		t.Fatalf("Modal must NOT open; got %+v", got.state.Modal)
	}
}

// Diff Enter on a row with NO existing comments queues the confirm
// prompt (PendingConfirm) for inline compose. The actual compose state
// is held inside PendingConfirm — Compose is not populated until the
// user presses `y`. Modal is left untouched.
func TestHandleKeyDiff_EnterOnUncommentedRowQueuesConfirm(t *testing.T) {
	mv := newComposeModel(t, composePatch, nil)
	mv.state.ViewerLogin = "you"
	mv.state.DiffCursor.Line = 6
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
	mv.state.DiffCursor.Line = 6
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
	mv.state.DiffCursor.Line = 6
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
	m.state.DiffCursor.Line = 6
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
	m.state.DiffCursor.Line = 6
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
	mv.state.DiffCursor.Line = 6
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

// Enter is a synonym for y on the confirm prompt — matches the
// "press enter to commit" muscle memory most TUI confirm dialogs use.
// The cancel set (n / Esc / q / Ctrl+C) is unaffected.
func TestHandleKey_PendingConfirmEnterStartsEditing(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")
	mv := newComposeModel(t, composePatch, nil)
	mv.state.ViewerLogin = "you"
	mv.state.DiffCursor.Line = 6
	mv.paneHeightDiff = 10
	if _, _ = mv.handleKeyDiff(tea.KeyMsg{Type: tea.KeyEnter}); mv.state.PendingConfirm == nil {
		t.Fatalf("precondition: PendingConfirm must be set after Diff Enter")
	}
	updated, _ := mv.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.state.PendingConfirm != nil {
		t.Fatalf("PendingConfirm must clear on Enter")
	}
	if got.state.Compose == nil || got.state.Compose.Kind != model.ComposeInline {
		t.Fatalf("Compose must be installed on Enter, got %+v", got.state.Compose)
	}
}

// y on an inline-range confirm must clear Visual at the moment of
// commit. Until y, the highlighted range stays visible.
func TestHandleKey_PendingConfirmYClearsVisualOnInlineRange(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")
	mv := newComposeModel(t, composePatch, nil)
	mv.state.ViewerLogin = "you"
	mv.state.DiffCursor.Line = 7
	mv.state.Visual = &model.VisualState{OriginPane: model.PaneDiff, AnchorLine: 6}
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
	mv.state.DiffCursor.Line = 6
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
	mv.state.DiffCursor.Line = 6
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
	mv.state.DiffCursor.Line = 6
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
	mv.state.DiffCursor.Line = 6
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
	if got.state.DiffCursor.Line != 6 {
		t.Fatalf("DiffCursor must not move while confirm prompt is up, got %d", got.state.DiffCursor.Line)
	}
	if got.state.FocusedPane != model.PaneDiff {
		t.Fatalf("FocusedPane must not change, got %v", got.state.FocusedPane)
	}
}

// The confirm prompt is owned by the centered modal, not the status bar.
// While PendingConfirm is set the status bar reverts to whatever the
// focused pane normally shows (suffix preserved); the y/n decision is
// surfaced exclusively through overlayConfirm. Locks the contract that
// a future change cannot duplicate the prompt back into the bar.
func TestStatusBarContent_DoesNotMirrorConfirm(t *testing.T) {
	cases := []model.ComposeKind{model.ComposeInline, model.ComposeReply, model.ComposeEdit}
	for _, k := range cases {
		mv := newComposeModel(t, composePatch, nil)
		mv.state.FocusedPane = model.PaneDiff
		mv.state.PendingConfirm = &model.PendingConfirm{
			Kind:    k,
			Compose: &model.ComposeState{Kind: k, Status: model.ComposeEditing},
		}
		ctx, suffix := mv.statusBarContent()
		if strings.Contains(ctx, "[y]es") || strings.Contains(ctx, "[n]o") {
			t.Fatalf("kind=%v: confirm prompt must NOT appear in status bar; got %q", k, ctx)
		}
		if ctx != mv.diffHint() {
			t.Fatalf("kind=%v: status bar should show focused-pane hint, got %q", k, ctx)
		}
		if suffix != statusCommonSuffix {
			t.Fatalf("kind=%v: common suffix must persist, got %q", k, suffix)
		}
	}
}

// The confirm modal renders a title (action verb), a target subject
// (path:line + side), and the [y]es / [n]o footer. Locks the user-
// visible contract — both halves of the prompt must reach the screen.
func TestOverlayConfirm_RendersTitleSubjectAndKeymap(t *testing.T) {
	mv := newComposeModel(t, composePatch, nil)
	mv.width = 80
	mv.height = 24
	mv.paneHeightDiff = 10
	mv.state.DiffCursor.Line = 6
	mv.startComposeInline()
	if mv.state.PendingConfirm == nil {
		t.Fatalf("precondition: PendingConfirm set")
	}
	body := strings.Repeat(strings.Repeat(" ", mv.width)+"\n", mv.height-1) + strings.Repeat(" ", mv.width)
	out := mv.overlayConfirm(body)
	for _, want := range []string{"Start new comment?", "foo.go:21 RIGHT", "[y]es", "[n]o"} {
		if !strings.Contains(out, want) {
			t.Fatalf("modal missing %q\n--- modal ---\n%s", want, out)
		}
	}
}

// Reply confirm derives its subject from the parent thread's root
// comment so the user can see WHO they are replying to before paying
// the editor open.
func TestOverlayConfirm_ReplySubjectFromRoot(t *testing.T) {
	root := &model.ReviewComment{
		ID: 100, NodeID: "PRRC_100", ThreadID: "PRT_abc", User: "alice",
		Path: "foo.go", Line: 21, Side: "RIGHT", CreatedAt: time.Unix(1, 0),
	}
	mv := newComposeModel(t, composePatch, []*model.ReviewComment{root})
	mv.width = 80
	mv.height = 24
	mv.state.DiffCursor.Line = 6
	mv.state.CommentsCursor = 0
	mv.startComposeReply()
	if mv.state.PendingConfirm == nil {
		t.Fatalf("precondition: PendingConfirm set")
	}
	body := strings.Repeat(strings.Repeat(" ", mv.width)+"\n", mv.height-1) + strings.Repeat(" ", mv.width)
	out := mv.overlayConfirm(body)
	for _, want := range []string{"Post reply?", "foo.go:21", "alice"} {
		if !strings.Contains(out, want) {
			t.Fatalf("reply modal missing %q\n--- modal ---\n%s", want, out)
		}
	}
}

// Edit confirm derives its subject from the comment under the cursor
// (looked up by NodeID). The action verb in the title disambiguates
// from inline / reply.
func TestOverlayConfirm_EditSubjectFromCursor(t *testing.T) {
	own := &model.ReviewComment{
		ID: 5, NodeID: "PRRC_5", User: "you", Path: "foo.go", Line: 21, Side: "RIGHT",
		Body: "draft", CreatedAt: time.Unix(1, 0),
	}
	mv := newComposeModel(t, composePatch, []*model.ReviewComment{own})
	mv.width = 80
	mv.height = 24
	mv.state.ViewerLogin = "you"
	mv.state.DiffCursor.Line = 6
	mv.state.CommentsCursor = 0
	mv.startComposeEdit()
	if mv.state.PendingConfirm == nil {
		t.Fatalf("precondition: PendingConfirm set")
	}
	body := strings.Repeat(strings.Repeat(" ", mv.width)+"\n", mv.height-1) + strings.Repeat(" ", mv.width)
	out := mv.overlayConfirm(body)
	for _, want := range []string{"Edit comment?", "foo.go:21 RIGHT"} {
		if !strings.Contains(out, want) {
			t.Fatalf("edit modal missing %q\n--- modal ---\n%s", want, out)
		}
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

// Symmetric case: opened via space from Comments, closed via space —
// focus stays on Comments.
func TestSpaceClose_StaysOnCommentsWhenOpenedFromComments(t *testing.T) {
	root := &model.ReviewComment{
		ID: 11, NodeID: "PRRC_11", ThreadID: "PRT_a", User: "alice",
		Path: "foo.go", Line: 21, Side: "RIGHT", CreatedAt: time.Unix(1, 0),
	}
	mv := newComposeModel(t, composePatch, []*model.ReviewComment{root})
	mv.state.FocusedPane = model.PaneComments
	mv.state.DiffCursor.Line = 6
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

// q close: modal opened from Files via space → q returns focus to
// Files (without quitting the app). Files is a stand-in for any
// space-opened modal — the close-restores-focus contract is per-pane
// agnostic; we use a non-Comments pane here so the test stays valid
// after Diff Enter stopped opening the Comments modal.
func TestQClose_RestoresFocusToOrigin(t *testing.T) {
	mv := newComposeModel(t, composePatch, nil)
	mv.state.FocusedPane = model.PaneFiles
	mv.toggleModal(model.PaneFiles)
	if mv.state.Modal == nil {
		t.Fatalf("precondition: Files modal open")
	}
	updated, cmd := mv.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatalf("q must not quit while modal is open; got cmd %v", cmd)
	}
	if got.state.Modal != nil {
		t.Fatalf("q must close the modal")
	}
	if got.state.FocusedPane != model.PaneFiles {
		t.Fatalf("focus must return to Files (the opener), got %v", got.state.FocusedPane)
	}
}

func TestEscClose_RestoresFocusToOrigin(t *testing.T) {
	mv := newComposeModel(t, composePatch, nil)
	mv.state.FocusedPane = model.PaneFiles
	mv.toggleModal(model.PaneFiles)
	updated, _ := mv.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(Model)
	if got.state.Modal != nil {
		t.Fatalf("Esc must close the modal")
	}
	if got.state.FocusedPane != model.PaneFiles {
		t.Fatalf("focus must return to Files on Esc, got %v", got.state.FocusedPane)
	}
}

func TestRetryComposeSubmit_RequiresFailedState(t *testing.T) {
	m := newComposeModel(t, composePatch, nil)
	m.state.DiffCursor.Line = 6
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
