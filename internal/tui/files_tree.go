package tui

import (
	"sort"
	"strings"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// buildFilesTree groups PR.Files by directory and returns a flat list of
// rows in tree-display order. Subtrees of folded dirs are skipped.
func buildFilesTree(files []*model.FileEntry, foldedDirs map[string]bool) []model.FilesRow {
	type node struct {
		name     string
		dir      string
		children map[string]*node
		files    []int
	}
	root := &node{children: map[string]*node{}}
	for i, f := range files {
		parts := strings.Split(f.Path, "/")
		cur := root
		for j := 0; j < len(parts)-1; j++ {
			child, ok := cur.children[parts[j]]
			if !ok {
				child = &node{
					name:     parts[j],
					dir:      strings.Join(parts[:j+1], "/"),
					children: map[string]*node{},
				}
				cur.children[parts[j]] = child
			}
			cur = child
		}
		cur.files = append(cur.files, i)
	}
	var out []model.FilesRow
	var walk func(n *node, depth int)
	walk = func(n *node, depth int) {
		dirs := make([]string, 0, len(n.children))
		for name := range n.children {
			dirs = append(dirs, name)
		}
		sort.Strings(dirs)
		for _, name := range dirs {
			child := n.children[name]
			out = append(out, model.FilesRow{
				Kind:      model.FilesRowDir,
				Depth:     depth,
				Path:      child.dir,
				FileIndex: -1,
			})
			if !foldedDirs[child.dir] {
				walk(child, depth+1)
			}
		}
		sorted := append([]int(nil), n.files...)
		sort.Slice(sorted, func(i, j int) bool {
			return files[sorted[i]].Path < files[sorted[j]].Path
		})
		for _, idx := range sorted {
			out = append(out, model.FilesRow{
				Kind:      model.FilesRowFile,
				Depth:     depth,
				Path:      files[idx].Path,
				FileIndex: idx,
			})
		}
	}
	walk(root, 0)
	return out
}

// filesTreeRows returns the current Files-pane tree rows. It rebuilds on
// every call (cheap for typical PRs) so callers do not need to invalidate.
func (m Model) filesTreeRows() []model.FilesRow {
	if m.state.PR == nil {
		return nil
	}
	return buildFilesTree(m.state.PR.Files, m.state.FoldedDirs)
}

// fileIndexFromTreeCursor returns the underlying PR.Files index for the row
// the cursor currently sits on, or -1 when the cursor is on a directory.
func (m Model) fileIndexFromTreeCursor() int {
	rows := m.filesTreeRows()
	if m.state.FilesCursor < 0 || m.state.FilesCursor >= len(rows) {
		return -1
	}
	r := rows[m.state.FilesCursor]
	if r.Kind != model.FilesRowFile {
		return -1
	}
	return r.FileIndex
}

// remapCursorOnTreeToggle keeps the cursor pointing at the same conceptual
// row when switching between flat and tree modes.
func (m *Model) remapCursorOnTreeToggle(prevTree bool) {
	if m.state.PR == nil {
		return
	}
	if prevTree {
		// tree → flat: cursor was a tree-row index; convert via FileIndex.
		rows := buildFilesTree(m.state.PR.Files, m.state.FoldedDirs)
		if m.state.FilesCursor >= 0 && m.state.FilesCursor < len(rows) {
			r := rows[m.state.FilesCursor]
			if r.Kind == model.FilesRowFile {
				m.state.FilesCursor = r.FileIndex
				return
			}
		}
		m.state.FilesCursor = 0
		return
	}
	// flat → tree: cursor was a file index; locate that file in the tree.
	rows := buildFilesTree(m.state.PR.Files, m.state.FoldedDirs)
	for i, r := range rows {
		if r.Kind == model.FilesRowFile && r.FileIndex == m.state.FilesCursor {
			m.state.FilesCursor = i
			return
		}
	}
	m.state.FilesCursor = 0
}
