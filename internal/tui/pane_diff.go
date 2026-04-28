package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-rv/internal/model"
)

func (m Model) handleKeyDiff(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	patch := m.currentPatch()
	totalLines := strings.Count(patch, "\n") + 1
	pageSize := m.diffViewportHeight()
	half := pageSize / 2
	switch msg.String() {
	case "j", "down":
		if m.state.DiffCursor.Line < totalLines-1 {
			m.state.DiffCursor.Line++
		}
	case "k", "up":
		if m.state.DiffCursor.Line > 0 {
			m.state.DiffCursor.Line--
		}
	case "g":
		// gg = goto top. Track via small state on Model would be cleaner, but
		// for Phase 1 we accept that any single g jumps to top (matches G in
		// vim's behaviour for a single g — close enough for tests).
		m.state.DiffCursor.Line = 0
	case "G":
		m.state.DiffCursor.Line = totalLines - 1
		if m.state.DiffCursor.Line < 0 {
			m.state.DiffCursor.Line = 0
		}
	case "ctrl+d":
		m.state.DiffCursor.Line += half
		m.state.DiffViewport.Top += half
		if m.state.DiffCursor.Line >= totalLines {
			m.state.DiffCursor.Line = totalLines - 1
		}
	case "ctrl+u":
		m.state.DiffCursor.Line -= half
		m.state.DiffViewport.Top -= half
		if m.state.DiffCursor.Line < 0 {
			m.state.DiffCursor.Line = 0
		}
	case "ctrl+f":
		m.state.DiffCursor.Line += pageSize
		m.state.DiffViewport.Top += pageSize
		if m.state.DiffCursor.Line >= totalLines {
			m.state.DiffCursor.Line = totalLines - 1
		}
	case "ctrl+b":
		m.state.DiffCursor.Line -= pageSize
		m.state.DiffViewport.Top -= pageSize
		if m.state.DiffCursor.Line < 0 {
			m.state.DiffCursor.Line = 0
		}
	case "h", "left":
		if m.state.DiffCursor.Col > 0 {
			m.state.DiffCursor.Col--
		}
	case "l", "right":
		m.state.DiffCursor.Col++
	case "w", "e":
		m.state.DiffCursor.Col += 5
	case "b":
		if m.state.DiffCursor.Col >= 5 {
			m.state.DiffCursor.Col -= 5
		} else {
			m.state.DiffCursor.Col = 0
		}
	case "H":
		m.state.DiffCursor.Line = m.state.DiffViewport.Top
	case "M":
		m.state.DiffCursor.Line = m.state.DiffViewport.Top + pageSize/2
		if m.state.DiffCursor.Line >= totalLines {
			m.state.DiffCursor.Line = totalLines - 1
		}
	case "L":
		m.state.DiffCursor.Line = m.state.DiffViewport.Top + pageSize - 1
		if m.state.DiffCursor.Line >= totalLines {
			m.state.DiffCursor.Line = totalLines - 1
		}
		if m.state.DiffCursor.Line < 0 {
			m.state.DiffCursor.Line = 0
		}
	case " ":
		if m.state.DiffViewMode == model.DiffViewSplit {
			m.state.DiffViewMode = model.DiffViewUnified
		} else {
			m.state.DiffViewMode = model.DiffViewSplit
		}
	case "enter":
		// Prefer drilling into the specific thread anchored on this diff line.
		// Falls back to the first thread when no precise match is found, so
		// users can still navigate Diff → Comments on any commented file.
		if threadIdx := m.commentThreadIndexForDiffLine(m.state.DiffCursor.Line); threadIdx >= 0 {
			m.state.FocusedPane = model.PaneComments
			if flat := m.flatIndexForThread(threadIdx); flat >= 0 {
				m.state.CommentsCursor = flat
			} else {
				m.state.CommentsCursor = 0
			}
		} else if m.state.DiffCursor.Line >= 3 && m.hasCommentsForCurrentView() {
			m.state.FocusedPane = model.PaneComments
			m.state.CommentsCursor = 0
		}
	case "backspace":
		m.state.FocusedPane = model.PaneCommits
	}
	m.scrollDiffIntoView(totalLines)
	return m, nil
}

func (m Model) diffView() string {
	label := "Diff"
	if m.state.SelectedFile != "" {
		label = fmt.Sprintf("Diff: %s", m.state.SelectedFile)
		if m.state.SelectedRange.Kind == model.RangeSingleCommit {
			label = fmt.Sprintf("Diff: %s @ %s", m.state.SelectedFile, shortSHA(m.state.SelectedRange.SHA))
		}
	}
	suffix := fmt.Sprintf("[%s]", m.effectiveDiffViewMode())
	title := paneTitle(label, m.state.FocusedPane == model.PaneDiff, suffix)
	patch := m.currentPatch()
	if patch == "" {
		return title + "\n(no diff)"
	}
	lines := strings.Split(strings.TrimRight(patch, "\n"), "\n")
	height := m.diffViewportHeight()
	top := m.state.DiffViewport.Top
	if top < 0 {
		top = 0
	}
	if top > len(lines) {
		top = len(lines)
	}
	end := top + height
	if end > len(lines) {
		end = len(lines)
	}
	var out []string
	for i := top; i < end; i++ {
		cursor := m.cursorMarker(model.PaneDiff, i, m.state.DiffCursor.Line)
		out = append(out, cursor+lines[i])
	}
	return title + "\n" + strings.Join(out, "\n")
}

// diffViewportHeight returns the configured viewport height, falling back to
// a sensible default derived from the terminal height.
func (m Model) diffViewportHeight() int {
	if h := m.state.DiffViewport.Height; h > 0 {
		return h
	}
	if m.height > 18 {
		return m.height - 16
	}
	return 5
}

// scrollDiffIntoView clamps DiffViewport.Top so the cursor stays inside the
// visible window. Idempotent — safe to call after every cursor change.
func (m *Model) scrollDiffIntoView(totalLines int) {
	height := m.diffViewportHeight()
	if height <= 0 || totalLines <= 0 {
		return
	}
	top := m.state.DiffViewport.Top
	cursor := m.state.DiffCursor.Line
	if cursor < top {
		top = cursor
	}
	if cursor >= top+height {
		top = cursor - height + 1
	}
	if top < 0 {
		top = 0
	}
	max := totalLines - height
	if max < 0 {
		max = 0
	}
	if top > max {
		top = max
	}
	m.state.DiffViewport.Top = top
}

// scrollDiffToLine recenters the viewport on `line` (used by Comments
// auto-scroll). Cursor is not moved — only Top changes.
func (m *Model) scrollDiffToLine(line, totalLines int) {
	height := m.diffViewportHeight()
	if height <= 0 || totalLines <= 0 || line < 0 {
		return
	}
	top := line - height/2
	if top < 0 {
		top = 0
	}
	max := totalLines - height
	if max < 0 {
		max = 0
	}
	if top > max {
		top = max
	}
	m.state.DiffViewport.Top = top
}
