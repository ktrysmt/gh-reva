package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-rv/internal/model"
)

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.state.Visual != nil {
		return m.handleKeyVisual(msg)
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		m.state.FocusedPane = nextPane(m.state.FocusedPane)
		return m, nil
	case "shift+tab":
		m.state.FocusedPane = prevPane(m.state.FocusedPane)
		return m, nil
	case "v":
		vs := &model.VisualState{
			OriginPane: m.state.FocusedPane,
			Linewise:   m.state.FocusedPane != model.PaneDiff,
		}
		switch m.state.FocusedPane {
		case model.PaneFiles:
			vs.Anchor = m.state.FilesCursor
		case model.PaneCommits:
			vs.Anchor = m.state.CommitsCursor
		case model.PaneComments:
			vs.Anchor = m.state.CommentsCursor
		case model.PaneDiff:
			vs.AnchorLine = m.state.DiffCursor.Line
			vs.AnchorCol = m.state.DiffCursor.Col
		}
		m.state.Visual = vs
		return m, nil
	}
	switch m.state.FocusedPane {
	case model.PaneFiles:
		return m.handleKeyFiles(msg)
	case model.PaneCommits:
		return m.handleKeyCommits(msg)
	case model.PaneDiff:
		return m.handleKeyDiff(msg)
	case model.PaneComments:
		return m.handleKeyComments(msg)
	}
	return m, nil
}

func nextPane(p model.PaneID) model.PaneID {
	if p == model.PaneComments {
		return model.PaneFiles
	}
	return p + 1
}

func prevPane(p model.PaneID) model.PaneID {
	if p == model.PaneFiles {
		return model.PaneComments
	}
	return p - 1
}
