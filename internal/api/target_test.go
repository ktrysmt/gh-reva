package api

import (
	"context"
	"testing"
)

func TestPRURL_DefaultsToGithubCom(t *testing.T) {
	t1 := Target{Owner: "octocat", Repo: "hello-world", Number: 42}
	if got := t1.PRURL(); got != "https://github.com/octocat/hello-world/pull/42" {
		t.Fatalf("default host: got %q", got)
	}
}

func TestPRURL_HonorsExplicitHost(t *testing.T) {
	t1 := Target{Host: "github.example.com", Owner: "octocat", Repo: "hello-world", Number: 42}
	if got := t1.PRURL(); got != "https://github.example.com/octocat/hello-world/pull/42" {
		t.Fatalf("custom host: got %q", got)
	}
}

// parseURL must populate Host from the URL's host segment so the PR URL
// rendered in the status bar matches the source URL's host (relevant on
// GitHub Enterprise where users paste `github.example.com` URLs).
func TestParseURL_CapturesHost(t *testing.T) {
	cases := []struct {
		in       string
		wantHost string
	}{
		{"https://github.com/octocat/hello/pull/1", "github.com"},
		{"https://github.example.com/foo/bar/pull/9", "github.example.com"},
	}
	for _, c := range cases {
		// parseURL is package-private and does not consult the Client
		// argument, so we route through the public ParseTargetArg with a
		// dummy nil context — the URL branch returns before any Client
		// call.
		tgt, err := ParseTargetArg(context.Background(), nil, []string{c.in})
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if tgt.Host != c.wantHost {
			t.Errorf("%s: host got %q want %q", c.in, tgt.Host, c.wantHost)
		}
	}
}

func TestPRShortForms_LadderOrdering(t *testing.T) {
	t1 := Target{Owner: "octocat", Repo: "hello-world", Number: 42}
	got := t1.PRShortForms()
	want := []string{
		"https://github.com/octocat/hello-world/pull/42",
		"octocat/hello-world/pulls/42",
		"octocat/hello-world/42",
		"hello-world/42",
	}
	if len(got) != len(want) {
		t.Fatalf("length: got %d want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("ladder[%d]: got %q want %q", i, got[i], w)
		}
	}
	// Ladder must be strictly descending in width — otherwise the
	// status bar's longest-fitting selection is meaningless.
	for i := 1; i < len(got); i++ {
		if len(got[i]) >= len(got[i-1]) {
			t.Fatalf("ladder must shrink at step %d: %q vs %q", i, got[i-1], got[i])
		}
	}
}
