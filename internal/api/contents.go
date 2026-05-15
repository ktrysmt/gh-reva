package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

// GetFileContents fetches the NEW-side file content at the given ref via
// GitHub's `GET /repos/{owner}/{repo}/contents/{path}?ref={ref}` endpoint.
// The response carries the body base64-encoded inside a JSON envelope —
// we decode it and split into lines (no trailing empty line). Per-Client
// cache keyed on (ref, path) so the Diff pane's context-expand feature
// pays the round-trip at most once per file per session.
//
// Errors are returned verbatim so the TUI can surface them as a Notice.
// "Not found" (deleted file at ref / submodule / symlink target missing)
// is the expected error; callers fall back to leaving the synthetic row
// in place and showing a hint instead of crashing.
func (c *ghClient) GetFileContents(ctx context.Context, owner, repo string, n int, ref, path string) ([]string, error) {
	c.cacheMu.Lock()
	if c.fileContents == nil {
		c.fileContents = map[fileContentsCacheKey][]string{}
	}
	key := fileContentsCacheKey{Ref: ref, Path: path}
	if v, ok := c.fileContents[key]; ok {
		c.cacheMu.Unlock()
		return v, nil
	}
	c.cacheMu.Unlock()

	urlPath := fmt.Sprintf("repos/%s/%s/contents/%s", owner, repo, encodeContentsPath(path))
	if ref != "" {
		urlPath += "?ref=" + url.QueryEscape(ref)
	}
	var body struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		Type     string `json:"type"`
	}
	if err := c.rest.DoWithContext(ctx, "GET", urlPath, nil, &body); err != nil {
		return nil, fmt.Errorf("contents %s@%s: %w", path, ref, err)
	}
	if body.Type != "" && body.Type != "file" {
		return nil, fmt.Errorf("contents %s@%s: unsupported type %q", path, ref, body.Type)
	}
	if body.Encoding != "" && body.Encoding != "base64" {
		return nil, fmt.Errorf("contents %s@%s: unexpected encoding %q", path, ref, body.Encoding)
	}
	// The GitHub API returns base64 with embedded newlines after every 60
	// chars; the std lib decoder rejects them under stdEncoding, so strip
	// whitespace first.
	clean := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, body.Content)
	raw, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		return nil, fmt.Errorf("contents %s@%s: decode: %w", path, ref, err)
	}
	lines := splitLines(strings.TrimRight(string(raw), "\n"))
	c.cacheMu.Lock()
	c.fileContents[key] = lines
	c.cacheMu.Unlock()
	return lines, nil
}

// encodeContentsPath escapes each path segment for the contents URL.
// Path-encoding (not query-encoding) is required: a `/` separator must
// survive but `?`, `#`, `+`, space etc. must escape.
func encodeContentsPath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

type fileContentsCacheKey struct {
	Ref  string
	Path string
}
