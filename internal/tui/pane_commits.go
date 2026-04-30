package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-rv/internal/model"
)

func (m Model) handleKeyCommits(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.state.PR == nil {
		return m, nil
	}
	commits := m.visibleCommits()
	switch msg.String() {
	case "j", "down":
		if m.state.CommitsCursor < len(commits)-1 {
			m.state.CommitsCursor++
			m.autoSelectCommit(commits)
		}
	case "k", "up":
		if m.state.CommitsCursor > 0 {
			m.state.CommitsCursor--
			m.autoSelectCommit(commits)
		}
	case "enter":
		// Enter focuses Diff without changing SelectedRange. Single-commit
		// drill is driven by j/k auto-select; pressing Enter without prior
		// j/k preserves the PR-wide diff that was set by the Files step.
		m.state.FocusedPane = model.PaneDiff
	case "backspace":
		m.state.FocusedPane = model.PaneFiles
	}
	return m, nil
}

// autoSelectCommit pins SelectedRange to the cursor commit so the Diff and
// Comments panes follow the cursor live. Visual mode is excluded so multi-row
// yank does not mutate the working slice.
func (m *Model) autoSelectCommit(commits []*model.Commit) {
	if m.state.Visual != nil {
		return
	}
	if m.state.CommitsCursor < 0 || m.state.CommitsCursor >= len(commits) {
		return
	}
	c := commits[m.state.CommitsCursor]
	if m.state.SelectedRange.Kind == model.RangeSingleCommit && m.state.SelectedRange.SHA == c.SHA {
		return
	}
	m.state.SelectedRange = model.CommitRange{Kind: model.RangeSingleCommit, SHA: c.SHA}
	m.state.DiffCursor = model.DiffCursor{}
	m.state.DiffViewport.Top = 0
	m.state.CommentsCursor = 0
}

func (m Model) commitsView() string {
	title := paneTitle("Commits", m.state.FocusedPane == model.PaneCommits, "")
	if m.state.PR == nil {
		return title
	}
	var rows []string
	commits := m.visibleCommits()
	for i, c := range commits {
		cursor := m.cursorMarker(model.PaneCommits, i, m.state.CommitsCursor)
		annotation := "    "
		if m.state.SelectedFile != "" {
			if k, ok := c.ChangedFiles[m.state.SelectedFile]; ok {
				annotation = "[" + changeKindShort(k) + "] "
			}
		}
		rows = append(rows, fmt.Sprintf("%s%s%s %s", cursor, annotation, c.ShortSHA, c.Message))
	}
	return title + "\n" + strings.Join(rows, "\n")
}

// visibleCommits filters PR.Commits to those that touch the SelectedFile.
// Without a SelectedFile (initial state before any file is loaded), all
// commits are returned.
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
