package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// Cursor index 0 in the Commits pane is the synthetic "All commits" row that
// represents RangeWholePR. Real commits sit at indices 1..N, so the
// visibleCommits() slice is offset by one against the Commits cursor. j/k
// auto-select translates that mapping in autoSelectCommit.

func (m Model) handleKeyCommits(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.state.PR == nil {
		return m, nil
	}
	commits := m.visibleCommits()
	key := msg.String()
	if handled := m.handlePendingG(key, func() {
		m.state.CommitsCursor = 0
		m.autoSelectCommit(commits)
	}); handled {
		return m, nil
	}
	switch key {
	case "j", "down":
		if m.state.CommitsCursor < len(commits) {
			m.state.CommitsCursor++
			m.autoSelectCommit(commits)
		}
	case "k", "up":
		if m.state.CommitsCursor > 0 {
			m.state.CommitsCursor--
			m.autoSelectCommit(commits)
		}
	case "G":
		m.state.CommitsCursor = len(commits)
		m.autoSelectCommit(commits)
	case " ":
		m.toggleModal(model.PaneCommits)
	}
	return m, nil
}

// autoSelectCommit pins SelectedRange to the cursor row so the Diff and
// Comments panes follow the cursor live. Cursor index 0 maps to RangeWholePR
// (the synthetic "All commits" row); indices 1..N map to commits[i-1] under
// RangeSingleCommit. Visual mode is excluded so multi-row yank does not
// mutate the working slice.
func (m *Model) autoSelectCommit(commits []*model.Commit) {
	if m.state.Visual != nil {
		return
	}
	idx := m.state.CommitsCursor
	if idx == 0 {
		if m.state.SelectedRange.Kind == model.RangeWholePR {
			return
		}
		m.state.SelectedRange = model.CommitRange{Kind: model.RangeWholePR}
		m.state.DiffCursor = model.DiffCursor{Side: model.DiffSideRight}
		m.state.DiffViewport.Top = 0
		m.state.CommentsCursor = 0
		return
	}
	if idx < 1 || idx > len(commits) {
		return
	}
	c := commits[idx-1]
	if m.state.SelectedRange.Kind == model.RangeSingleCommit && m.state.SelectedRange.SHA == c.SHA {
		return
	}
	m.state.SelectedRange = model.CommitRange{Kind: model.RangeSingleCommit, SHA: c.SHA}
	// Reset to the after column on commit switch — DiffCursor.Side
	// without an explicit value is empty string, which makes j/k
	// auto-skip treat every `+` and `-` row as "not on this side"
	// and freeze cursor motion. CLAUDE.md §4 #19 establishes the
	// "reset Side to RIGHT on context switch" rule for selectFile;
	// the same reset applies here so a commit switch does not strand
	// the cursor in a side-less limbo.
	m.state.DiffCursor = model.DiffCursor{Side: model.DiffSideRight}
	m.state.DiffViewport.Top = 0
	m.state.CommentsCursor = 0
}

func (m Model) commitsView() string {
	title := m.styledPaneTitle("Commits", m.state.FocusedPane == model.PaneCommits, "")
	if m.state.PR == nil {
		return title
	}
	var rows []string
	commits := m.visibleCommits()

	rows = append(rows, m.allCommitsRow(commits))

	for i, c := range commits {
		cursor := m.styledCursor(model.PaneCommits, i+1, m.state.CommitsCursor)
		annotation := "    "
		if m.state.SelectedFile != "" && m.state.SelectedFile != model.AllFilesPath {
			if k, ok := c.ChangedFiles[m.state.SelectedFile]; ok {
				annotation = "[" + m.styledStatus(k) + "] "
			}
		}
		// Search highlight on the message text (plain). The short SHA
		// already carries CommitSHA fg styling and nesting bg under fg
		// confuses lipgloss's SGR composition; we let the row cursor
		// `>` carry the visual signal for sha-only matches.
		sha := fg(c.ShortSHA, m.theme.CommitSHA)
		msg := m.searchHighlight(c.Message, model.PaneCommits)
		rows = append(rows, fmt.Sprintf("%s%s%s %s", cursor, annotation, sha, msg))
	}
	return title + "\n" + strings.Join(rows, "\n")
}

// allCommitsRow renders the synthetic row at index 0. Label form:
//
//	no file selected / AllFilesPath:  "[*] All commits (N)"
//	file selected, M == N:            "[*] All commits (N)"   // file in every commit
//	file selected, M < N:             "[*] All commits (M of N)"
//
// The annotation slot is fixed to the synthetic "[*]" marker regardless
// of file selection. Mirroring the file's [A]/[M]/[D]/[R] there made the
// row look identical to a real commit annotation column-wise; the user
// asked for a distinct marker that signals "virtual row" at a glance.
// The M-of-N count remains file-aware so reviewers still see how the
// scoped commit set narrows under file selection.
func (m Model) allCommitsRow(visible []*model.Commit) string {
	cursor := m.styledCursor(model.PaneCommits, 0, m.state.CommitsCursor)
	annotation := m.allRowMarker() + " "
	totalCommits := len(m.state.PR.Commits)
	visibleCount := len(visible)
	label := fmt.Sprintf("All commits (%d)", totalCommits)
	if m.state.SelectedFile != "" && m.state.SelectedFile != model.AllFilesPath {
		if visibleCount < totalCommits {
			label = fmt.Sprintf("All commits (%d of %d)", visibleCount, totalCommits)
		}
	}
	return cursor + annotation + fgBold(label, "")
}

// visibleCommits filters PR.Commits to those that touch the SelectedFile.
// Without a SelectedFile (initial state before any file is loaded) OR
// when SelectedFile is the AllFilesPath sentinel (Files pane's All row),
// every commit is returned — the user explicitly opted out of per-file
// scoping so the Commits pane shows the entire PR history. The synthetic
// "All commits" row at cursor index 0 of the Commits pane sits ABOVE
// this slice — see commitsView and autoSelectCommit for how the cursor
// maps that virtual row to RangeWholePR.
func (m Model) visibleCommits() []*model.Commit {
	if m.state.PR == nil {
		return nil
	}
	if m.state.SelectedFile == "" || m.state.SelectedFile == model.AllFilesPath {
		return m.state.PR.Commits
	}
	var out []*model.Commit
	for _, c := range m.state.PR.Commits {
		if _, ok := c.ChangedFiles[m.state.SelectedFile]; ok {
			out = append(out, c)
		}
	}
	return out
}
