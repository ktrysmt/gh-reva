package tui

import (
	"github.com/ktrysmt/gh-reva/internal/model"
)

type PRLoadedMsg struct {
	PR          *model.PR
	Diffs       map[string]string
	ViewerLogin string
}

// LoadStageMsg announces the start of a loading stage so the spinner can
// reflect progress before PRLoadedMsg arrives.
type LoadStageMsg struct {
	Stage model.LoadStage
}

// SpinnerTickMsg drives the loading spinner animation. The Update loop
// re-emits a tick while the PR is still loading, and stops once the data is
// available (LoadStageDone).
type SpinnerTickMsg struct{}

// ScrollDiffToLineMsg requests the Diff viewport be recentered on a given
// new-file line. Optional channel — current handlers wire scroll directly,
// but this message is reserved for cross-pane requests (e.g. CLI hooks).
type ScrollDiffToLineMsg struct {
	NewLine int
}

type ErrMsg struct {
	Err error
}

// composeBodyMsg fires when the comment-input source produces a body.
// The $EDITOR path emits this from tea.ExecProcess's exit callback;
// the textarea fallback emits it from its Ctrl+S save handler.
// Empty body (after TrimSpace) cancels — Compose state is cleared.
// A non-empty body transitions Compose to Submitting and the Update
// handler queues submitComposeCmd to POST it to GitHub as part of the
// user's pending review.
type composeBodyMsg struct {
	body string
	err  error
}

// composeSubmittedMsg fires when the GraphQL POST completes.
// Success: the Update handler appends `comment` (Pending=true since
// the review is still draft) to PR.Comments and clears Compose.
// Failure: ComposeStatus is flipped to Failed with ErrMsg populated;
// the body buffer is preserved so Ctrl+S retries without re-typing.
type composeSubmittedMsg struct {
	comment *model.ReviewComment
	err     error
}

// commentsRefreshedMsg carries the freshly-fetched comment list after
// a successful compose POST. The Update handler merges PR.Comments
// (preserving any locally-known Pending entries the refresh response
// has not seen yet) and recomputes per-file CommentCount so the pane
// chrome stays in sync.
type commentsRefreshedMsg struct {
	comments []*model.ReviewComment
	err      error
}
