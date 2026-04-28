package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-rv/internal/model"
)

type commentThread struct {
	Root    *model.ReviewComment
	Replies []*model.ReviewComment
}

func (m Model) handleKeyComments(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	flat := m.flatComments()
	switch msg.String() {
	case "j", "down":
		if m.state.CommentsCursor < len(flat)-1 {
			m.state.CommentsCursor++
		}
	case "k", "up":
		if m.state.CommentsCursor > 0 {
			m.state.CommentsCursor--
		}
	case "h", "left":
		if id := m.threadRootIDForCursor(); id != 0 {
			m.state.ThreadFolded[id] = true
			m.clampCommentsCursor()
		}
	case "l", "right":
		if id := m.threadRootIDForCursor(); id != 0 {
			delete(m.state.ThreadFolded, id)
		}
	case "backspace":
		m.state.FocusedPane = model.PaneDiff
	}
	m.syncDiffToCursorComment()
	return m, nil
}

// syncDiffToCursorComment auto-scrolls the Diff viewport so the comment under
// the Comments cursor is visible. Cursor in Diff is not moved.
func (m *Model) syncDiffToCursorComment() {
	flat := m.flatComments()
	if len(flat) == 0 || m.state.CommentsCursor >= len(flat) {
		return
	}
	c := flat[m.state.CommentsCursor]
	target := commentNewLine(c)
	patch := m.currentPatch()
	if patch == "" {
		return
	}
	bufIdx := bufferIndexForNewLine(patch, target)
	if bufIdx < 0 {
		return
	}
	totalLines := strings.Count(patch, "\n") + 1
	m.scrollDiffToLine(bufIdx, totalLines)
}

func (m Model) commentsView() string {
	title := paneTitle("Comments", m.state.FocusedPane == model.PaneComments, "")
	if m.state.PR == nil || m.state.SelectedFile == "" {
		return title
	}
	threads := m.threadsForView()
	if len(threads) == 0 {
		return title + "\n(no comments)"
	}
	var rows []string
	idx := 0
	for _, t := range threads {
		rows = append(rows, m.renderCommentRow(t.Root, 0, idx))
		idx++
		if !m.state.ThreadFolded[t.Root.ID] {
			for _, r := range t.Replies {
				rows = append(rows, m.renderCommentRow(r, 1, idx))
				idx++
			}
		}
	}
	return title + "\n" + strings.Join(rows, "\n")
}

func (m Model) renderCommentRow(c *model.ReviewComment, depth, idx int) string {
	cursor := m.cursorMarker(model.PaneComments, idx, m.state.CommentsCursor)
	in := indent(depth)
	date := c.CreatedAt.Format("2006-01-02")
	sha := shortSHA(c.CommitID)
	if sha == "" {
		sha = shortSHA(c.OriginalCommitID)
	}
	tag := ""
	if c.Outdated {
		tag = " [outdated]"
	}
	return fmt.Sprintf("%s%s%s: %s %s%s — %s", cursor, in, c.User, date, sha, tag, c.Body)
}

func (m Model) commentsForView() []*model.ReviewComment {
	if m.state.PR == nil || m.state.SelectedFile == "" {
		return nil
	}
	var out []*model.ReviewComment
	for _, c := range m.state.PR.Comments {
		if c.Path != m.state.SelectedFile {
			continue
		}
		switch m.state.SelectedRange.Kind {
		case model.RangeSingleCommit:
			if c.CommitID == m.state.SelectedRange.SHA || c.OriginalCommitID == m.state.SelectedRange.SHA {
				out = append(out, c)
			}
		default:
			if !c.Outdated {
				out = append(out, c)
			}
		}
	}
	return out
}

func (m Model) threadsForView() []*commentThread {
	comments := m.commentsForView()
	rootByID := map[int64]*commentThread{}
	var roots []*commentThread
	for _, c := range comments {
		if c.InReplyTo == 0 {
			t := &commentThread{Root: c}
			rootByID[c.ID] = t
			roots = append(roots, t)
		}
	}
	for _, c := range comments {
		if c.InReplyTo != 0 {
			if t, ok := rootByID[c.InReplyTo]; ok {
				t.Replies = append(t.Replies, c)
			}
		}
	}
	sort.SliceStable(roots, func(i, j int) bool {
		return roots[i].Root.CreatedAt.Before(roots[j].Root.CreatedAt)
	})
	for _, t := range roots {
		sort.SliceStable(t.Replies, func(i, j int) bool {
			return t.Replies[i].CreatedAt.Before(t.Replies[j].CreatedAt)
		})
	}
	return roots
}

func (m Model) flatComments() []*model.ReviewComment {
	var out []*model.ReviewComment
	for _, t := range m.threadsForView() {
		out = append(out, t.Root)
		if !m.state.ThreadFolded[t.Root.ID] {
			out = append(out, t.Replies...)
		}
	}
	return out
}

func (m Model) hasCommentsForCurrentView() bool {
	return len(m.commentsForView()) > 0
}

func (m Model) threadRootIDForCursor() int64 {
	threads := m.threadsForView()
	idx := 0
	for _, t := range threads {
		if idx == m.state.CommentsCursor {
			return t.Root.ID
		}
		idx++
		if !m.state.ThreadFolded[t.Root.ID] {
			for range t.Replies {
				if idx == m.state.CommentsCursor {
					return t.Root.ID
				}
				idx++
			}
		}
	}
	return 0
}

func (m *Model) clampCommentsCursor() {
	max := len(m.flatComments()) - 1
	if max < 0 {
		max = 0
	}
	if m.state.CommentsCursor > max {
		m.state.CommentsCursor = max
	}
}
