package theme

import (
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

// builtinDark is gh-reva's default palette, used both as the "builtin-dark"
// theme and as the per-field fallback when a chroma style omits a token.
// Tuned for 24-bit dark terminals; legibility on 256-color hosts is
// acceptable (lipgloss downsamples automatically).
func builtinDark() *Theme {
	syntax := styles.Get("github-dark")
	return &Theme{
		Name: builtinDarkName,

		PaneBorderActive:   lipgloss.Color("#58a6ff"),
		PaneBorderInactive: lipgloss.Color("#30363d"),
		PaneTitle:          lipgloss.Color("#c9d1d9"),
		PaneTitleActive:    lipgloss.Color("#58a6ff"),

		DiffPlus:       lipgloss.Color("#3fb950"),
		DiffMinus:      lipgloss.Color("#f85149"),
		DiffPlusBg:     lipgloss.Color("#172319"),
		DiffMinusBg:    lipgloss.Color("#23171a"),
		DiffContext:    lipgloss.Color("#c9d1d9"),
		DiffHunkHeader: lipgloss.Color("#79c0ff"),
		DiffFileHeader: lipgloss.Color("#d2a8ff"),
		DiffLineNumber: lipgloss.Color("#6e7681"),
		DiffSeparator:  lipgloss.Color("#444c56"),

		SyntaxStyle: syntax,

		CursorRow:     lipgloss.Color("#56d364"),
		CommentAnchor: lipgloss.Color("#f0883e"),
		VisualRangeBg: lipgloss.Color("#1f2937"),
		// Muted dark yellow chosen to read on dark backgrounds without
		// overpowering syntax foreground colors. Mirrors GitHub web's
		// search highlight idiom.
		SearchMatchBg: lipgloss.Color("#574b00"),

		StatusAdded:     lipgloss.Color("#3fb950"),
		StatusModified:  lipgloss.Color("#d29922"),
		StatusDeleted:   lipgloss.Color("#f85149"),
		StatusRenamed:   lipgloss.Color("#d2a8ff"),
		CommitSHA:       lipgloss.Color("#8b949e"),
		CommentAuthor:   lipgloss.Color("#79c0ff"),
		CommentDate:     lipgloss.Color("#6e7681"),
		CommentOutdated: lipgloss.Color("#f85149"),
		CommentPending:  lipgloss.Color("#d29922"),

		LoadingSpinner: lipgloss.Color("#58a6ff"),
		ErrorText:      lipgloss.Color("#f85149"),

		// "Neon yellow" splash ramp (palette D4 from the visual gallery):
		// light-yellow highlight, near-neon yellow outline, bright pale-
		// yellow fill. Tuned so the SVG (full-saturation rects) and the
		// terminal splash (▓ at ~75% fill against bg) both read as a
		// luminous yellow glow.
		LogoShade1: lipgloss.Color("#ffffe0"),
		LogoShade2: lipgloss.Color("#ffeb3b"),
		LogoShade3: lipgloss.Color("#fff176"),
	}
}
