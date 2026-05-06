package api

import (
	"context"
	"errors"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// errorClient is a Client that fails every call with a fixed error. Used to
// exercise UI error-path behaviour from CLI / E2E tests.
type errorClient struct {
	kind string
	err  error
}

func NewErrorClient(kind string) Client {
	var err error
	switch kind {
	case "unauth":
		err = errors.New("not authenticated: run `gh auth login`")
	case "not_found":
		err = errors.New("HTTP 404: pull request not found")
	case "rate_limit":
		err = errors.New("HTTP 429: API rate limit exceeded — try again later")
	default:
		err = errors.New("simulated error: " + kind)
	}
	return &errorClient{kind: kind, err: err}
}

func (c *errorClient) GetPR(ctx context.Context, owner, repo string, n int) (*model.PR, error) {
	return nil, c.err
}

func (c *errorClient) ListCommits(ctx context.Context, owner, repo string, n int) ([]*model.Commit, error) {
	return nil, c.err
}

func (c *errorClient) ListFiles(ctx context.Context, owner, repo string, n int) ([]*model.FileEntry, error) {
	return nil, c.err
}

func (c *errorClient) ListComments(ctx context.Context, owner, repo string, n int) ([]*model.ReviewComment, error) {
	return nil, c.err
}

func (c *errorClient) GetFileDiff(ctx context.Context, owner, repo string, n int, sha, path string) (string, error) {
	return "", c.err
}

func (c *errorClient) CreatePendingReviewThread(ctx context.Context, owner, repo string, n int, in CreatePendingThreadInput) (*model.ReviewComment, error) {
	return nil, c.err
}

func (c *errorClient) CreatePendingReviewThreadReply(ctx context.Context, owner, repo string, n int, parentThreadID, body string) (*model.ReviewComment, error) {
	return nil, c.err
}

func (c *errorClient) SubmitPendingReview(ctx context.Context, owner, repo string, n int, event model.SubmitEvent, body string) error {
	return c.err
}

func (c *errorClient) ResolveCurrentBranchPR(ctx context.Context) (string, string, int, error) {
	// Errors surface here for the no-arg flow. For the explicit-PR-arg flow,
	// ParseTargetArg's recovery branch (resolve.go:36-44) silently swallows
	// this error and falls back to currentRepository(); the cmd/root.go
	// pre-flight probe (`client.GetPR` before `tea.NewProgram`) catches the
	// injected error in that path instead, so non-PTY test environments
	// never see the bubbletea TTY check fire.
	return "", "", 0, c.err
}
