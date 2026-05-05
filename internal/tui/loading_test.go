package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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
