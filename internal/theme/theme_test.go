package theme_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/ktrysmt/gh-reva/internal/theme"
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

func TestResolveEmptyDefaultsToGruvbox(t *testing.T) {
	th, err := theme.Resolve("")
	if err != nil {
		t.Fatalf("Resolve(\"\"): %v", err)
	}
	if th == nil || th.Name != "gruvbox" {
		t.Fatalf("got %+v, want Name=gruvbox", th)
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
	// PaneTitleActive maps to chroma.GenericStrong; for dracula that is the
	// bold foreground (#f8f8f2). Spot-check this branch of the chroma
	// adapter — DiffPlus/DiffMinus are now uniform across themes, so
	// asserting on the diff-marker fg is no longer a meaningful chroma
	// extraction signal.
	if got := strings.ToLower(string(th.PaneTitleActive)); got != "#f8f8f2" {
		t.Errorf("PaneTitleActive = %q, want #f8f8f2", got)
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

func TestCursorRowIsThemeAccent(t *testing.T) {
	// gruvbox's chroma style sets GenericInserted.Colour to the editor base
	// (#282828), not a bright accent. Deriving CursorRow from GenericInserted
	// makes the cursor invisible. We map CursorRow to GenericStrong (the
	// same source as PaneTitleActive) so it always lands on a visible accent.
	for _, tc := range []struct {
		name string
		want string
	}{
		{"gruvbox", "#ebdbb2"},
		{"dracula", "#f8f8f2"},
	} {
		th, err := theme.Resolve(tc.name)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", tc.name, err)
		}
		if got := strings.ToLower(string(th.CursorRow)); got != tc.want {
			t.Errorf("CursorRow[%s] = %q, want %q", tc.name, got, tc.want)
		}
		// Cursor and active title must share a source so the accent is
		// internally consistent.
		if string(th.CursorRow) != string(th.PaneTitleActive) {
			t.Errorf("CursorRow[%s] = %q, want == PaneTitleActive %q",
				tc.name, th.CursorRow, th.PaneTitleActive)
		}
	}
}

func TestDiffMarkerFgIsUniformBright(t *testing.T) {
	// The +/- marker rune at the start of every changed row uses the theme's
	// DiffPlus / DiffMinus foreground. To keep the marker unambiguous and
	// distinguishable from syntax-highlighted code regardless of which
	// chroma palette the user picks, the values are hard-coded to a
	// saturated bright green / bright red — same intent as the uniform
	// dark bg.
	const (
		wantPlus  = "#3fb950"
		wantMinus = "#f85149"
	)
	for _, name := range []string{"builtin-dark", "gruvbox", "dracula", "monokai", "solarized-dark"} {
		th, err := theme.Resolve(name)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", name, err)
		}
		if got := strings.ToLower(string(th.DiffPlus)); got != wantPlus {
			t.Errorf("DiffPlus[%s] = %q, want %q", name, got, wantPlus)
		}
		if got := strings.ToLower(string(th.DiffMinus)); got != wantMinus {
			t.Errorf("DiffMinus[%s] = %q, want %q", name, got, wantMinus)
		}
	}
}

func TestDiffBgIsUniformDark(t *testing.T) {
	// Diff add/del row backgrounds are theme-independent: dark green / dark
	// red so the +/- distinction is unambiguous regardless of which palette
	// the user picks. Per-theme derivation collapses to near-black for
	// styles that store the bright color on Background instead of Colour
	// (gruvbox), so we hard-code uniform values.
	const (
		wantPlus  = "#172319"
		wantMinus = "#23171a"
	)
	for _, name := range []string{"builtin-dark", "gruvbox", "dracula", "monokai", "solarized-dark"} {
		th, err := theme.Resolve(name)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", name, err)
		}
		if got := strings.ToLower(string(th.DiffPlusBg)); got != wantPlus {
			t.Errorf("DiffPlusBg[%s] = %q, want %q", name, got, wantPlus)
		}
		if got := strings.ToLower(string(th.DiffMinusBg)); got != wantMinus {
			t.Errorf("DiffMinusBg[%s] = %q, want %q", name, got, wantMinus)
		}
	}
}

func TestGruvboxStatusBadgesVisible(t *testing.T) {
	// Gruvbox's GenericInserted/Deleted Colour is the editor base (#282828);
	// the bright accent lives on Background. The chroma adapter must detect
	// this inversion and surface the accent so file-row [A]/[D] badges and
	// DiffPlus/Minus foregrounds remain visible.
	th, err := theme.Resolve("gruvbox")
	if err != nil {
		t.Fatalf("Resolve(gruvbox): %v", err)
	}
	for name, c := range map[string]string{
		"StatusAdded":   string(th.StatusAdded),
		"StatusDeleted": string(th.StatusDeleted),
		"DiffPlus":      string(th.DiffPlus),
		"DiffMinus":     string(th.DiffMinus),
	} {
		if strings.EqualFold(c, "#282828") {
			t.Errorf("%s = %q (editor base), want a bright accent", name, c)
		}
	}
	// Spot-check the resolved values match gruvbox's bright accents.
	if got := strings.ToLower(string(th.StatusAdded)); got != "#b8bb26" {
		t.Errorf("StatusAdded = %q, want #b8bb26", got)
	}
	if got := strings.ToLower(string(th.StatusDeleted)); got != "#fb4934" {
		t.Errorf("StatusDeleted = %q, want #fb4934", got)
	}
}

func TestVisualRangeBgUsesEditorBackground(t *testing.T) {
	// VisualRangeBg paints the row-wide highlight applied to every line in
	// the Diff visual selection. It must be derived from the chroma style's
	// editor BACKGROUND (the actual `bg:#…` value on the Background entry),
	// not from its foreground Colour. Otherwise the row bg lands within a
	// few RGB ticks of CursorRow / DiffContext / pane chrome (all of which
	// are derived from the editor TEXT color), making the cursor `>` glyph
	// and the row content essentially invisible against the highlight.
	//
	// gruvbox is the canonical break case: editor bg is #282828, text is
	// #ebdbb2, and the prior `pickBrighten(chroma.Background, 0.15)` read
	// the .Colour field instead of .Background, producing #eee0bd — a
	// pale-cream that erases the cursor.
	for _, name := range []string{
		"gruvbox", "dracula", "monokai", "nord", "tokyonight-night", "solarized-dark",
	} {
		th, err := theme.Resolve(name)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", name, err)
		}
		bgR, bgG, bgB, ok := parseHex(string(th.VisualRangeBg))
		if !ok {
			t.Errorf("VisualRangeBg[%s] = %q, not parseable", name, th.VisualRangeBg)
			continue
		}
		curR, curG, curB, _ := parseHex(string(th.CursorRow))
		// Channel-wise distance must be visible (>= 64 on at least one
		// channel). Anything closer leaves the cursor practically
		// indistinguishable from the row highlight.
		dist := absInt(bgR-curR) + absInt(bgG-curG) + absInt(bgB-curB)
		if dist < 64 {
			t.Errorf("VisualRangeBg[%s] = %s is too close to CursorRow %s (manhattan distance %d, want >= 64)",
				name, th.VisualRangeBg, th.CursorRow, dist)
		}
		// Sanity: a row highlight on a dark editor must itself be on the
		// dark side of the spectrum. brightness > 200 means we picked the
		// text color (or worse, near-white).
		brightness := (bgR + bgG + bgB) / 3
		if brightness > 200 {
			t.Errorf("VisualRangeBg[%s] = %s has avg brightness %d (>200) — looks like the editor text color, not the editor bg",
				name, th.VisualRangeBg, brightness)
		}
	}
}

func parseHex(s string) (r, g, b int, ok bool) {
	if len(s) != 7 || s[0] != '#' {
		return 0, 0, 0, false
	}
	for i, shift := range []int{1, 3, 5} {
		_ = i
		v := 0
		for _, c := range s[shift : shift+2] {
			v <<= 4
			switch {
			case c >= '0' && c <= '9':
				v |= int(c - '0')
			case c >= 'a' && c <= 'f':
				v |= int(c-'a') + 10
			case c >= 'A' && c <= 'F':
				v |= int(c-'A') + 10
			default:
				return 0, 0, 0, false
			}
		}
		switch shift {
		case 1:
			r = v
		case 3:
			g = v
		case 5:
			b = v
		}
	}
	return r, g, b, true
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
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
		"CommentPending":     string(th.CommentPending),
		"CommentResolved":    string(th.CommentResolved),
		"LoadingSpinner":     string(th.LoadingSpinner),
		"ErrorText":          string(th.ErrorText),
		"LogoShade1":         string(th.LogoShade1),
		"LogoShade2":         string(th.LogoShade2),
		"LogoShade3":         string(th.LogoShade3),
	} {
		if !hexColor.MatchString(c) {
			t.Errorf("%s in theme %q = %q, want #rrggbb", name, th.Name, c)
		}
	}
}
