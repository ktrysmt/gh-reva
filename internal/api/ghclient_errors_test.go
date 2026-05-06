package api

import (
	"context"
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
