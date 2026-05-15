package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ktrysmt/gh-reva/internal/diff"
	"github.com/ktrysmt/gh-reva/internal/model"
)

// handlePendingG implements the shared `g`-prefix two-key state machine
// used by gg (gotoTop) in every pane. Returns true when the dispatcher
// should stop (either pending recorded or gotoTop fired); returns false
// when the caller should fall through to normal dispatch with the
// pending prefix already cleared. gotoTop is invoked on the second `g`.
func (m *Model) handlePendingG(key string, gotoTop func()) bool {
	if m.state.PendingPrefix == "g" {
		m.state.PendingPrefix = ""
		if key == "g" {
			gotoTop()
			return true
		}
		return false
	}
	if key == "g" {
		m.state.PendingPrefix = "g"
		return true
	}
	return false
}

// startSearch enters incsearch on the focused pane. The pre-search cursor
// state is snapshotted so Esc can restore it on cancel.
func (m *Model) startSearch() {
	if m.state.PR == nil {
		return
	}
	s := &model.SearchState{
		Status:               model.SearchEditing,
		TargetPane:           m.state.FocusedPane,
		SavedFilesCursor:     m.state.FilesCursor,
		SavedCommitsCursor:   m.state.CommitsCursor,
		SavedDiffCursor:      m.state.DiffCursor,
		SavedDiffViewportTop: m.state.DiffViewport.Top,
		SavedCommentsCursor:  m.state.CommentsCursor,
		SavedSelectedFile:    m.state.SelectedFile,
		SavedSelectedRange:   m.state.SelectedRange,
		SavedFocusedPane:     m.state.FocusedPane,
	}
	m.state.Search = s
}

// cancelSearch restores the pre-search cursor state and clears Search.
// Used by Esc / Ctrl+C while editing AND by an empty-query backspace.
func (m *Model) cancelSearch() {
	s := m.state.Search
	if s == nil {
		return
	}
	if s.SavedSelectedFile != "" && m.state.SelectedFile != s.SavedSelectedFile {
		m.state.SelectedFile = s.SavedSelectedFile
		m.state.SelectedRange = s.SavedSelectedRange
	}
	m.state.FilesCursor = s.SavedFilesCursor
	m.state.CommitsCursor = s.SavedCommitsCursor
	m.state.DiffCursor = s.SavedDiffCursor
	m.state.DiffViewport.Top = s.SavedDiffViewportTop
	m.state.CommentsCursor = s.SavedCommentsCursor
	m.state.FocusedPane = s.SavedFocusedPane
	m.state.Search = nil
}

// commitSearch transitions Editing → Active, leaving the cursor on the
// current match. An empty query or no-match search cancels with a notice
// instead of locking the user into an inert n/N session.
func (m *Model) commitSearch() {
	s := m.state.Search
	if s == nil {
		return
	}
	if s.Query == "" || len(s.Matches) == 0 {
		query := s.Query
		m.cancelSearch()
		if query != "" {
			m.state.Notice = "no match: " + query
		}
		return
	}
	s.Status = model.SearchActive
}

// recomputeSearch (re-)evaluates the query against the target pane and
// jumps the cursor to the nearest match. Called on every keystroke
// during Editing.
func (m *Model) recomputeSearch() {
	s := m.state.Search
	if s == nil {
		return
	}
	if s.Query == "" {
		s.Matches = nil
		s.CursorIdx = 0
		// Restore pre-search cursor while incremental query is empty.
		m.state.FilesCursor = s.SavedFilesCursor
		m.state.CommitsCursor = s.SavedCommitsCursor
		m.state.DiffCursor = s.SavedDiffCursor
		m.state.DiffViewport.Top = s.SavedDiffViewportTop
		m.state.CommentsCursor = s.SavedCommentsCursor
		return
	}
	s.Matches = m.collectMatches(s.TargetPane, s.Query)
	if len(s.Matches) == 0 {
		s.CursorIdx = 0
		return
	}
	// Pick the match nearest to (>=) the saved cursor; wrap to 0 if past end.
	saved := m.savedCursorForPane(s)
	idx := 0
	for i, mm := range s.Matches {
		if mm.Index >= saved {
			idx = i
			break
		}
	}
	s.CursorIdx = idx
	m.applySearchCursor(s)
}

// savedCursorForPane returns the pre-search cursor index for the
// search target pane. Used by recomputeSearch to pick the match nearest
// the user's prior position.
func (m Model) savedCursorForPane(s *model.SearchState) int {
	switch s.TargetPane {
	case model.PaneFiles:
		return s.SavedFilesCursor
	case model.PaneCommits:
		return s.SavedCommitsCursor
	case model.PaneDiff:
		return s.SavedDiffCursor.Line
	case model.PaneComments:
		return s.SavedCommentsCursor
	}
	return 0
}

// applySearchCursor moves the live cursor of TargetPane to the row
// pointed to by CursorIdx. For Files / Commits this also auto-selects
// (so Diff and Comments follow, mirroring j/k).
func (m *Model) applySearchCursor(s *model.SearchState) {
	if s == nil || len(s.Matches) == 0 || s.CursorIdx < 0 || s.CursorIdx >= len(s.Matches) {
		return
	}
	idx := s.Matches[s.CursorIdx].Index
	switch s.TargetPane {
	case model.PaneFiles:
		m.state.FilesCursor = idx
		if m.state.FilesTreeMode {
			m.autoSelectTree(m.filesTreeRows())
		} else {
			m.autoSelectFlat()
		}
	case model.PaneCommits:
		m.state.CommitsCursor = idx
		m.autoSelectCommit(m.visibleCommits())
	case model.PaneDiff:
		m.state.DiffCursor.Line = idx
		m.scrollDiffIntoView(len(m.patchLines()))
	case model.PaneComments:
		m.state.CommentsCursor = idx
		m.syncDiffToCursorComment()
	}
}

// searchAdvance cycles n/N through the Active match list. step=+1 is n,
// step=-1 is N. Wraps around at both ends.
func (m *Model) searchAdvance(step int) {
	s := m.state.Search
	if s == nil || s.Status != model.SearchActive || len(s.Matches) == 0 {
		return
	}
	n := len(s.Matches)
	s.CursorIdx = (s.CursorIdx + step + n) % n
	m.applySearchCursor(s)
}

// collectMatches returns the matches for `query` against `pane`. Smart-
// case literal substring: lowercase queries match case-insensitively,
// queries containing any uppercase rune match case-sensitively.
func (m Model) collectMatches(pane model.PaneID, query string) []model.SearchMatch {
	if query == "" || m.state.PR == nil {
		return nil
	}
	q := query
	smart := smartCaseFold(query)
	switch pane {
	case model.PaneFiles:
		return m.collectFileMatches(q, smart)
	case model.PaneCommits:
		return m.collectCommitMatches(q, smart)
	case model.PaneDiff:
		return m.collectDiffMatches(q, smart)
	case model.PaneComments:
		return m.collectCommentMatches(q, smart)
	}
	return nil
}

func smartCaseFold(q string) bool {
	for _, r := range q {
		if r >= 'A' && r <= 'Z' {
			return false // any uppercase → case-sensitive
		}
	}
	return true
}

func substr(haystack, needle string, foldCase bool) bool {
	if foldCase {
		return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
	}
	return strings.Contains(haystack, needle)
}

func (m Model) collectFileMatches(q string, fold bool) []model.SearchMatch {
	var out []model.SearchMatch
	if m.state.FilesTreeMode {
		for i, r := range m.filesTreeRows() {
			if r.Kind == model.FilesRowAll {
				// The synthetic "All (N files)" row is not a search
				// target — its literal text never carries user content.
				continue
			}
			if substr(r.Path, q, fold) {
				out = append(out, model.SearchMatch{Index: i})
			}
		}
		return out
	}
	// Cursor index 0 is the synthetic "All (N files)" row, so real
	// files live at cursor idx i+1.
	for i, f := range m.state.PR.Files {
		if substr(f.Path, q, fold) {
			out = append(out, model.SearchMatch{Index: i + 1})
		}
	}
	return out
}

func (m Model) collectCommitMatches(q string, fold bool) []model.SearchMatch {
	var out []model.SearchMatch
	commits := m.visibleCommits()
	for i, c := range commits {
		// Cursor index 0 is the synthetic "All commits" row, so real
		// commits live at cursor idx i+1.
		if substr(c.Message, q, fold) || substr(c.SHA, q, fold) || substr(c.ShortSHA, q, fold) {
			out = append(out, model.SearchMatch{Index: i + 1})
		}
	}
	return out
}

func (m Model) collectDiffMatches(q string, fold bool) []model.SearchMatch {
	var out []model.SearchMatch
	for i, line := range m.patchLines() {
		// Synthetic `···` rows carry the "N lines hidden" hint but the
		// underlying file content isn't loaded into the buffer cell, so
		// matching against the sentinel would produce phantom hits.
		// Excluding them mirrors GitHub web search (you can't `/` into
		// collapsed regions).
		if line == diff.SyntheticLine {
			continue
		}
		if substr(line, q, fold) {
			out = append(out, model.SearchMatch{Index: i})
		}
	}
	return out
}

func (m Model) collectCommentMatches(q string, fold bool) []model.SearchMatch {
	var out []model.SearchMatch
	flat := m.flatComments()
	for i, c := range flat {
		if substr(c.Body, q, fold) || substr(c.User, q, fold) {
			out = append(out, model.SearchMatch{Index: i})
		}
	}
	return out
}

// highlightMatches wraps every literal occurrence of `query` inside `s`
// with a bg-styled span. fold=true folds case (smart-case lower-only).
// Multi-byte runes are byte-indexed because strings.Index works on
// bytes; this stays correct for CJK fixtures since CJK is case-less and
// strings.ToLower preserves byte length on non-ASCII runes. Returns s
// unchanged when query is empty, bg is unset, or no match is present.
func highlightMatches(s, query string, fold bool, bg lipgloss.Color) string {
	if s == "" || query == "" || bg == "" {
		return s
	}
	haystack := s
	needle := query
	if fold {
		haystack = strings.ToLower(s)
		needle = strings.ToLower(query)
	}
	if !strings.Contains(haystack, needle) {
		return s
	}
	style := lipgloss.NewStyle().Background(bg)
	var b strings.Builder
	pos := 0
	for {
		idx := strings.Index(haystack[pos:], needle)
		if idx < 0 {
			b.WriteString(s[pos:])
			break
		}
		b.WriteString(s[pos : pos+idx])
		b.WriteString(style.Render(s[pos+idx : pos+idx+len(needle)]))
		pos = pos + idx + len(needle)
	}
	return b.String()
}

// searchHighlight applies match-bg highlight to `s` when an active
// Search session targets `pane`. No-op otherwise. Used by the Files
// and Commits renderers to mark the matched substring inside each row.
func (m Model) searchHighlight(s string, pane model.PaneID) string {
	ss := m.state.Search
	if ss == nil || ss.Query == "" || ss.TargetPane != pane {
		return s
	}
	return highlightMatches(s, ss.Query, smartCaseFold(ss.Query), m.theme.SearchMatchBg)
}

// searchMatchLines returns the set of buffer-line indices in the Diff
// pane that should carry a row-wide match-bg. Empty when Search does
// not target Diff. Used by renderUnifiedBufferLine / renderSplitBufferLine
// to apply highlight after the row is assembled.
func (m Model) searchMatchLines() map[int]bool {
	ss := m.state.Search
	if ss == nil || ss.Query == "" || ss.TargetPane != model.PaneDiff {
		return nil
	}
	out := map[int]bool{}
	for _, mm := range ss.Matches {
		out[mm.Index] = true
	}
	return out
}

// handleKeySearch absorbs every keystroke while a search query is being
// typed (Status == SearchEditing). Backspace shrinks the query (an empty
// query backspace cancels — vim convention). Enter commits to Active,
// Esc / Ctrl+C cancels and restores the pre-search cursor.
func (m Model) handleKeySearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.state.Search
	if s == nil || s.Status != model.SearchEditing {
		return m, nil
	}
	switch msg.String() {
	case "enter":
		m.commitSearch()
		return m, nil
	case "esc", "ctrl+c":
		m.cancelSearch()
		return m, nil
	case "backspace":
		if s.Query == "" {
			m.cancelSearch()
			return m, nil
		}
		runes := []rune(s.Query)
		s.Query = string(runes[:len(runes)-1])
		m.recomputeSearch()
		return m, nil
	case "tab", "shift+tab":
		// Inert: changing focus mid-query would silently rebind the
		// target pane and surprise the user. Press Esc to leave search.
		return m, nil
	}
	if msg.Type == tea.KeySpace {
		s.Query += " "
		m.recomputeSearch()
		return m, nil
	}
	if len(msg.Runes) > 0 {
		s.Query += string(msg.Runes)
		m.recomputeSearch()
		return m, nil
	}
	return m, nil
}
