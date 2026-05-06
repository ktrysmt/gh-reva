package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-reva/internal/model"
)

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Any keystroke clears a transient Notice (e.g. the "cannot edit
	// others' comments" hint). Cleared BEFORE dispatch so a handler can
	// re-set Notice in the same tick if it wants to. Compose / Help /
	// Visual modal handlers also benefit from auto-clear: a stray Esc
	// while a notice is up doubles as "dismiss notice".
	if m.state.Notice != "" {
		m.state.Notice = ""
	}
	// Compose absorbs all keystrokes when active so the textarea owns
	// input (Ctrl+S save, Esc cancel, runes append to body) and any
	// post-editor state (Submitting / Failed) cannot accidentally
	// trigger pane navigation behind the modal. The $EDITOR Editing
	// state cannot reach this branch in practice — bubbletea is
	// suspended during tea.ExecProcess — but the guard is unconditional
	// so a future refactor cannot regress the contract.
	if m.state.Compose != nil {
		return m.handleKeyTextarea(msg)
	}
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
		m.state.Modal = nil
		m.state.HelpOpen = true
		return m, nil
	case "q":
		// While a zoom modal is open, q closes the modal instead of
		// quitting the app — same idea as the Help handler. Quitting
		// from inside a modal would force the user to keep mental state
		// about "did I open the modal or not" before pressing q;
		// closing first and quitting on the next q is the less
		// surprising default. Ctrl+C still quits unconditionally — it
		// is the universal "force exit" gesture.
		if m.state.Modal != nil {
			m.state.Modal = nil
			return m, nil
		}
		return m, tea.Quit
	case "ctrl+c":
		// Mirror q's modal-close behavior: a stray Ctrl+C while a zoom
		// modal is open should close the modal, not drop the user out of
		// the program. The two modal-dismiss gestures (q and Ctrl+C) thus
		// stay symmetric — both close first, both quit on the next press
		// when no modal is open. Help modal has its own absorber in
		// handleKeyHelp; visual mode handles Ctrl+C in handleKeyVisual.
		if m.state.Modal != nil {
			m.state.Modal = nil
			return m, nil
		}
		return m, tea.Quit
	case "esc":
		// Esc dismisses the pane modal but otherwise stays as a no-op.
		// Keeping Esc no-op outside the modal preserves the existing
		// "no surprising side effect" contract elsewhere in the app.
		if m.state.Modal != nil {
			m.state.Modal = nil
		}
		return m, nil
	case "tab":
		m.state.DiffPendingPrefix = ""
		m.state.Modal = nil
		m.state.FocusedPane = nextPane(m.state.FocusedPane)
		return m, nil
	case "shift+tab":
		m.state.DiffPendingPrefix = ""
		m.state.Modal = nil
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
	case "enter":
		// Enter inside a Files / Commits zoom modal hands focus to the
		// Diff pane (the user has just picked the row they want to
		// inspect). Files-tree directory rows still fold/unfold via
		// the per-pane handler — that branch returns to handleKeyFiles
		// below. Comments modal Enter falls through to the standard
		// per-pane handler so the new edit/r dispatch (#3) keeps
		// working inside the modal.
		if m.state.Modal != nil {
			switch m.state.Modal.Pane {
			case model.PaneFiles:
				// Tree mode + dir row: fall through to the per-pane
				// handler so the directory fold/unfold gesture stays
				// available. fileIndexFromTreeCursor returns -1 when
				// the cursor is on a directory row.
				if m.state.FilesTreeMode && m.fileIndexFromTreeCursor() < 0 {
					break
				}
				m.state.Modal = nil
				m.state.FocusedPane = model.PaneDiff
				m.state.CommentsCursor = 0
				return m, nil
			case model.PaneCommits:
				m.state.Modal = nil
				m.state.FocusedPane = model.PaneDiff
				m.state.CommentsCursor = 0
				return m, nil
			}
		}
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
