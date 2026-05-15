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
	// Cursor space is [0, len(files)] inclusive — index 0 is the
	// synthetic "All (N files)" row; indices 1..N map to files[i-1].
	// Mirrors the Commits pane's "All commits" virtual row.
	total := len(m.state.PR.Files)
	key := msg.String()
	if handled := m.handlePendingG(key, func() {
		m.state.FilesCursor = 0
	}); handled {
		return m, nil
	}
	switch key {
	case "j", "down":
		if m.state.FilesCursor < total {
			m.state.FilesCursor++
		}
	case "k", "up":
		if m.state.FilesCursor > 0 {
			m.state.FilesCursor--
		}
	case "G":
		m.state.FilesCursor = total
	case "enter":
		// Commit the cursor file: select it and shift focus to Diff.
		// j/k/gg/G no longer auto-select (the per-keystroke Diff
		// re-render felt sluggish); Enter is the deliberate gesture
		// that updates SelectedFile.
		if m.state.FilesCursor < 0 || m.state.FilesCursor > total {
			return m, nil
		}
		if m.state.FilesCursor == 0 {
			m.selectAllFiles()
		} else {
			m.selectFile(m.state.PR.Files[m.state.FilesCursor-1].Path)
		}
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
		// selectFile + focus Diff. The synthetic All row commits the
		// cross-file view (selectAllFiles + focus Diff). Dir rows fold
		// / unfold and keep focus on Files. Out-of-range cursors no-op.
		if m.state.FilesCursor < 0 || m.state.FilesCursor >= len(rows) {
			return m, nil
		}
		r := rows[m.state.FilesCursor]
		switch r.Kind {
		case model.FilesRowAll:
			m.selectAllFiles()
			m.state.FocusedPane = model.PaneDiff
			return m, nil
		case model.FilesRowFile:
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

// selectAllFiles commits the synthetic "All" row at Files cursor index 0.
// SelectedFile flips to model.AllFilesPath, which signals downstream code
// (visibleCommits filter bypass, concat patch lookup in patchInfo, gutter
// marker suppression in diffmap, compose / comments bail-outs) to render
// the cross-file browse mode.
func (m *Model) selectAllFiles() {
	if m.state.SelectedFile != model.AllFilesPath {
		m.state.SelectedFile = model.AllFilesPath
		m.state.SelectedRange = model.CommitRange{Kind: model.RangeWholePR}
		m.state.DiffCursor = model.DiffCursor{Side: model.DiffSideRight}
		m.state.DiffViewport.Top = 0
		m.state.CommitsCursor = 0
		m.state.CommentsCursor = 0
	}
}

// autoSelectFlat keeps SelectedFile aligned with the Files cursor in flat mode.
// Visual mode is excluded so multi-row yank does not mutate the working file.
// Cursor 0 maps to the synthetic All row (selectAllFiles); cursor 1..N maps
// to PR.Files[i-1] (selectFile).
func (m *Model) autoSelectFlat() {
	if m.state.Visual != nil || m.state.PR == nil {
		return
	}
	if m.state.FilesCursor < 0 || m.state.FilesCursor > len(m.state.PR.Files) {
		return
	}
	if m.state.FilesCursor == 0 {
		m.selectAllFiles()
		return
	}
	m.selectFile(m.state.PR.Files[m.state.FilesCursor-1].Path)
}

// autoSelectTree mirrors autoSelectFlat for tree mode. Dir rows leave the
// current selection intact so users can fold/unfold without disturbing Diff.
// FilesRowAll commits the cross-file view.
func (m *Model) autoSelectTree(rows []model.FilesRow) {
	if m.state.Visual != nil {
		return
	}
	if m.state.FilesCursor < 0 || m.state.FilesCursor >= len(rows) {
		return
	}
	r := rows[m.state.FilesCursor]
	switch r.Kind {
	case model.FilesRowAll:
		m.selectAllFiles()
	case model.FilesRowFile:
		m.selectFile(r.Path)
	}
}

// advanceFile moves to the next/prev file in the Files pane while leaving
// FocusedPane unchanged. Used by Shift+J/K outside the Files pane so users
// can scrub through file diffs without losing context. Tree-mode walks
// skip dir rows so callers always land on a file. The synthetic All row
// is deliberately skipped — Shift+J/K is the "next file diff" gesture, so
// the All view (which isn't a real file) does not fit the contract. The
// user reaches All by Tab to Files + k / gg.
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
		if idx < 1 || idx > len(m.state.PR.Files) {
			return
		}
		m.state.FilesCursor = idx
		m.selectFile(m.state.PR.Files[idx-1].Path)
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
	rows = append(rows, m.allFilesRow())
	for i, f := range m.state.PR.Files {
		cursor := m.styledCursor(model.PaneFiles, i+1, m.state.FilesCursor)
		count := ""
		if f.CommentCount > 0 {
			count = fg(fmt.Sprintf(" (%d)", f.CommentCount), m.theme.CommitSHA)
		}
		status := "[" + m.styledStatus(f.Status) + "]"
		path := m.searchHighlight(f.Path, model.PaneFiles)
		rows = append(rows, fmt.Sprintf("%s %s %s%s", cursor, status, path, count))
	}
	return title + "\n" + strings.Join(rows, "\n")
}

// allFilesRow renders the synthetic Files row at cursor index 0. It is
// the symmetric counterpart of the Commits pane's "All commits" row.
// Selecting it (Enter or click) sets SelectedFile=AllFilesPath and the
// Diff column switches to a cross-file concatenated view; from there
// the Commits column lets the user walk the entire PR history without
// re-selecting individual files.
func (m Model) allFilesRow() string {
	cursor := m.styledCursor(model.PaneFiles, 0, m.state.FilesCursor)
	label := fmt.Sprintf("All (%d files)", len(m.state.PR.Files))
	return cursor + " " + m.allRowMarker() + " " + fgBold(label, "")
}

// allRowMarker returns the synthetic "[*]" annotation used by the All row
// in the Files and Commits panes. The bracket pair mirrors the per-row
// [A]/[M]/[D]/[R] annotation shape so column widths align, while the `*`
// glyph identifies the row as virtual (not a real file/commit). The
// marker carries a muted color (DiffLineNumber) so it does not compete
// visually with the per-row status annotations.
func (m Model) allRowMarker() string {
	return "[" + fg("*", m.theme.DiffLineNumber) + "]"
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
		case model.FilesRowAll:
			label := fmt.Sprintf("All (%d files)", len(m.state.PR.Files))
			out = append(out, fmt.Sprintf("%s%s %s %s", cursor, ind, m.allRowMarker(), fgBold(label, "")))
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
			status := "[" + m.styledStatus(f.Status) + "]"
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
