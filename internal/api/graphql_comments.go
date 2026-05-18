package api

import (
	"context"
	"fmt"
	"strings"
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
//
// Each thread's comments(first: 100) page also exposes pageInfo so
// listCommentsGraphQL can detect threads with > 100 comments and fall
// back to listThreadComments for the remainder. Without that fallback
// hot threads silently drop history past comment 100.
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
          isResolved
          path
          line
          originalLine
          startLine
          originalStartLine
          diffSide
          startDiffSide
          comments(first: 100) {
            pageInfo { hasNextPage endCursor }
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

// listThreadCommentsQuery follows up on a thread whose first
// comments(first: 100) page reported hasNextPage: true. Walked via
// node(id: $id) so we re-bind to the same thread by GraphQL identity
// without re-paginating the entire reviewThreads list.
const listThreadCommentsQuery = `
query($id: ID!, $cursor: String) {
  node(id: $id) {
    ... on PullRequestReviewThread {
      comments(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
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
	IsResolved        bool   `json:"isResolved"`
	Path              string `json:"path"`
	Line              int    `json:"line"`
	OriginalLine      int    `json:"originalLine"`
	StartLine         int    `json:"startLine"`
	OriginalStartLine int    `json:"originalStartLine"`
	DiffSide          string `json:"diffSide"`
	StartDiffSide     string `json:"startDiffSide"`
	Comments          struct {
		PageInfo struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
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
// Threads whose first comments page reports hasNextPage: true trigger a
// follow-up via listThreadComments so > 100-comment threads are not
// silently truncated.
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
			if t.Comments.PageInfo.HasNextPage {
				rest, err := c.listThreadComments(ctx, t.ID, t.Comments.PageInfo.EndCursor)
				if err != nil {
					return nil, "", err
				}
				for _, gc := range rest {
					out = append(out, convertGQLComment(gc, t))
				}
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

// listThreadComments paginates through a single thread's comments
// starting after `startCursor`. Used by listCommentsGraphQL when a
// thread's initial comments(first: 100) reports hasNextPage. Returns
// the additional comment nodes; the caller stitches them onto the
// thread's anchor info via convertGQLComment.
func (c *ghClient) listThreadComments(ctx context.Context, threadID, startCursor string) ([]gqlReviewComment, error) {
	var out []gqlReviewComment
	cursor := startCursor
	for {
		vars := map[string]interface{}{"id": threadID, "cursor": cursor}
		var resp struct {
			Node struct {
				Comments struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []gqlReviewComment `json:"nodes"`
				} `json:"comments"`
			} `json:"node"`
		}
		if err := c.gql.DoWithContext(ctx, listThreadCommentsQuery, vars, &resp); err != nil {
			return nil, fmt.Errorf("paginate thread %q: %w", threadID, err)
		}
		out = append(out, resp.Node.Comments.Nodes...)
		if !resp.Node.Comments.PageInfo.HasNextPage {
			break
		}
		cursor = resp.Node.Comments.PageInfo.EndCursor
	}
	return out, nil
}

// sanitizeCommentBody normalizes a comment body received from GitHub
// before it reaches model.ReviewComment.Body. CRLF line endings are
// folded to LF and bare CRs are stripped. The Comments column is joined
// horizontally with Files and Diff during render, so a literal `\r` in
// the body bytes resets the terminal cursor to column 0 of the current
// physical row and the next bytes overwrite the Files content to the
// left, producing the "Files column appears corrupt when comments are
// visible" symptom. Sanitizing at the API ingest covers every
// downstream consumer (renderer, clipboard yank, edit composer).
func sanitizeCommentBody(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	return strings.NewReplacer("\r\n", "\n", "\r", "").Replace(s)
}

// convertGQLComment merges thread-level anchor info (path / line /
// diffSide) with the per-comment payload to produce a fully-populated
// model.ReviewComment. Threads expose the line/side; comments
// themselves only expose body / author / commit metadata.
func convertGQLComment(gc gqlReviewComment, thread gqlReviewThread) *model.ReviewComment {
	rc := &model.ReviewComment{
		ID:                gc.DatabaseID,
		NodeID:            gc.ID,
		ThreadID:          thread.ID,
		Path:              thread.Path,
		CommitID:          gc.Commit.OID,
		OriginalCommitID:  gc.OriginalCommit.OID,
		Line:              thread.Line,
		OriginalLine:      thread.OriginalLine,
		Side:              thread.DiffSide,
		StartLine:         thread.StartLine,
		OriginalStartLine: thread.OriginalStartLine,
		StartSide:         thread.StartDiffSide,
		DiffHunk:          gc.DiffHunk,
		User:              gc.Author.Login,
		CreatedAt:         gc.CreatedAt,
		Body:              sanitizeCommentBody(gc.Body),
		Outdated:          thread.IsOutdated,
		Resolved:          thread.IsResolved,
		Pending:           gc.PullRequestReview.State == "PENDING",
	}
	if gc.ReplyTo != nil {
		rc.InReplyTo = gc.ReplyTo.DatabaseID
	}
	// Fall back from line==0 to originalLine so outdated anchors still
	// land on a buffer row in the Diff renderer (mirroring the REST
	// fallback behaviour). Same rule for the range upper edge.
	if rc.Line == 0 {
		rc.Line = rc.OriginalLine
	}
	if rc.StartLine == 0 {
		rc.StartLine = rc.OriginalStartLine
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
      isResolved
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
