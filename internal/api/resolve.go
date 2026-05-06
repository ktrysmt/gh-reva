package api

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"github.com/cli/go-gh/v2/pkg/repository"
)

// Target identifies a single PR. Host carries the GitHub host
// (`github.com` for github.com, e.g. `github.example.com` for GHES) so
// downstream URL builders can stay accurate on Enterprise installs.
// Empty Host is treated as `github.com` by URL helpers.
type Target struct {
	Host   string
	Owner  string
	Repo   string
	Number int
}

// PRURL returns the canonical web URL for the PR. Always uses https://
// and the singular `pull/<n>` segment matching GitHub's web UI.
func (t *Target) PRURL() string {
	host := t.Host
	if host == "" {
		host = "github.com"
	}
	return fmt.Sprintf("https://%s/%s/%s/pull/%d", host, t.Owner, t.Repo, t.Number)
}

// PRShortForms returns the URL shortening ladder used by the status
// bar. Order is longest → shortest so the renderer can pick the first
// form that fits the available width:
//
//  1. https://<host>/<owner>/<repo>/pull/<n> — full URL
//  2. <owner>/<repo>/pulls/<n>               — host-stripped (REST shape)
//  3. <owner>/<repo>/<n>                     — pulls segment dropped
//  4. <repo>/<n>                             — owner dropped
//
// Form 2 keeps the `pulls` segment (REST endpoint shape) rather than
// the web UI's `pull` because users searching the screen for a PR
// number alongside the repo are more likely to recognize the REST form
// from gh CLI output. Forms 3 and 4 sacrifice that for width.
func (t *Target) PRShortForms() []string {
	return []string{
		t.PRURL(),
		fmt.Sprintf("%s/%s/pulls/%d", t.Owner, t.Repo, t.Number),
		fmt.Sprintf("%s/%s/%d", t.Owner, t.Repo, t.Number),
		fmt.Sprintf("%s/%d", t.Repo, t.Number),
	}
}

func ParseTargetArg(ctx context.Context, c Client, args []string) (*Target, error) {
	if len(args) == 0 {
		host, owner, repo, n, err := resolveWithHost(ctx, c)
		if err != nil {
			return nil, err
		}
		return &Target{Host: host, Owner: owner, Repo: repo, Number: n}, nil
	}
	arg := args[0]
	if strings.HasPrefix(arg, "https://") || strings.HasPrefix(arg, "http://") {
		return parseURL(arg)
	}
	n, err := strconv.Atoi(arg)
	if err != nil {
		return nil, fmt.Errorf("invalid PR argument: %q", arg)
	}
	host, owner, repo, _, err := resolveWithHost(ctx, c)
	if err != nil {
		// Try without a current-branch PR. owner/repo can still come from the
		// repo metadata; the user supplied the PR number explicitly.
		h, o, r, rerr := currentRepository()
		if rerr != nil {
			return nil, err
		}
		return &Target{Host: h, Owner: o, Repo: r, Number: n}, nil
	}
	return &Target{Host: host, Owner: owner, Repo: repo, Number: n}, nil
}

// resolveWithHost wraps Client.ResolveCurrentBranchPR with the host
// derived from the local git remote. The Client interface returns
// (owner, repo, number) only; the host always comes from
// currentRepository so fixture / error / future Enterprise clients all
// agree on a single source of truth without growing the interface.
// Falls back to github.com when the local repo metadata is unavailable.
func resolveWithHost(ctx context.Context, c Client) (string, string, string, int, error) {
	owner, repo, n, err := c.ResolveCurrentBranchPR(ctx)
	if err != nil {
		return "", "", "", 0, err
	}
	host, _, _, herr := currentRepository()
	if herr != nil || host == "" {
		host = "github.com"
	}
	return host, owner, repo, n, nil
}

func (c *ghClient) ResolveCurrentBranchPR(ctx context.Context) (string, string, int, error) {
	_, owner, repo, err := currentRepository()
	if err != nil {
		return "", "", 0, err
	}
	branch, err := currentBranch()
	if err != nil {
		return "", "", 0, err
	}
	var prs []struct {
		Number int `json:"number"`
	}
	path := fmt.Sprintf("repos/%s/%s/pulls?head=%s:%s&state=open", owner, repo, owner, branch)
	if err := c.rest.DoWithContext(ctx, http.MethodGet, path, nil, &prs); err != nil {
		return "", "", 0, err
	}
	if len(prs) == 0 {
		return "", "", 0, fmt.Errorf("no open PR found for %s:%s", owner, branch)
	}
	return owner, repo, prs[0].Number, nil
}

// currentRepository returns (host, owner, repo) for the gh-recognised
// remote in the working directory. Host enables PR URL rendering on
// GitHub Enterprise installs without falling back to github.com.
func currentRepository() (string, string, string, error) {
	r, err := repository.Current()
	if err != nil {
		return "", "", "", err
	}
	return r.Host, r.Owner, r.Name, nil
}

func currentBranch() (string, error) {
	out, err := exec.Command("git", "branch", "--show-current").Output()
	if err != nil {
		return "", fmt.Errorf("git branch --show-current: %w", err)
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "", fmt.Errorf("could not determine current branch (detached HEAD?)")
	}
	return branch, nil
}

func parseURL(u string) (*Target, error) {
	stripped := strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	parts := strings.Split(stripped, "/")
	if len(parts) < 5 || parts[3] != "pull" {
		return nil, fmt.Errorf("not a PR URL: %s", u)
	}
	n, err := strconv.Atoi(parts[4])
	if err != nil {
		return nil, fmt.Errorf("invalid PR number in URL: %s", u)
	}
	return &Target{Host: parts[0], Owner: parts[1], Repo: parts[2], Number: n}, nil
}
