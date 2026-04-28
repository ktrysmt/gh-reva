package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-rv/internal/clipboard"
	"github.com/ktrysmt/gh-rv/internal/model"
)

func (m Model) handleKeyVisual(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state.Visual = nil
		return m, nil
	case "y":
		_ = clipboard.Yank(m.yankString())
		m.state.Visual = nil
		return m, nil
	case "tab", "shift+tab", "enter", "backspace", "v", " ", "q", "ctrl+c":
		// Focus / mode-changing keys are inert in visual mode.
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

func (m Model) yankString() string {
	if m.state.PR == nil {
		return ""
	}
	switch m.state.FocusedPane {
	case model.PaneFiles:
		if m.state.FilesTreeMode {
			treeRows := m.filesTreeRows()
			lo, hi := m.linewiseSelectionRange(model.PaneFiles, m.state.FilesCursor, len(treeRows))
			var rows []string
			for i := lo; i <= hi && i < len(treeRows); i++ {
				rows = append(rows, treeRows[i].Path)
			}
			return strings.Join(rows, "\n")
		}
		lo, hi := m.linewiseSelectionRange(model.PaneFiles, m.state.FilesCursor, len(m.state.PR.Files))
		var rows []string
		for i := lo; i <= hi && i < len(m.state.PR.Files); i++ {
			rows = append(rows, m.state.PR.Files[i].Path)
		}
		return strings.Join(rows, "\n")
	case model.PaneCommits:
		commits := m.visibleCommits()
		lo, hi := m.linewiseSelectionRange(model.PaneCommits, m.state.CommitsCursor, len(commits))
		var rows []string
		for i := lo; i <= hi && i < len(commits); i++ {
			c := commits[i]
			rows = append(rows, fmt.Sprintf("%s %s", c.ShortSHA, c.Message))
		}
		return strings.Join(rows, "\n")
	case model.PaneComments:
		flat := m.flatComments()
		lo, hi := m.linewiseSelectionRange(model.PaneComments, m.state.CommentsCursor, len(flat))
		var rows []string
		for i := lo; i <= hi && i < len(flat); i++ {
			c := flat[i]
			rows = append(rows, fmt.Sprintf("%s @ %s\n%s", c.User, c.CreatedAt.Format("2006-01-02"), c.Body))
		}
		return strings.Join(rows, "\n")
	case model.PaneDiff:
		patch := m.currentPatch()
		lines := strings.Split(strings.TrimRight(patch, "\n"), "\n")
		lo, hi := m.linewiseSelectionRange(model.PaneDiff, m.state.DiffCursor.Line, len(lines))
		var rows []string
		for i := lo; i <= hi && i < len(lines); i++ {
			rows = append(rows, lines[i])
		}
		return strings.Join(rows, "\n")
	}
	return ""
}

// linewiseSelectionRange returns the inclusive [lo, hi] line range that should
// be rendered as selected / yanked. When visual mode is active on `pane`, the
// range spans anchor → cursor; otherwise it collapses to the cursor row.
func (m Model) linewiseSelectionRange(pane model.PaneID, cursor, total int) (int, int) {
	if total == 0 {
		return 0, -1
	}
	if m.state.Visual == nil || m.state.Visual.OriginPane != pane {
		return cursor, cursor
	}
	anchor := m.state.Visual.Anchor
	if pane == model.PaneDiff {
		anchor = m.state.Visual.AnchorLine
	}
	lo, hi := anchor, cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi
}

func (m Model) inVisualRange(pane model.PaneID, idx int) bool {
	if m.state.Visual == nil || m.state.Visual.OriginPane != pane {
		return false
	}
	var cursor int
	switch pane {
	case model.PaneFiles:
		cursor = m.state.FilesCursor
	case model.PaneCommits:
		cursor = m.state.CommitsCursor
	case model.PaneComments:
		cursor = m.state.CommentsCursor
	case model.PaneDiff:
		cursor = m.state.DiffCursor.Line
	}
	anchor := m.state.Visual.Anchor
	if pane == model.PaneDiff {
		anchor = m.state.Visual.AnchorLine
	}
	if anchor > cursor {
		anchor, cursor = cursor, anchor
	}
	return idx >= anchor && idx <= cursor
}

func (m Model) cursorMarker(pane model.PaneID, idx, cursor int) string {
	if idx == cursor || m.inVisualRange(pane, idx) {
		return "> "
	}
	return "  "
}
