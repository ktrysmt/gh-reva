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
		}
	case "k", "up":
		if m.state.CommitsCursor > 0 {
			m.state.CommitsCursor--
		}
	case "enter":
		if m.state.CommitsCursor < len(commits) {
			c := commits[m.state.CommitsCursor]
			m.state.SelectedRange = model.CommitRange{Kind: model.RangeSingleCommit, SHA: c.SHA}
			m.state.DiffCursor = model.DiffCursor{}
			m.state.DiffViewport.Top = 0
			m.state.CommentsCursor = 0
			m.state.FocusedPane = model.PaneDiff
		}
	case "backspace":
		m.state.FocusedPane = model.PaneFiles
	}
	return m, nil
}

func (m Model) commitsView() string {
	suffix := ""
	if m.state.CommitFilterFile != "" {
		suffix = "(filter: " + m.state.CommitFilterFile + ")"
	}
	title := paneTitle("Commits", m.state.FocusedPane == model.PaneCommits, suffix)
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

func (m Model) visibleCommits() []*model.Commit {
	if m.state.PR == nil {
		return nil
	}
	if m.state.CommitFilterFile == "" {
		return m.state.PR.Commits
	}
	var out []*model.Commit
	for _, c := range m.state.PR.Commits {
		if _, ok := c.ChangedFiles[m.state.CommitFilterFile]; ok {
			out = append(out, c)
		}
	}
	return out
}
