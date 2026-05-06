package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-reva/internal/model"
)

type commentThread struct {
	Root    *model.ReviewComment
	Replies []*model.ReviewComment
}

func (m Model) handleKeyComments(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == " " {
		// Modal toggle does not move the Comments cursor, so skip the trailing
		// syncDiffToCursorComment — leaving the Diff viewport where the user
		// parked it before opening the zoomed view.
		m.toggleModal(model.PaneComments)
		return m, nil
	}
	flat := m.flatComments()
	key := msg.String()
	if handled := m.handlePendingG(key, func() {
		if len(flat) > 0 {
			m.state.CommentsCursor = 0
			m.syncDiffToCursorComment()
		}
	}); handled {
		return m, nil
	}
	switch key {
	case "j", "down":
		if m.state.CommentsCursor < len(flat)-1 {
			m.state.CommentsCursor++
		}
	case "k", "up":
		if m.state.CommentsCursor > 0 {
			m.state.CommentsCursor--
		}
	case "G":
		if n := len(flat); n > 0 {
			m.state.CommentsCursor = n - 1
		}
	case "enter":
		// Edit the cursor comment — only the viewer's own comments are
		// editable per GitHub's permission model. startComposeEdit
		// queues PendingConfirm on success; success is detected by
		// inspecting m.state.PendingConfirm (the call returns nil
		// either way because the editor launch is held until `y`).
		// On a foreign comment (or before the viewer login is known),
		// surface a status-bar notice steering the user to `r` for
		// reply instead of POSTing into a 403.
		m.startComposeEdit()
		if m.state.PendingConfirm != nil {
			return m, nil
		}
		if c := commentAtCursor(flat, m.state.CommentsCursor); c != nil && c.User != m.state.ViewerLogin {
			m.state.Notice = "cannot edit comments by other users (press r to reply)"
		}
		return m, nil
	case "r":
		// Reply to the thread under the cursor (the previous Enter
		// gesture). No-op when no thread is visible. The editor launch
		// is gated by the y/n confirm prompt; the immediate Cmd is nil.
		return m, m.startComposeReply()
	}
	m.syncDiffToCursorComment()
	return m, nil
}

// commentAtCursor returns the flat-list entry at idx, or nil when the
// index is out of range. Helper for handleKeyComments' notice gate so
// the bounds check stays out of the dispatch switch.
func commentAtCursor(flat []*model.ReviewComment, idx int) *model.ReviewComment {
	if idx < 0 || idx >= len(flat) {
		return nil
	}
	return flat[idx]
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
	lines := m.patchLines()
	if len(lines) == 0 {
		return
	}
	bufIdx := bufferIndexForNewLine(lines, target)
	if bufIdx < 0 {
		return
	}
	m.scrollDiffToLine(bufIdx, len(lines))
}

func (m Model) commentsView() string {
	title := m.styledPaneTitle("Comments", m.state.FocusedPane == model.PaneComments, "")
	if m.state.PR == nil || m.state.SelectedFile == "" {
		return title
	}
	threads := m.threadsForCursor()
	if len(threads) == 0 {
		return title + "\n(no comment at cursor)"
	}
	var rows []string
	idx := 0
	for ti, t := range threads {
		if ti > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, m.renderCommentRow(t.Root, 0, idx)...)
		idx++
		for _, r := range t.Replies {
			rows = append(rows, "")
			rows = append(rows, m.renderCommentRow(r, 1, idx)...)
			idx++
		}
	}
	return title + "\n" + strings.Join(rows, "\n")
}

// renderCommentRow returns one entry rendered as multiple display rows:
// row 0 is the header `<name>: <yyyy-mm-dd hh:mm> <hash>[ [outdated]]`,
// rows 1..N are the wrapped body indented past the header by 2 cols
// (so root body sits at depth+1*2 = 2 cols; reply body at 4 cols). The
// cursor `>` glyph appears on the header row only — body rows keep the
// 2-col cursor area blank so the indent visual stays consistent.
func (m Model) renderCommentRow(c *model.ReviewComment, depth, idx int) []string {
	cursor := m.styledCursor(model.PaneComments, idx, m.state.CommentsCursor)
	headIndent := indent(depth)
	bodyLeader := "  " + indent(depth+1) // 2 cols for cursor area + body indent
	bodyLeaderW := utf8.RuneCountInString(bodyLeader)

	date := c.CreatedAt.Local().Format("2006-01-02 15:04")
	sha := shortSHA(c.CommitID)
	if sha == "" {
		sha = shortSHA(c.OriginalCommitID)
	}
	// Pending takes precedence over outdated — a pending comment by
	// definition has not been posted yet, so the outdated bit cannot
	// fire on it. The tag is colored independently so the user can
	// tell at a glance which entries are local-only drafts.
	var tag string
	tagColor := m.theme.CommentOutdated
	switch {
	case c.Pending:
		tag = " [pending]"
		tagColor = m.theme.CommentPending
	case c.Outdated:
		tag = " [outdated]"
	}
	header := fmt.Sprintf("%s%s%s: %s %s%s",
		cursor, headIndent,
		fg(c.User, m.theme.CommentAuthor),
		fg(date, m.theme.CommentDate),
		fg(sha, m.theme.CommitSHA),
		fg(tag, tagColor),
	)

	wrapWidth := m.paneWidthComments
	if wrapWidth <= 0 {
		wrapWidth = m.width
	}
	out := []string{header}
	if wrapWidth <= 0 {
		out = append(out, bodyLeader+c.Body)
		return out
	}
	// bodyWidth is exactly the cells available after the indent. A min-10
	// floor used to live here as a "readable wrap" defense, but it pushed
	// rendered rows past paneWidthComments and forced renderPaneBox::padTrunc
	// to silently truncate. Respect the pane budget instead — at extremely
	// narrow widths the body collapses to one rune per row, ugly but
	// non-corrupt; the alternative was a quiet width-violation.
	bodyWidth := wrapWidth - bodyLeaderW
	if bodyWidth < 1 {
		bodyWidth = 1
	}
	out = append(out, renderCommentBody(c.Body, bodyLeader, bodyWidth)...)
	return out
}

// renderCommentBody turns the comment body into one display row per source
// line, matching how GitHub renders PR comment bodies: single `\n` is a
// hard line break (the source line gets its own row), `\n\n+` is a
// paragraph break (an extra blank row separates the surrounding rows).
// Lines longer than `bodyWidth` cells are wrapped via `wrapText`. Leading
// and trailing blank lines are elided so the body never starts or ends
// with stray empty rows. Fenced code blocks need no special handling: each
// fence-internal `\n` is already a row break under this rule.
func renderCommentBody(body, bodyLeader string, bodyWidth int) []string {
	var out []string
	emitted := 0
	pendingBlank := false
	for _, line := range strings.Split(body, "\n") {
		if line == "" {
			if emitted > 0 {
				pendingBlank = true
			}
			continue
		}
		if pendingBlank {
			out = append(out, "")
			pendingBlank = false
		}
		for _, ch := range wrapText(line, bodyWidth) {
			out = append(out, bodyLeader+ch)
		}
		emitted++
	}
	return out
}

// threadsForCursor returns the comment threads anchored at the current Diff
// cursor's buffer line. Empty when the cursor is not on a ◆ row, when no
// patch is loaded, or when no thread targets the cursor's new-file line.
// Ordering matches threadsForView (chronological by root time).
func (m Model) threadsForCursor() []*commentThread {
	all := m.threadsForView()
	if len(all) == 0 {
		return nil
	}
	mapping := m.patchNewLineNumbers()
	if len(mapping) == 0 {
		return nil
	}
	cursor := m.state.DiffCursor.Line
	if cursor < 0 || cursor >= len(mapping) {
		return nil
	}
	target := mapping[cursor]
	if target <= 0 {
		return nil
	}
	var out []*commentThread
	for _, t := range all {
		if anyCommentOnLine(t, target) {
			out = append(out, t)
		}
	}
	return out
}

func anyCommentOnLine(t *commentThread, line int) bool {
	if commentNewLine(t.Root) == line {
		return true
	}
	for _, r := range t.Replies {
		if commentNewLine(r) == line {
			return true
		}
	}
	return false
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

// flatComments returns the comment list backing Comments-pane navigation
// (j/k cursor, visual selection, yank). It mirrors what commentsView is
// rendering — i.e. only the threads anchored at the current Diff cursor
// row — so the cursor index never drifts past the visible content.
func (m Model) flatComments() []*model.ReviewComment {
	var out []*model.ReviewComment
	for _, t := range m.threadsForCursor() {
		out = append(out, t.Root)
		out = append(out, t.Replies...)
	}
	return out
}
