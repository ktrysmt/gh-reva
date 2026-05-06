package theme

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/lipgloss"
)

// fromChroma builds a Theme by extracting per-token colors from a chroma
// Style. Tokens that the style does not resolve fall back to the builtin
// dark palette so every Theme field is always populated. Style.Get walks
// parent tokens, so most "missing" tokens still return a synthesized
// color via Background / Text inheritance.
//
// canonicalName is the registry key the user requested. We use it for
// Theme.Name rather than s.Name so case differences (e.g. "rpgle" vs
// "RPGLE") round-trip cleanly.
func fromChroma(s *chroma.Style, canonicalName string) *Theme {
	fb := builtinDark()
	editorBg := s.Get(chroma.Background).Background
	pick := func(t chroma.TokenType, fallback lipgloss.Color) lipgloss.Color {
		c := s.Get(t).Colour
		if !c.IsSet() {
			return fallback
		}
		return lipgloss.Color(c.String())
	}
	pickBrighten := func(t chroma.TokenType, factor float64, fallback lipgloss.Color) lipgloss.Color {
		c := s.Get(t).Colour
		if !c.IsSet() {
			return fallback
		}
		return lipgloss.Color(c.Brighten(factor).String())
	}
	// pickBgBrighten reads the entry's `.Background` field — the actual
	// editor bg in `<entry style="bg:#…"/>`. Used for row highlights
	// (VisualRangeBg) so they sit a few shades above the editor base
	// instead of next to the editor TEXT color, which would collide with
	// CursorRow / DiffContext and make the cursor `>` glyph invisible.
	pickBgBrighten := func(t chroma.TokenType, factor float64, fallback lipgloss.Color) lipgloss.Color {
		bg := s.Get(t).Background
		if !bg.IsSet() {
			return fallback
		}
		return lipgloss.Color(bg.Brighten(factor).String())
	}
	// pickAccent handles chroma styles (notably the gruvbox family) that
	// store the bright accent on Background and the editor base on Colour
	// for diff-related tokens. When the entry's Colour matches the editor
	// background, fall through to Background instead — otherwise the
	// resulting Theme color collapses to near-black and renders invisibly.
	pickAccent := func(t chroma.TokenType, fallback lipgloss.Color) lipgloss.Color {
		e := s.Get(t)
		if e.Colour.IsSet() && (!editorBg.IsSet() || e.Colour != editorBg) {
			return lipgloss.Color(e.Colour.String())
		}
		if e.Background.IsSet() && (!editorBg.IsSet() || e.Background != editorBg) {
			return lipgloss.Color(e.Background.String())
		}
		if e.Colour.IsSet() {
			return lipgloss.Color(e.Colour.String())
		}
		return fallback
	}
	return &Theme{
		Name: canonicalName,

		PaneBorderActive:   pick(chroma.GenericStrong, fb.PaneBorderActive),
		PaneBorderInactive: pickBrighten(chroma.LineNumbers, -0.3, fb.PaneBorderInactive),
		PaneTitle:          pick(chroma.Text, fb.PaneTitle),
		PaneTitleActive:    pick(chroma.GenericStrong, fb.PaneTitleActive),

		// Diff +/- marker fg and the row-wide bg are theme-independent.
		// Marker fg is a saturated bright green / red so the +/- rune is
		// unambiguous against syntax-highlighted code; bg is a near-black
		// dark green / dark red so the change extent reads at a glance.
		// Per-theme derivation could collapse either to the editor base
		// for inverted-convention styles (gruvbox) or to a hue that does
		// not read as red/green (rose-pine).
		DiffPlus:       fb.DiffPlus,
		DiffMinus:      fb.DiffMinus,
		DiffPlusBg:     fb.DiffPlusBg,
		DiffMinusBg:    fb.DiffMinusBg,
		DiffContext:    pick(chroma.Text, fb.DiffContext),
		DiffHunkHeader: pick(chroma.GenericSubheading, fb.DiffHunkHeader),
		DiffFileHeader: pick(chroma.GenericHeading, fb.DiffFileHeader),
		DiffLineNumber: pick(chroma.LineNumbers, fb.DiffLineNumber),
		DiffSeparator:  pickBrighten(chroma.LineNumbers, -0.4, fb.DiffSeparator),

		SyntaxStyle: s,

		// Cursor "> " uses the same source as the active pane title so
		// the focus accent stays visually consistent across the UI.
		CursorRow:     pick(chroma.GenericStrong, fb.CursorRow),
		CommentAnchor: pick(chroma.GenericEmph, fb.CommentAnchor),
		VisualRangeBg: pickBgBrighten(chroma.Background, 0.15, fb.VisualRangeBg),

		StatusAdded:     pickAccent(chroma.GenericInserted, fb.StatusAdded),
		StatusModified:  pick(chroma.GenericSubheading, fb.StatusModified),
		StatusDeleted:   pickAccent(chroma.GenericDeleted, fb.StatusDeleted),
		StatusRenamed:   pick(chroma.GenericHeading, fb.StatusRenamed),
		CommitSHA:       pick(chroma.LineNumbers, fb.CommitSHA),
		CommentAuthor:   pick(chroma.GenericStrong, fb.CommentAuthor),
		CommentDate:     pickBrighten(chroma.Text, -0.4, fb.CommentDate),
		CommentOutdated: pick(chroma.GenericError, fb.CommentOutdated),
		CommentPending:  pick(chroma.GenericSubheading, fb.CommentPending),

		LoadingSpinner: pick(chroma.GenericStrong, fb.LoadingSpinner),
		ErrorText:      pick(chroma.GenericError, fb.ErrorText),

		LogoShade1: pick(chroma.GenericStrong, fb.LogoShade1),
		LogoShade2: pick(chroma.GenericSubheading, fb.LogoShade2),
		LogoShade3: pickBrighten(chroma.LineNumbers, -0.2, fb.LogoShade3),
	}
}
