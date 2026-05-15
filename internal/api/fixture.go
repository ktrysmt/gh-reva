package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// nextFixtureCommentID returns a monotonically increasing comment ID
// shaped like a real GitHub review-comment id (~10-digit int64) so the
// Comments-pane `#<id>` slot stays within the typical column width.
// The previous `time.Now().UnixNano()` generator produced 19-digit ids
// which pushed the trailing `[pending]` / `[outdated]` state tag past
// the right edge of the column for narrow Comments widths.
var fixtureCommentIDCounter int64 = 2_000_000_000

func nextFixtureCommentID() int64 {
	return atomic.AddInt64(&fixtureCommentIDCounter, 1)
}

type fixtureData struct {
	PR       *model.PR              `json:"pr"`
	Commits  []*model.Commit        `json:"commits"`
	Files    []*model.FileEntry     `json:"files"`
	Comments []*model.ReviewComment `json:"comments"`
	Diffs    map[string]string      `json:"diffs"`

	// FileContents maps "<ref>::<path>" → raw file body. The fixture
	// serves these in lieu of GitHub's `repos/.../contents` endpoint to
	// power the Diff pane's context-expand feature. Keys use the same
	// shape as Diffs for consistency.
	FileContents map[string]string `json:"file_contents"`
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

// GetFileContents returns the fixture's stored NEW-side body for
// (ref, path). The fixture key shape is "<ref>::<path>"; trailing
// newlines are trimmed so callers observe len(lines) == file line
// count (no phantom blank tail line at EOF).
func (c *fixtureClient) GetFileContents(ctx context.Context, owner, repo string, n int, ref, path string) ([]string, error) {
	c.wait()
	key := ref + "::" + path
	body, ok := c.d.FileContents[key]
	if !ok {
		return nil, fmt.Errorf("fixture: no file contents for %q", key)
	}
	trimmed := body
	for len(trimmed) > 0 && trimmed[len(trimmed)-1] == '\n' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	if trimmed == "" {
		return []string{}, nil
	}
	return splitLines(trimmed), nil
}

func splitLines(s string) []string {
	out := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start <= len(s) {
		out = append(out, s[start:])
	}
	return out
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
// nextFixtureCommentID so they are session-unique and stay realistic-
// sized (~10 digits, matching live GitHub review-comment ids).
func (c *fixtureClient) CreatePendingReviewThread(ctx context.Context, owner, repo string, n int, in CreatePendingThreadInput) (*model.ReviewComment, error) {
	c.wait()
	now := time.Now()
	id := nextFixtureCommentID()
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
	id := nextFixtureCommentID()
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

// UpdateReviewComment edits a comment in the in-memory fixture. Match
// is by NodeID; mismatch returns an error so tests catch typos rather
// than silently no-op'ing. Mirrors the real client's behaviour: only
// the body changes (CreatedAt et al. stay), and the response is the
// updated comment.
func (c *fixtureClient) UpdateReviewComment(ctx context.Context, owner, repo string, n int, commentNodeID, body string) (*model.ReviewComment, error) {
	c.wait()
	for _, p := range c.d.Comments {
		if p.NodeID == commentNodeID {
			p.Body = body
			return p, nil
		}
	}
	return nil, fmt.Errorf("fixture: comment %q not found", commentNodeID)
}

// ViewerLogin returns the synthetic "you" login the fixture uses for
// locally-authored comments (compose / reply impls set User="you").
// The string matches the fixture's own writes so the TUI's
// own-vs-others Enter gate behaves the same against fixtures and live
// GitHub.
func (c *fixtureClient) ViewerLogin(ctx context.Context) (string, error) {
	return "you", nil
}

func (c *fixtureClient) bumpFileCommentCount(path string) {
	for _, f := range c.d.Files {
		if f.Path == path {
			f.CommentCount++
			return
		}
	}
}
