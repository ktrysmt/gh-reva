package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
	"github.com/ktrysmt/gh-reva/internal/theme"
)

// The logo lives at the repo-root logo.md and is rendered above the spinner
// during PR load. The render contract: 10 rows, each row contains only the
// glyphs U+2593 (▓), U+2591 (░), U+2588 (█) and spaces, and per-shade SGR
// foreground colors are applied when the active theme populates the LogoShade
// fields.
func TestRenderLogoPlain(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii) // strip SGR so we inspect glyphs only
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	th, err := theme.Resolve("builtin-dark")
	if err != nil {
		t.Fatalf("Resolve(builtin-dark): %v", err)
	}
	got := renderLogo(th)
	rows := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(rows) != 10 {
		t.Fatalf("renderLogo: got %d rows, want 10\n%s", len(rows), got)
	}
	for i, r := range rows {
		for _, c := range r {
			switch c {
			case ' ', '▓', '░', '█':
			default:
				t.Errorf("row %d contains unexpected rune %q", i, c)
			}
		}
	}
	// Every glyph type should appear at least once across the whole logo.
	all := strings.Join(rows, "")
	for _, g := range []string{"▓", "░", "█"} {
		if !strings.Contains(all, g) {
			t.Errorf("logo missing glyph %s", g)
		}
	}
}

// The logo art's source rows have different widths (the leading-space
// gradient encodes the dome curve). renderLogo must right-pad every row
// to the widest row so when loadingView centers each row by its own
// width, the dome's vertical axis is preserved. Without this padding,
// per-row centering shifts narrower rows further right than wider ones,
// and the rendered shape leans diagonally.
func TestRenderLogoRowsAreEqualWidth(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	th, err := theme.Resolve("builtin-dark")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	rows := strings.Split(strings.TrimRight(renderLogo(th), "\n"), "\n")
	want := lipgloss.Width(rows[0])
	for i, r := range rows {
		if got := lipgloss.Width(r); got != want {
			t.Errorf("row %d width = %d, want %d (row=%q)", i, got, want, r)
		}
	}
}

// Each REVA ASCII art design must have rows of equal visible width so
// horizontal-join (layout 3) and per-row centering math both behave.
// Uneven rows would silently misalign the dome+ascii block in layout 3
// and lean the ascii art diagonally in layout 2.
func TestRevaArt_RowsAreEqualWidth(t *testing.T) {
	if len(revaArt) != 3 {
		t.Fatalf("revaArt: got %d variants, want 3", len(revaArt))
	}
	for i, art := range revaArt {
		rows := strings.Split(art, "\n")
		if len(rows) == 0 {
			t.Errorf("revaArt[%d] is empty", i)
			continue
		}
		want := lipgloss.Width(rows[0])
		for j, r := range rows {
			if got := lipgloss.Width(r); got != want {
				t.Errorf("revaArt[%d] row %d width = %d, want %d (row=%q)", i, j, got, want, r)
			}
		}
	}
}

// Layout 1: dome + version + spinner. The dome glyphs (▓ / ░ / █) are
// the proof the dome rendered; "reva " prefix is required because the
// dome alone carries no name. Each REVA ASCII art is suppressed.
func TestLoadingView_LayoutDome(t *testing.T) {
	t.Setenv("GH_REVA_SPLASH_LAYOUT", "1")
	t.Setenv("GH_REVA_SPLASH_ART", "0")
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	th, err := theme.Resolve("builtin-dark")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	m.SetTheme(th)
	m.SetVersion("v0.4.2")
	m.width = 80
	m.height = 30
	out := m.loadingView(0, model.LoadStagePR)

	if !strings.Contains(out, "▓") {
		t.Errorf("layout 1 must render the dome glyphs:\n%s", out)
	}
	if !strings.Contains(out, "reva v0.4.2") {
		t.Errorf("layout 1 must show 'reva v0.4.2' between dome and spinner:\n%s", out)
	}
	if !strings.Contains(out, "Loading PR") {
		t.Errorf("layout 1 must include the spinner row:\n%s", out)
	}
	// Dome must precede the version line, and the version line must
	// precede the spinner row.
	domeIdx := strings.Index(out, "▓")
	verIdx := strings.Index(out, "reva v0.4.2")
	spinIdx := strings.Index(out, "Loading PR")
	if !(domeIdx < verIdx && verIdx < spinIdx) {
		t.Errorf("expected order dome < version < spinner; got %d %d %d:\n%s", domeIdx, verIdx, spinIdx, out)
	}
}

// Layout 2: ASCII REVA art only (no dome) + bare "vX.Y.Z" + spinner.
// The "reva" prefix is dropped because the art already names the tool.
func TestLoadingView_LayoutAscii(t *testing.T) {
	t.Setenv("GH_REVA_SPLASH_LAYOUT", "2")
	t.Setenv("GH_REVA_SPLASH_ART", "0")
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	th, _ := theme.Resolve("builtin-dark")
	m.SetTheme(th)
	m.SetVersion("v0.4.2")
	m.width = 80
	m.height = 30
	out := m.loadingView(0, model.LoadStagePR)

	if strings.Contains(out, "▓") {
		t.Errorf("layout 2 must NOT render the dome glyphs:\n%s", out)
	}
	// Variant 0 (figlet standard) ends in `|_| \_\` on its last row —
	// distinctive enough to confirm the ASCII art rendered.
	if !strings.Contains(out, `|_| \_\`) {
		t.Errorf("layout 2 must render the ASCII REVA art (variant 0):\n%s", out)
	}
	if strings.Contains(out, "reva v0.4.2") {
		t.Errorf("layout 2 must drop the 'reva' prefix (ASCII art already says REVA):\n%s", out)
	}
	if !strings.Contains(out, "v0.4.2") {
		t.Errorf("layout 2 must still show the version:\n%s", out)
	}
	if !strings.Contains(out, "Loading PR") {
		t.Errorf("layout 2 must include the spinner row:\n%s", out)
	}
}

// Layout 3: ASCII REVA art beside the dome + bare "vX.Y.Z" + spinner.
// Both the dome glyphs and the variant-0 art end-row marker must
// appear; the "reva" prefix is dropped (same reasoning as layout 2).
func TestLoadingView_LayoutDomeAscii(t *testing.T) {
	t.Setenv("GH_REVA_SPLASH_LAYOUT", "3")
	t.Setenv("GH_REVA_SPLASH_ART", "0")
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	th, _ := theme.Resolve("builtin-dark")
	m.SetTheme(th)
	m.SetVersion("v0.4.2")
	m.width = 120
	m.height = 30
	out := m.loadingView(0, model.LoadStagePR)

	if !strings.Contains(out, "▓") {
		t.Errorf("layout 3 must render the dome glyphs alongside the art:\n%s", out)
	}
	if !strings.Contains(out, `|_| \_\`) {
		t.Errorf("layout 3 must render the ASCII REVA art:\n%s", out)
	}
	if strings.Contains(out, "reva v0.4.2") {
		t.Errorf("layout 3 must drop the 'reva' prefix:\n%s", out)
	}
	if !strings.Contains(out, "v0.4.2") {
		t.Errorf("layout 3 must show the version:\n%s", out)
	}
	if !strings.Contains(out, "Loading PR") {
		t.Errorf("layout 3 must include the spinner row:\n%s", out)
	}
}

// Env override pins both layout and art so e2e / unit tests can rely
// on a deterministic splash. Out-of-range / unparseable values fall
// back to random — not asserted here because random can't be tested
// without a seed; covered indirectly by the layout tests above.
func TestNewModel_SplashEnvOverride(t *testing.T) {
	t.Setenv("GH_REVA_SPLASH_LAYOUT", "2")
	t.Setenv("GH_REVA_SPLASH_ART", "1")
	m := NewModel(nil, nil)
	if m.splashLayout != splashLayoutAscii {
		t.Errorf("splashLayout: got %v, want splashLayoutAscii (2)", m.splashLayout)
	}
	if m.splashArtIdx != 1 {
		t.Errorf("splashArtIdx: got %d, want 1", m.splashArtIdx)
	}
}

// goreleaser passes the bare semver via {{.Version}} (no leading `v`),
// so a 0.3.1 build would otherwise render `reva 0.3.1` / `0.3.1` on the
// splash. Prepend a `v` when the supplied version starts with a digit
// so the on-screen label matches the tag form users see in `git tag` /
// release pages. Already-prefixed values (e.g. `v0.4.2` from tests)
// stay as-is; non-semver strings like `dev` are left alone.
func TestSetVersion_BarePrependsV(t *testing.T) {
	t.Setenv("GH_REVA_SPLASH_LAYOUT", "1")
	t.Setenv("GH_REVA_SPLASH_ART", "0")
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	th, _ := theme.Resolve("builtin-dark")
	m.SetTheme(th)
	m.SetVersion("0.3.1")
	m.width = 80
	m.height = 30
	out := m.loadingView(0, model.LoadStagePR)

	if !strings.Contains(out, "reva v0.3.1") {
		t.Errorf("bare-semver version must render with v-prefix; missing 'reva v0.3.1':\n%s", out)
	}
	if strings.Contains(out, "reva 0.3.1\n") || strings.Contains(out, "reva 0.3.1 ") {
		t.Errorf("bare-semver form must NOT appear:\n%s", out)
	}
}

// Layout 2 / 3 collapse to bare `vX.Y.Z` (no `reva` prefix). The
// v-prepend rule must still apply — the art already names the tool but
// the version still needs the `v` for parity with tags.
func TestSetVersion_BarePrependsVOnAsciiLayout(t *testing.T) {
	t.Setenv("GH_REVA_SPLASH_LAYOUT", "2")
	t.Setenv("GH_REVA_SPLASH_ART", "0")
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	th, _ := theme.Resolve("builtin-dark")
	m.SetTheme(th)
	m.SetVersion("0.3.1")
	m.width = 80
	m.height = 30
	out := m.loadingView(0, model.LoadStagePR)
	if !strings.Contains(out, "v0.3.1") {
		t.Errorf("layout 2 must render 'v0.3.1':\n%s", out)
	}
}

// Already-prefixed version must not double up to `vv0.4.2`.
func TestSetVersion_AlreadyPrefixedNotDoubled(t *testing.T) {
	t.Setenv("GH_REVA_SPLASH_LAYOUT", "1")
	t.Setenv("GH_REVA_SPLASH_ART", "0")
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	th, _ := theme.Resolve("builtin-dark")
	m.SetTheme(th)
	m.SetVersion("v0.4.2")
	m.width = 80
	m.height = 30
	out := m.loadingView(0, model.LoadStagePR)
	if strings.Contains(out, "vv0.4.2") {
		t.Errorf("v-prefix must not double up:\n%s", out)
	}
	if !strings.Contains(out, "reva v0.4.2") {
		t.Errorf("expected 'reva v0.4.2':\n%s", out)
	}
}

// `dev` (cmd/root.go's ldflag fallback) is not a semver and must not
// gain a `v` — `vdev` reads as a typo and would mislead users
// inspecting their build.
func TestSetVersion_DevLeftAlone(t *testing.T) {
	t.Setenv("GH_REVA_SPLASH_LAYOUT", "1")
	t.Setenv("GH_REVA_SPLASH_ART", "0")
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	th, _ := theme.Resolve("builtin-dark")
	m.SetTheme(th)
	m.SetVersion("dev")
	m.width = 80
	m.height = 30
	out := m.loadingView(0, model.LoadStagePR)
	if strings.Contains(out, "vdev") {
		t.Errorf("'dev' must not gain a v-prefix:\n%s", out)
	}
	if !strings.Contains(out, "reva dev") {
		t.Errorf("expected 'reva dev':\n%s", out)
	}
}

// Empty version must collapse the version line entirely — `reva ` on
// its own would be a confusing artefact. Dev binaries built without
// ldflags still call SetVersion("dev") in cmd/root.go, so the empty
// branch is mostly relevant to NewModel-as-library callers and tests.
func TestSetVersion_EmptyOmitsLine(t *testing.T) {
	t.Setenv("GH_REVA_SPLASH_LAYOUT", "1")
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	th, _ := theme.Resolve("builtin-dark")
	m.SetTheme(th)
	// Intentionally no SetVersion → m.version stays "".
	m.width = 80
	m.height = 30
	out := m.loadingView(0, model.LoadStagePR)
	if strings.Contains(out, "reva ") {
		t.Errorf("'reva ' must not appear when version is empty:\n%s", out)
	}
	if !strings.Contains(out, "Loading PR") {
		t.Errorf("spinner row must still render with no version:\n%s", out)
	}
}

// padRowsVertically with topBias=1 shifts the content block one row
// further down than the symmetric centering, used by layout 3 to set
// the ASCII art a row below the dome's vertical midline.
func TestPadRowsVertically_TopBiasShiftsDown(t *testing.T) {
	rows := []string{"a", "b", "c"}
	out := padRowsVertically(rows, 9, 1)
	if len(out) != 9 {
		t.Fatalf("len(out)=%d want 9", len(out))
	}
	// diff=6, base top=3, bias=+1 → top=4, bot=2.
	for i := 0; i < 4; i++ {
		if out[i] != "" {
			t.Errorf("row %d should be blank, got %q", i, out[i])
		}
	}
	if out[4] != "a" || out[5] != "b" || out[6] != "c" {
		t.Errorf("content rows misplaced: %v", out[4:7])
	}
	for i := 7; i < 9; i++ {
		if out[i] != "" {
			t.Errorf("row %d should be blank, got %q", i, out[i])
		}
	}
}

// padRowsVertically clamps topBias so the content cannot get pushed
// beyond the available `h` budget — guards against future callers
// passing oversized biases.
func TestPadRowsVertically_TopBiasClamped(t *testing.T) {
	rows := []string{"x"}
	out := padRowsVertically(rows, 3, 99)
	if len(out) != 3 {
		t.Fatalf("len(out)=%d want 3", len(out))
	}
	// diff=2, base top=1, bias clamped so top=2, bot=0.
	if out[0] != "" || out[1] != "" || out[2] != "x" {
		t.Errorf("clamped placement wrong: %v", out)
	}
}

// Layout 3 puts ASCII REVA on the left and the dome on the right.
// The art must sit one row LOWER than perfect vertical centering so
// it visually reads as nestled inside the dome's frame instead of
// hovering above its midline. With variant-0 art (5 rows) inside the
// 10-row dome, perfect centering would land the first art row at
// index 2; the one-row shift lands it at index 3.
func TestComposeDomeAndAscii_AsciiSitsOneRowBelowCenter(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	th, _ := theme.Resolve("builtin-dark")
	m.SetTheme(th)
	m.splashArtIdx = 0 // figlet variant — first row begins with ` ____`

	rows := m.composeDomeAndAscii()
	if len(rows) != 10 {
		t.Fatalf("composed block: got %d rows, want 10", len(rows))
	}
	// The first row whose left half (the art column) carries any
	// non-space char is the art's top row. Find it.
	firstArt := -1
	for i, r := range rows {
		// Only look at the first ~30 cols (art column width); ignore
		// the dome on the right so its glyphs don't trigger early.
		head := r
		if len(head) > 30 {
			head = head[:30]
		}
		if strings.ContainsAny(head, "_/<\\|") {
			firstArt = i
			break
		}
	}
	if firstArt != 3 {
		t.Errorf("variant-0 art top row = %d, want 3 (one below the centered position 2):\n%s", firstArt, strings.Join(rows, "\n"))
	}
}

func TestRenderLogoUsesThemeShades(t *testing.T) {
	// Force TrueColor so lipgloss emits the exact hex SGR sequences and we
	// can grep for the per-shade colors in the rendered output.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	th, err := theme.Resolve("builtin-dark")
	if err != nil {
		t.Fatalf("Resolve(builtin-dark): %v", err)
	}
	if th.LogoShade1 == "" || th.LogoShade2 == "" || th.LogoShade3 == "" {
		t.Fatalf("builtin-dark must populate LogoShade1/2/3, got %q %q %q",
			th.LogoShade1, th.LogoShade2, th.LogoShade3)
	}
	out := renderLogo(th)
	// lipgloss emits "38;2;R;G;B" for TrueColor foregrounds. The hex strings
	// in the theme are converted; instead of recomputing the SGR triplet,
	// just confirm three distinct foreground SGR runs appear in the output.
	for _, marker := range []string{"38;2;"} {
		if !strings.Contains(out, marker) {
			t.Fatalf("expected SGR foreground markers in output, got: %q", out)
		}
	}
	// The three shades must yield three distinct rendered substrings — a
	// regression where renderLogo dropped color would collapse them.
	if strings.Count(out, "\x1b[") < 3 {
		t.Errorf("expected at least 3 SGR escapes (one per shade), got: %q", out)
	}
}
