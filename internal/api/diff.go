package api

import (
	"context"
	"fmt"
	"strings"
)

// GetFileDiff returns a unified diff string for a single file.
// sha == "" means "PR-wide diff" (head..base).
func (c *ghClient) GetFileDiff(ctx context.Context, owner, repo string, n int, sha, path string) (string, error) {
	var (
		files []ghFile
		err   error
	)
	if sha == "" {
		files, err = c.fetchPRFiles(ctx, owner, repo, n)
	} else {
		var detail *ghCommit
		detail, err = c.fetchCommit(ctx, owner, repo, sha)
		if err == nil {
			files = detail.Files
		}
	}
	if err != nil {
		return "", err
	}
	for _, f := range files {
		if f.Filename != path {
			continue
		}
		return renderUnifiedDiff(f), nil
	}
	return "", nil
}

func renderUnifiedDiff(f ghFile) string {
	if f.Patch == "" {
		return ""
	}
	var oldHeader, newHeader string
	switch f.Status {
	case "added":
		oldHeader = "--- /dev/null"
		newHeader = "+++ b/" + f.Filename
	case "removed":
		oldHeader = "--- a/" + f.Filename
		newHeader = "+++ /dev/null"
	case "renamed":
		prev := f.PreviousFilename
		if prev == "" {
			prev = f.Filename
		}
		oldHeader = "--- a/" + prev
		newHeader = "+++ b/" + f.Filename
	default:
		oldHeader = "--- a/" + f.Filename
		newHeader = "+++ b/" + f.Filename
	}
	patch := strings.TrimRight(f.Patch, "\n")
	return fmt.Sprintf("%s\n%s\n%s\n", oldHeader, newHeader, patch)
}
