package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// Cursor index 0 in the Commits pane is the synthetic "All commits" row that
// represents RangeWholePR. Real commits sit at indices 1..N, so the
// visibleCommits() slice is offset by one against the Commits cursor. j/k
// auto-select translates that mapping in autoSelectCommit.

func (m Model) handleKeyCommits(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.state.PR == nil {
		return m, nil
	}
	commits := m.visibleCommits()
	switch msg.String() {
	case "j", "down":
		if m.state.CommitsCursor < len(commits) {
			m.state.CommitsCursor++
			m.autoSelectCommit(commits)
		}
	case "k", "up":
		if m.state.CommitsCursor > 0 {
			m.state.CommitsCursor--
			m.autoSelectCommit(commits)
		}
	case " ":
		m.toggleModal(model.PaneCommits)
	}
	return m, nil
}

// autoSelectCommit pins SelectedRange to the cursor row so the Diff and
// Comments panes follow the cursor live. Cursor index 0 maps to RangeWholePR
// (the synthetic "All commits" row); indices 1..N map to commits[i-1] under
// RangeSingleCommit. Visual mode is excluded so multi-row yank does not
// mutate the working slice.
func (m *Model) autoSelectCommit(commits []*model.Commit) {
	if m.state.Visual != nil {
		return
	}
	idx := m.state.CommitsCursor
	if idx == 0 {
		if m.state.SelectedRange.Kind == model.RangeWholePR {
			return
		}
		m.state.SelectedRange = model.CommitRange{Kind: model.RangeWholePR}
		m.state.DiffCursor = model.DiffCursor{}
		m.state.DiffViewport.Top = 0
		m.state.CommentsCursor = 0
		return
	}
	if idx < 1 || idx > len(commits) {
		return
	}
	c := commits[idx-1]
	if m.state.SelectedRange.Kind == model.RangeSingleCommit && m.state.SelectedRange.SHA == c.SHA {
		return
	}
	m.state.SelectedRange = model.CommitRange{Kind: model.RangeSingleCommit, SHA: c.SHA}
	m.state.DiffCursor = model.DiffCursor{}
	m.state.DiffViewport.Top = 0
	m.state.CommentsCursor = 0
}

func (m Model) commitsView() string {
	title := m.styledPaneTitle("Commits", m.state.FocusedPane == model.PaneCommits, "")
	if m.state.PR == nil {
		return title
	}
	var rows []string
	commits := m.visibleCommits()

	rows = append(rows, m.allCommitsRow(commits))

	for i, c := range commits {
		cursor := m.styledCursor(model.PaneCommits, i+1, m.state.CommitsCursor)
		annotation := "    "
		if m.state.SelectedFile != "" {
			if k, ok := c.ChangedFiles[m.state.SelectedFile]; ok {
				annotation = "[" + m.styledStatus(k) + "] "
			}
		}
		sha := fg(c.ShortSHA, m.theme.CommitSHA)
		rows = append(rows, fmt.Sprintf("%s%s%s %s", cursor, annotation, sha, c.Message))
	}
	return title + "\n" + strings.Join(rows, "\n")
}

// allCommitsRow renders the synthetic row at index 0. Label form:
//
//	no file selected:                 "All commits (N)"
//	file selected, M == N:            "All commits (N)"   // file in every commit
//	file selected, M < N:             "All commits (M of N)"
//
// When a file is selected, the annotation slot mirrors the file's PR-level
// change-kind so reviewers see the overall status without drilling in.
func (m Model) allCommitsRow(visible []*model.Commit) string {
	cursor := m.styledCursor(model.PaneCommits, 0, m.state.CommitsCursor)
	annotation := "    "
	totalCommits := len(m.state.PR.Commits)
	visibleCount := len(visible)
	label := fmt.Sprintf("All commits (%d)", totalCommits)
	if m.state.SelectedFile != "" {
		if status, ok := m.fileStatusFor(m.state.SelectedFile); ok {
			annotation = "[" + m.styledStatus(status) + "] "
		}
		if visibleCount < totalCommits {
			label = fmt.Sprintf("All commits (%d of %d)", visibleCount, totalCommits)
		}
	}
	return cursor + annotation + fgBold(label, "")
}

func (m Model) fileStatusFor(path string) (model.ChangeKind, bool) {
	if m.state.PR == nil {
		return 0, false
	}
	for _, f := range m.state.PR.Files {
		if f.Path == path {
			return f.Status, true
		}
	}
	return 0, false
}

// visibleCommits filters PR.Commits to those that touch the SelectedFile.
// Without a SelectedFile (initial state before any file is loaded), all
// commits are returned. The synthetic "All commits" row at cursor index 0
// of the Commits pane sits ABOVE this slice — see commitsView and
// autoSelectCommit for how the cursor maps that virtual row to RangeWholePR.
func (m Model) visibleCommits() []*model.Commit {
	if m.state.PR == nil {
		return nil
	}
	if m.state.SelectedFile == "" {
		return m.state.PR.Commits
	}
	var out []*model.Commit
	for _, c := range m.state.PR.Commits {
		if _, ok := c.ChangedFiles[m.state.SelectedFile]; ok {
			out = append(out, c)
		}
	}
	return out
}
