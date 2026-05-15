package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-reva/internal/api"
	"github.com/ktrysmt/gh-reva/internal/diff"
	"github.com/ktrysmt/gh-reva/internal/model"
)

// editorEnv returns the user's preferred editor, honoring VISUAL before
// EDITOR per POSIX convention. Empty result triggers the textarea
// fallback path. Held as a package-level variable so tests can stub it
// without mutating process env.
var editorEnv = func() string {
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	return os.Getenv("EDITOR")
}

// startComposeInline queues the inline-compose confirm prompt anchored
// at the current Diff cursor (or visual range if active). Returns nil
// in every case: the editor / textarea launch is deferred to
// confirmComposeStart, which fires when the user presses `y`. Returns
// without queueing when the cursor is on a header / hunk row or no
// patch is loaded; the caller can detect "no-op" by checking
// `m.state.PendingConfirm == nil` after the call.
func (m *Model) startComposeInline() tea.Cmd {
	if !m.buildComposeInline() {
		return nil
	}
	m.requestComposeConfirm()
	return nil
}

// startComposeReply queues the reply-compose confirm prompt for the
// thread the Comments cursor is sitting on. Returns nil; the editor
// launch is held until `y`. The caller can probe success via
// `m.state.PendingConfirm != nil`.
func (m *Model) startComposeReply() tea.Cmd {
	if !m.buildComposeReply() {
		return nil
	}
	m.requestComposeConfirm()
	return nil
}

// startComposeEdit queues the edit-compose confirm prompt for the
// comment under the Comments cursor. Returns nil; the editor launch is
// held until `y`. Foreign authors and missing NodeIDs short-circuit
// inside buildComposeEdit and leave PendingConfirm nil — the caller
// surfaces a notice in that case.
func (m *Model) startComposeEdit() tea.Cmd {
	if !m.buildComposeEdit() {
		return nil
	}
	m.requestComposeConfirm()
	return nil
}

// requestComposeConfirm moves the freshly-built Compose payload into
// PendingConfirm. Compose is cleared so the global Compose absorber in
// handleKey does NOT engage — that absorber routes every key through
// the textarea, which would swallow the `y` / `n` keystrokes the
// confirm dispatcher needs to see.
func (m *Model) requestComposeConfirm() {
	cs := m.state.Compose
	if cs == nil {
		return
	}
	m.state.Compose = nil
	m.state.PendingConfirm = &model.PendingConfirm{Kind: cs.Kind, Compose: cs}
}

// confirmComposeStart commits the parked compose payload: PendingConfirm
// clears, Compose is restored, the visual range banner (if any) is
// dropped, and the body-collection Cmd is returned. Returns nil when
// PendingConfirm is unset (defensive — handleKeyConfirm gates on this).
//
// Visual is cleared at this point rather than at build time so the
// highlighted range stays on screen during the confirm prompt; only
// once the user presses `y` does the banner disappear behind the editor.
func (m *Model) confirmComposeStart() tea.Cmd {
	pc := m.state.PendingConfirm
	if pc == nil || pc.Compose == nil {
		return nil
	}
	m.state.PendingConfirm = nil
	m.state.Compose = pc.Compose
	if pc.Compose.Kind == model.ComposeInline && m.state.Visual != nil && m.state.Visual.OriginPane == model.PaneDiff {
		m.state.Visual = nil
	}
	return m.beginEditing()
}

// cancelComposeConfirm discards the parked payload. The visual range
// (for inline-range cancels) stays intact so the user can refine the
// selection and re-issue Enter.
func (m *Model) cancelComposeConfirm() {
	m.state.PendingConfirm = nil
}

// buildComposeInline populates m.state.Compose with the inline target
// derived from the current Diff cursor / visual range. Returns false
// when the inputs cannot anchor a comment (header rows, no patch, no
// PR loaded). Pulled out so unit tests can drive the state machine
// without launching an editor.
func (m *Model) buildComposeInline() bool {
	if m.state == nil || m.state.PR == nil || m.state.SelectedFile == "" {
		return false
	}
	// The synthetic All view spans every file; a single comment anchor
	// cannot identify which file the user means, so compose is disabled
	// here. The user must select a real file first.
	if m.state.SelectedFile == model.AllFilesPath {
		m.state.Notice = "comments unavailable in All view (select a file first)"
		return false
	}
	if m.currentPatch() == "" {
		return false
	}
	cs := &model.ComposeState{
		Kind:      model.ComposeInline,
		Status:    model.ComposeEditing,
		Path:      m.state.SelectedFile,
		CommitSHA: m.state.PR.HeadSHA,
	}
	cursorSide := string(m.state.DiffCursor.Side)
	if cursorSide == "" {
		cursorSide = "RIGHT"
	}
	if v := m.state.Visual; v != nil && v.OriginPane == model.PaneDiff {
		r, ok := m.resolveRangeAug(v.AnchorLine, m.state.DiffCursor.Line)
		if !ok {
			return false
		}
		// resolveRangeAug's per-endpoint Side defaults to RIGHT for
		// context rows. Override end Side with the cursor's column when
		// the end row is context — h/l on a context line is the user's
		// gesture for "post on this side" and must reach the GitHub
		// payload. `+`/`-` rows ignore the override (their Side is
		// fixed by the line type) because auto-skip guarantees the
		// cursor Side already matches.
		side := r.Side
		startSide := r.StartSide
		if m.isContextRowAug(m.state.DiffCursor.Line) {
			side = cursorSide
		}
		anchorBuf := m.state.DiffCursor.Line
		if v.AnchorLine < anchorBuf {
			anchorBuf = v.AnchorLine
		}
		if r.StartLine > 0 && m.isContextRowAug(anchorBuf) {
			startSide = cursorSide
		}
		cs.Line = m.lineForSideAug(m.state.DiffCursor.Line, side)
		cs.Side = side
		if r.StartLine > 0 {
			sl := m.lineForSideAug(anchorBuf, startSide)
			cs.StartLine = &sl
			cs.StartSide = startSide
		}
		// Visual is left intact here so the highlighted range stays
		// visible while the confirm prompt is up. confirmComposeStart
		// clears it the moment the user commits with `y`.
	} else {
		a, ok := m.resolveAnchorAug(m.state.DiffCursor.Line)
		if !ok {
			return false
		}
		side := a.Side
		if a.Kind == ' ' {
			side = cursorSide
		}
		line := a.NewLine
		if side == "LEFT" {
			line = a.OldLine
		}
		cs.Line = line
		cs.Side = side
	}
	m.state.Compose = cs
	return true
}

// resolveAnchorAug returns the OLD/NEW line numbers and Side for the
// buffer index in the AUGMENTED patch buffer (synthetic + expanded
// context aware). Synthetic / file-header / hunk rows return ok=false.
func (m Model) resolveAnchorAug(idx int) (diff.Anchor, bool) {
	specs := m.patchSpecs()
	if idx < 0 || idx >= len(specs) {
		return diff.Anchor{}, false
	}
	s := specs[idx]
	if s.Kind == 'h' || s.Kind == '@' || s.Kind == 's' {
		return diff.Anchor{}, false
	}
	side := "RIGHT"
	if s.Kind == '-' {
		side = "LEFT"
	}
	return diff.Anchor{
		Kind:    s.Kind,
		OldLine: s.OldLn,
		NewLine: s.NewLn,
		Side:    side,
	}, true
}

// resolveRangeAug mirrors diff.ResolveRange but walks the augmented
// buffer's specs so synthetic rows correctly reject as endpoints and
// expanded-context rows produce the right OLD/NEW pair.
func (m Model) resolveRangeAug(anchor, cursor int) (diff.Range, bool) {
	a, okA := m.resolveAnchorAug(anchor)
	b, okB := m.resolveAnchorAug(cursor)
	if !okA || !okB {
		return diff.Range{}, false
	}
	pick := func(an diff.Anchor) int {
		if an.Side == "LEFT" {
			return an.OldLine
		}
		return an.NewLine
	}
	if anchor == cursor {
		return diff.Range{Line: pick(a), Side: a.Side}, true
	}
	startSpec, endSpec := a, b
	if anchor > cursor {
		startSpec, endSpec = b, a
	}
	return diff.Range{
		StartLine: pick(startSpec),
		StartSide: startSpec.Side,
		Line:      pick(endSpec),
		Side:      endSpec.Side,
	}, true
}

// isContextRowAug reports whether the row at `idx` in the augmented
// buffer is a context row. Synthetic / header / hunk all return false
// so the cursor.Side override in buildComposeInline only fires where
// ambiguity exists.
func (m Model) isContextRowAug(idx int) bool {
	a, ok := m.resolveAnchorAug(idx)
	if !ok {
		return false
	}
	return a.Kind == ' '
}

// lineForSideAug returns the file-line number at the augmented buffer's
// `idx` on the requested Side. LEFT → OldLine, RIGHT → NewLine. 0 when
// the row has no counterpart on that Side (or is synthetic / header).
func (m Model) lineForSideAug(idx int, side string) int {
	a, ok := m.resolveAnchorAug(idx)
	if !ok {
		return 0
	}
	if side == "LEFT" {
		return a.OldLine
	}
	return a.NewLine
}

// buildComposeReply populates m.state.Compose with the reply target
// derived from the Comments cursor. Returns false when no thread is
// under the cursor (e.g. cursor not on a ◆ row, or panes not yet
// populated). Captures the parent thread's GraphQL node ID so the
// reply mutation can route to addPullRequestReviewThreadReply
// without a separate lookup.
func (m *Model) buildComposeReply() bool {
	if m.state == nil || m.state.PR == nil {
		return false
	}
	threadID := m.threadIdentityForCursor()
	if threadID == "" {
		return false
	}
	m.state.Compose = &model.ComposeState{
		Kind:           model.ComposeReply,
		Status:         model.ComposeEditing,
		ParentThreadID: threadID,
	}
	return true
}

// buildComposeEdit populates m.state.Compose for an in-place body
// edit on the comment under the Comments cursor. Returns false when:
//
//   - no Comments cursor (no PR / no flatComments)
//   - the cursor comment has no NodeID (cannot identify on GitHub)
//   - the cursor comment was authored by a non-viewer (GitHub rejects
//     the edit anyway; we gate before the POST so the user gets a
//     fast no-op rather than a roundtrip + 403)
//
// The pre-edit body is copied into Compose.Body so the editor /
// textarea opens with existing text instead of a blank buffer.
func (m *Model) buildComposeEdit() bool {
	if m.state == nil || m.state.PR == nil {
		return false
	}
	flat := m.flatComments()
	if len(flat) == 0 {
		return false
	}
	idx := m.state.CommentsCursor
	if idx < 0 || idx >= len(flat) {
		return false
	}
	target := flat[idx]
	if target == nil || target.NodeID == "" {
		return false
	}
	if m.state.ViewerLogin == "" || target.User != m.state.ViewerLogin {
		return false
	}
	m.state.Compose = &model.ComposeState{
		Kind:              model.ComposeEdit,
		Status:            model.ComposeEditing,
		EditCommentNodeID: target.NodeID,
		Body:              target.Body,
	}
	return true
}

// threadIdentityForCursor returns the GraphQL thread node ID for the
// thread the Comments cursor is sitting on. Empty signals "no thread
// visible" so the caller can no-op. The flat ordering is `[root,
// replies..., next root, replies..., ...]`, so the cursor index
// identifies which thread we are in by walking until index matches.
func (m Model) threadIdentityForCursor() string {
	threads := m.threadsForCursor()
	idx := m.state.CommentsCursor
	walked := 0
	for _, t := range threads {
		if idx == walked {
			return t.Root.ThreadID
		}
		walked++
		for range t.Replies {
			if idx == walked {
				return t.Root.ThreadID
			}
			walked++
		}
	}
	return ""
}

// beginEditing returns the Cmd that drives the body-collection step.
// $EDITOR / $VISUAL → external editor via tea.ExecProcess.
// neither set → textarea fallback (UseTextarea=true; key handler owns input).
func (m *Model) beginEditing() tea.Cmd {
	if editorEnv() == "" {
		m.state.Compose.UseTextarea = true
		return nil
	}
	return runEditorCmd(m.state.Compose.Body)
}

// buildEditorCmd assembles the *exec.Cmd that runs `shellCmd` (an
// already-quoted "<editor> <tempfile>" string). When invoked from inside
// a tmux session ($TMUX non-empty), the editor floats in a centered
// `display-popup` so gh-reva's TUI stays painted underneath instead of
// being swapped out for the full-screen editor. Outside tmux, the
// canonical `sh -c <shellCmd>` path runs the editor inline as before.
//
// `-E` makes tmux close the popup automatically when the editor exits
// (regardless of exit code), so vim `:q!` and friends still return
// control to gh-reva. `-w 80% -h 80%` keeps the popup roomy enough for
// real edits while leaving the surrounding pane chrome visible.
func buildEditorCmd(shellCmd string) *exec.Cmd {
	if os.Getenv("TMUX") != "" {
		return exec.Command("tmux", "display-popup", "-E", "-w", "80%", "-h", "80%", shellCmd)
	}
	return exec.Command("sh", "-c", shellCmd)
}

// runEditorCmd writes a tempfile (pre-populated with `initialBody` if
// non-empty so edit flows start on the existing text), runs $EDITOR on
// it, and on exit reads the file back, deletes it, and emits
// composeBodyMsg with the result. Empty body (after TrimSpace) is the
// user's signal to cancel.
//
// Editor invocation goes through `sh -c "<EDITOR> <quoted-path>"` rather
// than splitting EDITOR on whitespace ourselves: matches the convention
// of git commit / crontab -e / visudo, so EDITOR='code --wait',
// EDITOR='vim -p', EDITOR='nvim +Glog', and editor paths with spaces
// (e.g. /Applications/Visual Studio Code.app/...) all work as the user
// expects from their shell. The tempfile path is shell-quoted with
// shellSingleQuote — os.CreateTemp emits alphanumeric paths in
// practice, but the quote keeps the contract honest.
//
// Dispatch by $TMUX:
//   - TMUX set → runEditorOverlay: cmd.Run() directly inside a tea.Cmd
//     goroutine, bypassing tea.ExecProcess. The popup is rendered by
//     the tmux server as an overlay over reva's pane, so we MUST keep
//     bubbletea in alt-screen — releasing the terminal would force a
//     redraw of the underlying shell screen, which is then what the
//     popup overlays (instead of reva's frame).
//   - TMUX unset → tea.ExecProcess: the editor takes over the whole
//     terminal, so bubbletea must release alt-screen / raw mode first.
func runEditorCmd(initialBody string) tea.Cmd {
	f, err := os.CreateTemp("", "gh-reva-compose-*.md")
	if err != nil {
		return func() tea.Msg { return composeBodyMsg{err: err} }
	}
	tmpPath := f.Name()
	if initialBody != "" {
		if _, err := f.WriteString(initialBody); err != nil {
			_ = f.Close()
			_ = os.Remove(tmpPath)
			return func() tea.Msg { return composeBodyMsg{err: err} }
		}
	}
	_ = f.Close()
	editor := editorEnv()
	shellCmd := fmt.Sprintf("%s%s %s", editor, startInsertFlag(editor), shellSingleQuote(tmpPath))
	cmd := buildEditorCmd(shellCmd)
	if os.Getenv("TMUX") != "" {
		return runEditorOverlay(cmd, tmpPath)
	}
	return tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		return readComposeBody(tmpPath, execErr)
	})
}

// runEditorOverlay drives the tmux display-popup branch: blocks on
// cmd.Run() inside a tea.Cmd goroutine while bubbletea keeps rendering
// to its pane. tmux's display-popup -E IPC blocks the calling client
// until the popup closes, which is what synchronises body readback to
// editor exit. stdin/stdout/stderr are intentionally left at exec.Cmd's
// default (/dev/null) — the popup's PTY is allocated by the tmux server
// independently of this client process, and routing our bubbletea I/O
// into the client would corrupt the alt-screen frame.
func runEditorOverlay(cmd *exec.Cmd, tmpPath string) tea.Cmd {
	return func() tea.Msg {
		return readComposeBody(tmpPath, cmd.Run())
	}
}

// readComposeBody is the post-exit contract shared by both editor
// branches: cleans up the tempfile, surfaces the editor's exit error if
// any, otherwise returns the file contents as the new compose body.
func readComposeBody(tmpPath string, execErr error) tea.Msg {
	defer os.Remove(tmpPath)
	if execErr != nil {
		return composeBodyMsg{err: execErr}
	}
	b, readErr := os.ReadFile(tmpPath)
	if readErr != nil {
		return composeBodyMsg{err: readErr}
	}
	return composeBodyMsg{body: string(b)}
}

// startInsertFlag returns " +startinsert" when `editor` is a vim-family
// command (vim, nvim, vi, gvim, mvim — possibly with a leading directory
// or `.exe` suffix, possibly followed by additional flags), otherwise
// "". Vim honors `+startinsert` by entering Insert mode at startup,
// matching the user's expectation that a fresh comment buffer is ready
// for typing without the extra `i` keystroke. Detection runs against
// the first whitespace token so wrappers like `nvim +Glog` still match.
func startInsertFlag(editor string) string {
	fields := strings.Fields(editor)
	if len(fields) == 0 {
		return ""
	}
	base := strings.TrimSuffix(filepath.Base(fields[0]), ".exe")
	switch base {
	case "vim", "nvim", "vi", "gvim", "mvim":
		return " +startinsert"
	}
	return ""
}

// shellSingleQuote wraps s in POSIX single quotes, escaping any embedded
// single quote as `'\''`. Single-quoting is preferred over double
// because no metacharacter (`$`, backtick, backslash) is interpreted
// inside `'...'`, making the output a literal string regardless of
// content.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// applyComposeBody is the Update side of composeBodyMsg. Editor errors
// and empty body both cancel without a POST. Non-empty body transitions
// the state to Submitting and queues submitComposeCmd.
func (m *Model) applyComposeBody(msg composeBodyMsg) tea.Cmd {
	if m.state.Compose == nil {
		return nil
	}
	if msg.err != nil {
		// Editor failed (could not launch, exit non-zero, etc.). Show
		// the error so the user knows; preserve any in-progress body
		// from the textarea path so they can retry.
		m.state.Compose.Status = model.ComposeFailed
		m.state.Compose.ErrMsg = msg.err.Error()
		return nil
	}
	body := strings.TrimSpace(msg.body)
	if body == "" {
		m.state.Compose = nil
		return nil
	}
	m.state.Compose.Body = body
	m.state.Compose.Status = model.ComposeSubmitting
	return submitComposeCmd(m.client, m.target, *m.state.Compose)
}

// submitComposeCmd dispatches the right GraphQL mutation for the
// compose payload. Inline → CreatePendingReviewThread; Reply →
// CreatePendingReviewThreadReply; Edit → UpdateReviewComment. The
// Compose value is captured by copy at Cmd-build time so a later
// state mutation (cancel, retry) does not race with the in-flight
// call.
func submitComposeCmd(client api.Client, target *api.Target, cs model.ComposeState) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		switch cs.Kind {
		case model.ComposeReply:
			rc, err := client.CreatePendingReviewThreadReply(ctx, target.Owner, target.Repo, target.Number, cs.ParentThreadID, cs.Body)
			return composeSubmittedMsg{comment: rc, err: err}
		case model.ComposeEdit:
			rc, err := client.UpdateReviewComment(ctx, target.Owner, target.Repo, target.Number, cs.EditCommentNodeID, cs.Body)
			return composeSubmittedMsg{comment: rc, err: err}
		default:
			in := api.CreatePendingThreadInput{
				Path:      cs.Path,
				CommitOID: cs.CommitSHA,
				Line:      cs.Line,
				Side:      cs.Side,
				StartLine: cs.StartLine,
				StartSide: cs.StartSide,
				Body:      cs.Body,
			}
			rc, err := client.CreatePendingReviewThread(ctx, target.Owner, target.Repo, target.Number, in)
			return composeSubmittedMsg{comment: rc, err: err}
		}
	}
}

// applyComposeSubmitted is the Update side of composeSubmittedMsg.
// Success optimistically updates PR.Comments and queues
// refreshCommentsCmd; the kind decides whether to append (Inline /
// Reply) or replace-by-NodeID (Edit).
//
// Edits never bump CommentCount — the comment already existed before
// the edit. Inline / Reply append a new entry and bump the count for
// the affected path. The refresh then heals any drift between the
// optimistic copy and GitHub's authoritative state. Failure flips
// status to Failed without dropping the body so the user can retry.
func (m *Model) applyComposeSubmitted(msg composeSubmittedMsg) tea.Cmd {
	if m.state.Compose == nil {
		return nil
	}
	if msg.err != nil {
		m.state.Compose.Status = model.ComposeFailed
		m.state.Compose.ErrMsg = msg.err.Error()
		return nil
	}
	kind := m.state.Compose.Kind
	if msg.comment != nil && m.state.PR != nil {
		if kind == model.ComposeEdit {
			replaceCommentByNodeID(m.state.PR.Comments, msg.comment)
		} else {
			m.state.PR.Comments = append(m.state.PR.Comments, msg.comment)
			bumpFileCommentCount(m.state.PR.Files, msg.comment.Path)
		}
		// PR.Comments mutated — drop the threadsForView cache so the
		// next render rebuilds the thread tree with the new entry.
		if m.threadsCache != nil {
			m.threadsCache.valid = false
		}
	}
	// Auto-reveal the Comments column after a successful submit so the
	// freshly-posted draft is visible. Without this, a user who hid the
	// column via Ctrl+E and then posted from Diff would have to remember
	// the toggle gesture before they could see what they just wrote.
	// Failure leaves CommentsHidden alone (the user's deliberate toggle
	// shouldn't bounce on every API hiccup).
	m.state.CommentsHidden = false
	m.state.Compose = nil
	return refreshCommentsCmd(m.client, m.target)
}

// replaceCommentByNodeID swaps the body / pending-state of any comment
// whose NodeID matches `next`. Used by the Edit flow to apply the
// optimistic update — anchor (Path / Line / Side) is left as-is
// because GitHub's edit mutation does not move threads; only the body
// (and timestamps, indirectly) change.
func replaceCommentByNodeID(list []*model.ReviewComment, next *model.ReviewComment) {
	if next == nil || next.NodeID == "" {
		return
	}
	for _, c := range list {
		if c == nil || c.NodeID != next.NodeID {
			continue
		}
		c.Body = next.Body
		c.Pending = next.Pending
		// CreatedAt/Author intentionally left untouched: an edit on
		// GitHub does not move the comment's posting timestamp, and
		// the author cannot change.
		return
	}
}

// retryComposeSubmit re-issues the in-flight POST after a failed
// attempt. Used by the textarea Ctrl+S handler when Status is Failed —
// the body buffer is preserved on failure precisely so this retry
// does not need to re-prompt.
func (m *Model) retryComposeSubmit() tea.Cmd {
	if m.state.Compose == nil || m.state.Compose.Status != model.ComposeFailed {
		return nil
	}
	m.state.Compose.Status = model.ComposeSubmitting
	m.state.Compose.ErrMsg = ""
	return submitComposeCmd(m.client, m.target, *m.state.Compose)
}

func bumpFileCommentCount(files []*model.FileEntry, path string) {
	for _, f := range files {
		if f.Path == path {
			f.CommentCount++
			return
		}
	}
}
