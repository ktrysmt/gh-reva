package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ktrysmt/gh-reva/internal/model"
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
	backfillThreadIDs(d.Comments)
	return &fixtureClient{d: &d}, nil
}

// backfillThreadIDs populates synthetic GraphQL-style thread IDs on
// fixture comments so the reply compose flow has something to point
// at without needing every test JSON file to carry node ids. Roots
// (InReplyTo == 0) get a per-comment ID; replies inherit their
// parent's thread ID.
func backfillThreadIDs(comments []*model.ReviewComment) {
	rootThread := map[int64]string{}
	for _, c := range comments {
		if c.InReplyTo != 0 {
			continue
		}
		if c.ThreadID == "" {
			c.ThreadID = fmt.Sprintf("PRT_fixture_%d", c.ID)
		}
		rootThread[c.ID] = c.ThreadID
	}
	for _, c := range comments {
		if c.InReplyTo == 0 {
			continue
		}
		if c.ThreadID == "" {
			if id, ok := rootThread[c.InReplyTo]; ok {
				c.ThreadID = id
			} else {
				c.ThreadID = fmt.Sprintf("PRT_fixture_%d", c.ID)
			}
		}
	}
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

// CreatePendingReviewThread appends a synthetic ReviewComment
// (Pending=true) to the in-memory fixture as if it were posted under
// a pending review on GitHub. ID / NodeID / ThreadID are derived from
// time.Now().UnixNano() so they are unique within a session yet stay
// distinct from real GitHub IDs.
func (c *fixtureClient) CreatePendingReviewThread(ctx context.Context, owner, repo string, n int, in CreatePendingThreadInput) (*model.ReviewComment, error) {
	c.wait()
	now := time.Now()
	id := now.UnixNano()
	rc := &model.ReviewComment{
		ID:        id,
		NodeID:    fmt.Sprintf("PRRC_pending_%d", id),
		ThreadID:  fmt.Sprintf("PRRT_pending_%d", id),
		Path:      in.Path,
		CommitID:  in.CommitOID,
		Line:      in.Line,
		Side:      in.Side,
		User:      "you",
		CreatedAt: now.UTC(),
		Body:      in.Body,
		Pending:   true,
	}
	c.d.Comments = append(c.d.Comments, rc)
	c.bumpFileCommentCount(in.Path)
	return rc, nil
}

// CreatePendingReviewThreadReply appends a pending reply that
// inherits Path / CommitID / Line / Side / ThreadID from the parent
// thread (looked up by ThreadID across the in-memory comment list).
func (c *fixtureClient) CreatePendingReviewThreadReply(ctx context.Context, owner, repo string, n int, parentThreadID, body string) (*model.ReviewComment, error) {
	c.wait()
	var parent *model.ReviewComment
	for _, p := range c.d.Comments {
		if p.ThreadID == parentThreadID {
			parent = p
			break
		}
	}
	if parent == nil {
		return nil, fmt.Errorf("fixture: thread %q not found", parentThreadID)
	}
	now := time.Now()
	id := now.UnixNano()
	rc := &model.ReviewComment{
		ID:        id,
		NodeID:    fmt.Sprintf("PRRC_pending_%d", id),
		ThreadID:  parent.ThreadID,
		Path:      parent.Path,
		CommitID:  parent.CommitID,
		Line:      parent.Line,
		Side:      parent.Side,
		InReplyTo: parent.ID,
		User:      "you",
		CreatedAt: now.UTC(),
		Body:      body,
		Pending:   true,
	}
	c.d.Comments = append(c.d.Comments, rc)
	c.bumpFileCommentCount(parent.Path)
	return rc, nil
}

// SubmitPendingReview flips Pending=false on every previously-saved
// pending comment in the fixture. Mirrors the real-API behaviour where
// submitting the review surfaces the comments publicly; ListComments
// callers see the updated state on the next refetch.
func (c *fixtureClient) SubmitPendingReview(ctx context.Context, owner, repo string, n int, event model.SubmitEvent, body string) error {
	c.wait()
	for _, p := range c.d.Comments {
		if p.Pending {
			p.Pending = false
		}
	}
	return nil
}

func (c *fixtureClient) bumpFileCommentCount(path string) {
	for _, f := range c.d.Files {
		if f.Path == path {
			f.CommentCount++
			return
		}
	}
}
