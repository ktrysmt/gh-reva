package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// commitDetailConcurrency caps the number of in-flight `GET
// /repos/.../commits/<sha>` requests issued by ListCommits. GitHub's
// secondary rate-limit guidance discourages "many concurrent requests";
// 8 has been measured as a sweet spot — high enough to amortize round-
// trip latency on PRs with dozens of commits, low enough to stay clear
// of the burst threshold.
const commitDetailConcurrency = 8

var errNotImplemented = errors.New("not implemented")

type ghPRResp struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Base   struct {
		SHA string `json:"sha"`
	} `json:"base"`
	Head struct {
		SHA string `json:"sha"`
	} `json:"head"`
}

type ghFile struct {
	Filename         string `json:"filename"`
	Status           string `json:"status"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Patch            string `json:"patch"`
	PreviousFilename string `json:"previous_filename,omitempty"`
}

type ghCommitListItem struct {
	SHA    string `json:"sha"`
	Commit struct {
		Author struct {
			Name string    `json:"name"`
			Date time.Time `json:"date"`
		} `json:"author"`
		Message string `json:"message"`
	} `json:"commit"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
}

type ghCommit struct {
	SHA    string   `json:"sha"`
	Files  []ghFile `json:"files"`
	Commit struct {
		Author struct {
			Name string    `json:"name"`
			Date time.Time `json:"date"`
		} `json:"author"`
		Message string `json:"message"`
	} `json:"commit"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
}

func (c *ghClient) GetPR(ctx context.Context, owner, repo string, n int) (*model.PR, error) {
	var r ghPRResp
	path := fmt.Sprintf("repos/%s/%s/pulls/%d", owner, repo, n)
	if err := c.rest.DoWithContext(ctx, http.MethodGet, path, nil, &r); err != nil {
		return nil, err
	}
	return &model.PR{
		Owner:   owner,
		Repo:    repo,
		Number:  r.Number,
		Title:   r.Title,
		BaseSHA: r.Base.SHA,
		HeadSHA: r.Head.SHA,
	}, nil
}

func (c *ghClient) ListCommits(ctx context.Context, owner, repo string, n int) ([]*model.Commit, error) {
	var list []ghCommitListItem
	path := fmt.Sprintf("repos/%s/%s/pulls/%d/commits?per_page=100", owner, repo, n)
	if err := c.paginate(ctx, path, &list); err != nil {
		return nil, err
	}
	// Per-commit detail fetches run in parallel under a bounded worker
	// pool so a 60-commit PR doesn't pay 60 sequential round-trips on
	// the loading splash. Writes to `out` are index-disjoint so the
	// final slice is race-free without per-element locking; cache
	// writes inside fetchCommit are guarded by ghClient.cacheMu.
	out := make([]*model.Commit, len(list))
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(commitDetailConcurrency)
	for i, item := range list {
		i, item := i, item
		eg.Go(func() error {
			detail, err := c.fetchCommit(egCtx, owner, repo, item.SHA)
			if err != nil {
				return err
			}
			out[i] = &model.Commit{
				SHA:          item.SHA,
				ShortSHA:     shortSHA(item.SHA),
				Message:      firstLine(item.Commit.Message),
				Author:       commitAuthor(item),
				Date:         item.Commit.Author.Date,
				ChangedFiles: filesToChangeKinds(detail.Files),
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListFiles returns the PR's file roster without per-file comment
// counts. Earlier this method also called ListComments to populate
// CommentCount, which forced a hidden serial dependency on the comments
// fetch — incompatible with loadPRCmd's parallel fan-out. The TUI's
// load assembler now derives CommentCount from the independently-
// fetched comments list before constructing PRLoadedMsg, so callers
// must apply that same count derivation themselves if they need it.
func (c *ghClient) ListFiles(ctx context.Context, owner, repo string, n int) ([]*model.FileEntry, error) {
	files, err := c.fetchPRFiles(ctx, owner, repo, n)
	if err != nil {
		return nil, err
	}
	out := make([]*model.FileEntry, 0, len(files))
	for _, f := range files {
		out = append(out, &model.FileEntry{
			Path:      f.Filename,
			Status:    statusToChangeKind(f.Status),
			Additions: f.Additions,
			Deletions: f.Deletions,
		})
	}
	return out, nil
}

// ListComments fetches the PR's review comments via GraphQL so we
// capture the GraphQL node ID + thread ID needed by the
// addPullRequestReviewThreadReply mutation. The REST endpoint cannot
// return thread IDs (REST has no "thread" abstraction), so the
// migration to GraphQL is mandatory once we want pending-review POSTs.
func (c *ghClient) ListComments(ctx context.Context, owner, repo string, n int) ([]*model.ReviewComment, error) {
	c.cacheMu.Lock()
	cached, ok := c.comments[n]
	c.cacheMu.Unlock()
	if ok {
		return cached, nil
	}
	out, prID, err := c.listCommentsGraphQL(ctx, owner, repo, n)
	if err != nil {
		return nil, err
	}
	c.cacheMu.Lock()
	if prID != "" {
		c.prNodeID[n] = prID
	}
	c.comments[n] = out
	c.cacheMu.Unlock()
	return out, nil
}

func (c *ghClient) fetchPRFiles(ctx context.Context, owner, repo string, n int) ([]ghFile, error) {
	c.cacheMu.Lock()
	cached, ok := c.prFiles[n]
	c.cacheMu.Unlock()
	if ok {
		return cached, nil
	}
	var files []ghFile
	path := fmt.Sprintf("repos/%s/%s/pulls/%d/files?per_page=100", owner, repo, n)
	if err := c.paginate(ctx, path, &files); err != nil {
		return nil, err
	}
	c.cacheMu.Lock()
	c.prFiles[n] = files
	c.cacheMu.Unlock()
	return files, nil
}

func (c *ghClient) fetchCommit(ctx context.Context, owner, repo, sha string) (*ghCommit, error) {
	c.cacheMu.Lock()
	cached, ok := c.commits[sha]
	c.cacheMu.Unlock()
	if ok {
		return cached, nil
	}
	var detail ghCommit
	path := fmt.Sprintf("repos/%s/%s/commits/%s", owner, repo, sha)
	if err := c.rest.DoWithContext(ctx, http.MethodGet, path, nil, &detail); err != nil {
		return nil, err
	}
	c.cacheMu.Lock()
	c.commits[sha] = &detail
	c.cacheMu.Unlock()
	return &detail, nil
}

func filesToChangeKinds(files []ghFile) map[string]model.ChangeKind {
	out := map[string]model.ChangeKind{}
	for _, f := range files {
		out[f.Filename] = statusToChangeKind(f.Status)
	}
	return out
}

func statusToChangeKind(status string) model.ChangeKind {
	switch status {
	case "added", "copied":
		return model.ChangeAdded
	case "removed":
		return model.ChangeDeleted
	case "renamed":
		return model.ChangeRenamed
	default:
		return model.ChangeModified
	}
}

func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}

func commitAuthor(item ghCommitListItem) string {
	if item.Author.Login != "" {
		return item.Author.Login
	}
	return item.Commit.Author.Name
}
