package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// searchModelFixture builds a Model with multiple files / commits / comments
// + a small patch so the search and gg/G state machines can be exercised
// without touching the real GitHub client.
func searchModelFixture(t *testing.T) Model {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	files := []*model.FileEntry{
		{Path: "src/alpha.go", Status: model.ChangeModified},
		{Path: "src/beta.go", Status: model.ChangeAdded},
		{Path: "src/gamma_test.go", Status: model.ChangeModified},
		{Path: "internal/util.go", Status: model.ChangeRenamed},
		{Path: "README.md", Status: model.ChangeModified},
	}
	commits := []*model.Commit{
		{SHA: "aaaaaaaaaa", ShortSHA: "aaaaaaa", Message: "fix: handle nil header",
			ChangedFiles: map[string]model.ChangeKind{"src/alpha.go": model.ChangeModified}},
		{SHA: "bbbbbbbbbb", ShortSHA: "bbbbbbb", Message: "feat: add beta module",
			ChangedFiles: map[string]model.ChangeKind{"src/beta.go": model.ChangeAdded}},
		{SHA: "cccccccccc", ShortSHA: "ccccccc", Message: "test: gamma coverage",
			ChangedFiles: map[string]model.ChangeKind{"src/gamma_test.go": model.ChangeModified}},
		{SHA: "dddddddddd", ShortSHA: "ddddddd", Message: "refactor: util helpers",
			ChangedFiles: map[string]model.ChangeKind{"internal/util.go": model.ChangeRenamed}},
	}
	comments := []*model.ReviewComment{
		{
			ID: 100, Path: "src/alpha.go", CommitID: "aaaaaaaaaa",
			Line: 2, User: "alice",
			CreatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local),
			Body:      "alpha note: needs rename",
		},
		{
			ID: 101, Path: "src/alpha.go", CommitID: "aaaaaaaaaa",
			Line: 2, InReplyTo: 100, User: "bob",
			CreatedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.Local),
			Body:      "agreed, will rename helper",
		},
		{
			ID: 102, Path: "src/alpha.go", CommitID: "aaaaaaaaaa",
			Line: 4, User: "carol",
			CreatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.Local),
			Body:      "different anchor, no match here",
		},
	}
	m.state.PR = &model.PR{
		Owner: "o", Repo: "r", Number: 1, HeadSHA: "head1234",
		Files: files, Commits: commits, Comments: comments,
	}
	m.state.SelectedFile = "src/alpha.go"
	m.state.DiffCache[diffKey("", "src/alpha.go")] = strings.Join([]string{
		"@@ -1,3 +1,5 @@",
		" line1",
		"+helper added",
		"+TODO finish",
		" tail",
		" final",
	}, "\n")
	m.paneWidthComments = 60
	m.paneWidthDiff = 80
	return m
}

// ---- gg / G ---------------------------------------------------------------

func TestFiles_ggGotoTop(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneFiles
	m.state.FilesCursor = 3
	m.state.SelectedFile = m.state.PR.Files[3].Path
	mm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = mm.(Model)
	if m.state.PendingPrefix != "g" {
		t.Fatalf("first g should set PendingPrefix; got %q", m.state.PendingPrefix)
	}
	if m.state.FilesCursor != 3 {
		t.Fatalf("first g must not move cursor; got %d", m.state.FilesCursor)
	}
	mm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = mm.(Model)
	if m.state.PendingPrefix != "" {
		t.Fatalf("second g should clear PendingPrefix; got %q", m.state.PendingPrefix)
	}
	if m.state.FilesCursor != 0 {
		t.Fatalf("gg should jump Files cursor to 0; got %d", m.state.FilesCursor)
	}
	if m.state.SelectedFile != m.state.PR.Files[0].Path {
		t.Fatalf("gg in Files should auto-select file[0]; SelectedFile=%q", m.state.SelectedFile)
	}
}

func TestFiles_GGotoBottom(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneFiles
	m.state.FilesCursor = 1
	mm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = mm.(Model)
	last := len(m.state.PR.Files) - 1
	if m.state.FilesCursor != last {
		t.Fatalf("G should jump Files cursor to last (%d); got %d", last, m.state.FilesCursor)
	}
	if m.state.SelectedFile != m.state.PR.Files[last].Path {
		t.Fatalf("G in Files should auto-select last file; SelectedFile=%q", m.state.SelectedFile)
	}
}

func TestCommits_ggGotoTop(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneCommits
	m.state.SelectedFile = ""
	m.state.CommitsCursor = 3
	mm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = mm.(Model)
	mm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = mm.(Model)
	if m.state.CommitsCursor != 0 {
		t.Fatalf("gg in Commits should jump to row 0 (All commits); got %d", m.state.CommitsCursor)
	}
	if m.state.SelectedRange.Kind != model.RangeWholePR {
		t.Fatalf("gg in Commits should select RangeWholePR; got %v", m.state.SelectedRange)
	}
}

func TestCommits_GGotoBottom(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneCommits
	m.state.SelectedFile = ""
	m.state.CommitsCursor = 0
	mm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = mm.(Model)
	commits := m.visibleCommits()
	if m.state.CommitsCursor != len(commits) {
		t.Fatalf("G in Commits should jump to last commit row (%d); got %d", len(commits), m.state.CommitsCursor)
	}
}

func TestDiff_GGotoBottom(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneDiff
	m.paneHeightDiff = 10
	mm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = mm.(Model)
	last := len(m.patchLines()) - 1
	if m.state.DiffCursor.Line != last {
		t.Fatalf("G in Diff should jump cursor to last buffer line (%d); got %d", last, m.state.DiffCursor.Line)
	}
}

func TestComments_ggGotoTop(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneComments
	m.state.DiffCursor.Line = 2 // anchored thread (alpha note + reply)
	flat := m.flatComments()
	if len(flat) < 2 {
		t.Fatalf("expected anchored thread to expose >=2 comments; got %d", len(flat))
	}
	m.state.CommentsCursor = 1
	mm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = mm.(Model)
	mm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = mm.(Model)
	if m.state.CommentsCursor != 0 {
		t.Fatalf("gg in Comments should jump to row 0; got %d", m.state.CommentsCursor)
	}
}

func TestPendingPrefix_ClearedOnTab(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneFiles
	m.state.PendingPrefix = "g"
	mm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(Model)
	if m.state.PendingPrefix != "" {
		t.Fatalf("Tab must clear PendingPrefix; got %q", m.state.PendingPrefix)
	}
}

// ---- / search -------------------------------------------------------------

func TestSearch_OpensOnSlashInFiles(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneFiles
	mm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = mm.(Model)
	if m.state.Search == nil {
		t.Fatalf("/ should start a Search session")
	}
	if m.state.Search.TargetPane != model.PaneFiles {
		t.Fatalf("Search.TargetPane should be the focused pane; got %v", m.state.Search.TargetPane)
	}
	if m.state.Search.Status != model.SearchEditing {
		t.Fatalf("Search should start in Editing; got %v", m.state.Search.Status)
	}
}

func TestSearch_IncrementalJumpsCursor_Files(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneFiles
	m.state.FilesCursor = 0
	mm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = mm.(Model)
	// Type "gamma" — should jump to src/gamma_test.go (idx 2).
	for _, r := range "gamma" {
		mm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = mm.(Model)
	}
	if m.state.FilesCursor != 2 {
		t.Fatalf("incremental search 'gamma' should land on file idx 2 (src/gamma_test.go); got %d", m.state.FilesCursor)
	}
	if m.state.SelectedFile != "src/gamma_test.go" {
		t.Fatalf("Files search should auto-select; SelectedFile=%q", m.state.SelectedFile)
	}
}

func TestSearch_EnterCommitsAndNCycles(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneFiles
	m.state.FilesCursor = 0
	mm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = mm.(Model)
	for _, r := range ".go" {
		mm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = mm.(Model)
	}
	mm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(Model)
	if m.state.Search == nil || m.state.Search.Status != model.SearchActive {
		t.Fatalf("Enter should commit Search to Active; got %+v", m.state.Search)
	}
	matches := m.state.Search.Matches
	if len(matches) < 3 {
		t.Fatalf("expected >=3 matches for '.go'; got %d (%+v)", len(matches), matches)
	}
	startIdx := m.state.Search.CursorIdx
	mm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = mm.(Model)
	if m.state.Search.CursorIdx == startIdx {
		t.Fatalf("n should advance Search.CursorIdx; stayed at %d", startIdx)
	}
	mm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	m = mm.(Model)
	if m.state.Search.CursorIdx != startIdx {
		t.Fatalf("N should rewind to previous match; got %d, want %d", m.state.Search.CursorIdx, startIdx)
	}
}

func TestSearch_EscRestoresCursor(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneFiles
	m.state.FilesCursor = 1
	prevSelected := m.state.SelectedFile
	mm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = mm.(Model)
	for _, r := range "gamma" {
		mm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = mm.(Model)
	}
	if m.state.FilesCursor == 1 {
		t.Fatalf("setup invariant: search should have moved cursor away from 1")
	}
	mm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(Model)
	if m.state.Search != nil {
		t.Fatalf("Esc should clear Search; got %+v", m.state.Search)
	}
	if m.state.FilesCursor != 1 {
		t.Fatalf("Esc should restore FilesCursor to 1; got %d", m.state.FilesCursor)
	}
	if m.state.SelectedFile != prevSelected {
		t.Fatalf("Esc should restore SelectedFile; got %q want %q", m.state.SelectedFile, prevSelected)
	}
}

func TestSearch_NoMatchCancelsAndNotices(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneFiles
	mm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = mm.(Model)
	for _, r := range "nope" {
		mm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = mm.(Model)
	}
	mm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(Model)
	if m.state.Search != nil {
		t.Fatalf("Enter on no-match query should cancel Search; got %+v", m.state.Search)
	}
	if !strings.Contains(m.state.Notice, "no match") {
		t.Fatalf("expected a 'no match' Notice; got %q", m.state.Notice)
	}
}

func TestSearch_SmartCase(t *testing.T) {
	m := searchModelFixture(t)
	// Lowercase query should match case-insensitively.
	if got := m.collectMatches(model.PaneFiles, "readme"); len(got) != 1 {
		t.Fatalf("lowercase 'readme' should fold-match README.md; got %d", len(got))
	}
	// Uppercase query should be case-sensitive.
	if got := m.collectMatches(model.PaneFiles, "README"); len(got) != 1 {
		t.Fatalf("'README' should match README.md; got %d", len(got))
	}
	if got := m.collectMatches(model.PaneFiles, "Readme"); len(got) != 0 {
		t.Fatalf("'Readme' (mixed case) should be case-sensitive miss; got %d", len(got))
	}
}

func TestSearch_BackspaceShrinksAndEmptyCancels(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneFiles
	mm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = mm.(Model)
	for _, r := range "ab" {
		mm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = mm.(Model)
	}
	mm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	m = mm.(Model)
	if m.state.Search == nil || m.state.Search.Query != "a" {
		t.Fatalf("Backspace should drop one rune; got %+v", m.state.Search)
	}
	mm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	m = mm.(Model)
	mm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	m = mm.(Model)
	if m.state.Search != nil {
		t.Fatalf("Backspace on empty query should cancel Search; got %+v", m.state.Search)
	}
}

func TestSearch_DiffMatchesBufferLine(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneDiff
	m.paneHeightDiff = 10
	mm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = mm.(Model)
	for _, r := range "TODO" {
		mm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = mm.(Model)
	}
	// "+TODO finish" lives at buffer index 3.
	if m.state.DiffCursor.Line != 3 {
		t.Fatalf("Diff search 'TODO' should land cursor on buffer line 3; got %d", m.state.DiffCursor.Line)
	}
}

// `/` in Comments is intentionally disabled until the modal-vs-flat UX
// is decided. Pressing `/` must be a silent no-op — no Search session
// starts, no prompt appears.
func TestSearch_CommentsPaneDisablesSlash(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneComments
	m.state.DiffCursor.Line = 2
	mm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = mm.(Model)
	if m.state.Search != nil {
		t.Fatalf("/ in Comments must NOT start a Search; got %+v", m.state.Search)
	}
}

// Tab while Search is Active terminates the session — moving focus
// implies the user has navigated to the row they wanted.
func TestSearch_TabClearsActive(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneFiles
	m.startSearch()
	m.state.Search.Query = ".go"
	m.recomputeSearch()
	m.commitSearch()
	if m.state.Search == nil || m.state.Search.Status != model.SearchActive {
		t.Fatalf("setup: Search should be Active; got %+v", m.state.Search)
	}
	mm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(Model)
	if m.state.Search != nil {
		t.Fatalf("Tab should clear Active Search; got %+v", m.state.Search)
	}
	if m.state.FocusedPane != model.PaneCommits {
		t.Fatalf("Tab should still advance focus to Commits after clearing search; got %v", m.state.FocusedPane)
	}
}

// Shift+Tab while Active also terminates Search.
func TestSearch_ShiftTabClearsActive(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneFiles
	m.startSearch()
	m.state.Search.Query = ".go"
	m.recomputeSearch()
	m.commitSearch()
	mm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = mm.(Model)
	if m.state.Search != nil {
		t.Fatalf("Shift+Tab should clear Active Search; got %+v", m.state.Search)
	}
}

// Ctrl+C while Active terminates Search instead of quitting.
func TestSearch_CtrlCClearsActive(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneFiles
	m.startSearch()
	m.state.Search.Query = ".go"
	m.recomputeSearch()
	m.commitSearch()
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Fatalf("Ctrl+C while Active must NOT issue tea.Quit; got cmd=%v", cmd)
	}
	// After clearing, Ctrl+C should fall through to quit on the next press.
}

// ---- highlight ------------------------------------------------------------

// highlightMatches is the substring colorizer used by Files / Commits
// renderers. Verifies that matches receive an SGR-styled span and
// non-matching parts pass through untouched.
func TestHighlight_WrapsMatchedSubstring(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	bg := lipgloss.Color("#574b00")
	got := highlightMatches("src/foo.go", "foo", true, bg)
	if got == "src/foo.go" {
		t.Errorf("expected SGR-styled output for a match; got the input verbatim: %q", got)
	}
	if !strings.Contains(got, "foo") {
		t.Errorf("highlight output must still contain the matched literal; got %q", got)
	}
	if !strings.Contains(got, "src/") || !strings.Contains(got, ".go") {
		t.Errorf("highlight output must preserve surrounding text; got %q", got)
	}
}

func TestHighlight_NoMatchReturnsUnchanged(t *testing.T) {
	bg := lipgloss.Color("#574b00")
	if got := highlightMatches("src/foo.go", "bar", true, bg); got != "src/foo.go" {
		t.Errorf("no-match input should be returned verbatim; got %q", got)
	}
}

// FilesViewHighlightsMatchedPath pins that the Files renderer wraps the
// matched substring with the search-bg SGR span. Color profile is left
// at TrueColor (NewModel default) so the SGR code is emitted.
func TestFilesView_HighlightsMatchedPath(t *testing.T) {
	m := searchModelFixture(t)
	// searchModelFixture pinned the profile to Ascii for stable text
	// assertions; re-enable TrueColor here so SGR escapes survive into
	// the rendered output for SGR-presence checks.
	lipgloss.SetColorProfile(termenv.TrueColor)
	m.state.FocusedPane = model.PaneFiles
	m.paneWidthFiles = 40
	m.startSearch()
	m.state.Search.Query = "gamma"
	m.recomputeSearch()
	got := m.filesView()
	// Plain string still appears; the bg-styled span wraps "gamma".
	if !strings.Contains(got, "gamma") {
		t.Fatalf("filesView should still render the matched path; got:\n%s", got)
	}
	// SGR escape introduced by the bg style must appear (TrueColor → CSI).
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("filesView with active search must emit SGR codes for the highlight band; got:\n%s", got)
	}
}

// statusBarShowsSearchPrompt pins that the status bar exposes the live
// query while Editing — tested via composeStatusBar's input rather than
// the full bar so the test stays robust against width / URL ladder
// changes.
func TestStatusBar_EditingShowsPrompt(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneFiles
	m.startSearch()
	m.state.Search.Query = "abc"
	got, suffix := m.statusBarContent()
	if got != "/abc_" {
		t.Errorf("Editing status bar should render '/<query>_'; got %q", got)
	}
	if suffix != "" {
		t.Errorf("Editing status bar should drop suffix; got %q", suffix)
	}
}

func TestStatusBar_ActiveShowsCount(t *testing.T) {
	m := searchModelFixture(t)
	m.state.FocusedPane = model.PaneFiles
	m.startSearch()
	m.state.Search.Query = ".go"
	m.recomputeSearch()
	m.commitSearch()
	got, _ := m.statusBarContent()
	if !strings.Contains(got, "n:next") || !strings.Contains(got, "/.go") {
		t.Errorf("Active status bar should expose n/N hint and the query; got %q", got)
	}
	if !strings.Contains(got, "[1/") {
		t.Errorf("Active status bar should expose [idx/count]; got %q", got)
	}
}
