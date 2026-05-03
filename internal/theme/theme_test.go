package theme_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/ktrysmt/gh-rv/internal/theme"
)

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func TestResolveBuiltinDark(t *testing.T) {
	th, err := theme.Resolve("builtin-dark")
	if err != nil {
		t.Fatalf("Resolve(\"builtin-dark\"): %v", err)
	}
	if th == nil {
		t.Fatal("Resolve returned nil theme")
	}
	if th.Name != "builtin-dark" {
		t.Errorf("Name = %q, want builtin-dark", th.Name)
	}
	for name, c := range map[string]string{
		"DiffPlus":         string(th.DiffPlus),
		"DiffMinus":        string(th.DiffMinus),
		"PaneBorderActive": string(th.PaneBorderActive),
		"CursorRow":        string(th.CursorRow),
		"StatusAdded":      string(th.StatusAdded),
		"CommentAuthor":    string(th.CommentAuthor),
		"ErrorText":        string(th.ErrorText),
	} {
		if !hexColor.MatchString(c) {
			t.Errorf("%s = %q, want #rrggbb", name, c)
		}
	}
}

func TestResolveEmptyDefaultsToBuiltin(t *testing.T) {
	th, err := theme.Resolve("")
	if err != nil {
		t.Fatalf("Resolve(\"\"): %v", err)
	}
	if th == nil || th.Name != "builtin-dark" {
		t.Fatalf("got %+v, want Name=builtin-dark", th)
	}
}

func TestResolveUnknownErrors(t *testing.T) {
	_, err := theme.Resolve("does-not-exist-xyz")
	if err == nil {
		t.Fatal("expected error for unknown theme, got nil")
	}
	if !strings.Contains(err.Error(), "unknown theme") {
		t.Errorf("error message = %q, want it to contain 'unknown theme'", err.Error())
	}
}

func TestResolveChromaDracula(t *testing.T) {
	th, err := theme.Resolve("dracula")
	if err != nil {
		t.Fatalf("Resolve(\"dracula\"): %v", err)
	}
	if th.Name != "dracula" {
		t.Errorf("Name = %q, want dracula", th.Name)
	}
	// chroma's dracula style sets GenericInserted to #50fa7b and
	// GenericDeleted to #ff5555. Match case-insensitively to keep the test
	// resilient to minor case changes.
	if got := strings.ToLower(string(th.DiffPlus)); got != "#50fa7b" {
		t.Errorf("DiffPlus = %q, want #50fa7b", got)
	}
	if got := strings.ToLower(string(th.DiffMinus)); got != "#ff5555" {
		t.Errorf("DiffMinus = %q, want #ff5555", got)
	}
	// every field is a valid hex.
	checkAllHex(t, th)
}

func TestResolveChromaTokyoNight(t *testing.T) {
	// Smoke check on a popular modern theme: must resolve and produce hex.
	th, err := theme.Resolve("tokyonight-night")
	if err != nil {
		t.Fatalf("Resolve(\"tokyonight-night\"): %v", err)
	}
	if th.Name != "tokyonight-night" {
		t.Errorf("Name = %q", th.Name)
	}
	checkAllHex(t, th)
}

func TestListThemesIncludesBuiltinAndChroma(t *testing.T) {
	names := theme.ListThemes()
	if len(names) < 50 {
		t.Fatalf("ListThemes returned %d entries, expected ~75+", len(names))
	}
	if names[0] != "builtin-dark" {
		t.Errorf("first entry = %q, want builtin-dark", names[0])
	}
	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
	}
	for _, want := range []string{"builtin-dark", "dracula", "monokai", "github-dark", "nord", "solarized-dark"} {
		if !have[want] {
			t.Errorf("ListThemes missing %q", want)
		}
	}
}

func TestEveryListedThemeResolves(t *testing.T) {
	// Every name returned by ListThemes must Resolve without error and
	// produce a fully-populated palette. Guards against a chroma style that
	// happens to lack a token we read.
	for _, n := range theme.ListThemes() {
		th, err := theme.Resolve(n)
		if err != nil {
			t.Errorf("Resolve(%q): %v", n, err)
			continue
		}
		if th.Name != n {
			t.Errorf("Resolve(%q).Name = %q", n, th.Name)
		}
		checkAllHex(t, th)
		if th.SyntaxStyle == nil {
			t.Errorf("Resolve(%q): SyntaxStyle is nil", n)
		}
	}
}

func TestBuiltinDarkSyntaxStyleIsGitHubDark(t *testing.T) {
	// builtin-dark deliberately uses GitHub-dark's syntax palette because
	// the chrome colors are GitHub-inspired. A swap here changes the in-line
	// look of every diff under the default theme.
	th, err := theme.Resolve("builtin-dark")
	if err != nil {
		t.Fatalf("Resolve(builtin-dark): %v", err)
	}
	if th.SyntaxStyle == nil {
		t.Fatal("SyntaxStyle is nil")
	}
	if got := th.SyntaxStyle.Name; got != "github-dark" {
		t.Errorf("SyntaxStyle.Name = %q, want github-dark", got)
	}
}

func checkAllHex(t *testing.T, th *theme.Theme) {
	t.Helper()
	for name, c := range map[string]string{
		"PaneBorderActive":   string(th.PaneBorderActive),
		"PaneBorderInactive": string(th.PaneBorderInactive),
		"PaneTitle":          string(th.PaneTitle),
		"PaneTitleActive":    string(th.PaneTitleActive),
		"DiffPlus":           string(th.DiffPlus),
		"DiffMinus":          string(th.DiffMinus),
		"DiffPlusBg":         string(th.DiffPlusBg),
		"DiffMinusBg":        string(th.DiffMinusBg),
		"DiffContext":        string(th.DiffContext),
		"DiffHunkHeader":     string(th.DiffHunkHeader),
		"DiffFileHeader":     string(th.DiffFileHeader),
		"DiffLineNumber":     string(th.DiffLineNumber),
		"DiffSeparator":      string(th.DiffSeparator),
		"CursorRow":          string(th.CursorRow),
		"CommentAnchor":      string(th.CommentAnchor),
		"VisualRangeBg":      string(th.VisualRangeBg),
		"StatusAdded":        string(th.StatusAdded),
		"StatusModified":     string(th.StatusModified),
		"StatusDeleted":      string(th.StatusDeleted),
		"StatusRenamed":      string(th.StatusRenamed),
		"CommitSHA":          string(th.CommitSHA),
		"CommentAuthor":      string(th.CommentAuthor),
		"CommentDate":        string(th.CommentDate),
		"CommentOutdated":    string(th.CommentOutdated),
		"LoadingSpinner":     string(th.LoadingSpinner),
		"ErrorText":          string(th.ErrorText),
	} {
		if !hexColor.MatchString(c) {
			t.Errorf("%s in theme %q = %q, want #rrggbb", name, th.Name, c)
		}
	}
}
