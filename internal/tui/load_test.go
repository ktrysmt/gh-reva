package tui

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ktrysmt/gh-reva/internal/api"
	"github.com/ktrysmt/gh-reva/internal/model"
)

// timingClient is a fake api.Client whose top-level read methods sleep
// `delay` to model network latency. Concurrent calls increment `peak` so
// the test can assert real parallelism instead of inferring it from
// elapsed time alone (a slow CI machine could mask a sequential
// regression on time-based assertions otherwise).
type timingClient struct {
	delay     time.Duration
	inflight  int32
	peak      int32
	pr        *model.PR
	commits   []*model.Commit
	files     []*model.FileEntry
	comments  []*model.ReviewComment
	viewer    string
	diffs     map[string]string
	calls     []string
	callsMu   chan struct{} // guards `calls`
	callOrder []string      // ordered start markers for diagnostics
}

func newTimingClient(delay time.Duration) *timingClient {
	return &timingClient{
		delay:    delay,
		pr:       &model.PR{Owner: "o", Repo: "r", Number: 1, HeadSHA: "head", BaseSHA: "base"},
		commits:  []*model.Commit{{SHA: "abc1234567", ShortSHA: "abc1234", Message: "c0"}},
		files:    []*model.FileEntry{{Path: "foo.go", Status: model.ChangeModified}},
		comments: []*model.ReviewComment{{ID: 1, NodeID: "n1", Path: "foo.go", User: "you"}},
		viewer:   "you",
		diffs:    map[string]string{},
		callsMu:  make(chan struct{}, 1),
	}
}

func (c *timingClient) sleep(name string) {
	c.callsMu <- struct{}{}
	c.callOrder = append(c.callOrder, name)
	<-c.callsMu

	cur := atomic.AddInt32(&c.inflight, 1)
	for {
		old := atomic.LoadInt32(&c.peak)
		if cur <= old || atomic.CompareAndSwapInt32(&c.peak, old, cur) {
			break
		}
	}
	time.Sleep(c.delay)
	atomic.AddInt32(&c.inflight, -1)
}

func (c *timingClient) GetPR(ctx context.Context, owner, repo string, n int) (*model.PR, error) {
	c.sleep("GetPR")
	return c.pr, nil
}
func (c *timingClient) ListCommits(ctx context.Context, owner, repo string, n int) ([]*model.Commit, error) {
	c.sleep("ListCommits")
	return c.commits, nil
}
func (c *timingClient) ListFiles(ctx context.Context, owner, repo string, n int) ([]*model.FileEntry, error) {
	c.sleep("ListFiles")
	return c.files, nil
}
func (c *timingClient) ListComments(ctx context.Context, owner, repo string, n int) ([]*model.ReviewComment, error) {
	c.sleep("ListComments")
	return c.comments, nil
}
func (c *timingClient) GetFileDiff(ctx context.Context, owner, repo string, n int, sha, path string) (string, error) {
	return c.diffs[sha+"::"+path], nil
}
func (c *timingClient) GetFileContents(ctx context.Context, owner, repo string, n int, ref, path string) ([]string, error) {
	return nil, nil
}
func (c *timingClient) ViewerLogin(ctx context.Context) (string, error) {
	c.sleep("ViewerLogin")
	return c.viewer, nil
}
func (c *timingClient) ResolveCurrentBranchPR(ctx context.Context) (string, string, int, error) {
	return "o", "r", 1, nil
}
func (c *timingClient) CreatePendingReviewThread(ctx context.Context, owner, repo string, n int, in api.CreatePendingThreadInput) (*model.ReviewComment, error) {
	return nil, nil
}
func (c *timingClient) CreatePendingReviewThreadReply(ctx context.Context, owner, repo string, n int, parentThreadID, body string) (*model.ReviewComment, error) {
	return nil, nil
}
func (c *timingClient) UpdateReviewComment(ctx context.Context, owner, repo string, n int, commentNodeID, body string) (*model.ReviewComment, error) {
	return nil, nil
}

// loadPRCmd must fan its independent read methods (GetPR, ListCommits,
// ListFiles, ListComments, ViewerLogin) out concurrently. The original
// tea.Sequence pipeline serialized them — total wall time ≈ 5 * RTT for
// the load-bearing minimum even on PRs with no patch data, leaving the
// splash up gratuitously long.
//
// 5 methods × 80ms each: sequential ≥ 400ms, parallel ≈ 80ms. Threshold
// 250ms catches a regression to sequential while keeping a generous
// margin for slow CI machines. The peak-in-flight check (>=2)
// independently guards against a misleading time-based pass on a
// machine that happened to undersleep.
func TestLoadPRCmd_RunsIndependentStagesInParallel(t *testing.T) {
	c := newTimingClient(80 * time.Millisecond)
	target := &api.Target{Owner: "o", Repo: "r", Number: 1}

	cmd := loadPRCmd(c, target)
	if cmd == nil {
		t.Fatalf("loadPRCmd returned nil cmd")
	}
	start := time.Now()
	msg := cmd()
	elapsed := time.Since(start)

	loaded, ok := msg.(PRLoadedMsg)
	if !ok {
		t.Fatalf("expected PRLoadedMsg, got %T (%+v)", msg, msg)
	}
	if loaded.PR == nil {
		t.Fatalf("PRLoadedMsg.PR is nil")
	}
	if loaded.ViewerLogin != "you" {
		t.Fatalf("ViewerLogin = %q, want %q", loaded.ViewerLogin, "you")
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("loadPRCmd ran sequentially (%v >= 250ms); expected parallel fan-out. Order: %v", elapsed, c.callOrder)
	}
	if peak := atomic.LoadInt32(&c.peak); peak < 2 {
		t.Fatalf("peak in-flight stages = %d; expected >=2 (parallelism not engaged). Order: %v", peak, c.callOrder)
	}
}

// PRLoadedMsg.PR.Files[i].CommentCount must be derived from the
// independently-fetched comments list, not from the ListFiles response.
// In the parallel-load world ListFiles no longer round-trips through
// ListComments, so the count derivation moved into loadPRCmd's
// assembler.
func TestLoadPRCmd_ComputesCommentCountsInAssembler(t *testing.T) {
	c := newTimingClient(0)
	c.files = []*model.FileEntry{
		{Path: "foo.go", Status: model.ChangeModified},
		{Path: "bar.go", Status: model.ChangeAdded},
	}
	c.comments = []*model.ReviewComment{
		{ID: 1, NodeID: "n1", Path: "foo.go"},
		{ID: 2, NodeID: "n2", Path: "foo.go"},
		{ID: 3, NodeID: "n3", Path: "bar.go"},
		// Outdated comment must NOT count — mirrors §4 #25 behaviour.
		{ID: 4, NodeID: "n4", Path: "foo.go", Outdated: true},
	}
	target := &api.Target{Owner: "o", Repo: "r", Number: 1}

	msg := loadPRCmd(c, target)()
	loaded, ok := msg.(PRLoadedMsg)
	if !ok {
		t.Fatalf("expected PRLoadedMsg, got %T", msg)
	}
	got := map[string]int{}
	for _, f := range loaded.PR.Files {
		got[f.Path] = f.CommentCount
	}
	if got["foo.go"] != 2 {
		t.Fatalf("foo.go CommentCount = %d, want 2 (outdated must be excluded)", got["foo.go"])
	}
	if got["bar.go"] != 1 {
		t.Fatalf("bar.go CommentCount = %d, want 1", got["bar.go"])
	}
}

// Errors from any one stage must surface as ErrMsg, not a partial
// PRLoadedMsg. Without this the TUI would render with a nil
// commits/files slice and panic on the first cursor move.
func TestLoadPRCmd_PropagatesStageError(t *testing.T) {
	c := api.NewErrorClient("not_found")
	target := &api.Target{Owner: "o", Repo: "r", Number: 1}
	msg := loadPRCmd(c, target)()
	if _, ok := msg.(ErrMsg); !ok {
		t.Fatalf("expected ErrMsg, got %T (%+v)", msg, msg)
	}
}
