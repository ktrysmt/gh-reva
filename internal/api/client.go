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
}

type ghClient struct {
	rest *gha.RESTClient

	prFiles  map[int][]ghFile
	commits  map[string]*ghCommit
	comments map[int][]*model.ReviewComment
}

func NewGHClient() (Client, error) {
	rest, err := gha.DefaultRESTClient()
	if err != nil {
		return nil, err
	}
	return &ghClient{
		rest:     rest,
		prFiles:  map[int][]ghFile{},
		commits:  map[string]*ghCommit{},
		comments: map[int][]*model.ReviewComment{},
	}, nil
}
