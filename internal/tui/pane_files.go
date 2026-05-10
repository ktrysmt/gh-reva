package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-reva/internal/model"
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
	key := msg.String()
	if handled := m.handlePendingG(key, func() {
		m.state.FilesCursor = 0
	}); handled {
		return m, nil
	}
	switch key {
	case "j", "down":
		if m.state.FilesCursor < len(m.state.PR.Files)-1 {
			m.state.FilesCursor++
		}
	case "k", "up":
		if m.state.FilesCursor > 0 {
			m.state.FilesCursor--
		}
	case "G":
		if n := len(m.state.PR.Files); n > 0 {
			m.state.FilesCursor = n - 1
		}
	case "enter":
		// Commit the cursor file: select it and shift focus to Diff.
		// j/k/gg/G no longer auto-select (the per-keystroke Diff
		// re-render felt sluggish); Enter is the deliberate gesture
		// that updates SelectedFile.
		if m.state.FilesCursor < 0 || m.state.FilesCursor >= len(m.state.PR.Files) {
			return m, nil
		}
		m.selectFile(m.state.PR.Files[m.state.FilesCursor].Path)
		m.state.FocusedPane = model.PaneDiff
	case " ":
		m.toggleModal(model.PaneFiles)
	case "t":
		prev := m.state.FilesTreeMode
		m.state.FilesTreeMode = !prev
		m.remapCursorOnTreeToggle(prev)
	}
	return m, nil
}

func (m Model) handleKeyFilesTree(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.filesTreeRows()
	key := msg.String()
	if handled := m.handlePendingG(key, func() {
		m.state.FilesCursor = 0
	}); handled {
		return m, nil
	}
	switch key {
	case "j", "down":
		if m.state.FilesCursor < len(rows)-1 {
			m.state.FilesCursor++
		}
	case "k", "up":
		if m.state.FilesCursor > 0 {
			m.state.FilesCursor--
		}
	case "G":
		if n := len(rows); n > 0 {
			m.state.FilesCursor = n - 1
		}
	case " ":
		m.toggleModal(model.PaneFiles)
	case "t":
		prev := m.state.FilesTreeMode
		m.state.FilesTreeMode = !prev
		m.remapCursorOnTreeToggle(prev)
	case "enter":
		// File rows commit the cursor file (peer to flat-mode Enter):
		// selectFile + focus Diff. Dir rows fold / unfold and keep
		// focus on Files. Out-of-range cursors no-op.
		if m.state.FilesCursor < 0 || m.state.FilesCursor >= len(rows) {
			return m, nil
		}
		r := rows[m.state.FilesCursor]
		if r.Kind == model.FilesRowFile {
			m.selectFile(r.Path)
			m.state.FocusedPane = model.PaneDiff
			return m, nil
		}
		if m.state.FoldedDirs[r.Path] {
			delete(m.state.FoldedDirs, r.Path)
		} else {
			m.state.FoldedDirs[r.Path] = true
		}
		// Cursor stays on the dir row.
	}
	return m, nil
}

func (m *Model) selectFile(path string) {
	if m.state.SelectedFile != path {
		m.state.SelectedFile = path
		m.state.SelectedRange = model.CommitRange{Kind: model.RangeWholePR}
		// Reset to the after column so per-file context Enter posts to
		// RIGHT by default (CLAUDE.md §4 #19 chose "保持しない。初期列に
		// リセット" for file switches). Persisting Side across files
		// would surprise the user — they would press Enter on a fresh
		// file and post to the side they last touched on a different
		// file, with no visual cue tying the two together.
		m.state.DiffCursor = model.DiffCursor{Side: model.DiffSideRight}
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
		path := m.searchHighlight(f.Path, model.PaneFiles)
		rows = append(rows, fmt.Sprintf("%s %s %s%s", cursor, status, path, count))
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
			name := m.searchHighlight(baseName(r.Path), model.PaneFiles)
			out = append(out, fmt.Sprintf("%s%s%s%s/", cursor, ind, fg(marker, m.theme.DiffLineNumber), name))
		default:
			f := m.state.PR.Files[r.FileIndex]
			count := ""
			if f.CommentCount > 0 {
				count = fg(fmt.Sprintf(" (%d)", f.CommentCount), m.theme.CommitSHA)
			}
			status := m.styledStatus(f.Status)
			// Search matches against the full path; highlight what's
			// visible in tree mode (basename), so the user still sees
			// the band when the query lives in the file's leaf segment.
			name := m.searchHighlight(baseName(f.Path), model.PaneFiles)
			out = append(out, fmt.Sprintf("%s%s %s %s%s", cursor, ind, status, name, count))
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
