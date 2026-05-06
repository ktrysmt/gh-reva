package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-reva/internal/model"
)

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Help modal absorbs all keystrokes except its dismiss set. It takes
	// precedence over visual / pane routing so the modal can be reached and
	// dismissed from any prior state without leaking keys to the body.
	if m.state.HelpOpen {
		return m.handleKeyHelp(msg)
	}
	if m.state.Visual != nil {
		return m.handleKeyVisual(msg)
	}
	switch msg.String() {
	case "?":
		m.state.DiffPendingPrefix = ""
		m.state.HelpOpen = true
		return m, nil
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		m.state.DiffPendingPrefix = ""
		m.state.FocusedPane = nextPane(m.state.FocusedPane)
		return m, nil
	case "shift+tab":
		m.state.DiffPendingPrefix = ""
		m.state.FocusedPane = prevPane(m.state.FocusedPane)
		return m, nil
	case "v":
		m.state.DiffPendingPrefix = ""
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
		}
		m.state.Visual = vs
		return m, nil
	case "J":
		m.state.DiffPendingPrefix = ""
		m.advanceFile(true)
		return m, nil
	case "K":
		m.state.DiffPendingPrefix = ""
		m.advanceFile(false)
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

// handleKeyHelp is the keystroke router while the Help modal is open.
// Dismiss set: `?` (toggle off), `Esc`, `Ctrl+C`, `q`. Every other key is
// absorbed so the body cursor / focus / visual state cannot move behind
// the modal — the user reads the keymap, dismisses, then resumes.
//
// Note that `q` here closes the modal instead of quitting the program;
// quitting from the modal would force the user to keep mental state about
// "did I open help or not" before pressing q. Closing first and quitting
// on the next q is the less surprising default.
func (m Model) handleKeyHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "?", "esc", "ctrl+c", "q":
		m.state.HelpOpen = false
		return m, nil
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
