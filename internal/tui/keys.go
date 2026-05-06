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
	// PendingConfirm absorbs all keystrokes while a `[y]es / [n]o`
	// prompt is up so the user cannot accidentally navigate away or
	// quit. Sits ahead of the Compose absorber because the parked
	// payload lives in PendingConfirm.Compose, not AppState.Compose,
	// and the y/n dispatch needs to see raw runes (the textarea
	// absorber would append them to the body).
	if m.state.PendingConfirm != nil {
		return m.handleKeyConfirm(msg)
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
	// Search Editing absorbs all keystrokes so the user can type the
	// query without keys leaking to pane navigation. Active state (post-
	// Enter) falls through to normal dispatch so n/N can cycle while
	// j/k / Tab / etc. still work.
	if m.state.Search != nil && m.state.Search.Status == model.SearchEditing {
		return m.handleKeySearch(msg)
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
	case "/":
		// Re-enter Editing while Active too — `/` after a finished search
		// is the user's gesture to refine the query without losing the
		// saved-cursor restore path.
		if m.state.PR == nil {
			return m, nil
		}
		if m.state.Search != nil {
			// Refine: keep the saved cursors, re-open Editing with the
			// existing query so the user can extend it without retyping.
			m.state.Search.Status = model.SearchEditing
			return m, nil
		}
		// Comments-pane search is intentionally disabled until the
		// "search inside zoom modal vs flat list" UX is decided.
		// Modeled as a silent no-op so the user sees no prompt; the
		// per-pane status hint already omits `/:search` for Comments.
		if m.state.FocusedPane == model.PaneComments {
			return m, nil
		}
		m.state.PendingPrefix = ""
		m.startSearch()
		return m, nil
	case "n":
		if m.state.Search != nil && m.state.Search.Status == model.SearchActive {
			m.searchAdvance(1)
			return m, nil
		}
	case "N":
		if m.state.Search != nil && m.state.Search.Status == model.SearchActive {
			m.searchAdvance(-1)
			return m, nil
		}
	case "?":
		m.state.PendingPrefix = ""
		// Opening Help while a zoom modal is up closes the modal first
		// (Help layers above), so the post-Help close lands the user
		// back on the modal's opener pane. closeModal is a no-op when
		// Modal is already nil.
		m.closeModal()
		m.state.HelpOpen = true
		return m, nil
	case "q":
		// While a zoom modal is open, q closes the modal instead of
		// quitting the app — same idea as the Help handler. Quitting
		// from inside a modal would force the user to keep mental state
		// about "did I open the modal or not" before pressing q;
		// closing first and quitting on the next q is the less
		// surprising default. closeModal restores focus to the pane the
		// user opened the modal from (Diff if it was a Diff Enter
		// handoff, the original pane if it was `<space>`).
		if m.state.Modal != nil {
			m.closeModal()
			return m, nil
		}
		return m, tea.Quit
	case "ctrl+c":
		// Active search ends on Ctrl+C (peer to Esc). Sits ahead of the
		// modal-close branch so the user can clear the search highlight
		// without dropping out of any zoom modal that might still be open.
		if m.state.Search != nil && m.state.Search.Status == model.SearchActive {
			m.state.Search = nil
			return m, nil
		}
		// Mirror q's modal-close behavior: a stray Ctrl+C while a zoom
		// modal is open should close the modal, not drop the user out of
		// the program. The two modal-dismiss gestures (q and Ctrl+C) thus
		// stay symmetric — both close first, both quit on the next press
		// when no modal is open. Help modal has its own absorber in
		// handleKeyHelp; visual mode handles Ctrl+C in handleKeyVisual.
		if m.state.Modal != nil {
			m.closeModal()
			return m, nil
		}
		return m, tea.Quit
	case "esc":
		// Esc clears an Active search so n/N stop cycling. Modal close
		// stays the prior contract; outside both, Esc is a no-op so
		// existing flows are not changed.
		if m.state.Search != nil && m.state.Search.Status == model.SearchActive {
			m.state.Search = nil
			return m, nil
		}
		if m.state.Modal != nil {
			m.closeModal()
		}
		return m, nil
	case "tab":
		m.state.PendingPrefix = ""
		// Tab while a search is Active terminates the session: changing
		// pane focus implies the user already navigated to the row they
		// were after, so n/N should stop intercepting and the highlight
		// should clear. Editing-phase search never reaches this branch
		// (handleKeySearch absorbs Tab earlier).
		if m.state.Search != nil && m.state.Search.Status == model.SearchActive {
			m.state.Search = nil
		}
		// Tab from inside a modal: first restore focus to the opener
		// pane via closeModal, then advance from there. Without the
		// restore, the modal-side FocusedPane (e.g. Comments after a
		// Diff Enter handoff) would steer the next/prev calculation —
		// surprising because the user is mentally still on the opener.
		if m.state.Modal != nil {
			m.closeModal()
		}
		m.state.FocusedPane = nextPane(m.state.FocusedPane)
		return m, nil
	case "shift+tab":
		m.state.PendingPrefix = ""
		if m.state.Search != nil && m.state.Search.Status == model.SearchActive {
			m.state.Search = nil
		}
		if m.state.Modal != nil {
			m.closeModal()
		}
		m.state.FocusedPane = prevPane(m.state.FocusedPane)
		return m, nil
	case "v":
		m.state.PendingPrefix = ""
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
		m.state.PendingPrefix = ""
		m.advanceFile(true)
		return m, nil
	case "K":
		m.state.PendingPrefix = ""
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

// handleKeyConfirm absorbs every keystroke while a `[y]es / [n]o`
// prompt is up for a parked compose payload. The dispatch is narrow on
// purpose — every other key (j/k, tab, ?, runes) is silently swallowed
// so the user cannot navigate behind the prompt or accidentally toss
// the parked payload.
//
//   - y          → commit: PendingConfirm clears, Compose receives the
//     payload, the editor / textarea Cmd starts.
//   - n          → cancel: payload discarded.
//   - Esc        → cancel.
//   - q, Ctrl+C  → cancel (do not quit; symmetric with the modal-close
//     "soft cancel" contract — a stray q during the prompt should not
//     drop the user out of the program with a draft inflight).
//   - other      → absorbed (no-op).
func (m Model) handleKeyConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		return m, m.confirmComposeStart()
	case "n", "esc", "q", "ctrl+c":
		m.cancelComposeConfirm()
		return m, nil
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
