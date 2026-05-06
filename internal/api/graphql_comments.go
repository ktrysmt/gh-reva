package api

import (
	"context"
	"fmt"
	"time"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// listCommentsQuery walks pullRequest.reviewThreads via cursor pagination.
// Anchor fields (path / line / diffSide) live on the THREAD object in
// GitHub's GraphQL schema — `PullRequestReviewComment` itself only
// carries body/author/diffHunk/commit and the deprecated position
// fields, so we read line/side from the thread and then attach them to
// every comment in that thread when flattening. Pending state is
// detected via `pullRequestReview.state == PENDING`.
const listCommentsQuery = `
query($owner: String!, $name: String!, $number: Int!, $cursor: String) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      id
      reviewThreads(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id
          isOutdated
          path
          line
          originalLine
          startLine
          originalStartLine
          diffSide
          startDiffSide
          comments(first: 100) {
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
    }
  }
}`

type gqlReviewComment struct {
	ID         string `json:"id"`
	DatabaseID int64  `json:"databaseId"`
	Author     struct {
		Login string `json:"login"`
	} `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	DiffHunk  string    `json:"diffHunk"`
	Commit    struct {
		OID string `json:"oid"`
	} `json:"commit"`
	OriginalCommit struct {
		OID string `json:"oid"`
	} `json:"originalCommit"`
	ReplyTo *struct {
		DatabaseID int64 `json:"databaseId"`
	} `json:"replyTo"`
	PullRequestReview struct {
		State string `json:"state"`
	} `json:"pullRequestReview"`
}

type gqlReviewThread struct {
	ID                string `json:"id"`
	IsOutdated        bool   `json:"isOutdated"`
	Path              string `json:"path"`
	Line              int    `json:"line"`
	OriginalLine      int    `json:"originalLine"`
	StartLine         int    `json:"startLine"`
	OriginalStartLine int    `json:"originalStartLine"`
	DiffSide          string `json:"diffSide"`
	StartDiffSide     string `json:"startDiffSide"`
	Comments          struct {
		Nodes []gqlReviewComment `json:"nodes"`
	} `json:"comments"`
}

type gqlListCommentsResponse struct {
	Repository struct {
		PullRequest struct {
			ID            string `json:"id"`
			ReviewThreads struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []gqlReviewThread `json:"nodes"`
			} `json:"reviewThreads"`
		} `json:"pullRequest"`
	} `json:"repository"`
}

// listCommentsGraphQL paginates through reviewThreads, flattens to
// ReviewComment, and returns the PR's GraphQL node ID alongside (so
// callers can cache it for subsequent mutations without a second fetch).
func (c *ghClient) listCommentsGraphQL(ctx context.Context, owner, repo string, number int) ([]*model.ReviewComment, string, error) {
	var (
		out    []*model.ReviewComment
		prID   string
		cursor *string
	)
	for {
		vars := map[string]interface{}{
			"owner":  owner,
			"name":   repo,
			"number": number,
			"cursor": cursor,
		}
		var resp gqlListCommentsResponse
		if err := c.gql.DoWithContext(ctx, listCommentsQuery, vars, &resp); err != nil {
			return nil, "", fmt.Errorf("list comments: %w", err)
		}
		pr := resp.Repository.PullRequest
		if prID == "" {
			prID = pr.ID
		}
		for _, t := range pr.ReviewThreads.Nodes {
			for _, gc := range t.Comments.Nodes {
				rc := convertGQLComment(gc, t)
				out = append(out, rc)
			}
		}
		if !pr.ReviewThreads.PageInfo.HasNextPage {
			break
		}
		next := pr.ReviewThreads.PageInfo.EndCursor
		cursor = &next
	}
	return out, prID, nil
}

// convertGQLComment merges thread-level anchor info (path / line /
// diffSide) with the per-comment payload to produce a fully-populated
// model.ReviewComment. Threads expose the line/side; comments
// themselves only expose body / author / commit metadata.
func convertGQLComment(gc gqlReviewComment, thread gqlReviewThread) *model.ReviewComment {
	rc := &model.ReviewComment{
		ID:               gc.DatabaseID,
		NodeID:           gc.ID,
		ThreadID:         thread.ID,
		Path:             thread.Path,
		CommitID:         gc.Commit.OID,
		OriginalCommitID: gc.OriginalCommit.OID,
		Line:             thread.Line,
		OriginalLine:     thread.OriginalLine,
		Side:             thread.DiffSide,
		DiffHunk:         gc.DiffHunk,
		User:             gc.Author.Login,
		CreatedAt:        gc.CreatedAt,
		Body:             gc.Body,
		Outdated:         thread.IsOutdated,
		Pending:          gc.PullRequestReview.State == "PENDING",
	}
	if gc.ReplyTo != nil {
		rc.InReplyTo = gc.ReplyTo.DatabaseID
	}
	// Fall back from line==0 to originalLine so outdated anchors still
	// land on a buffer row in the Diff renderer (mirroring the REST
	// fallback behaviour).
	if rc.Line == 0 {
		rc.Line = rc.OriginalLine
	}
	return rc
}

// fetchThreadInfo loads anchor info for a single thread via
// `node(id: $id)`. Used by CreatePendingReviewThreadReply to enrich
// the mutation response — the reply mutation only returns the comment,
// not the thread, so line / diffSide / path have to come from a
// separate query.
func (c *ghClient) fetchThreadInfo(ctx context.Context, threadID string) (gqlReviewThread, error) {
	const q = `
query($id: ID!) {
  node(id: $id) {
    ... on PullRequestReviewThread {
      id
      isOutdated
      path
      line
      originalLine
      startLine
      originalStartLine
      diffSide
      startDiffSide
    }
  }
}`
	var resp struct {
		Node gqlReviewThread `json:"node"`
	}
	if err := c.gql.DoWithContext(ctx, q, map[string]interface{}{"id": threadID}, &resp); err != nil {
		return gqlReviewThread{}, fmt.Errorf("fetch thread info: %w", err)
	}
	return resp.Node, nil
}
