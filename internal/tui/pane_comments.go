package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ktrysmt/gh-reva/internal/model"
)

type commentThread struct {
	Root    *model.ReviewComment
	Replies []*model.ReviewComment
}

func (m Model) handleKeyComments(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == " " {
		// No-op when the cursor row carries no threads — opening a
		// zoom modal that just wraps the "(no comment at cursor)"
		// placeholder is noise; reserve the gesture for when there's
		// actual content to zoom. Modal toggle skips the trailing
		// syncDiffToCursorComment so the Diff viewport stays where
		// the user parked it before opening the zoom.
		if len(m.threadsForCursor()) == 0 {
			return m, nil
		}
		m.toggleModal(model.PaneComments)
		return m, nil
	}
	flat := m.flatComments()
	key := msg.String()
	if handled := m.handlePendingG(key, func() {
		if len(flat) > 0 {
			m.state.CommentsCursor = 0
			m.state.CommentsTop = 0
			m.syncDiffToCursorComment()
		}
	}); handled {
		return m, nil
	}
	switch key {
	case "j", "down":
		m.scrollCommentsBy(1)
	case "k", "up":
		m.scrollCommentsBy(-1)
	case "G":
		m.scrollCommentsToBottom()
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

// formatRangeTag returns the Comments-header range label for a
// multi-line range comment, or "" for single-line comments and
// replies. Both endpoints carry their Side prefix so the reader can
// tell same-side (`R2-R4`) and mixed-side (`L5-R10`) ranges apart at
// a glance without consulting the underlying comment. Falls back from
// StartLine to OriginalStartLine and from Line to OriginalLine the
// same way the gutter-anchor lookup does, so outdated range comments
// still surface their historical span.
func formatRangeTag(c *model.ReviewComment) string {
	start := c.StartLine
	if start <= 0 {
		start = c.OriginalStartLine
	}
	if start <= 0 {
		return ""
	}
	end := c.Line
	if end <= 0 {
		end = c.OriginalLine
	}
	if end <= 0 {
		return ""
	}
	side := c.Side
	if side == "" {
		side = "RIGHT"
	}
	startSide := c.StartSide
	if startSide == "" {
		startSide = side
	}
	return fmt.Sprintf("%s%d-%s%d", sideAbbrev(startSide), start, sideAbbrev(side), end)
}

func sideAbbrev(side string) string {
	if side == "LEFT" {
		return "L"
	}
	return "R"
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

// commentsMaxTopFor returns the largest valid CommentsTop given the
// total rendered row count and the viewport's row budget. Anything
// beyond this would strand the viewport past the last row.
func commentsMaxTopFor(rowsLen, viewportHeight int) int {
	if rowsLen <= viewportHeight || viewportHeight <= 0 {
		return 0
	}
	return rowsLen - viewportHeight
}

// commentsCursorFromTop derives the "current comment" — the index of
// the comment that owns the viewport-top row — from CommentsTop. The
// owner is the comment whose header is the most recent at or before
// `top`; rows between headerAt[i] and headerAt[i+1]-1 all belong to
// comment i (including the inter-thread separator blank line, which
// trails the preceding thread visually). Falls back to 0 when no
// headers exist so the cursor never points past the visible content.
func (m Model) commentsCursorFromTop(top int) int {
	_, headerAt := m.commentsLayout()
	if len(headerAt) == 0 {
		return 0
	}
	cursor := 0
	for i, h := range headerAt {
		if h <= top {
			cursor = i
			continue
		}
		break
	}
	return cursor
}

// scrollCommentsBy nudges CommentsTop by `delta` display rows (positive
// = down, negative = up), clamped to [0, maxTop]. Re-derives
// CommentsCursor from the new viewport top; when the derived cursor
// crosses a comment boundary, the Diff pane syncs to the new current
// comment so the reviewer's anchor row stays in step with what they're
// reading.
//
// j/k drive this with delta=±1 to surface long-body middles a row at a
// time — the previous comment-unit jump skipped over them entirely.
func (m *Model) scrollCommentsBy(delta int) {
	rows, _ := m.commentsLayout()
	if len(rows) == 0 {
		return
	}
	maxTop := commentsMaxTopFor(len(rows), m.commentsViewportHeight())
	next := m.state.CommentsTop + delta
	if next < 0 {
		next = 0
	}
	if next > maxTop {
		next = maxTop
	}
	if next == m.state.CommentsTop {
		return
	}
	m.state.CommentsTop = next
	prev := m.state.CommentsCursor
	m.state.CommentsCursor = m.commentsCursorFromTop(next)
	if m.state.CommentsCursor != prev {
		m.syncDiffToCursorComment()
	}
}

// scrollCommentsToBottom is G's implementation — jump the viewport to
// the last scrollable position so the final comment is visible. The
// derived current-comment becomes whichever comment owns that bottom-
// most viewport-top row (typically the last thread).
func (m *Model) scrollCommentsToBottom() {
	rows, _ := m.commentsLayout()
	if len(rows) == 0 {
		return
	}
	maxTop := commentsMaxTopFor(len(rows), m.commentsViewportHeight())
	if maxTop == m.state.CommentsTop {
		return
	}
	m.state.CommentsTop = maxTop
	prev := m.state.CommentsCursor
	m.state.CommentsCursor = m.commentsCursorFromTop(maxTop)
	if m.state.CommentsCursor != prev {
		m.syncDiffToCursorComment()
	}
}

// syncDiffToCursorComment auto-scrolls the Diff viewport so the comment under
// the Comments cursor is visible. Cursor in Diff is not moved.
//
// Side-aware: a LEFT-side comment's c.Line is an OLD-file line number,
// so the buffer-index lookup picks the corresponding `-` row (via
// oldLineNumbers) instead of falling through newLineNumbers and missing.
func (m *Model) syncDiffToCursorComment() {
	flat := m.flatComments()
	if len(flat) == 0 || m.state.CommentsCursor >= len(flat) {
		return
	}
	c := flat[m.state.CommentsCursor]
	lines := m.patchLines()
	if len(lines) == 0 {
		return
	}
	bufIdx := commentBufferIndex(c, m.patchOldLineNumbers(), m.patchNewLineNumbers())
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
	if m.state.SelectedFile == model.AllFilesPath {
		// All view spans every file; the per-anchor Comments column has
		// no sensible content to show. Diff Enter / `r` are blocked
		// upstream by buildComposeInline so the user gets a Notice on
		// attempted compose.
		return title + "\n(no file selected)\nComments disabled in All view"
	}
	rows, _ := m.commentsLayout()
	if len(rows) == 0 {
		return title + "\n(no comment at cursor)"
	}
	top := m.state.CommentsTop
	if top < 0 {
		top = 0
	}
	if top >= len(rows) {
		top = len(rows) - 1
	}
	return title + "\n" + strings.Join(rows[top:], "\n")
}

// commentsLayout walks the threads visible at the current Diff cursor and
// emits the body rows commentsView() / commentIndexAtDisplayRow() /
// scrollCommentsIntoView() all consume in lock-step. headerAt[i] is the
// row index inside `rows` where flat comment i's header sits — used by
// the scroll math to land the cursored header inside the viewport.
//
// Empty return when threadsForCursor() is empty so callers can route to
// the "(no comment at cursor)" placeholder without re-checking.
func (m Model) commentsLayout() (rows []string, headerAt []int) {
	threads := m.threadsForCursor()
	if len(threads) == 0 {
		return nil, nil
	}
	idx := 0
	for ti, t := range threads {
		if ti > 0 {
			rows = append(rows, "")
		}
		headerAt = append(headerAt, len(rows))
		rows = append(rows, m.renderCommentRow(t.Root, 0, idx)...)
		idx++
		for _, r := range t.Replies {
			rows = append(rows, "")
			headerAt = append(headerAt, len(rows))
			rows = append(rows, m.renderCommentRow(r, 1, idx)...)
			idx++
		}
	}
	return rows, headerAt
}

// commentsViewportHeight returns the body row budget for the Comments
// pane — the visible window height into which commentsLayout's `rows`
// gets sliced. Mirrors diffViewportHeight()'s fallback ladder so tests
// that don't drive a full measureLayout still get a sensible non-zero
// height.
func (m Model) commentsViewportHeight() int {
	if m.paneHeightComments > 0 {
		return m.paneHeightComments
	}
	if m.height > 18 {
		return m.height - 16
	}
	return 5
}

// renderCommentRow returns one entry rendered as multiple display rows:
// row 0 is the header `[resolved] <name>: <yyyy-mm-dd hh:mm> <hash>[ [outdated]]`
// where the leading `[resolved]` tag is present only when the thread has
// been resolved on GitHub. Rows 1..N are the wrapped body indented past
// the header by 2 cols (so root body sits at depth+1*2 = 2 cols; reply
// body at 4 cols). The cursor `>` glyph appears on the header row only —
// body rows keep the 2-col cursor area blank so the indent visual stays
// consistent.
//
// Tag layout. `[resolved]` sits at the LINE HEAD (immediately after the
// cursor / depth indent, before the author) so it is the first content
// the reviewer reads — resolved threads can usually be skipped, so the
// signal earns the top-left slot. `[pending]` / `[outdated]` keep
// the trailing position after `<sha>` they have always lived in:
//   - pending is local-only state (no thread is resolved before its
//     parent review is submitted), and trailing emphasis is enough.
//   - outdated lives next to the commit hash because the hash itself
//     hints why the anchor drifted; keeping them together preserves
//     that adjacency.
//
// Compatibility note: pending and outdated are mutually exclusive
// (pending entries are drafts and never carry GitHub flags), so the
// trailing slot still renders at most one tag. resolved can co-exist
// with outdated (a resolved-but-now-stale thread) — both render, one at
// each end. resolved + pending is impossible: a draft has no thread to
// resolve yet, so this branch is unreachable in practice; we still
// honor `c.Resolved` if a fixture sets both, since suppressing the
// head tag based on the trailing one would surprise.
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
	// Multi-line range tag rendered as ` <range>` between the SHA and
	// `#<id>`. Replaces the previous in-gutter ┌/│ visual: ranges still
	// surface their upper edge here, without colliding with neighbouring
	// ◆ anchors. Empty string for single-line comments and replies.
	var rangeTag string
	if r := formatRangeTag(c); r != "" {
		rangeTag = " " + fg(r, m.theme.DiffLineNumber)
	}
	// Per-comment numeric ID rendered as ` #<id>` between the range tag
	// and any trailing state tag — lets reviewers copy the literal id
	// for API / link references without leaving the TUI. Skipped when
	// id == 0 (pre-POST drafts could surface here before convertGQLComment
	// stamps the databaseId).
	var idTag string
	if c.ID != 0 {
		idTag = " " + fg(fmt.Sprintf("#%d", c.ID), m.theme.CommitSHA)
	}
	// Trailing tag (pending OR outdated) — pending wins because pending
	// entries are drafts and cannot also be public-state outdated.
	var trailingTag string
	trailingColor := m.theme.CommentOutdated
	switch {
	case c.Pending:
		trailingTag = " [pending]"
		trailingColor = m.theme.CommentPending
	case c.Outdated:
		trailingTag = " [outdated]"
	}
	// Leading [resolved] tag at line head, before the author name. The
	// trailing blank lives inside the tag string so the empty / present
	// branches don't fight over spacing.
	var leadingTag string
	if c.Resolved {
		leadingTag = fg("[resolved] ", m.theme.CommentResolved)
	}
	header := fmt.Sprintf("%s%s%s%s: %s %s%s%s%s",
		cursor, headIndent, leadingTag,
		fg(c.User, m.theme.CommentAuthor),
		fg(date, m.theme.CommentDate),
		fg(sha, m.theme.CommitSHA),
		rangeTag,
		idTag,
		fg(trailingTag, trailingColor),
	)

	wrapWidth := m.paneWidthComments
	if wrapWidth <= 0 {
		wrapWidth = m.width
	}
	// At narrow Comments widths the optional `<range>` / `#<id>` slots
	// push the trailing `[pending]` / `[outdated]` tag past the right
	// edge of the column and renderPaneBox::padTrunc silently clips it
	// — clipping the critical state tag is worse than dropping the
	// reference id or the range tag. Degrade in two steps: drop `#<id>`
	// first (it's the most easily re-derivable), then drop the range
	// tag too if the header still overflows. lipgloss.Width strips SGR
	// escapes so the comparison reflects rendered cell count.
	rebuild := func(includeRange, includeID bool) string {
		rt := rangeTag
		if !includeRange {
			rt = ""
		}
		it := idTag
		if !includeID {
			it = ""
		}
		return fmt.Sprintf("%s%s%s%s: %s %s%s%s%s",
			cursor, headIndent, leadingTag,
			fg(c.User, m.theme.CommentAuthor),
			fg(date, m.theme.CommentDate),
			fg(sha, m.theme.CommitSHA),
			rt,
			it,
			fg(trailingTag, trailingColor),
		)
	}
	if wrapWidth > 0 && lipgloss.Width(header) > wrapWidth {
		if idTag != "" {
			header = rebuild(true, false)
		}
		if rangeTag != "" && lipgloss.Width(header) > wrapWidth {
			header = rebuild(false, false)
		}
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
// cursor's buffer line AND matching the cursor's Side. Empty when the
// cursor is not on a ◆ row, when no patch is loaded, or when no thread
// anchors at the cursor's buffer index on the current Side. Ordering
// matches threadsForView (chronological by root time).
//
// File-overview exception: when the cursor sits on a file-metadata row
// (`---` / `+++` file header or `@@` hunk header) the per-cursor + per-
// Side filter is bypassed and the full `threadsForView()` list is
// returned. Those rows carry no real file line number, so the "thread
// at this exact anchor" contract has nothing to filter against; falling
// back to a file-wide overview lets the user skim every comment from
// the headers without hunting for an anchored row first. Synthetic
// `···` rows and body rows (+/-/ context) keep the strict filter.
//
// Side-aware in two senses (only when NOT on a meta row):
//
//  1. Each thread's anchor buffer index is computed via
//     commentBufferIndex (LEFT comments → oldLineNumbers; others →
//     newLineNumbers). Matching a thread to the cursor by buffer index
//     lets LEFT comments anchor on `-` rows, which the previous "look
//     up mapping[cursor]" approach silently dropped (mapping[cursor] is
//     0 for `-` rows under newLineNumbers).
//  2. The thread's root.Side must match cursor.Side. Without this
//     filter a context buffer row carrying both a LEFT and a RIGHT
//     thread would render both regardless of which column the user is
//     currently parked in — which defeats the per-column comment UX.
//     Empty / missing root.Side is treated as RIGHT (legacy comments
//     pre-dating the Side field; matches GitHub's display default).
func (m Model) threadsForCursor() []*commentThread {
	all := m.threadsForView()
	if len(all) == 0 {
		return nil
	}
	cursor := m.state.DiffCursor.Line
	if cursor < 0 {
		return nil
	}
	if m.cursorOnFileMetaRow(cursor) {
		return all
	}
	newMap := m.patchNewLineNumbers()
	oldMap := m.patchOldLineNumbers()
	if len(newMap) == 0 && len(oldMap) == 0 {
		return nil
	}
	side := m.state.DiffCursor.Side
	if side == "" {
		side = model.DiffSideRight
	}
	var out []*commentThread
	for _, t := range all {
		if !threadOnSide(t, side) {
			continue
		}
		if anyCommentAtBuffer(t, cursor, oldMap, newMap) {
			out = append(out, t)
		}
	}
	return out
}

// cursorOnFileMetaRow reports whether the buffer row at idx is a file-
// metadata row (kind 'h' for `---` / `+++`, kind '@' for `@@`). These
// rows have no underlying file line, so threadsForCursor uses them as
// the trigger for the file-overview short-circuit. Synthetic rows
// (kind 's') are intentionally excluded — Enter on them expands the
// gap, and showing every thread there would conflict with the
// keystroke's primary purpose.
func (m Model) cursorOnFileMetaRow(idx int) bool {
	lines := m.patchLines()
	if idx < 0 || idx >= len(lines) {
		return false
	}
	kind := diffLineKind(lines[idx])
	return kind == 'h' || kind == '@'
}

// threadOnSide reports whether a thread belongs to the given column.
// The root's Side decides for the whole thread — replies inherit it on
// GitHub. Empty Side defaults to RIGHT (legacy comments).
func threadOnSide(t *commentThread, side model.DiffSide) bool {
	rs := model.DiffSide(t.Root.Side)
	if rs == "" {
		rs = model.DiffSideRight
	}
	return rs == side
}

func anyCommentAtBuffer(t *commentThread, cursor int, oldNums, newNums []int) bool {
	if commentBufferIndex(t.Root, oldNums, newNums) == cursor {
		return true
	}
	for _, r := range t.Replies {
		if commentBufferIndex(r, oldNums, newNums) == cursor {
			return true
		}
	}
	return false
}

func (m Model) commentsForView() []*model.ReviewComment {
	if m.state.PR == nil || m.state.SelectedFile == "" || m.state.SelectedFile == model.AllFilesPath {
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

// threadsViewCache memoizes the most recent threadsForView() result.
// Single-entry: a key mismatch (different file or range) rebuilds and
// overwrites; PR.Comments mutations invalidate via `valid = false` from
// applyComposeSubmitted / applyCommentsRefreshed. The cache pointer
// lives on Model so propagation across Bubbletea's value-receiver
// Update mirrors the patchLinesC / rowCache pattern.
type threadsViewCache struct {
	valid     bool
	file      string
	rangeKind model.CommitRangeKind
	rangeSHA  string
	threads   []*commentThread
	// gen bumps on every successful rebuild. Downstream caches keyed on
	// "the threads we last saw" (e.g. patchInfo.markersGen) compare gen
	// to detect staleness without holding a slice pointer that would
	// pin freed memory or lie when the underlying array is reused.
	gen uint64
}

func (m Model) threadsForView() []*commentThread {
	file := m.state.SelectedFile
	rangeKind := m.state.SelectedRange.Kind
	rangeSHA := m.state.SelectedRange.SHA
	if m.threadsCache != nil && m.threadsCache.valid &&
		m.threadsCache.file == file &&
		m.threadsCache.rangeKind == rangeKind &&
		m.threadsCache.rangeSHA == rangeSHA {
		return m.threadsCache.threads
	}
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
	if m.threadsCache != nil {
		m.threadsCache.valid = true
		m.threadsCache.file = file
		m.threadsCache.rangeKind = rangeKind
		m.threadsCache.rangeSHA = rangeSHA
		m.threadsCache.threads = roots
		m.threadsCache.gen++
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
