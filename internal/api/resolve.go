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

type Target struct {
	Owner  string
	Repo   string
	Number int
}

func ParseTargetArg(ctx context.Context, c Client, args []string) (*Target, error) {
	if len(args) == 0 {
		owner, repo, n, err := c.ResolveCurrentBranchPR(ctx)
		if err != nil {
			return nil, err
		}
		return &Target{Owner: owner, Repo: repo, Number: n}, nil
	}
	arg := args[0]
	if strings.HasPrefix(arg, "https://") || strings.HasPrefix(arg, "http://") {
		return parseURL(arg)
	}
	n, err := strconv.Atoi(arg)
	if err != nil {
		return nil, fmt.Errorf("invalid PR argument: %q", arg)
	}
	owner, repo, _, err := c.ResolveCurrentBranchPR(ctx)
	if err != nil {
		// Try without a current-branch PR. owner/repo can still come from the
		// repo metadata; the user supplied the PR number explicitly.
		o, r, rerr := currentRepository()
		if rerr != nil {
			return nil, err
		}
		return &Target{Owner: o, Repo: r, Number: n}, nil
	}
	return &Target{Owner: owner, Repo: repo, Number: n}, nil
}

func (c *ghClient) ResolveCurrentBranchPR(ctx context.Context) (string, string, int, error) {
	owner, repo, err := currentRepository()
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

func currentRepository() (string, string, error) {
	r, err := repository.Current()
	if err != nil {
		return "", "", err
	}
	return r.Owner, r.Name, nil
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
	return &Target{Owner: parts[1], Repo: parts[2], Number: n}, nil
}
