package api

import (
	"context"
	"fmt"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// ViewerLogin returns the authenticated user's GitHub login. Cached
// per-Client; the first call issues a tiny `query { viewer { login } }`
// (or returns the cache populated as a side effect of
// ensurePendingReview's discovery query). Subsequent calls reuse the
// cached value. Used by the Comments-pane Enter dispatch to gate
// "edit own comment" vs "reply-only on others' comments".
func (c *ghClient) ViewerLogin(ctx context.Context) (string, error) {
	if c.viewerLogin != "" {
		return c.viewerLogin, nil
	}
	const q = `query { viewer { login } }`
	var resp struct {
		Viewer struct {
			Login string `json:"login"`
		} `json:"viewer"`
	}
	if err := c.gql.DoWithContext(ctx, q, nil, &resp); err != nil {
		return "", fmt.Errorf("viewer login: %w", err)
	}
	if resp.Viewer.Login == "" {
		return "", fmt.Errorf("viewer login: empty response")
	}
	c.viewerLogin = resp.Viewer.Login
	return resp.Viewer.Login, nil
}

// ensurePRNodeID returns the GraphQL node ID for the given PR. The
// listCommentsGraphQL pass populates the cache as a side effect; the
// fallback path below issues a tiny PR-only query when callers reach
// the POST methods before they ever called ListComments.
func (c *ghClient) ensurePRNodeID(ctx context.Context, owner, repo string, n int) (string, error) {
	if id, ok := c.prNodeID[n]; ok && id != "" {
		return id, nil
	}
	const q = `
query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) { id }
  }
}`
	var resp struct {
		Repository struct {
			PullRequest struct {
				ID string `json:"id"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	vars := map[string]interface{}{"owner": owner, "name": repo, "number": n}
	if err := c.gql.DoWithContext(ctx, q, vars, &resp); err != nil {
		return "", fmt.Errorf("resolve PR node id: %w", err)
	}
	if resp.Repository.PullRequest.ID == "" {
		return "", fmt.Errorf("PR %s/%s#%d has no node id", owner, repo, n)
	}
	c.prNodeID[n] = resp.Repository.PullRequest.ID
	return resp.Repository.PullRequest.ID, nil
}

// ensurePendingReview returns the node ID of the user's current
// pending review on the given PR, creating one via
// addPullRequestReview (event omitted) if none exists. The cache is
// per-ghClient instance; SubmitPendingReview drops the entry on
// success so the next compose POST starts a fresh draft.
//
// Discovery filters reviews by `states: PENDING` and matches author
// against `viewer.login` so a user with a previously-submitted
// (APPROVED / COMMENTED / CHANGES_REQUESTED) review on the same PR
// does not collide with the "one pending review per user per PR"
// constraint. Earlier the query used `viewerLatestReview` which
// returns the *latest* review regardless of state — when that latest
// review was non-PENDING, the code below would call
// addPullRequestReview and the GraphQL API would fail with 422
// because a separate PENDING review already existed.
func (c *ghClient) ensurePendingReview(ctx context.Context, owner, repo string, n int) (string, error) {
	if id, ok := c.pendingReviewID[n]; ok && id != "" {
		return id, nil
	}
	prID, err := c.ensurePRNodeID(ctx, owner, repo, n)
	if err != nil {
		return "", err
	}
	// 1. Look for an existing PENDING review by the viewer. The
	// `states: [PENDING]` filter already narrows server-side; we still
	// match author client-side because GitHub does not expose a
	// `reviews(author:)` filter and any user could theoretically have a
	// pending review (it just isn't visible to non-authors).
	const pendingQ = `
query($owner: String!, $name: String!, $number: Int!) {
  viewer { login }
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviews(states: [PENDING], first: 50) {
        nodes { id author { login } }
      }
    }
  }
}`
	var pendingResp struct {
		Viewer struct {
			Login string `json:"login"`
		} `json:"viewer"`
		Repository struct {
			PullRequest struct {
				Reviews struct {
					Nodes []struct {
						ID     string `json:"id"`
						Author struct {
							Login string `json:"login"`
						} `json:"author"`
					} `json:"nodes"`
				} `json:"reviews"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	vars := map[string]interface{}{"owner": owner, "name": repo, "number": n}
	if err := c.gql.DoWithContext(ctx, pendingQ, vars, &pendingResp); err != nil {
		return "", fmt.Errorf("query pending review: %w", err)
	}
	viewerLogin := pendingResp.Viewer.Login
	if viewerLogin != "" {
		// Side-effect cache: ensurePendingReview already pays for the
		// viewer query, so let ViewerLogin readers reuse the result
		// instead of double-billing the round trip.
		c.viewerLogin = viewerLogin
	}
	for _, r := range pendingResp.Repository.PullRequest.Reviews.Nodes {
		if r.Author.Login == viewerLogin {
			c.pendingReviewID[n] = r.ID
			return r.ID, nil
		}
	}
	// 2. None exists — create a fresh pending review.
	const createMut = `
mutation($input: AddPullRequestReviewInput!) {
  addPullRequestReview(input: $input) {
    pullRequestReview { id }
  }
}`
	var createResp struct {
		AddPullRequestReview struct {
			PullRequestReview struct {
				ID string `json:"id"`
			} `json:"pullRequestReview"`
		} `json:"addPullRequestReview"`
	}
	createVars := map[string]interface{}{
		"input": map[string]interface{}{
			"pullRequestId": prID,
		},
	}
	if err := c.gql.DoWithContext(ctx, createMut, createVars, &createResp); err != nil {
		return "", fmt.Errorf("create pending review: %w", err)
	}
	id := createResp.AddPullRequestReview.PullRequestReview.ID
	if id == "" {
		return "", fmt.Errorf("create pending review: empty id")
	}
	c.pendingReviewID[n] = id
	return id, nil
}

// CreatePendingReviewThread implements Client by ensuring a pending
// review exists, then issuing addPullRequestReviewThread. The mutation
// returns the new thread + its first comment, which we shape back into
// a model.ReviewComment with Pending=true.
func (c *ghClient) CreatePendingReviewThread(ctx context.Context, owner, repo string, n int, in CreatePendingThreadInput) (*model.ReviewComment, error) {
	reviewID, err := c.ensurePendingReview(ctx, owner, repo, n)
	if err != nil {
		return nil, err
	}
	input := map[string]interface{}{
		"pullRequestReviewId": reviewID,
		"path":                in.Path,
		"line":                in.Line,
		"side":                in.Side,
		"body":                in.Body,
		"subjectType":         "LINE",
	}
	if in.StartLine != nil && in.StartSide != "" {
		input["startLine"] = *in.StartLine
		input["startSide"] = in.StartSide
	}
	const mut = `
mutation($input: AddPullRequestReviewThreadInput!) {
  addPullRequestReviewThread(input: $input) {
    thread {
      id
      isOutdated
      path
      line
      originalLine
      startLine
      originalStartLine
      diffSide
      startDiffSide
      comments(first: 1) {
        nodes {
          id
          databaseId
          author { login }
          body
          createdAt
          diffHunk
          commit { oid }
          originalCommit { oid }
          replyTo { databaseId }
          pullRequestReview { state }
        }
      }
    }
  }
}`
	var resp struct {
		AddPullRequestReviewThread struct {
			Thread gqlReviewThread `json:"thread"`
		} `json:"addPullRequestReviewThread"`
	}
	if err := c.gql.DoWithContext(ctx, mut, map[string]interface{}{"input": input}, &resp); err != nil {
		return nil, fmt.Errorf("addPullRequestReviewThread: %w", err)
	}
	thread := resp.AddPullRequestReviewThread.Thread
	if len(thread.Comments.Nodes) == 0 {
		return nil, fmt.Errorf("addPullRequestReviewThread: no comment in response")
	}
	rc := convertGQLComment(thread.Comments.Nodes[0], thread)
	c.invalidateCommentsCache(n)
	return rc, nil
}

// CreatePendingReviewThreadReply attaches a reply to an existing
// thread on the user's pending review. Replies inherit the thread's
// anchor; the GraphQL response gives us back the new comment fully
// resolved.
func (c *ghClient) CreatePendingReviewThreadReply(ctx context.Context, owner, repo string, n int, parentThreadID, body string) (*model.ReviewComment, error) {
	reviewID, err := c.ensurePendingReview(ctx, owner, repo, n)
	if err != nil {
		return nil, err
	}
	const mut = `
mutation($input: AddPullRequestReviewThreadReplyInput!) {
  addPullRequestReviewThreadReply(input: $input) {
    comment {
      id
      databaseId
      author { login }
      body
      createdAt
      diffHunk
      commit { oid }
      originalCommit { oid }
      replyTo { databaseId }
      pullRequestReview { state }
    }
  }
}`
	input := map[string]interface{}{
		"pullRequestReviewId":       reviewID,
		"pullRequestReviewThreadId": parentThreadID,
		"body":                      body,
	}
	var resp struct {
		AddPullRequestReviewThreadReply struct {
			Comment gqlReviewComment `json:"comment"`
		} `json:"addPullRequestReviewThreadReply"`
	}
	if err := c.gql.DoWithContext(ctx, mut, map[string]interface{}{"input": input}, &resp); err != nil {
		return nil, fmt.Errorf("addPullRequestReviewThreadReply: %w", err)
	}
	// The reply mutation's payload only carries the comment, not the
	// thread — anchor info (path / line / diffSide) has to come from a
	// follow-up node query so the returned ReviewComment is fully
	// populated. Errors there are non-fatal: the reply still posted
	// successfully on GitHub. Recovery uses the cached comment list to
	// find the parent thread's anchor (Path / Line / Side); this keeps
	// the immediate UI render stable even if the network blipped.
	// Belt-and-braces: the upstream call site queues refreshCommentsCmd
	// after every successful POST, so the cached anchor only matters
	// for the brief interval before the refresh lands.
	thread, ferr := c.fetchThreadInfo(ctx, parentThreadID)
	if ferr != nil || thread.Path == "" {
		thread = c.fallbackThreadFromCache(n, parentThreadID, thread)
	}
	if thread.ID == "" {
		thread.ID = parentThreadID
	}
	rc := convertGQLComment(resp.AddPullRequestReviewThreadReply.Comment, thread)
	c.invalidateCommentsCache(n)
	return rc, nil
}

// UpdateReviewComment edits the body of an existing PR review comment
// via GraphQL `updatePullRequestReviewComment`. GitHub only accepts the
// mutation when the authenticated viewer is the comment's author; the
// callsite in the TUI (`pane_comments.handleKeyComments`) already gates
// the keystroke on a viewer-vs-User comparison, so the GraphQL 403 path
// here is a defence-in-depth signal rather than the primary UX channel.
//
// The mutation returns the updated PullRequestReviewComment alone (no
// thread anchor); we re-stitch Path / Line / DiffSide from the cached
// pre-edit comment in `c.comments[n]` keyed on the same NodeID. The
// upstream applyComposeSubmitted handler queues refreshCommentsCmd so
// any drift from this stitching is healed by the canonical refresh
// within the same frame.
func (c *ghClient) UpdateReviewComment(ctx context.Context, owner, repo string, n int, commentNodeID, body string) (*model.ReviewComment, error) {
	const mut = `
mutation($input: UpdatePullRequestReviewCommentInput!) {
  updatePullRequestReviewComment(input: $input) {
    pullRequestReviewComment {
      id
      databaseId
      author { login }
      body
      createdAt
      diffHunk
      commit { oid }
      originalCommit { oid }
      replyTo { databaseId }
      pullRequestReview { state }
    }
  }
}`
	input := map[string]interface{}{
		"pullRequestReviewCommentId": commentNodeID,
		"body":                       body,
	}
	var resp struct {
		UpdatePullRequestReviewComment struct {
			Comment gqlReviewComment `json:"pullRequestReviewComment"`
		} `json:"updatePullRequestReviewComment"`
	}
	if err := c.gql.DoWithContext(ctx, mut, map[string]interface{}{"input": input}, &resp); err != nil {
		return nil, fmt.Errorf("updatePullRequestReviewComment: %w", err)
	}
	thread := c.fallbackThreadFromCache(n, "", gqlReviewThread{})
	thread = c.threadByCommentNodeID(n, commentNodeID, thread)
	rc := convertGQLComment(resp.UpdatePullRequestReviewComment.Comment, thread)
	c.invalidateCommentsCache(n)
	return rc, nil
}

// threadByCommentNodeID looks up a cached comment by its NodeID and
// merges the comment's anchor onto `partial`. Used by
// UpdateReviewComment to recover Path / Line / DiffSide that the
// updatePullRequestReviewComment mutation does not return on its own.
func (c *ghClient) threadByCommentNodeID(n int, commentNodeID string, partial gqlReviewThread) gqlReviewThread {
	cached, ok := c.comments[n]
	if !ok {
		return partial
	}
	for _, rc := range cached {
		if rc.NodeID != commentNodeID {
			continue
		}
		out := partial
		if out.ID == "" {
			out.ID = rc.ThreadID
		}
		if out.Path == "" {
			out.Path = rc.Path
		}
		if out.Line == 0 {
			out.Line = rc.Line
		}
		if out.OriginalLine == 0 {
			out.OriginalLine = rc.OriginalLine
		}
		if out.DiffSide == "" {
			out.DiffSide = rc.Side
		}
		return out
	}
	return partial
}

// fallbackThreadFromCache rebuilds anchor info (path / line / diffSide)
// from the cached comment list when fetchThreadInfo errors or returns
// an empty thread. Looks up any cached comment whose ThreadID matches
// `parentThreadID` and copies its anchor onto the returned thread
// stub. Falls back to the input `partial` (which already carries
// whatever fetchThreadInfo did manage to surface) when no cached match
// is found, so callers never see a freshly-zeroed struct.
func (c *ghClient) fallbackThreadFromCache(n int, parentThreadID string, partial gqlReviewThread) gqlReviewThread {
	cached, ok := c.comments[n]
	if !ok {
		return partial
	}
	for _, rc := range cached {
		if rc.ThreadID != parentThreadID {
			continue
		}
		out := partial
		if out.ID == "" {
			out.ID = parentThreadID
		}
		if out.Path == "" {
			out.Path = rc.Path
		}
		if out.Line == 0 {
			out.Line = rc.Line
		}
		if out.OriginalLine == 0 {
			out.OriginalLine = rc.OriginalLine
		}
		if out.DiffSide == "" {
			out.DiffSide = rc.Side
		}
		return out
	}
	return partial
}

// invalidateCommentsCache drops the cached comment listing + files
// listing for a PR so the next List* refetches state. Called after
// every pending POST so the next refresh sees fresh data.
func (c *ghClient) invalidateCommentsCache(n int) {
	delete(c.comments, n)
	delete(c.prFiles, n)
}
