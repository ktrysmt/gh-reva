package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-reva/internal/api"
	"github.com/ktrysmt/gh-reva/internal/model"
)

// refreshCommentsCmd re-runs ListComments and pipes the result back as
// commentsRefreshedMsg. Used after every successful compose POST so
// state.PR.Comments converges on GitHub's authoritative view; also
// reusable from any future "force refresh" gesture.
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
//
// The merge step preserves any locally-known Pending comments that the
// refresh response does not yet include. GitHub's reviewThreads
// endpoint has eventual-consistency lag relative to
// addPullRequestReviewThread — a refresh fired right after a
// successful POST can return the pre-POST snapshot, and a naive
// REPLACE would silently drop the user's just-posted draft from the
// UI until the binary is restarted (the bug reported by users hitting
// "compose, post, comment vanishes"). Once a refresh confirms a draft,
// the merge takes the refresh's authoritative copy.
func (m *Model) applyCommentsRefreshed(msg commentsRefreshedMsg) {
	if msg.err != nil || m.state.PR == nil {
		return
	}
	merged := mergeRefreshedComments(m.state.PR.Comments, msg.comments)
	m.state.PR.Comments = merged
	counts := map[string]int{}
	for _, c := range merged {
		if !c.Outdated {
			counts[c.Path]++
		}
	}
	for _, f := range m.state.PR.Files {
		f.CommentCount = counts[f.Path]
	}
	// PR.Comments was just replaced wholesale — drop the threadsForView
	// cache so subsequent renders pick up the merged list.
	if m.threadsCache != nil {
		m.threadsCache.valid = false
	}
}

// mergeRefreshedComments returns refreshed plus any Pending comments in
// `local` whose NodeID is not represented in `refreshed`. NodeID is the
// stable GraphQL identity GitHub assigns at thread / reply creation
// time, so it survives across refresh roundtrips even when the
// listing's eventual-consistency window misses the comment.
//
// Comments without a NodeID (older fixtures, hypothetical future
// transports) are left to the refresh's authority — the merge cannot
// safely identify them as duplicates without a stable key, and
// preserving them blindly would risk doubling.
func mergeRefreshedComments(local, refreshed []*model.ReviewComment) []*model.ReviewComment {
	if len(local) == 0 {
		return refreshed
	}
	refreshedIDs := make(map[string]bool, len(refreshed))
	for _, c := range refreshed {
		if c != nil && c.NodeID != "" {
			refreshedIDs[c.NodeID] = true
		}
	}
	var preserved []*model.ReviewComment
	for _, c := range local {
		if c == nil || !c.Pending || c.NodeID == "" {
			continue
		}
		if refreshedIDs[c.NodeID] {
			continue
		}
		preserved = append(preserved, c)
	}
	if len(preserved) == 0 {
		return refreshed
	}
	out := make([]*model.ReviewComment, 0, len(refreshed)+len(preserved))
	out = append(out, refreshed...)
	out = append(out, preserved...)
	return out
}
