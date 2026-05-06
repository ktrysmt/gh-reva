// Package theme defines the gh-reva color palette and how it is resolved from
// either the bundled builtin theme or a chroma styles registry entry. The
// package returns lipgloss.Color values; rendering is the caller's job.
package theme

import (
	"fmt"
	"sort"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

// Theme is the per-render-role color palette for gh-reva. All fields are
// lipgloss.Color (TrueColor hex strings or ANSI indices). Dark backgrounds
// are assumed.
type Theme struct {
	Name string

	PaneBorderActive   lipgloss.Color
	PaneBorderInactive lipgloss.Color
	PaneTitle          lipgloss.Color
	PaneTitleActive    lipgloss.Color

	DiffPlus       lipgloss.Color
	DiffMinus      lipgloss.Color
	DiffPlusBg     lipgloss.Color // near-black green; row bg for + lines
	DiffMinusBg    lipgloss.Color // near-black red; row bg for - lines
	DiffContext    lipgloss.Color
	DiffHunkHeader lipgloss.Color
	DiffFileHeader lipgloss.Color
	DiffLineNumber lipgloss.Color
	DiffSeparator  lipgloss.Color

	// SyntaxStyle drives per-token foreground coloring inside diff lines.
	// Always non-nil after Resolve. For builtin-dark we ship "github-dark"
	// as a sensible default; chroma themes use their own style.
	SyntaxStyle *chroma.Style

	CursorRow     lipgloss.Color
	CommentAnchor lipgloss.Color
	VisualRangeBg lipgloss.Color
	SearchMatchBg lipgloss.Color

	StatusAdded     lipgloss.Color
	StatusModified  lipgloss.Color
	StatusDeleted   lipgloss.Color
	StatusRenamed   lipgloss.Color
	CommitSHA       lipgloss.Color
	CommentAuthor   lipgloss.Color
	CommentDate     lipgloss.Color
	CommentOutdated lipgloss.Color
	CommentPending  lipgloss.Color

	LoadingSpinner lipgloss.Color
	ErrorText      lipgloss.Color

	// Logo shades drive the per-glyph coloring of the splash logo on the
	// loading screen. Shade1 = brightest (█), Shade2 = mid (▓),
	// Shade3 = dimmest (░). Themes that omit them fall back to builtin.
	LogoShade1 lipgloss.Color
	LogoShade2 lipgloss.Color
	LogoShade3 lipgloss.Color
}

const (
	builtinDarkName  = "builtin-dark"
	defaultThemeName = "gruvbox"
)

// Resolve returns the theme registered under name. An empty name resolves
// to defaultThemeName ("gruvbox"). Unknown names produce an error so the CLI
// can fail fast.
//
// Lookup order:
//  1. "" → Resolve(defaultThemeName)
//  2. "builtin-dark" → builtinDark()
//  3. chroma styles registry (74 themes) → fromChroma
//  4. otherwise → error
//
// Membership is checked against styles.Names() rather than styles.Get's
// fallback signal because chroma stores some styles under a registry key
// that differs in case from Style.Name (e.g. registry "rpgle" → Style.Name
// "RPGLE"). We canonicalize on the registry key so Theme.Name round-trips.
func Resolve(name string) (*Theme, error) {
	if name == "" {
		name = defaultThemeName
	}
	if name == builtinDarkName {
		return builtinDark(), nil
	}
	if !chromaHas(name) {
		return nil, fmt.Errorf("unknown theme: %s", name)
	}
	return fromChroma(styles.Get(name), name), nil
}

func chromaHas(name string) bool {
	for _, n := range styles.Names() {
		if n == name {
			return true
		}
	}
	return false
}

// ListThemes returns every theme name that Resolve will accept, with
// "builtin-dark" first followed by every chroma-registered style sorted
// alphabetically.
func ListThemes() []string {
	chromaNames := append([]string{}, styles.Names()...)
	sort.Strings(chromaNames)
	return append([]string{builtinDarkName}, chromaNames...)
}
