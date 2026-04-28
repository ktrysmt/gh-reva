package tui

import (
	"github.com/ktrysmt/gh-rv/internal/model"
)

type PRLoadedMsg struct {
	PR    *model.PR
	Diffs map[string]string
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
