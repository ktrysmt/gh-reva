//go:build ignore

// gen_large_fixture writes a stress-test PR fixture to the path provided as
// the first CLI argument. Usage:
//
//	go run testdata/gen_large_fixture.go testdata/large-pr.json
//
// The generated fixture contains 60 commits, 120 files, and a handful of
// review comments — enough to exercise the responsiveness of the renderer
// without bloating the repo.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type pr struct {
	Owner   string `json:"owner"`
	Repo    string `json:"repo"`
	Number  int    `json:"number"`
	Title   string `json:"title"`
	BaseSHA string `json:"base_sha"`
	HeadSHA string `json:"head_sha"`
}

type commit struct {
	SHA          string         `json:"sha"`
	ShortSHA     string         `json:"short_sha"`
	Message      string         `json:"message"`
	Author       string         `json:"author"`
	Date         time.Time      `json:"date"`
	ChangedFiles map[string]int `json:"changed_files"`
}

type fileEntry struct {
	Path         string `json:"path"`
	Status       int    `json:"status"`
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
	CommentCount int    `json:"comment_count"`
}

type reviewComment struct {
	ID               int64     `json:"id"`
	Path             string    `json:"path"`
	CommitID         string    `json:"commit_id"`
	OriginalCommitID string    `json:"original_commit_id"`
	Line             int       `json:"line"`
	OriginalLine     int       `json:"original_line"`
	DiffHunk         string    `json:"diff_hunk"`
	InReplyTo        int64     `json:"in_reply_to"`
	User             string    `json:"user"`
	CreatedAt        time.Time `json:"created_at"`
	Body             string    `json:"body"`
	Outdated         bool      `json:"outdated"`
}

type fixture struct {
	PR       pr                `json:"pr"`
	Commits  []commit          `json:"commits"`
	Files    []fileEntry       `json:"files"`
	Comments []reviewComment   `json:"comments"`
	Diffs    map[string]string `json:"diffs"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: gen_large_fixture <out.json>")
		os.Exit(2)
	}
	out := os.Args[1]

	const numCommits = 60
	const numFiles = 120
	dirs := []string{"core", "api", "ui", "store", "infra", "tests", "docs"}

	files := make([]fileEntry, 0, numFiles)
	paths := make([]string, 0, numFiles)
	for i := 0; i < numFiles; i++ {
		dir := dirs[i%len(dirs)]
		sub := dir + "/sub" + fmt.Sprint((i/len(dirs))%4)
		path := fmt.Sprintf("%s/file_%03d.go", sub, i)
		paths = append(paths, path)
		files = append(files, fileEntry{
			Path:      path,
			Status:    i % 4,
			Additions: 5 + i%30,
			Deletions: i % 7,
		})
	}

	commits := make([]commit, 0, numCommits)
	base := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < numCommits; i++ {
		sha := fmt.Sprintf("%07x%033x", i+1, 0)
		changed := map[string]int{}
		for j := 0; j < 4; j++ {
			p := paths[(i*7+j)%numFiles]
			changed[p] = (i + j) % 4
		}
		commits = append(commits, commit{
			SHA:          sha,
			ShortSHA:     sha[:7],
			Message:      fmt.Sprintf("commit %02d: refactor module", i),
			Author:       []string{"alice", "bob", "carol", "dave"}[i%4],
			Date:         base.Add(time.Duration(i) * time.Hour),
			ChangedFiles: changed,
		})
	}

	headSHA := commits[len(commits)-1].SHA

	comments := []reviewComment{
		{
			ID: 9001, Path: paths[0], CommitID: headSHA, OriginalCommitID: headSHA,
			Line: 5, OriginalLine: 5, InReplyTo: 0,
			User: "alice", CreatedAt: base.Add(2 * time.Hour),
			Body: "Consider extracting this into a helper.",
		},
		{
			ID: 9002, Path: paths[0], CommitID: headSHA, OriginalCommitID: headSHA,
			Line: 8, OriginalLine: 8, InReplyTo: 9001,
			User: "bob", CreatedAt: base.Add(3 * time.Hour),
			Body: "Agreed — will follow up.",
		},
		{
			ID: 9003, Path: paths[20], CommitID: headSHA, OriginalCommitID: headSHA,
			Line: 12, OriginalLine: 12, InReplyTo: 0,
			User: "carol", CreatedAt: base.Add(4 * time.Hour),
			Body: "Tests for this branch?",
		},
	}

	commentCounts := map[string]int{}
	for _, c := range comments {
		if !c.Outdated {
			commentCounts[c.Path]++
		}
	}
	for i := range files {
		files[i].CommentCount = commentCounts[files[i].Path]
	}

	diffs := map[string]string{}
	for _, p := range paths {
		diffs["pr/"+p] = renderDiff(p)
	}

	f := fixture{
		PR: pr{
			Owner: "stress", Repo: "test", Number: 1,
			Title:   "Stress fixture",
			BaseSHA: "0000001" + strings.Repeat("0", 33),
			HeadSHA: headSHA,
		},
		Commits:  commits,
		Files:    files,
		Comments: comments,
		Diffs:    diffs,
	}

	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d commits, %d files, %d comments)\n", out, len(commits), len(files), len(comments))
}

func renderDiff(path string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "--- a/%s\n", path)
	fmt.Fprintf(&b, "+++ b/%s\n", path)
	b.WriteString("@@ -1,5 +1,15 @@\n")
	b.WriteString(" package main\n")
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&b, "+// stress line %d for %s\n", i, path)
	}
	return b.String()
}
