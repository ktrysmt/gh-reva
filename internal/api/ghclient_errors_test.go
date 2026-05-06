package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	gha "github.com/cli/go-gh/v2/pkg/api"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// redirectTransport rewrites every outbound request to the test server so
// that the go-gh REST client (which mints HTTPS URLs from the configured
// host) can be redirected at an httptest.Server.
type redirectTransport struct{ target *url.URL }

func (r *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = r.target.Scheme
	req.URL.Host = r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// newTestGHClient wires a ghClient at an httptest.Server. The handler
// receives every request — distinguish endpoints by req.URL.Path. Both
// the REST and the GraphQL clients route to the same server, so a
// single handler can switch on `/graphql` vs `/repos/...` paths.
func newTestGHClient(t *testing.T, handler http.HandlerFunc) (*ghClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	rest, err := gha.NewRESTClient(gha.ClientOptions{
		Host:      "test.invalid",
		AuthToken: "test-token",
		Transport: &redirectTransport{target: u},
	})
	if err != nil {
		t.Fatalf("new rest client: %v", err)
	}
	gql, err := gha.NewGraphQLClient(gha.ClientOptions{
		Host:      "test.invalid",
		AuthToken: "test-token",
		Transport: &redirectTransport{target: u},
	})
	if err != nil {
		t.Fatalf("new graphql client: %v", err)
	}
	return &ghClient{
		rest:            rest,
		gql:             gql,
		prFiles:         map[int][]ghFile{},
		commits:         map[string]*ghCommit{},
		comments:        map[int][]*model.ReviewComment{},
		prNodeID:        map[int]string{},
		pendingReviewID: map[int]string{},
	}, srv
}

func TestGHClientGetPR_Unauthorized(t *testing.T) {
	c, _ := newTestGHClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	})
	_, err := c.GetPR(context.Background(), "octocat", "hello", 1)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 in error, got %v", err)
	}
}

func TestGHClientGetPR_NotFound(t *testing.T) {
	c, _ := newTestGHClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	})
	_, err := c.GetPR(context.Background(), "octocat", "hello", 999)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 in error, got %v", err)
	}
}

func TestGHClientListCommits_RateLimit(t *testing.T) {
	c, _ := newTestGHClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	})
	_, err := c.ListCommits(context.Background(), "octocat", "hello", 1)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	low := strings.ToLower(err.Error())
	if !strings.Contains(low, "rate limit") && !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected rate-limit error, got %v", err)
	}
}

func TestGHClientListFiles_Pagination(t *testing.T) {
	// Two-page REST response for /pulls/1/files; GraphQL stub for the
	// reviewThreads query that ListComments now uses internally.
	filesCalls := 0
	c, _ := newTestGHClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/graphql" || strings.HasSuffix(r.URL.Path, "/graphql"):
			// ListComments call from ListFiles. Return one empty page so
			// the count loop terminates immediately.
			_, _ = w.Write([]byte(`{"data":{"repository":{"pullRequest":{"id":"PR_1","reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}}}}}`))
		case strings.Contains(r.URL.Path, "/pulls/1/files"):
			filesCalls++
			switch filesCalls {
			case 1:
				w.Header().Set("Link", `<https://test.invalid/repos/octocat/hello/pulls/1/files?page=2>; rel="next"`)
				_, _ = w.Write([]byte(`[{"filename":"a.go","status":"modified","additions":1,"deletions":0,"patch":"@@ -1 +1 @@"}]`))
			default:
				_, _ = w.Write([]byte(`[{"filename":"b.go","status":"added","additions":2,"deletions":0,"patch":"@@ +1 +1,2 @@"}]`))
			}
		default:
			http.NotFound(w, r)
		}
	})
	files, err := c.ListFiles(context.Background(), "octocat", "hello", 1)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files across pages, got %d", len(files))
	}
	if files[0].Path != "a.go" || files[1].Path != "b.go" {
		t.Fatalf("unexpected file order: %#v", files)
	}
}

// readGraphQLBody pulls the request body, JSON-decodes it, and returns
// the query string (used to discriminate which GraphQL operation a
// stub handler is currently responding to).
func readGraphQLBody(t *testing.T, r *http.Request) string {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read graphql body: %v", err)
	}
	var env struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("decode graphql envelope: %v", err)
	}
	return env.Query
}

// ensurePendingReview must reuse a PENDING review owned by the viewer.
// Earlier the discovery query was viewerLatestReview, which returns the
// most-recent review regardless of state — when the latest review was
// non-PENDING and a separate PENDING review already existed, the next
// path called addPullRequestReview and tripped GitHub's "one pending
// review per user per PR" 422.
func TestEnsurePendingReview_ReusesViewerPending(t *testing.T) {
	addCalls := 0
	c, _ := newTestGHClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/graphql") {
			http.NotFound(w, r)
			return
		}
		q := readGraphQLBody(t, r)
		switch {
		case strings.Contains(q, "pullRequest(number: $number) { id }"):
			_, _ = w.Write([]byte(`{"data":{"repository":{"pullRequest":{"id":"PR_42"}}}}`))
		case strings.Contains(q, "reviews(states: [PENDING]"):
			_, _ = w.Write([]byte(`{"data":{
				"viewer":{"login":"alice"},
				"repository":{"pullRequest":{"reviews":{"nodes":[
					{"id":"PRR_pending_alice","author":{"login":"alice"}}
				]}}}
			}}`))
		case strings.Contains(q, "addPullRequestReview"):
			addCalls++
			_, _ = w.Write([]byte(`{"data":{"addPullRequestReview":{"pullRequestReview":{"id":"PRR_new"}}}}`))
		default:
			t.Fatalf("unexpected graphql query: %s", q)
		}
	})
	id, err := c.ensurePendingReview(context.Background(), "octocat", "hello", 42)
	if err != nil {
		t.Fatalf("ensurePendingReview: %v", err)
	}
	if id != "PRR_pending_alice" {
		t.Fatalf("expected reuse of PRR_pending_alice, got %q", id)
	}
	if addCalls != 0 {
		t.Fatalf("addPullRequestReview must NOT be called when a viewer-owned pending review exists; got %d calls", addCalls)
	}
}

// When the only PENDING review on the PR is owned by another user, the
// viewer still has none — addPullRequestReview is the correct next
// step. Without the author-match filter, the previous behaviour would
// have happily re-used another reviewer's draft.
func TestEnsurePendingReview_IgnoresOtherUsersPending(t *testing.T) {
	addCalls := 0
	c, _ := newTestGHClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/graphql") {
			http.NotFound(w, r)
			return
		}
		q := readGraphQLBody(t, r)
		switch {
		case strings.Contains(q, "pullRequest(number: $number) { id }"):
			_, _ = w.Write([]byte(`{"data":{"repository":{"pullRequest":{"id":"PR_42"}}}}`))
		case strings.Contains(q, "reviews(states: [PENDING]"):
			_, _ = w.Write([]byte(`{"data":{
				"viewer":{"login":"alice"},
				"repository":{"pullRequest":{"reviews":{"nodes":[
					{"id":"PRR_pending_bob","author":{"login":"bob"}}
				]}}}
			}}`))
		case strings.Contains(q, "addPullRequestReview"):
			addCalls++
			_, _ = w.Write([]byte(`{"data":{"addPullRequestReview":{"pullRequestReview":{"id":"PRR_new"}}}}`))
		default:
			t.Fatalf("unexpected graphql query: %s", q)
		}
	})
	id, err := c.ensurePendingReview(context.Background(), "octocat", "hello", 42)
	if err != nil {
		t.Fatalf("ensurePendingReview: %v", err)
	}
	if id != "PRR_new" {
		t.Fatalf("expected freshly created PRR_new, got %q", id)
	}
	if addCalls != 1 {
		t.Fatalf("addPullRequestReview must run exactly once when no viewer-owned pending exists; got %d calls", addCalls)
	}
}

// Threads with > 100 comments must trigger a follow-up node(id) query
// that pages through the remainder. Without it, comments past the
// first page silently disappear from the UI.
func TestListComments_PaginatesLongThread(t *testing.T) {
	threadCalls := 0
	c, _ := newTestGHClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/graphql") {
			http.NotFound(w, r)
			return
		}
		q := readGraphQLBody(t, r)
		switch {
		case strings.Contains(q, "reviewThreads(first: 100"):
			// One thread whose first comments page declares hasNextPage.
			_, _ = w.Write([]byte(`{"data":{"repository":{"pullRequest":{
				"id":"PR_1",
				"reviewThreads":{
					"pageInfo":{"hasNextPage":false,"endCursor":null},
					"nodes":[{
						"id":"PRT_x",
						"isOutdated":false,
						"path":"foo.go",
						"line":10,"originalLine":0,"startLine":0,"originalStartLine":0,
						"diffSide":"RIGHT","startDiffSide":"",
						"comments":{
							"pageInfo":{"hasNextPage":true,"endCursor":"c1"},
							"nodes":[
								{"id":"c-root","databaseId":1,"author":{"login":"a"},"body":"root","createdAt":"2024-01-01T00:00:00Z","diffHunk":"@@","commit":{"oid":"sha"},"originalCommit":{"oid":"sha"},"replyTo":null,"pullRequestReview":{"state":"COMMENTED"}}
							]
						}
					}]
				}
			}}}}`))
		case strings.Contains(q, "node(id: $id)"):
			threadCalls++
			// First follow-up returns one more comment with hasNextPage; second returns the tail.
			if threadCalls == 1 {
				_, _ = w.Write([]byte(`{"data":{"node":{"comments":{
					"pageInfo":{"hasNextPage":true,"endCursor":"c2"},
					"nodes":[{"id":"c-r1","databaseId":2,"author":{"login":"b"},"body":"reply1","createdAt":"2024-01-02T00:00:00Z","diffHunk":"@@","commit":{"oid":"sha"},"originalCommit":{"oid":"sha"},"replyTo":{"databaseId":1},"pullRequestReview":{"state":"COMMENTED"}}]
				}}}}`))
			} else {
				_, _ = w.Write([]byte(`{"data":{"node":{"comments":{
					"pageInfo":{"hasNextPage":false,"endCursor":null},
					"nodes":[{"id":"c-r2","databaseId":3,"author":{"login":"c"},"body":"reply2","createdAt":"2024-01-03T00:00:00Z","diffHunk":"@@","commit":{"oid":"sha"},"originalCommit":{"oid":"sha"},"replyTo":{"databaseId":1},"pullRequestReview":{"state":"COMMENTED"}}]
				}}}}`))
			}
		default:
			t.Fatalf("unexpected graphql query: %s", q)
		}
	})
	out, err := c.ListComments(context.Background(), "octocat", "hello", 1)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 comments after inner pagination, got %d", len(out))
	}
	if threadCalls != 2 {
		t.Fatalf("expected 2 inner-thread page fetches, got %d", threadCalls)
	}
	if out[0].Body != "root" || out[1].Body != "reply1" || out[2].Body != "reply2" {
		t.Fatalf("comment order wrong: %#v", out)
	}
}

// Cache hit on second call: the second invocation must NOT re-issue the
// discovery query. Important because compose POST and submit share the
// same review id and roundtripping it twice doubles the latency.
func TestEnsurePendingReview_CachesSecondCall(t *testing.T) {
	queryCalls := 0
	c, _ := newTestGHClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/graphql") {
			http.NotFound(w, r)
			return
		}
		q := readGraphQLBody(t, r)
		switch {
		case strings.Contains(q, "pullRequest(number: $number) { id }"):
			_, _ = w.Write([]byte(`{"data":{"repository":{"pullRequest":{"id":"PR_42"}}}}`))
		case strings.Contains(q, "reviews(states: [PENDING]"):
			queryCalls++
			_, _ = w.Write([]byte(`{"data":{
				"viewer":{"login":"alice"},
				"repository":{"pullRequest":{"reviews":{"nodes":[
					{"id":"PRR_alice","author":{"login":"alice"}}
				]}}}
			}}`))
		default:
			t.Fatalf("unexpected graphql query: %s", q)
		}
	})
	if _, err := c.ensurePendingReview(context.Background(), "octocat", "hello", 42); err != nil {
		t.Fatalf("first ensurePendingReview: %v", err)
	}
	if _, err := c.ensurePendingReview(context.Background(), "octocat", "hello", 42); err != nil {
		t.Fatalf("second ensurePendingReview: %v", err)
	}
	if queryCalls != 1 {
		t.Fatalf("discovery query must only run on first call; got %d", queryCalls)
	}
}
