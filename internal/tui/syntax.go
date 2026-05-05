package tui

import (
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/lipgloss"
)

// syntaxCache memoizes styledDiffCell results so the chroma tokenizer runs
// at most once per (lexer, bg, cell-content) tuple. Diff renders happen on
// every keypress; without the cache, viewing a 100-line file at 30 rows
// would re-tokenize 30 lines per frame and tuistory's startup window would
// race the 5s waitReady deadline. The map is shared across render passes
// and key composition embeds the cache discriminators inline so concurrent
// reads stay lock-light.
type syntaxCache struct {
	m sync.Map // key string -> styled string
}

// currentLexer picks a chroma lexer for the currently selected file.
// lexers.Match dispatches on extension / glob; if nothing fits we use the
// fallback (plaintext) so token output is just one Text token per line.
func (m Model) currentLexer() chroma.Lexer {
	if m.state == nil || m.state.SelectedFile == "" {
		return lexers.Fallback
	}
	if lex := lexers.Match(m.state.SelectedFile); lex != nil {
		return lex
	}
	return lexers.Fallback
}

// styledDiffCell renders a diff cell with a row-wide background color and
// per-token foreground coloring sourced from the theme's chroma syntax
// style. The cell's leading character is the diff marker (`+`, `-`, or
// space, including the wrap-continuation alignment space) and is excluded
// from lexer input — it would parse as a syntax error in most languages.
//
// The leading `+` / `-` rune itself is rendered in bold with the theme's
// DiffPlus / DiffMinus foreground (uniform bright green / red across
// themes) so the marker reads at a glance against syntax-highlighted code.
//
// When bg is the zero value the marker keeps the terminal default
// background and the rest is syntax-highlighted on the default bg
// (used by context lines).
func (m Model) styledDiffCell(cell string, bg lipgloss.Color) string {
	if cell == "" {
		return cell
	}
	lexer := m.currentLexer()
	style := m.theme.SyntaxStyle
	styleName := ""
	if style != nil {
		styleName = style.Name
	}
	// Embed the lexer name and bg in the key so cache entries do not bleed
	// across files of different languages or +/- vs context contexts.
	// markerPlus / markerMinus are theme-uniform constants so they do not
	// participate in the cache key.
	key := lexer.Config().Name + "\x00" + styleName + "\x00" + string(bg) + "\x00" + cell
	if v, ok := m.syntaxCache.m.Load(key); ok {
		return v.(string)
	}
	out := tokenizeAndStyle(cell, bg, m.theme.DiffPlus, m.theme.DiffMinus, lexer, style)
	m.syntaxCache.m.Store(key, out)
	return out
}

func tokenizeAndStyle(cell string, bg, markerPlus, markerMinus lipgloss.Color, lexer chroma.Lexer, style *chroma.Style) string {
	runes := []rune(cell)
	marker := string(runes[0])
	content := string(runes[1:])

	bgStyle := lipgloss.NewStyle()
	if bg != "" {
		bgStyle = bgStyle.Background(bg)
	}

	markerStyle := bgStyle
	switch marker {
	case "+":
		markerStyle = markerStyle.Foreground(markerPlus).Bold(true)
	case "-":
		markerStyle = markerStyle.Foreground(markerMinus).Bold(true)
	}

	var sb strings.Builder
	sb.WriteString(markerStyle.Render(marker))

	if style == nil {
		sb.WriteString(bgStyle.Render(content))
		return sb.String()
	}

	iter, err := lexer.Tokenise(nil, content)
	if err != nil {
		sb.WriteString(bgStyle.Render(content))
		return sb.String()
	}
	for tok := iter(); tok != chroma.EOF; tok = iter() {
		// Chroma's line-oriented lexers (e.g. JavaScript) auto-append a
		// trailing newline to the input so their regex anchors match, and
		// the synthesized newline shows up inside a Whitespace / Text
		// token. Letting that `\n` through breaks our diff cell across two
		// terminal rows — the next half-cell ends up on the row below,
		// producing the stripe pattern observed in PR #1's diff view.
		// We render one cell at a time, so any `\n` / `\r` in token
		// values is by definition spurious; strip them before rendering.
		val := tok.Value
		if strings.ContainsAny(val, "\n\r") {
			val = strings.NewReplacer("\n", "", "\r", "").Replace(val)
		}
		if val == "" {
			continue
		}
		seg := bgStyle
		entry := style.Get(tok.Type)
		if entry.Colour.IsSet() {
			seg = seg.Foreground(lipgloss.Color(entry.Colour.String()))
		}
		if entry.Bold == chroma.Yes {
			seg = seg.Bold(true)
		}
		if entry.Italic == chroma.Yes {
			seg = seg.Italic(true)
		}
		sb.WriteString(seg.Render(val))
	}
	return sb.String()
}
