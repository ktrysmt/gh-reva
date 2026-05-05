package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
)

func makeCommitsModel(t *testing.T, files []*model.FileEntry, commits []*model.Commit, selectedFile string) Model {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	m.state.PR = &model.PR{Files: files, Commits: commits}
	m.state.SelectedFile = selectedFile
	return m
}

func TestAllCommitsRowLabel(t *testing.T) {
	commit := func(sha, msg string, files map[string]model.ChangeKind) *model.Commit {
		return &model.Commit{SHA: sha, ShortSHA: sha[:7], Message: msg, ChangedFiles: files}
	}

	cases := []struct {
		name     string
		files    []*model.FileEntry
		commits  []*model.Commit
		selected string
		want     string
	}{
		{
			name: "no file selected → total only",
			files: []*model.FileEntry{
				{Path: "a.go", Status: model.ChangeModified},
			},
			commits: []*model.Commit{
				commit("aaaaaaaa", "first", map[string]model.ChangeKind{"a.go": model.ChangeModified}),
				commit("bbbbbbbb", "second", map[string]model.ChangeKind{"a.go": model.ChangeModified}),
				commit("cccccccc", "third", nil),
			},
			selected: "",
			want:     "All commits (3)",
		},
		{
			name: "filtered to file touched by every commit → total only",
			files: []*model.FileEntry{
				{Path: "a.go", Status: model.ChangeModified},
			},
			commits: []*model.Commit{
				commit("aaaaaaaa", "first", map[string]model.ChangeKind{"a.go": model.ChangeModified}),
				commit("bbbbbbbb", "second", map[string]model.ChangeKind{"a.go": model.ChangeModified}),
				commit("cccccccc", "third", map[string]model.ChangeKind{"a.go": model.ChangeModified}),
			},
			selected: "a.go",
			want:     "All commits (3)",
		},
		{
			name: "filtered to file touched by some commits → M of N",
			files: []*model.FileEntry{
				{Path: "a.go", Status: model.ChangeModified},
			},
			commits: []*model.Commit{
				commit("aaaaaaaa", "first", map[string]model.ChangeKind{"a.go": model.ChangeModified}),
				commit("bbbbbbbb", "second", map[string]model.ChangeKind{"a.go": model.ChangeModified}),
				commit("cccccccc", "third", map[string]model.ChangeKind{"b.go": model.ChangeModified}),
			},
			selected: "a.go",
			want:     "All commits (2 of 3)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := makeCommitsModel(t, tc.files, tc.commits, tc.selected)
			row := m.allCommitsRow(m.visibleCommits())
			if !strings.Contains(row, tc.want) {
				t.Fatalf("row should contain %q\ngot: %q", tc.want, row)
			}
		})
	}
}
