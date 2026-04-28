package api

import (
	"context"
	"errors"

	"github.com/ktrysmt/gh-rv/internal/model"
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

func (c *errorClient) ResolveCurrentBranchPR(ctx context.Context) (string, string, int, error) {
	// All injected error kinds surface here so the error is reported before
	// the TUI starts (avoiding interference from the bubbletea TTY check in
	// non-PTY test environments).
	return "", "", 0, c.err
}
