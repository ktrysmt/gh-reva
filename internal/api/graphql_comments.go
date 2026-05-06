package api

import (
	"context"
	"fmt"
	"time"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// listCommentsQuery walks pullRequest.reviewThreads via cursor pagination,
// flattening each thread's comments into a single ReviewComment slice.
// The thread's `id` is captured on every comment in the thread so the
// reply mutation has it ready without a separate round-trip. Pending
// comments are detected via `pullRequestReview.state == PENDING`.
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
          comments(first: 100) {
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
    }
  }
}`

type gqlReviewComment struct {
	ID                string `json:"id"`
	DatabaseID        int64  `json:"databaseId"`
	Author            struct {
		Login string `json:"login"`
	} `json:"author"`
	Body           string    `json:"body"`
	CreatedAt      time.Time `json:"createdAt"`
	Path           string    `json:"path"`
	Line           int       `json:"line"`
	OriginalLine   int       `json:"originalLine"`
	Side           string    `json:"side"`
	DiffHunk       string    `json:"diffHunk"`
	Commit         struct {
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
	ID         string `json:"id"`
	IsOutdated bool   `json:"isOutdated"`
	Comments   struct {
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

func convertGQLComment(gc gqlReviewComment, thread gqlReviewThread) *model.ReviewComment {
	rc := &model.ReviewComment{
		ID:               gc.DatabaseID,
		NodeID:           gc.ID,
		ThreadID:         thread.ID,
		Path:             gc.Path,
		CommitID:         gc.Commit.OID,
		OriginalCommitID: gc.OriginalCommit.OID,
		Line:             gc.Line,
		OriginalLine:     gc.OriginalLine,
		Side:             gc.Side,
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
