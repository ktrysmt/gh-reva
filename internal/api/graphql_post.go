package api

import (
	"context"
	"fmt"

	"github.com/ktrysmt/gh-reva/internal/model"
)

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
func (c *ghClient) ensurePendingReview(ctx context.Context, owner, repo string, n int) (string, error) {
	if id, ok := c.pendingReviewID[n]; ok && id != "" {
		return id, nil
	}
	prID, err := c.ensurePRNodeID(ctx, owner, repo, n)
	if err != nil {
		return "", err
	}
	// 1. Look for an existing PENDING review by the viewer.
	const viewerQ = `
query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      viewerLatestReview { id state }
    }
  }
}`
	var viewerResp struct {
		Repository struct {
			PullRequest struct {
				ViewerLatestReview *struct {
					ID    string `json:"id"`
					State string `json:"state"`
				} `json:"viewerLatestReview"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	vars := map[string]interface{}{"owner": owner, "name": repo, "number": n}
	if err := c.gql.DoWithContext(ctx, viewerQ, vars, &viewerResp); err != nil {
		return "", fmt.Errorf("query pending review: %w", err)
	}
	if r := viewerResp.Repository.PullRequest.ViewerLatestReview; r != nil && r.State == "PENDING" {
		c.pendingReviewID[n] = r.ID
		return r.ID, nil
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
      comments(first: 1) {
        nodes {
          id
          databaseId
          author { login }
          body
          createdAt
          path
          line
          originalLine
          side
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
      path
      line
      originalLine
      side
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
	rc := convertGQLComment(resp.AddPullRequestReviewThreadReply.Comment, gqlReviewThread{ID: parentThreadID})
	c.invalidateCommentsCache(n)
	return rc, nil
}

// SubmitPendingReview finalizes the cached pending review with the
// chosen event. The cache is cleared on success so the next compose
// POST starts a new draft. Body is optional (mirrors the GraphQL
// schema's nullable body field).
func (c *ghClient) SubmitPendingReview(ctx context.Context, owner, repo string, n int, event model.SubmitEvent, body string) error {
	reviewID, err := c.ensurePendingReview(ctx, owner, repo, n)
	if err != nil {
		return err
	}
	const mut = `
mutation($input: SubmitPullRequestReviewInput!) {
  submitPullRequestReview(input: $input) {
    pullRequestReview { id state }
  }
}`
	input := map[string]interface{}{
		"pullRequestReviewId": reviewID,
		"event":               string(event),
	}
	if body != "" {
		input["body"] = body
	}
	var resp struct {
		SubmitPullRequestReview struct {
			PullRequestReview struct {
				ID    string `json:"id"`
				State string `json:"state"`
			} `json:"pullRequestReview"`
		} `json:"submitPullRequestReview"`
	}
	if err := c.gql.DoWithContext(ctx, mut, map[string]interface{}{"input": input}, &resp); err != nil {
		return fmt.Errorf("submitPullRequestReview: %w", err)
	}
	delete(c.pendingReviewID, n)
	c.invalidateCommentsCache(n)
	return nil
}

// invalidateCommentsCache drops the cached comment listing + files
// listing for a PR so the next List* refetches state. POST and submit
// both flip Pending state; the file CommentCount is computed inside
// ListFiles so that cache must die too.
func (c *ghClient) invalidateCommentsCache(n int) {
	delete(c.comments, n)
	delete(c.prFiles, n)
}
