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
	return &Theme{
		Name: canonicalName,

		PaneBorderActive:   pick(chroma.GenericStrong, fb.PaneBorderActive),
		PaneBorderInactive: pickBrighten(chroma.LineNumbers, -0.3, fb.PaneBorderInactive),
		PaneTitle:          pick(chroma.Text, fb.PaneTitle),
		PaneTitleActive:    pick(chroma.GenericStrong, fb.PaneTitleActive),

		DiffPlus: pick(chroma.GenericInserted, fb.DiffPlus),
		DiffMinus: pick(chroma.GenericDeleted, fb.DiffMinus),
		// "限りなく黒に近い" — pull each diff bg from a heavily-darkened
		// fg so the bg always reads as a near-black tint of the same hue.
		DiffPlusBg:     pickBrighten(chroma.GenericInserted, -0.85, fb.DiffPlusBg),
		DiffMinusBg:    pickBrighten(chroma.GenericDeleted, -0.85, fb.DiffMinusBg),
		DiffContext:    pick(chroma.Text, fb.DiffContext),
		DiffHunkHeader: pick(chroma.GenericSubheading, fb.DiffHunkHeader),
		DiffFileHeader: pick(chroma.GenericHeading, fb.DiffFileHeader),
		DiffLineNumber: pick(chroma.LineNumbers, fb.DiffLineNumber),
		DiffSeparator:  pickBrighten(chroma.LineNumbers, -0.4, fb.DiffSeparator),

		SyntaxStyle: s,

		CursorRow:     pick(chroma.GenericInserted, fb.CursorRow),
		CommentAnchor: pick(chroma.GenericEmph, fb.CommentAnchor),
		VisualRangeBg: pickBrighten(chroma.Background, 0.15, fb.VisualRangeBg),

		StatusAdded:     pick(chroma.GenericInserted, fb.StatusAdded),
		StatusModified:  pick(chroma.GenericSubheading, fb.StatusModified),
		StatusDeleted:   pick(chroma.GenericDeleted, fb.StatusDeleted),
		StatusRenamed:   pick(chroma.GenericHeading, fb.StatusRenamed),
		CommitSHA:       pick(chroma.LineNumbers, fb.CommitSHA),
		CommentAuthor:   pick(chroma.GenericStrong, fb.CommentAuthor),
		CommentDate:     pickBrighten(chroma.Text, -0.4, fb.CommentDate),
		CommentOutdated: pick(chroma.GenericError, fb.CommentOutdated),

		LoadingSpinner: pick(chroma.GenericStrong, fb.LoadingSpinner),
		ErrorText:      pick(chroma.GenericError, fb.ErrorText),
	}
}
