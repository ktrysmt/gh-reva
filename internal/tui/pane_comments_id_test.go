package tui

import (
	"strings"
	"testing"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// patchSnippet mirrors what commentsModelFixture loads under
// diffKey("", "src/foo.go"); the IDSlot test re-keys it onto the
// single-commit SHA so an outdated comment still renders.
const patchSnippet = "@@ -1,3 +1,5 @@\n line1\n+addedLine2\n+addedLine3\n line4\n line5"

// Comments header carries the per-comment numeric ID rendered as
// `#<id>` between the commit short-SHA and any trailing tag. Lets
// reviewers copy the literal ID without breaking out to the web UI.
func TestCommentsView_HeaderIncludesCommentID(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.DiffCursor.Line = 2 // anchor of t1 root + reply (ID 1 + 2)

	got := m.commentsView()
	if !strings.Contains(got, "#1") {
		t.Errorf("Comments header must include `#1` for root comment ID=1:\n%s", got)
	}
	if !strings.Contains(got, "#2") {
		t.Errorf("Comments header must include `#2` for reply comment ID=2:\n%s", got)
	}
}

// `#<id>` sits between the short SHA and the trailing tag so the order
// is `<sha> #<id> [outdated]`. Pins the slot so a future refactor
// doesn't quietly drift the layout.
func TestCommentsView_IDSlotBetweenShaAndTrailingTag(t *testing.T) {
	m := commentsModelFixture(t)
	// Single-commit range honors outdated comments (WholePR filters them
	// out), so the [outdated] tag renders next to the root header — which
	// is what the test asserts the order against.
	m.state.PR.Comments[0].Outdated = true
	m.state.SelectedRange = model.CommitRange{Kind: model.RangeSingleCommit, SHA: "abcdef0123456"}
	m.state.DiffCache[diffKey("abcdef0123456", "src/foo.go")] = patchSnippet
	m.state.DiffCursor.Line = 2

	got := m.commentsView()
	// Find the carol row (root of T1, ID=1).
	var headerLine string
	for _, l := range strings.Split(got, "\n") {
		if strings.Contains(l, "carol:") {
			headerLine = l
			break
		}
	}
	if headerLine == "" {
		t.Fatalf("could not find carol's header line:\n%s", got)
	}
	shaIdx := strings.Index(headerLine, "abcdef0")
	idIdx := strings.Index(headerLine, "#1")
	outIdx := strings.Index(headerLine, "[outdated]")
	if shaIdx < 0 || idIdx < 0 || outIdx < 0 {
		t.Fatalf("missing slot. sha=%d id=%d outdated=%d in %q", shaIdx, idIdx, outIdx, headerLine)
	}
	if !(shaIdx < idIdx && idIdx < outIdx) {
		t.Errorf("expected order sha < #id < [outdated]; got sha=%d id=%d outdated=%d\n%s", shaIdx, idIdx, outIdx, headerLine)
	}
}
