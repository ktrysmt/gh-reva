package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ktrysmt/gh-reva/internal/api"
	"github.com/ktrysmt/gh-reva/internal/model"
)

// startSubmitReview opens the submit-review modal. No-op when there is
// no PR loaded; the modal still opens with PendingCount=0 so the user
// can see "no pending comments" if they hit `R` by mistake — pressing
// `c` (comment) will then submit an empty review which GitHub accepts
// for general feedback. Approve / request_changes with zero comments
// are also valid GitHub operations.
func (m *Model) startSubmitReview() tea.Cmd {
	if m.state == nil || m.state.PR == nil {
		return nil
	}
	count := pendingCommentCount(m.state.PR.Comments)
	m.state.SubmitReview = &model.SubmitReviewState{
		Status:       model.SubmitChoosing,
		PendingCount: count,
	}
	return nil
}

// handleKeySubmit is the keystroke router used while the submit-review
// modal is open. It absorbs every keystroke so background panes stay
// frozen — same contract as compose / help / zoom modals.
//
//	a → APPROVE       c → COMMENT        r → REQUEST_CHANGES
//	ctrl+s → retry (only valid in Failed state)
//	esc / ctrl+c → cancel modal (Choosing/Failed; uninterruptible while Submitting)
func (m Model) handleKeySubmit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	sr := m.state.SubmitReview
	if sr == nil {
		return m, nil
	}
	if sr.Status == model.SubmitSubmitting {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.state.SubmitReview = nil
		}
		return m, nil
	}
	switch msg.String() {
	case "esc", "ctrl+c":
		m.state.SubmitReview = nil
		return m, nil
	case "ctrl+s":
		if sr.Status == model.SubmitFailed {
			sr.Status = model.SubmitSubmitting
			sr.ErrMsg = ""
			return m, submitReviewCmd(m.client, m.target, sr.Event)
		}
		return m, nil
	case "a":
		sr.Event = model.SubmitApprove
		sr.Status = model.SubmitSubmitting
		return m, submitReviewCmd(m.client, m.target, sr.Event)
	case "c":
		sr.Event = model.SubmitComment
		sr.Status = model.SubmitSubmitting
		return m, submitReviewCmd(m.client, m.target, sr.Event)
	case "r":
		sr.Event = model.SubmitRequestChanges
		sr.Status = model.SubmitSubmitting
		return m, submitReviewCmd(m.client, m.target, sr.Event)
	}
	return m, nil
}

// submitReviewCmd issues the GraphQL submit mutation and emits
// submitReviewDoneMsg with the result. Body is empty by design — the
// review summary body is a future feature; today we focus on flipping
// the pending review to public.
func submitReviewCmd(client api.Client, target *api.Target, event model.SubmitEvent) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := client.SubmitPendingReview(ctx, target.Owner, target.Repo, target.Number, event, "")
		return submitReviewDoneMsg{err: err}
	}
}

// applySubmitReviewDone is the Update side of submitReviewDoneMsg.
// Success closes the modal and triggers a comment refetch so the
// just-published comments lose their Pending flag in the UI. Failure
// keeps the modal open with the chosen event preserved for retry.
func (m *Model) applySubmitReviewDone(msg submitReviewDoneMsg) tea.Cmd {
	if m.state.SubmitReview == nil {
		return nil
	}
	if msg.err != nil {
		m.state.SubmitReview.Status = model.SubmitFailed
		m.state.SubmitReview.ErrMsg = msg.err.Error()
		return nil
	}
	m.state.SubmitReview = nil
	return refreshCommentsCmd(m.client, m.target)
}

// refreshCommentsCmd re-runs ListComments and pipes the result back as
// commentsRefreshedMsg. Used after a successful submit so the Pending
// flags flip; also reusable from any future "force refresh" gesture.
func refreshCommentsCmd(client api.Client, target *api.Target) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		comments, err := client.ListComments(ctx, target.Owner, target.Repo, target.Number)
		return commentsRefreshedMsg{comments: comments, err: err}
	}
}

// applyCommentsRefreshed swaps in the freshly-fetched comment list and
// recomputes per-file CommentCount so the Files pane chrome stays in
// sync. Errors are surfaced via err but the previous comment list is
// kept (failing to refresh shouldn't blow away the user's view).
func (m *Model) applyCommentsRefreshed(msg commentsRefreshedMsg) {
	if msg.err != nil || m.state.PR == nil {
		return
	}
	m.state.PR.Comments = msg.comments
	counts := map[string]int{}
	for _, c := range msg.comments {
		if !c.Outdated {
			counts[c.Path]++
		}
	}
	for _, f := range m.state.PR.Files {
		f.CommentCount = counts[f.Path]
	}
}

func pendingCommentCount(cs []*model.ReviewComment) int {
	n := 0
	for _, c := range cs {
		if c.Pending {
			n++
		}
	}
	return n
}

// overlaySubmit splices the submit-review modal over the body when
// SubmitReview is non-nil. Layout mirrors overlayCompose but the
// content is fixed (no body editing).
func (m Model) overlaySubmit(body string) string {
	sr := m.state.SubmitReview
	if sr == nil {
		return body
	}
	if m.width <= 0 || m.height <= 0 {
		return body
	}
	innerW := composeModalWidth(m.width)
	rows := submitModalRows(sr, innerW, m.theme.CommentDate, m.theme.ErrorText, m.theme.CommentPending)
	popup := composeModalBox(rows, innerW, m.theme.PaneBorderActive)
	popupRows := strings.Split(popup, "\n")

	outerW := innerW + 2
	left := (m.width - outerW) / 2
	if left < 0 {
		left = 0
	}
	top := (m.height - len(popupRows)) / 2
	if top < 0 {
		top = 0
	}
	bodyRows := strings.Split(body, "\n")
	for i, pr := range popupRows {
		r := top + i
		if r < 0 || r >= len(bodyRows) {
			continue
		}
		bodyRows[r] = spliceMid(bodyRows[r], pr, left, outerW)
	}
	return strings.Join(bodyRows, "\n")
}

// submitModalRows builds the rendered row list for the submit-review
// modal. Layout:
//
//	Submit review
//	(blank)
//	N pending comment(s)
//	(blank)
//	[a] approve
//	[c] comment
//	[r] request changes
//	(blank)
//	<status / hint line>
func submitModalRows(sr *model.SubmitReviewState, innerW int, dateColor, errColor, pendingColor lipgloss.Color) []string {
	contentW := innerW - 2
	if contentW < 1 {
		contentW = 1
	}
	rows := []string{
		fgBold("Submit review", dateColor),
		"",
		fg(pluralize(sr.PendingCount, "pending comment", "pending comments"), pendingColor),
		"",
		"  [a] approve",
		"  [c] comment",
		"  [r] request changes",
		"",
	}
	switch sr.Status {
	case model.SubmitSubmitting:
		rows = append(rows, fg("submitting review…", dateColor))
	case model.SubmitFailed:
		errMsg := "failed: " + sr.ErrMsg
		for _, r := range wrapText(errMsg, contentW) {
			rows = append(rows, fg(r, errColor))
		}
		rows = append(rows, fg("ctrl+s:retry  esc:cancel", dateColor))
	default:
		rows = append(rows, fg("a/c/r:choose  esc:cancel", dateColor))
	}
	return rows
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return formatN(n) + " " + plural
}

func formatN(n int) string {
	if n == 0 {
		return "0"
	}
	var b strings.Builder
	if n < 0 {
		b.WriteByte('-')
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	b.Write(digits)
	return b.String()
}
