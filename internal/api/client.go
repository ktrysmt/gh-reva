package api

import (
	"context"

	gha "github.com/cli/go-gh/v2/pkg/api"

	"github.com/ktrysmt/gh-reva/internal/model"
)

type Client interface {
	GetPR(ctx context.Context, owner, repo string, n int) (*model.PR, error)
	ListCommits(ctx context.Context, owner, repo string, n int) ([]*model.Commit, error)
	ListFiles(ctx context.Context, owner, repo string, n int) ([]*model.FileEntry, error)
	ListComments(ctx context.Context, owner, repo string, n int) ([]*model.ReviewComment, error)
	// GetFileDiff returns the unified diff for a single file. sha == "" means the PR-wide diff.
	GetFileDiff(ctx context.Context, owner, repo string, n int, sha, path string) (string, error)
	ResolveCurrentBranchPR(ctx context.Context) (string, string, int, error)

	// CreatePendingReviewThread posts a new inline review comment as
	// part of the user's pending review on this PR. If no pending
	// review exists, one is created via addPullRequestReview (event
	// omitted) on demand. The returned ReviewComment has Pending=true.
	CreatePendingReviewThread(ctx context.Context, owner, repo string, n int, in CreatePendingThreadInput) (*model.ReviewComment, error)

	// CreatePendingReviewThreadReply posts a reply under an existing
	// review thread, attached to the same pending review. parentThreadID
	// is the GraphQL node ID of the thread (NOT a comment node ID — the
	// reply mutation requires the thread's identity).
	CreatePendingReviewThreadReply(ctx context.Context, owner, repo string, n int, parentThreadID, body string) (*model.ReviewComment, error)

	// UpdateReviewComment edits the body of an existing PR review
	// comment via GraphQL `updatePullRequestReviewComment`. The mutation
	// is permitted only on comments authored by the viewer; callers must
	// gate the keystroke with a viewer-vs-comment.User check before
	// invoking. commentNodeID is the comment's GraphQL node ID (NOT
	// thread ID — the comment mutation operates on the comment row).
	UpdateReviewComment(ctx context.Context, owner, repo string, n int, commentNodeID, body string) (*model.ReviewComment, error)

	// ViewerLogin returns the GitHub login of the authenticated user.
	// Cached per-Client; the first call issues `query { viewer { login } }`
	// (or piggybacks on ensurePendingReview's response) and subsequent
	// callers reuse the result. Used by the Comments-pane Enter dispatch
	// to decide whether the cursor comment is editable (own) or
	// reply-only (others').
	ViewerLogin(ctx context.Context) (string, error)
}

// CreatePendingThreadInput is the payload for CreatePendingReviewThread.
// StartLine == nil and StartSide == "" mark a single-line comment;
// non-nil values trigger the multi-line API path.
type CreatePendingThreadInput struct {
	Path      string
	CommitOID string
	Line      int
	Side      string
	StartLine *int
	StartSide string
	Body      string
}

type ghClient struct {
	rest *gha.RESTClient
	gql  *gha.GraphQLClient

	prFiles  map[int][]ghFile
	commits  map[string]*ghCommit
	comments map[int][]*model.ReviewComment

	// prNodeID caches the GraphQL node ID for each PR number we've
	// touched. Mutations need a node ID, not the integer number, so we
	// fetch it lazily on first ListComments / pending-POST call.
	prNodeID map[int]string

	// pendingReviewID caches the GraphQL node ID of the user's current
	// pending review on each PR. Empty entry means "no pending review
	// known yet" — the next compose POST will run the discovery query
	// to reuse an existing PENDING review or create one. The cache is
	// per-Client (per-process) — gh-reva no longer exposes a "submit
	// pending review" gesture, so the entry survives until process exit.
	pendingReviewID map[int]string

	// viewerLogin caches the authenticated user's GitHub login.
	// Populated lazily by ViewerLogin (or as a side effect of
	// ensurePendingReview's discovery query, which already fetches the
	// viewer alongside the PENDING review filter).
	viewerLogin string
}

func NewGHClient() (Client, error) {
	rest, err := gha.DefaultRESTClient()
	if err != nil {
		return nil, err
	}
	gql, err := gha.DefaultGraphQLClient()
	if err != nil {
		return nil, err
	}
	return &ghClient{
		rest:            rest,
		gql:             gql,
		prFiles:         map[int][]ghFile{},
		commits:         map[string]*ghCommit{},
		comments:        map[int][]*model.ReviewComment{},
		prNodeID:        map[int]string{},
		pendingReviewID: map[int]string{},
	}, nil
}
