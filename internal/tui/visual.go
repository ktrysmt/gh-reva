package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-reva/internal/clipboard"
	"github.com/ktrysmt/gh-reva/internal/model"
)

func (m Model) handleKeyVisual(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		// Ctrl-C dismisses visual mode like Esc — in vim's parlance it is
		// the universal "cancel current state" gesture. Suppressing the
		// global Ctrl-C → Quit handler in this branch keeps the program
		// running, so a stray Ctrl-C during selection just drops back to
		// normal mode rather than killing the TUI mid-review.
		m.state.Visual = nil
		return m, nil
	case "y":
		_ = clipboard.Yank(m.yankString())
		m.state.Visual = nil
		return m, nil
	case "tab", "shift+tab", "enter", "backspace", "v", " ", "q":
		// State-mutating / mode keys are inert in visual mode:
		//   Tab / Shift-Tab — would move focus mid-selection.
		//   Enter           — would still toggle Files-tree dir folds.
		//   Space           — would toggle hover (Files/Commits) or
		//                     split⇄unified (Diff) mid-selection.
		//   v               — would re-enter visual on top of itself.
		//   q               — the global handler quits the TUI; suppressing
		//                     it here means a stray `q` during a selection
		//                     just no-ops instead of dropping the user out
		//                     of the program with their range unyanked.
		//                     (Use Esc / Ctrl-C to leave visual without
		//                     yanking, or `y` to yank and exit.)
		//   Backspace       — unbound today but kept here so a future
		//                     re-bind cannot accidentally fire during
		//                     selection.
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
		// Cursor space is `len(commits) + 1` to account for the synthetic
		// "All commits" row at index 0; the virtual row contributes nothing
		// to the clipboard so the loop skips it.
		commits := m.visibleCommits()
		total := len(commits) + 1
		lo, hi := m.linewiseSelectionRange(model.PaneCommits, m.state.CommitsCursor, total)
		var rows []string
		for i := lo; i <= hi && i < total; i++ {
			if i == 0 {
				continue
			}
			c := commits[i-1]
			rows = append(rows, fmt.Sprintf("%s %s", c.ShortSHA, c.Message))
		}
		return strings.Join(rows, "\n")
	case model.PaneComments:
		flat := m.flatComments()
		lo, hi := m.linewiseSelectionRange(model.PaneComments, m.state.CommentsCursor, len(flat))
		var rows []string
		for i := lo; i <= hi && i < len(flat); i++ {
			c := flat[i]
			// Mirror the Comments-pane header timestamp exactly:
			// local TZ + "yyyy-mm-dd hh:mm". Yanking the same string the
			// user reads on screen avoids a day-boundary surprise where
			// the visible date and the clipboard date diverged because
			// yank was UTC and the header was Local.
			rows = append(rows, fmt.Sprintf("%s @ %s\n%s", c.User, c.CreatedAt.Local().Format("2006-01-02 15:04"), c.Body))
		}
		return strings.Join(rows, "\n")
	case model.PaneDiff:
		// Route through m.patchLines() so the cached split is reused and
		// the trailing-newline-trim convention stays in one place. An
		// unloaded patch yields a nil slice, which collapses to an empty
		// yank via linewiseSelectionRange — same behaviour the manual
		// strings.Split path produced.
		lines := m.patchLines()
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
