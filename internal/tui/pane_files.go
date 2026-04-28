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
		}
	case "k", "up":
		if m.state.FilesCursor > 0 {
			m.state.FilesCursor--
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
	case " ":
		if m.state.FilesCursor < len(m.state.PR.Files) {
			f := m.state.PR.Files[m.state.FilesCursor]
			m.toggleCommitFilter(f.Path)
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
		}
	case "k", "up":
		if m.state.FilesCursor > 0 {
			m.state.FilesCursor--
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
	case " ":
		if m.state.FilesCursor < 0 || m.state.FilesCursor >= len(rows) {
			return m, nil
		}
		r := rows[m.state.FilesCursor]
		if r.Kind == model.FilesRowFile {
			m.toggleCommitFilter(r.Path)
		}
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

func (m *Model) toggleCommitFilter(path string) {
	if m.state.CommitFilterFile == path {
		m.state.CommitFilterFile = ""
	} else {
		m.state.CommitFilterFile = path
	}
	m.state.CommitsCursor = 0
}

func (m Model) filesView() string {
	title := paneTitle("Files", m.state.FocusedPane == model.PaneFiles, "")
	if m.state.PR == nil {
		return title
	}
	if m.state.FilesTreeMode {
		return title + "\n" + m.filesTreeRender()
	}
	var rows []string
	for i, f := range m.state.PR.Files {
		cursor := m.cursorMarker(model.PaneFiles, i, m.state.FilesCursor)
		filterMark := " "
		if f.Path == m.state.CommitFilterFile {
			filterMark = "*"
		}
		count := ""
		if f.CommentCount > 0 {
			count = fmt.Sprintf(" (%d)", f.CommentCount)
		}
		rows = append(rows, fmt.Sprintf("%s%s%s %s%s", cursor, filterMark, changeKindShort(f.Status), f.Path, count))
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
		cursor := m.cursorMarker(model.PaneFiles, i, m.state.FilesCursor)
		ind := indent(r.Depth)
		switch r.Kind {
		case model.FilesRowDir:
			marker := "v "
			if m.state.FoldedDirs[r.Path] {
				marker = "> "
			}
			name := baseName(r.Path)
			out = append(out, fmt.Sprintf("%s%s%s%s/", cursor, ind, marker, name))
		default:
			f := m.state.PR.Files[r.FileIndex]
			filterMark := " "
			if f.Path == m.state.CommitFilterFile {
				filterMark = "*"
			}
			count := ""
			if f.CommentCount > 0 {
				count = fmt.Sprintf(" (%d)", f.CommentCount)
			}
			out = append(out, fmt.Sprintf("%s%s%s%s %s%s", cursor, ind, filterMark, changeKindShort(f.Status), baseName(f.Path), count))
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
