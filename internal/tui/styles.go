package tui

import (
	"strings"
	"unicode/utf8"

	"github.com/rivo/uniseg"

	"github.com/ktrysmt/gh-reva/internal/model"
)

func paneTitle(label string, active bool, suffix string) string {
	prefix := "  "
	if active {
		prefix = "▶ "
	}
	if suffix != "" {
		return prefix + label + " " + suffix
	}
	return prefix + label
}

// styledPaneTitle is paneTitle plus theme coloring: active titles get the
// active-title color in bold; inactive titles get the inactive-title color.
// Width-aware truncation still goes through fitPaneTitle on the raw text so
// SGR codes never participate in length math.
func (m Model) styledPaneTitle(label string, active bool, suffix string) string {
	var raw string
	if w := paneInnerWidth(m, label); w > 0 {
		raw = fitPaneTitle(label, suffix, active, w)
	} else {
		raw = paneTitle(label, active, suffix)
	}
	prefix := "  "
	if active {
		prefix = "▶ "
	}
	// Re-apply color to the prefix and the rest separately. The prefix is
	// always exactly 2 cells, so a rune slice is safe.
	runes := []rune(raw)
	body := string(runes[2:])
	if active {
		return fgBold(prefix, m.theme.PaneTitleActive) + fgBold(body, m.theme.PaneTitleActive)
	}
	return prefix + fg(body, m.theme.PaneTitle)
}

// paneInnerWidth picks the right per-pane width budget for fitPaneTitle.
// Returns 0 when no budget has been computed yet (pre-first-frame), letting
// callers fall back to the un-fitted form. Matches by prefix so dynamic
// labels like "Diff: path @ sha" still pick the Diff budget.
func paneInnerWidth(m Model, label string) int {
	switch {
	case strings.HasPrefix(label, "Files"):
		return m.paneWidthFiles
	case strings.HasPrefix(label, "Commits"):
		return m.paneWidthCommits
	case strings.HasPrefix(label, "Diff"):
		return m.paneWidthDiff
	case strings.HasPrefix(label, "Comments"):
		return m.paneWidthComments
	}
	return 0
}

func cursorPrefix(isCursor bool) string {
	if isCursor {
		return "> "
	}
	return "  "
}

// styledCursor returns the row-prefix glyph (`> ` or `  `) with the cursor
// row color and bold weight applied when the row is the cursor or part of
// the visual range. Mirrors cursorMarker's logic but produces a colored
// string instead of plain text.
func (m Model) styledCursor(pane model.PaneID, idx, cursor int) string {
	if idx == cursor || m.inVisualRange(pane, idx) {
		return fgBold("> ", m.theme.CursorRow)
	}
	return "  "
}

func changeKindShort(k model.ChangeKind) string {
	switch k {
	case model.ChangeAdded:
		return "A"
	case model.ChangeModified:
		return "M"
	case model.ChangeDeleted:
		return "D"
	case model.ChangeRenamed:
		return "R"
	}
	return "?"
}

// styledStatus returns the single-letter change-kind glyph wrapped in the
// matching theme color. Used by Files and Commits panes for the per-row
// `[A]/[M]/[D]/[R]` annotations.
func (m Model) styledStatus(k model.ChangeKind) string {
	letter := changeKindShort(k)
	switch k {
	case model.ChangeAdded:
		return fg(letter, m.theme.StatusAdded)
	case model.ChangeModified:
		return fg(letter, m.theme.StatusModified)
	case model.ChangeDeleted:
		return fg(letter, m.theme.StatusDeleted)
	case model.ChangeRenamed:
		return fg(letter, m.theme.StatusRenamed)
	}
	return letter
}

func shortSHA(s string) string {
	if len(s) < 7 {
		return s
	}
	return s[:7]
}

func indent(level int) string {
	if level <= 0 {
		return ""
	}
	return strings.Repeat("  ", level)
}

// fitPaneTitle composes a pane title that fits in `width` columns while
// preserving the suffix (e.g. mode tag) at the right. When the full title
// would overflow, the label is shrunk with an ellipsis; the suffix stays
// visible so users can still see the view-mode marker on narrow terminals.
func fitPaneTitle(label, suffix string, active bool, width int) string {
	prefix := "  "
	if active {
		prefix = "▶ "
	}
	full := prefix + label
	if suffix != "" {
		full += " " + suffix
	}
	if width <= 0 || utf8.RuneCountInString(full) <= width {
		return full
	}
	if suffix != "" {
		// Try to keep the full suffix, shrink the label.
		reserve := utf8.RuneCountInString(prefix) + 1 + utf8.RuneCountInString(suffix)
		avail := width - reserve
		if avail >= 2 {
			runes := []rune(label)
			if len(runes) > avail {
				label = string(runes[:avail-1]) + "…"
			}
			return prefix + label + " " + suffix
		}
	}
	// Fall back to a hard truncate of the whole title.
	runes := []rune(full)
	if width >= 2 {
		return string(runes[:width-1]) + "…"
	}
	if width == 1 {
		return string(runes[:1])
	}
	return ""
}

// displayWidth returns the terminal-cell width of s using grapheme-cluster
// arithmetic. Substitute for runewidth.StringWidth — the bare runewidth
// table miscounts regional-indicator flag pairs and VS16 emoji (it sums
// each codepoint as 1 while every modern terminal renders the cluster as
// 2). Cluster-aware measurement matches what lipgloss.Width / x/ansi report
// on the same input, which is what padTrunc and the modal layout use, so
// wrapText decisions stay consistent with the row-padding stage.
func displayWidth(s string) int { return uniseg.StringWidth(s) }

// wrapText word-wraps text into lines of at most `width` display cells.
// Width is measured in terminal cells (CJK / emoji = 2), not rune count,
// so the result fits inside a width-`width` column on screen. Words longer
// than width are hard-broken on grapheme-cluster boundaries with cell
// accounting; a cluster wider than the remaining budget rolls to the next
// chunk. width <= 0 disables wrapping. Word boundaries (see `splitWrapWords`)
// only fire on whitespace whose both sides are ASCII word runes; an
// ASCII↔CJK or CJK↔CJK whitespace is preserved inside the running word so
// a short ASCII prefix can't be stranded on its own row when the following
// CJK segment exceeds the remaining budget. Grapheme-cluster iteration
// (instead of rune-by-rune) keeps ZWJ-joined emoji, regional-indicator
// flags, VS16 sequences, and skin-tone modifiers intact across breaks —
// the cluster ❤️ (U+2764 + VS16) stays as one unit, so a wrap never lands
// between the base and its variation selector.
func wrapText(text string, width int) []string {
	if width <= 0 || displayWidth(text) <= width {
		return []string{text}
	}
	words := splitWrapWords(text)
	if len(words) == 0 {
		return []string{text}
	}
	var out []string
	var line strings.Builder
	lineW := 0
	flush := func() {
		if line.Len() > 0 {
			out = append(out, line.String())
			line.Reset()
			lineW = 0
		}
	}
	// hardBreak splits a wide token into chunks each fitting within `width`
	// display cells, walking grapheme clusters so multi-codepoint emoji stay
	// together. Used when a single token exceeds the budget.
	hardBreak := func(s string) []string {
		var chunks []string
		var cur strings.Builder
		curW := 0
		g := uniseg.NewGraphemes(s)
		for g.Next() {
			cluster := g.Str()
			cw := g.Width()
			if cw == 0 {
				cur.WriteString(cluster)
				continue
			}
			if curW+cw > width && cur.Len() > 0 {
				chunks = append(chunks, cur.String())
				cur.Reset()
				curW = 0
			}
			cur.WriteString(cluster)
			curW += cw
		}
		if cur.Len() > 0 {
			chunks = append(chunks, cur.String())
		}
		return chunks
	}
	for _, w := range words {
		wW := displayWidth(w)
		if wW > width {
			flush()
			chunks := hardBreak(w)
			// Push all but the last chunk as their own rows; let the last
			// chunk become the head of the new accumulator so subsequent
			// short words can still pack into it.
			for i, c := range chunks {
				if i < len(chunks)-1 {
					out = append(out, c)
				} else {
					line.WriteString(c)
					lineW = displayWidth(c)
				}
			}
			continue
		}
		sep := 0
		if lineW > 0 {
			sep = 1
		}
		if lineW+sep+wW <= width {
			if sep == 1 {
				line.WriteByte(' ')
			}
			line.WriteString(w)
			lineW += sep + wW
		} else {
			flush()
			line.WriteString(w)
			lineW = wW
		}
	}
	flush()
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// splitWrapWords splits text into wrap-aware words. A run of whitespace
// is treated as a word boundary only when the rune immediately before
// the run AND the rune immediately after the run are both ASCII word
// runes (letters, digits, punctuation — anything ASCII that is not
// whitespace itself). Otherwise the whitespace is collapsed to a single
// space and kept inside the running word. This mirrors the typographic
// convention that CJK text doesn't use whitespace as a word boundary,
// so a short ASCII prefix like "slack" sitting beside a long CJK run
// ("slack コマンドの…") doesn't get stranded by the wrap algorithm
// when the CJK side overflows the column on its own.
func splitWrapWords(text string) []string {
	if text == "" {
		return nil
	}
	runes := []rune(text)
	var words []string
	var cur strings.Builder
	i := 0
	for i < len(runes) {
		r := runes[i]
		if r != ' ' && r != '\t' {
			cur.WriteRune(r)
			i++
			continue
		}
		// Consume the whole whitespace run.
		j := i
		for j < len(runes) && (runes[j] == ' ' || runes[j] == '\t') {
			j++
		}
		var prev, next rune
		if cur.Len() > 0 {
			for k := len(runes[:i]); k > 0; k-- {
				rp := runes[k-1]
				if rp != ' ' && rp != '\t' {
					prev = rp
					break
				}
			}
		}
		if j < len(runes) {
			next = runes[j]
		}
		if isASCIIWordRune(prev) && isASCIIWordRune(next) {
			if cur.Len() > 0 {
				words = append(words, cur.String())
				cur.Reset()
			}
		} else {
			if cur.Len() > 0 || j < len(runes) {
				cur.WriteByte(' ')
			}
		}
		i = j
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}
	return words
}

// isASCIIWordRune reports whether r is a non-whitespace ASCII rune that
// can act as a word-boundary anchor for `splitWrapWords`.
func isASCIIWordRune(r rune) bool {
	if r == 0 || r >= 128 {
		return false
	}
	switch r {
	case ' ', '\t':
		return false
	}
	return true
}
