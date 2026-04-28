package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ktrysmt/gh-rv/internal/model"
)

type fixtureData struct {
	PR       *model.PR              `json:"pr"`
	Commits  []*model.Commit        `json:"commits"`
	Files    []*model.FileEntry     `json:"files"`
	Comments []*model.ReviewComment `json:"comments"`
	Diffs    map[string]string      `json:"diffs"`
}

type fixtureClient struct {
	d         *fixtureData
	stageWait time.Duration // artificial per-call delay for spinner testing
}

func NewFixtureClient(path string) (Client, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixture: %w", err)
	}
	var d fixtureData
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("parse fixture: %w", err)
	}
	if d.PR == nil {
		return nil, fmt.Errorf("fixture missing pr field")
	}
	return &fixtureClient{d: &d}, nil
}

// WithSlowLoad returns a copy of c that injects `d` into every method call so
// the loading spinner can be observed during tests.
func WithSlowLoad(c Client, d time.Duration) Client {
	if fc, ok := c.(*fixtureClient); ok {
		copy := *fc
		copy.stageWait = d
		return &copy
	}
	return c
}

func (c *fixtureClient) wait() {
	if c.stageWait > 0 {
		time.Sleep(c.stageWait)
	}
}

func (c *fixtureClient) GetPR(ctx context.Context, owner, repo string, n int) (*model.PR, error) {
	c.wait()
	pr := *c.d.PR
	return &pr, nil
}

func (c *fixtureClient) ListCommits(ctx context.Context, owner, repo string, n int) ([]*model.Commit, error) {
	c.wait()
	return c.d.Commits, nil
}

func (c *fixtureClient) ListFiles(ctx context.Context, owner, repo string, n int) ([]*model.FileEntry, error) {
	c.wait()
	return c.d.Files, nil
}

func (c *fixtureClient) ListComments(ctx context.Context, owner, repo string, n int) ([]*model.ReviewComment, error) {
	c.wait()
	return c.d.Comments, nil
}

func (c *fixtureClient) GetFileDiff(ctx context.Context, owner, repo string, n int, sha, path string) (string, error) {
	key := sha + "/" + path
	if sha == "" {
		key = "pr/" + path
	}
	if d, ok := c.d.Diffs[key]; ok {
		return d, nil
	}
	return "", nil
}

func (c *fixtureClient) ResolveCurrentBranchPR(ctx context.Context) (string, string, int, error) {
	return c.d.PR.Owner, c.d.PR.Repo, c.d.PR.Number, nil
}
