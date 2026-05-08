package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// listCommitsServer wires a fake REST server that:
//   - serves /repos/o/r/pulls/1/commits with `n` items (sha = "sha0..shaN-1"), and
//   - serves /repos/o/r/commits/<sha> with a per-call delay so wall-clock
//     latency reveals whether ListCommits' per-commit detail loop runs
//     in parallel.
//
// recordPeak true → handler tracks max concurrent in-flight detail calls
// in *peak (atomically) so the test can assert real concurrency.
func listCommitsServer(t *testing.T, n int, perCallDelay time.Duration, peak *int32) *ghClient {
	t.Helper()
	var inflight int32
	c, _ := newTestGHClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/pulls/1/commits"):
			list := make([]map[string]any, 0, n)
			for i := 0; i < n; i++ {
				list = append(list, map[string]any{
					"sha": fmt.Sprintf("sha%d", i),
					"commit": map[string]any{
						"author":  map[string]any{"name": "x", "date": "2024-01-01T00:00:00Z"},
						"message": fmt.Sprintf("c%d", i),
					},
					"author": map[string]any{"login": "x"},
				})
			}
			_ = json.NewEncoder(w).Encode(list)
		case strings.Contains(r.URL.Path, "/commits/sha"):
			cur := atomic.AddInt32(&inflight, 1)
			if peak != nil {
				for {
					old := atomic.LoadInt32(peak)
					if cur <= old || atomic.CompareAndSwapInt32(peak, old, cur) {
						break
					}
				}
			}
			if perCallDelay > 0 {
				time.Sleep(perCallDelay)
			}
			atomic.AddInt32(&inflight, -1)
			sha := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			_, _ = fmt.Fprintf(w, `{"sha":%q,"files":[],"commit":{"author":{"name":"x","date":"2024-01-01T00:00:00Z"},"message":"x"},"author":{"login":"x"}}`, sha)
		default:
			http.NotFound(w, r)
		}
	})
	return c
}

// ListCommits must fan its per-commit detail fetches out concurrently
// (capped at the worker-pool limit) so a multi-commit PR doesn't pay
// N * RTT to fetch each commit's `files`. Without parallelism, a 60-
// commit PR stalls on 60 sequential REST round-trips on the very first
// loading screen.
//
// 5 commits × 80ms detail delay → sequential ≈ 400ms; parallel ≈ 80ms.
// Threshold 250ms catches a regression to sequential while keeping a
// generous margin for slow CI machines.
func TestListCommits_FetchesCommitDetailsInParallel(t *testing.T) {
	var peak int32
	c := listCommitsServer(t, 5, 80*time.Millisecond, &peak)

	start := time.Now()
	out, err := c.ListCommits(context.Background(), "o", "r", 1)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	if len(out) != 5 {
		t.Fatalf("got %d commits, want 5", len(out))
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("ListCommits ran sequentially (%v >= 250ms); expected parallel fetchCommit", elapsed)
	}
	if peak < 2 {
		t.Fatalf("peak in-flight commit-detail calls = %d; expected >=2 (parallelism not engaged)", peak)
	}
}

// Even with parallel fetch, the returned slice must mirror the list
// ordering returned by /pulls/N/commits. A naive `append` from worker
// goroutines would produce a non-deterministic order keyed on response
// arrival, breaking visibleCommits' chronological assumption (CLAUDE.md
// §4 #15) and Commits-pane cursor stability.
//
// Server randomizes per-detail latency within [10ms, 60ms] so the wire
// arrival order is shuffled relative to the list; the slice returned by
// ListCommits must still be sha0..sha4 in order.
func TestListCommits_PreservesListOrderUnderParallel(t *testing.T) {
	const n = 5
	var rngMu sync.Mutex
	rng := struct{ i int }{}
	delays := []time.Duration{60 * time.Millisecond, 10 * time.Millisecond, 50 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond}
	c, _ := newTestGHClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/pulls/1/commits"):
			list := make([]map[string]any, 0, n)
			for i := 0; i < n; i++ {
				list = append(list, map[string]any{
					"sha": fmt.Sprintf("sha%d", i),
					"commit": map[string]any{
						"author":  map[string]any{"name": "x", "date": "2024-01-01T00:00:00Z"},
						"message": fmt.Sprintf("c%d", i),
					},
					"author": map[string]any{"login": "x"},
				})
			}
			_ = json.NewEncoder(w).Encode(list)
		case strings.Contains(r.URL.Path, "/commits/sha"):
			rngMu.Lock()
			d := delays[rng.i%len(delays)]
			rng.i++
			rngMu.Unlock()
			time.Sleep(d)
			sha := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			_, _ = fmt.Fprintf(w, `{"sha":%q,"files":[],"commit":{"author":{"name":"x","date":"2024-01-01T00:00:00Z"},"message":"x"},"author":{"login":"x"}}`, sha)
		default:
			http.NotFound(w, r)
		}
	})

	out, err := c.ListCommits(context.Background(), "o", "r", 1)
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	if len(out) != n {
		t.Fatalf("got %d commits, want %d", len(out), n)
	}
	for i, com := range out {
		want := fmt.Sprintf("sha%d", i)
		if com.SHA != want {
			t.Fatalf("commits[%d].SHA = %q, want %q (parallel impl must not reorder by response arrival)", i, com.SHA, want)
		}
	}
}

// One commit's detail returning 5xx must surface as a ListCommits
// error — silently dropping a commit would corrupt the visibleCommits
// filter without any user-visible signal.
func TestListCommits_PropagatesDetailError(t *testing.T) {
	c, _ := newTestGHClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/pulls/1/commits"):
			list := []map[string]any{
				{"sha": "sha0", "commit": map[string]any{"author": map[string]any{"name": "x", "date": "2024-01-01T00:00:00Z"}, "message": "c0"}, "author": map[string]any{"login": "x"}},
				{"sha": "sha1", "commit": map[string]any{"author": map[string]any{"name": "x", "date": "2024-01-01T00:00:00Z"}, "message": "c1"}, "author": map[string]any{"login": "x"}},
				{"sha": "sha2", "commit": map[string]any{"author": map[string]any{"name": "x", "date": "2024-01-01T00:00:00Z"}, "message": "c2"}, "author": map[string]any{"login": "x"}},
			}
			_ = json.NewEncoder(w).Encode(list)
		case strings.HasSuffix(r.URL.Path, "/commits/sha1"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
		case strings.Contains(r.URL.Path, "/commits/sha"):
			sha := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			_, _ = fmt.Fprintf(w, `{"sha":%q,"files":[],"commit":{"author":{"name":"x","date":"2024-01-01T00:00:00Z"},"message":"x"},"author":{"login":"x"}}`, sha)
		default:
			http.NotFound(w, r)
		}
	})

	if _, err := c.ListCommits(context.Background(), "o", "r", 1); err == nil {
		t.Fatalf("expected error from sha1 detail 500, got nil")
	}
}
