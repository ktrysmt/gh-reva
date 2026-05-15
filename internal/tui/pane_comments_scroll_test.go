package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// commentsManyFixture builds a Model whose Diff cursor sits on a single
// `+` buffer row anchoring a long thread (one root + N replies). The pane
// height is pinned small so the rendered comments overflow the viewport
// — the scenario the user reported: "jk doesn't transition the screen".
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

// When the cursor moves past the bottom of the viewport, the rendered
// pane body must slide down so the cursor row stays inside the visible
// window. Before the fix, j was a no-op visually because commentsView
// emitted the full row set and renderPaneBox just clipped at the bottom.
func TestComments_JKScrollsViewportWhenCursorOverflowsBottom(t *testing.T) {
	m := commentsManyFixture(t, 20)
	// Walk down past the visible window. With paneHeightComments=6 the
	// initial render covers ~6 rows; sending 10 j keys is enough to push
	// the cursor several rows below the initial bottom.
	for i := 0; i < 10; i++ {
		m = pressCommentsKey(t, m, "j")
	}
	got := m.commentsView()
	// The header row of comment at flat-index 0 (carol) must be off-screen
	// once the viewport has scrolled; alice0 (reply 0) must also be off
	// because the cursor has moved past several replies.
	if strings.Contains(got, "carol:") {
		t.Errorf("after 10×j the viewport must have scrolled off the root header; commentsView still contains 'carol:':\n%s", got)
	}
	// The cursored comment (alice9 at flat-index 10 — root=0 + 10 replies)
	// must be visible in the rendered output.
	if !strings.Contains(got, "alice9:") {
		t.Errorf("after 10×j the cursored comment alice9 must be visible in commentsView; got:\n%s", got)
	}
}

// k from the middle of the list scrolls the viewport up so a previously
// off-screen earlier comment becomes visible again. Symmetric to the j
// case so we can pin both directions.
func TestComments_KScrollsViewportWhenCursorOverflowsTop(t *testing.T) {
	m := commentsManyFixture(t, 20)
	// First push cursor down so the viewport scrolls past the root.
	for i := 0; i < 10; i++ {
		m = pressCommentsKey(t, m, "j")
	}
	// Now walk back up. By the time we reach idx 0 the root must reappear.
	for i := 0; i < 10; i++ {
		m = pressCommentsKey(t, m, "k")
	}
	got := m.commentsView()
	if !strings.Contains(got, "carol:") {
		t.Errorf("after k-walking back to idx 0 the root header (carol) must be visible:\n%s", got)
	}
}

// G jumps the cursor to the last comment and the viewport must follow so
// the last comment is rendered. gg returns to the top with the root
// visible.
func TestComments_GAndGgScrollViewport(t *testing.T) {
	m := commentsManyFixture(t, 20)
	m = pressCommentsKey(t, m, "G")
	gotG := m.commentsView()
	if !strings.Contains(gotG, "alice19:") {
		t.Errorf("G must scroll the viewport to the last comment (alice19):\n%s", gotG)
	}
	m = pressCommentsKey(t, m, "g")
	m = pressCommentsKey(t, m, "g")
	gotGg := m.commentsView()
	if !strings.Contains(gotGg, "carol:") {
		t.Errorf("gg must scroll the viewport back to the first comment (carol):\n%s", gotGg)
	}
}

// Switching the Diff cursor to a different anchor row (which resets
// CommentsCursor to 0 via focusCommentsAtCursor / autoSelectCommit) also
// resets the Comments viewport so the user lands at the top of the new
// thread list.
func TestComments_TopResetsOnAnchorChange(t *testing.T) {
	m := commentsManyFixture(t, 20)
	for i := 0; i < 10; i++ {
		m = pressCommentsKey(t, m, "j")
	}
	if m.state.CommentsTop == 0 {
		t.Fatalf("setup precondition: after 10×j CommentsTop should be > 0; got 0")
	}
	// Simulate the Diff Enter handoff path: cursor moves + CommentsCursor
	// gets reset to 0. The viewport offset must also reset.
	m.state.CommentsCursor = 0
	m.scrollCommentsIntoView()
	if m.state.CommentsTop != 0 {
		t.Errorf("CommentsTop must reset to 0 when cursor is reset to 0; got %d", m.state.CommentsTop)
	}
}
