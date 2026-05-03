package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-rv/internal/model"
)

func (m Model) handleKeyFiles(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.state.PR == nil {
		return m, nil
	}
	if m.state.FilesTreeMode {
		return m.handleKeyFilesTree(msg)
	}
	return m.handleKeyFilesFlat(msg)
}

func (m Model) handleKeyFilesFlat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.state.FilesCursor < len(m.state.PR.Files)-1 {
			m.state.FilesCursor++
			m.autoSelectFlat()
		}
	case "k", "up":
		if m.state.FilesCursor > 0 {
			m.state.FilesCursor--
			m.autoSelectFlat()
		}
	case "t":
		prev := m.state.FilesTreeMode
		m.state.FilesTreeMode = !prev
		m.remapCursorOnTreeToggle(prev)
	case "enter":
		if m.state.FilesCursor < len(m.state.PR.Files) {
			m.selectFile(m.state.PR.Files[m.state.FilesCursor].Path)
			m.state.FocusedPane = model.PaneCommits
		}
	}
	return m, nil
}

func (m Model) handleKeyFilesTree(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.filesTreeRows()
	switch msg.String() {
	case "j", "down":
		if m.state.FilesCursor < len(rows)-1 {
			m.state.FilesCursor++
			m.autoSelectTree(rows)
		}
	case "k", "up":
		if m.state.FilesCursor > 0 {
			m.state.FilesCursor--
			m.autoSelectTree(rows)
		}
	case "t":
		prev := m.state.FilesTreeMode
		m.state.FilesTreeMode = !prev
		m.remapCursorOnTreeToggle(prev)
	case "enter":
		if m.state.FilesCursor < 0 || m.state.FilesCursor >= len(rows) {
			return m, nil
		}
		r := rows[m.state.FilesCursor]
		if r.Kind == model.FilesRowDir {
			if m.state.FoldedDirs[r.Path] {
				delete(m.state.FoldedDirs, r.Path)
			} else {
				m.state.FoldedDirs[r.Path] = true
			}
			// Cursor stays on the dir row.
			return m, nil
		}
		m.selectFile(r.Path)
		m.state.FocusedPane = model.PaneCommits
	}
	return m, nil
}

func (m *Model) selectFile(path string) {
	if m.state.SelectedFile != path {
		m.state.SelectedFile = path
		m.state.SelectedRange = model.CommitRange{Kind: model.RangeWholePR}
		m.state.DiffCursor = model.DiffCursor{}
		m.state.DiffViewport.Top = 0
		m.state.CommitsCursor = 0
		m.state.CommentsCursor = 0
	}
}

// autoSelectFlat keeps SelectedFile aligned with the Files cursor in flat mode.
// Visual mode is excluded so multi-row yank does not mutate the working file.
func (m *Model) autoSelectFlat() {
	if m.state.Visual != nil || m.state.PR == nil {
		return
	}
	if m.state.FilesCursor < 0 || m.state.FilesCursor >= len(m.state.PR.Files) {
		return
	}
	m.selectFile(m.state.PR.Files[m.state.FilesCursor].Path)
}

// autoSelectTree mirrors autoSelectFlat for tree mode. Dir rows leave the
// current selection intact so users can fold/unfold without disturbing Diff.
func (m *Model) autoSelectTree(rows []model.FilesRow) {
	if m.state.Visual != nil {
		return
	}
	if m.state.FilesCursor < 0 || m.state.FilesCursor >= len(rows) {
		return
	}
	r := rows[m.state.FilesCursor]
	if r.Kind == model.FilesRowFile {
		m.selectFile(r.Path)
	}
}

// advanceFile moves to the next/prev file in the Files pane while leaving
// FocusedPane unchanged. Used by Shift+J/K outside the Files pane so users can
// scrub through file diffs without losing context. Tree-mode walks skip dir
// rows so callers always land on a file.
func (m *Model) advanceFile(forward bool) {
	if m.state.PR == nil || len(m.state.PR.Files) == 0 {
		return
	}
	step := 1
	if !forward {
		step = -1
	}
	if !m.state.FilesTreeMode {
		idx := m.state.FilesCursor + step
		if idx < 0 || idx >= len(m.state.PR.Files) {
			return
		}
		m.state.FilesCursor = idx
		m.selectFile(m.state.PR.Files[idx].Path)
		return
	}
	rows := m.filesTreeRows()
	if len(rows) == 0 {
		return
	}
	for i := m.state.FilesCursor + step; i >= 0 && i < len(rows); i += step {
		if rows[i].Kind == model.FilesRowFile {
			m.state.FilesCursor = i
			m.selectFile(rows[i].Path)
			return
		}
	}
}

func (m Model) filesView() string {
	title := m.styledPaneTitle("Files", m.state.FocusedPane == model.PaneFiles, "")
	if m.state.PR == nil {
		return title
	}
	if m.state.FilesTreeMode {
		return title + "\n" + m.filesTreeRender()
	}
	var rows []string
	for i, f := range m.state.PR.Files {
		cursor := m.styledCursor(model.PaneFiles, i, m.state.FilesCursor)
		count := ""
		if f.CommentCount > 0 {
			count = fg(fmt.Sprintf(" (%d)", f.CommentCount), m.theme.CommitSHA)
		}
		status := m.styledStatus(f.Status)
		rows = append(rows, fmt.Sprintf("%s %s %s%s", cursor, status, f.Path, count))
	}
	return title + "\n" + strings.Join(rows, "\n")
}

func (m Model) filesTreeRender() string {
	rows := m.filesTreeRows()
	if len(rows) == 0 {
		return "(no files)"
	}
	var out []string
	for i, r := range rows {
		cursor := m.styledCursor(model.PaneFiles, i, m.state.FilesCursor)
		ind := indent(r.Depth)
		switch r.Kind {
		case model.FilesRowDir:
			marker := "v "
			if m.state.FoldedDirs[r.Path] {
				marker = "> "
			}
			name := baseName(r.Path)
			out = append(out, fmt.Sprintf("%s%s%s%s/", cursor, ind, fg(marker, m.theme.DiffLineNumber), name))
		default:
			f := m.state.PR.Files[r.FileIndex]
			count := ""
			if f.CommentCount > 0 {
				count = fg(fmt.Sprintf(" (%d)", f.CommentCount), m.theme.CommitSHA)
			}
			status := m.styledStatus(f.Status)
			out = append(out, fmt.Sprintf("%s%s %s %s%s", cursor, ind, status, baseName(f.Path), count))
		}
	}
	return strings.Join(out, "\n")
}

func baseName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
