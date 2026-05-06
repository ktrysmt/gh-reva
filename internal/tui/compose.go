package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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

// startComposeInline opens a Compose session anchored at the current
// Diff cursor (or visual range if active). Returns nil when the cursor
// is on a header / hunk row or no patch is loaded — the caller should
// treat nil as "key was a no-op". Otherwise returns the body-collection
// Cmd ($EDITOR exec, or nil for the textarea fallback).
func (m *Model) startComposeInline() tea.Cmd {
	if !m.buildComposeInline() {
		return nil
	}
	return m.beginEditing()
}

// startComposeReply opens a Compose session targeting the thread the
// Comments cursor is currently sitting on. Returns nil when no thread
// is under the cursor.
func (m *Model) startComposeReply() tea.Cmd {
	if !m.buildComposeReply() {
		return nil
	}
	return m.beginEditing()
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
	patch := m.currentPatch()
	if patch == "" {
		return false
	}
	cs := &model.ComposeState{
		Kind:      model.ComposeInline,
		Status:    model.ComposeEditing,
		Path:      m.state.SelectedFile,
		CommitSHA: m.state.PR.HeadSHA,
	}
	if v := m.state.Visual; v != nil && v.OriginPane == model.PaneDiff {
		r, ok := diff.ResolveRange(patch, v.AnchorLine, m.state.DiffCursor.Line)
		if !ok {
			return false
		}
		cs.Line = r.Line
		cs.Side = r.Side
		if r.StartLine > 0 {
			sl := r.StartLine
			cs.StartLine = &sl
			cs.StartSide = r.StartSide
		}
		// Visual selection is consumed by entering Compose; otherwise
		// the visual banner would linger behind the editor / textarea.
		m.state.Visual = nil
	} else {
		a, ok := diff.ResolveAnchor(patch, m.state.DiffCursor.Line)
		if !ok {
			return false
		}
		line := a.NewLine
		if a.Side == "LEFT" {
			line = a.OldLine
		}
		cs.Line = line
		cs.Side = a.Side
	}
	m.state.Compose = cs
	return true
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
	threadID, parentDBID := m.threadIdentityForCursor()
	if threadID == "" {
		return false
	}
	m.state.Compose = &model.ComposeState{
		Kind:           model.ComposeReply,
		Status:         model.ComposeEditing,
		ParentThreadID: threadID,
		ParentDBID:     parentDBID,
	}
	return true
}

// threadIdentityForCursor returns the GraphQL thread node ID and the
// integer DB id of the root comment for the thread the Comments cursor
// is sitting on. Empty thread ID signals "no thread visible" so the
// caller can no-op. The flat ordering is `[root, replies..., next root,
// replies..., ...]`, so the cursor index identifies which thread we are
// in by walking until index matches.
func (m Model) threadIdentityForCursor() (string, int64) {
	threads := m.threadsForCursor()
	idx := m.state.CommentsCursor
	walked := 0
	for _, t := range threads {
		if idx == walked {
			return t.Root.ThreadID, t.Root.ID
		}
		walked++
		for range t.Replies {
			if idx == walked {
				return t.Root.ThreadID, t.Root.ID
			}
			walked++
		}
	}
	return "", 0
}

// beginEditing returns the Cmd that drives the body-collection step.
// $EDITOR / $VISUAL → external editor via tea.ExecProcess.
// neither set → textarea fallback (UseTextarea=true; key handler owns input).
func (m *Model) beginEditing() tea.Cmd {
	if editorEnv() == "" {
		m.state.Compose.UseTextarea = true
		return nil
	}
	return runEditorCmd()
}

// runEditorCmd writes a tempfile, hands the terminal to $EDITOR via
// tea.ExecProcess, and on exit reads the file back, deletes it, and
// emits composeBodyMsg with the result. Empty body (after TrimSpace)
// is the user's signal to cancel.
func runEditorCmd() tea.Cmd {
	f, err := os.CreateTemp("", "gh-reva-compose-*.md")
	if err != nil {
		return func() tea.Msg { return composeBodyMsg{err: err} }
	}
	tmpPath := f.Name()
	_ = f.Close()
	editor := editorEnv()
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		_ = os.Remove(tmpPath)
		return func() tea.Msg { return composeBodyMsg{err: fmt.Errorf("no editor configured")} }
	}
	args := append(parts[1:], tmpPath)
	cmd := exec.Command(parts[0], args...)
	return tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		defer os.Remove(tmpPath)
		if execErr != nil {
			return composeBodyMsg{err: execErr}
		}
		b, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			return composeBodyMsg{err: readErr}
		}
		return composeBodyMsg{body: string(b)}
	})
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
// CreatePendingReviewThreadReply. The Compose value is captured by
// copy at Cmd-build time so a later state mutation (cancel, retry)
// does not race with the in-flight call.
func submitComposeCmd(client api.Client, target *api.Target, cs model.ComposeState) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		switch cs.Kind {
		case model.ComposeReply:
			rc, err := client.CreatePendingReviewThreadReply(ctx, target.Owner, target.Repo, target.Number, cs.ParentThreadID, cs.Body)
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
// Success appends the returned comment to PR.Comments and clears
// Compose. Failure flips status to Failed without dropping the body
// so the user can retry from the modal.
func (m *Model) applyComposeSubmitted(msg composeSubmittedMsg) {
	if m.state.Compose == nil {
		return
	}
	if msg.err != nil {
		m.state.Compose.Status = model.ComposeFailed
		m.state.Compose.ErrMsg = msg.err.Error()
		return
	}
	if msg.comment != nil && m.state.PR != nil {
		m.state.PR.Comments = append(m.state.PR.Comments, msg.comment)
		bumpFileCommentCount(m.state.PR.Files, msg.comment.Path)
	}
	m.state.Compose = nil
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
