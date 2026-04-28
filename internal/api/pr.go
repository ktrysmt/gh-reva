package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ktrysmt/gh-rv/internal/model"
)

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

type ghReviewComment struct {
	ID                 int64     `json:"id"`
	Path               string    `json:"path"`
	CommitID           string    `json:"commit_id"`
	OriginalCommitID   string    `json:"original_commit_id"`
	Line               int       `json:"line"`
	OriginalLine       int       `json:"original_line"`
	DiffHunk           string    `json:"diff_hunk"`
	InReplyToID        int64     `json:"in_reply_to_id"`
	User               struct{ Login string } `json:"user"`
	CreatedAt          time.Time `json:"created_at"`
	Body               string    `json:"body"`
	Position           *int      `json:"position"`
	OriginalPosition   int       `json:"original_position"`
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
	out := make([]*model.Commit, 0, len(list))
	for _, item := range list {
		detail, err := c.fetchCommit(ctx, owner, repo, item.SHA)
		if err != nil {
			return nil, err
		}
		out = append(out, &model.Commit{
			SHA:          item.SHA,
			ShortSHA:     shortSHA(item.SHA),
			Message:      firstLine(item.Commit.Message),
			Author:       commitAuthor(item),
			Date:         item.Commit.Author.Date,
			ChangedFiles: filesToChangeKinds(detail.Files),
		})
	}
	return out, nil
}

func (c *ghClient) ListFiles(ctx context.Context, owner, repo string, n int) ([]*model.FileEntry, error) {
	files, err := c.fetchPRFiles(ctx, owner, repo, n)
	if err != nil {
		return nil, err
	}
	comments, err := c.ListComments(ctx, owner, repo, n)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, cm := range comments {
		if !cm.Outdated {
			counts[cm.Path]++
		}
	}
	out := make([]*model.FileEntry, 0, len(files))
	for _, f := range files {
		out = append(out, &model.FileEntry{
			Path:         f.Filename,
			Status:       statusToChangeKind(f.Status),
			Additions:    f.Additions,
			Deletions:    f.Deletions,
			CommentCount: counts[f.Filename],
		})
	}
	return out, nil
}

func (c *ghClient) ListComments(ctx context.Context, owner, repo string, n int) ([]*model.ReviewComment, error) {
	if cached, ok := c.comments[n]; ok {
		return cached, nil
	}
	var raw []ghReviewComment
	path := fmt.Sprintf("repos/%s/%s/pulls/%d/comments?per_page=100", owner, repo, n)
	if err := c.paginate(ctx, path, &raw); err != nil {
		return nil, err
	}
	out := make([]*model.ReviewComment, 0, len(raw))
	for _, r := range raw {
		out = append(out, &model.ReviewComment{
			ID:               r.ID,
			Path:             r.Path,
			CommitID:         r.CommitID,
			OriginalCommitID: r.OriginalCommitID,
			Line:             r.Line,
			OriginalLine:     r.OriginalLine,
			DiffHunk:         r.DiffHunk,
			InReplyTo:        r.InReplyToID,
			User:             r.User.Login,
			CreatedAt:        r.CreatedAt,
			Body:             r.Body,
			Outdated:         r.Position == nil && r.OriginalPosition > 0,
		})
	}
	c.comments[n] = out
	return out, nil
}

func (c *ghClient) fetchPRFiles(ctx context.Context, owner, repo string, n int) ([]ghFile, error) {
	if cached, ok := c.prFiles[n]; ok {
		return cached, nil
	}
	var files []ghFile
	path := fmt.Sprintf("repos/%s/%s/pulls/%d/files?per_page=100", owner, repo, n)
	if err := c.paginate(ctx, path, &files); err != nil {
		return nil, err
	}
	c.prFiles[n] = files
	return files, nil
}

func (c *ghClient) fetchCommit(ctx context.Context, owner, repo, sha string) (*ghCommit, error) {
	if cached, ok := c.commits[sha]; ok {
		return cached, nil
	}
	var detail ghCommit
	path := fmt.Sprintf("repos/%s/%s/commits/%s", owner, repo, sha)
	if err := c.rest.DoWithContext(ctx, http.MethodGet, path, nil, &detail); err != nil {
		return nil, err
	}
	c.commits[sha] = &detail
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
