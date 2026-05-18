package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// commentsManyFixture builds a Model whose Diff cursor sits on a single
// `+` buffer row anchoring a long thread (one root + N replies). The pane
// height is pinned small so the rendered comments overflow the viewport.
// At paneWidthComments=50 each fixture body fits on one row, so the
// commentsLayout output is deterministic:
//
//	row 0: carol header
//	row 1: "root body"
//	row 2: ""           (separator before reply 0)
//	row 3: alice0 header
//	row 4: "reply 0 body"
//	row 5: ""           (separator before reply 1)
//	row 6: alice1 header
//	...
//	row 3*N:   aliceN-1 body (reply at index N-1; last row = 3+3*(N-1)+1 = 3N+1)
//
// headerAt[i] is therefore 0 for the root and 3*i for reply i-1; total
// rows = 2 + 3*replyCount.
func commentsManyFixture(t *testing.T, replyCount int) Model {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	base := time.Date(2024, 1, 15, 13, 0, 0, 0, time.Local)
	root := &model.ReviewComment{
		ID: 1000, Path: "src/foo.go", CommitID: "abcdef0123456",
		Line: 2, User: "carol", CreatedAt: base,
		Body: "root body",
	}
	comments := []*model.ReviewComment{root}
	for i := 0; i < replyCount; i++ {
		comments = append(comments, &model.ReviewComment{
			ID: int64(1001 + i), Path: "src/foo.go", CommitID: "abcdef0123456",
			Line: 2, InReplyTo: 1000, User: fmt.Sprintf("alice%d", i),
			CreatedAt: base.Add(time.Duration(i+1) * time.Minute),
			Body:      fmt.Sprintf("reply %d body", i),
		})
	}
	m.state.PR = &model.PR{
		Owner: "o", Repo: "r", Number: 1,
		Files:    []*model.FileEntry{{Path: "src/foo.go", Status: model.ChangeModified}},
		Comments: comments,
	}
	m.state.SelectedFile = "src/foo.go"
	m.state.DiffCache[diffKey("", "src/foo.go")] = strings.Join([]string{
		"@@ -1,1 +1,2 @@",
		" line1",
		"+addedLine2",
	}, "\n")
	m.state.DiffCursor.Line = 2
	m.state.FocusedPane = model.PaneComments
	m.paneWidthComments = 50
	m.paneHeightComments = 6 // small viewport so scroll is forced
	return m
}

// pressCommentsKey drives a Comments-pane keystroke through handleKey so
// scroll behaviour is observed end-to-end (including handleKey's
// pre-dispatch hooks).
func pressCommentsKey(t *testing.T, m Model, key string) Model {
	t.Helper()
	if m.state.FocusedPane != model.PaneComments {
		t.Fatalf("FocusedPane must be Comments, got %v", m.state.FocusedPane)
	}
	var msg tea.KeyMsg
	switch key {
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	out, _ := m.handleKey(msg)
	return out.(Model)
}

// TestComments_J_AdvancesViewportByOneRow pins the core invariant: j is
// a row-level scroll, not a comment-level jump. After a single j the
// viewport-top advances by exactly one display row so a long comment's
// middle body becomes readable.
func TestComments_J_AdvancesViewportByOneRow(t *testing.T) {
	m := commentsManyFixture(t, 20)
	if m.state.CommentsTop != 0 {
		t.Fatalf("setup precondition: CommentsTop must start at 0; got %d", m.state.CommentsTop)
	}
	m = pressCommentsKey(t, m, "j")
	if m.state.CommentsTop != 1 {
		t.Errorf("after 1×j CommentsTop must advance by exactly 1 row; got %d", m.state.CommentsTop)
	}
	m = pressCommentsKey(t, m, "j")
	if m.state.CommentsTop != 2 {
		t.Errorf("after 2×j CommentsTop must advance by exactly 1 each step; got %d", m.state.CommentsTop)
	}
}

// TestComments_J_KeepsCurrentCommentWhileBodyVisible pins the "current
// comment" derivation. When a single j leaves the cursor inside the
// current comment's body (row 1 is carol's body), CommentsCursor stays
// at 0 — Diff auto-scroll must NOT fire because the focused comment
// hasn't changed.
func TestComments_J_KeepsCurrentCommentWhileBodyVisible(t *testing.T) {
	m := commentsManyFixture(t, 20)
	m = pressCommentsKey(t, m, "j")
	if m.state.CommentsCursor != 0 {
		t.Errorf("after 1×j (row 1 = carol's body) the current comment must remain carol (cursor 0); got %d",
			m.state.CommentsCursor)
	}
}

// TestComments_J_AdvancesCurrentCommentAtHeaderBoundary pins that the
// derived current-comment advances when the viewport-top row crosses a
// header boundary. After 3×j the top row is alice0's header (row 3 =
// headerAt[1]); the current comment must become reply 0.
func TestComments_J_AdvancesCurrentCommentAtHeaderBoundary(t *testing.T) {
	m := commentsManyFixture(t, 20)
	for i := 0; i < 3; i++ {
		m = pressCommentsKey(t, m, "j")
	}
	if m.state.CommentsCursor != 1 {
		t.Errorf("after 3×j CommentsTop=3 sits on alice0's header; current comment must be 1; got %d",
			m.state.CommentsCursor)
	}
}

// TestComments_K_ScrollsViewportUp pins the symmetric inverse: k rewinds
// the viewport one row, exposing earlier content.
func TestComments_K_ScrollsViewportUp(t *testing.T) {
	m := commentsManyFixture(t, 20)
	for i := 0; i < 5; i++ {
		m = pressCommentsKey(t, m, "j")
	}
	if m.state.CommentsTop != 5 {
		t.Fatalf("setup: after 5×j CommentsTop=5 expected; got %d", m.state.CommentsTop)
	}
	m = pressCommentsKey(t, m, "k")
	if m.state.CommentsTop != 4 {
		t.Errorf("after 1×k CommentsTop must rewind by 1 row; got %d", m.state.CommentsTop)
	}
}

// TestComments_K_ClampsAtZero pins the lower bound — k at row 0 is a
// no-op, never producing a negative CommentsTop.
func TestComments_K_ClampsAtZero(t *testing.T) {
	m := commentsManyFixture(t, 20)
	m = pressCommentsKey(t, m, "k")
	if m.state.CommentsTop != 0 {
		t.Errorf("k at CommentsTop=0 must clamp; got %d", m.state.CommentsTop)
	}
}

// TestComments_J_ClampsAtMaxTop pins the upper bound — j past the last
// scrollable row is a no-op so the viewport never strands past the end
// of content.
func TestComments_J_ClampsAtMaxTop(t *testing.T) {
	m := commentsManyFixture(t, 20)
	rows, _ := m.commentsLayout()
	maxTop := len(rows) - m.commentsViewportHeight()
	for i := 0; i < maxTop+10; i++ {
		m = pressCommentsKey(t, m, "j")
	}
	if m.state.CommentsTop != maxTop {
		t.Errorf("j past maxTop must clamp; got CommentsTop=%d want %d", m.state.CommentsTop, maxTop)
	}
}

// TestComments_J_RendersScrolledBody pins that a single j actually
// brings the body row into the top slot — the user-facing payoff is
// that long-comment bodies become readable mid-scroll.
func TestComments_J_RendersScrolledBody(t *testing.T) {
	m := commentsManyFixture(t, 20)
	m = pressCommentsKey(t, m, "j")
	got := m.commentsView()
	lines := strings.Split(got, "\n")
	// title (line 0) + carol-body row should be the first content line at
	// CommentsTop=1; carol's header (row 0) is now off-screen.
	if len(lines) < 2 || !strings.Contains(lines[1], "root body") {
		t.Errorf("after 1×j the first content row must be the body 'root body'; got:\n%s", got)
	}
	// carol's header (carol:) must no longer be in the visible output.
	for _, l := range lines[1:] {
		if strings.Contains(l, "carol:") {
			t.Errorf("after 1×j carol's header must be off-screen; rendered:\n%s", got)
			break
		}
	}
}

// TestComments_G_ScrollsToBottom pins that G scrolls the viewport so the
// last rendered row is visible (CommentsTop == maxTop). The last reply's
// body must appear in the rendered output.
func TestComments_G_ScrollsToBottom(t *testing.T) {
	m := commentsManyFixture(t, 20)
	m = pressCommentsKey(t, m, "G")
	rows, _ := m.commentsLayout()
	maxTop := len(rows) - m.commentsViewportHeight()
	if maxTop < 0 {
		maxTop = 0
	}
	if m.state.CommentsTop != maxTop {
		t.Errorf("G must scroll to maxTop=%d; got CommentsTop=%d", maxTop, m.state.CommentsTop)
	}
	got := m.commentsView()
	if !strings.Contains(got, "alice19:") {
		t.Errorf("after G the last reply (alice19) must be visible:\n%s", got)
	}
}

// TestComments_Gg_ScrollsToTop pins that gg resets the viewport AND the
// current-comment cursor to the first comment, mirroring vim's top-of-
// buffer gesture.
func TestComments_Gg_ScrollsToTop(t *testing.T) {
	m := commentsManyFixture(t, 20)
	m = pressCommentsKey(t, m, "G")
	if m.state.CommentsTop == 0 {
		t.Fatalf("setup: G must have moved CommentsTop > 0")
	}
	m = pressCommentsKey(t, m, "g")
	m = pressCommentsKey(t, m, "g")
	if m.state.CommentsTop != 0 {
		t.Errorf("gg must reset CommentsTop to 0; got %d", m.state.CommentsTop)
	}
	if m.state.CommentsCursor != 0 {
		t.Errorf("gg must reset CommentsCursor to 0; got %d", m.state.CommentsCursor)
	}
	got := m.commentsView()
	if !strings.Contains(got, "carol:") {
		t.Errorf("after gg the root header (carol) must be visible:\n%s", got)
	}
}

// TestComments_TopResetsOnAnchorChange pins that the explicit reset
// pathways (Diff Enter handoff via focusCommentsAtCursor, file selection
// via selectFile, commit selection via autoSelectCommit) drop the user
// back at the top of the new thread list — both CommentsCursor AND
// CommentsTop must reset to 0 so the viewport doesn't strand on the
// previous thread's scroll offset.
func TestComments_TopResetsOnAnchorChange(t *testing.T) {
	m := commentsManyFixture(t, 20)
	for i := 0; i < 10; i++ {
		m = pressCommentsKey(t, m, "j")
	}
	if m.state.CommentsTop == 0 {
		t.Fatalf("setup precondition: after 10×j CommentsTop should be > 0; got 0")
	}
	// focusCommentsAtCursor is the Diff Enter handoff path; it must reset
	// both cursor and viewport in lockstep.
	m.focusCommentsAtCursor()
	if m.state.CommentsTop != 0 {
		t.Errorf("focusCommentsAtCursor must reset CommentsTop to 0; got %d", m.state.CommentsTop)
	}
	if m.state.CommentsCursor != 0 {
		t.Errorf("focusCommentsAtCursor must reset CommentsCursor to 0; got %d", m.state.CommentsCursor)
	}
}
