package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// TestContextCellRoutesThroughStyledDiffCell pins the contract that context
// rows in the Diff pane are syntax-highlighted via the same path as +/-
// rows. Before this change, context rows used a flat foreground (cheaper
// but visually inconsistent). The rowCache + syntaxCache pair makes
// per-token tokenization a one-shot cost per (lexer, bg, cell) tuple.
//
// `go test` runs without a TTY so lipgloss defaults to Ascii profile and
// strips SGR. Force TrueColor for the duration of the test so the
// path-difference is visible in the rendered string.
func TestContextCellRoutesThroughStyledDiffCell(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	m.state.SelectedFile = "test.go"
	cell := " func main() { return }"
	got := m.colorDiffCell(cell, ' ', false)
	want := m.styledDiffCell(cell, "")
	if got != want {
		t.Errorf("context cell should be syntax-highlighted:\n got  = %q\n want = %q", got, want)
	}
}

// TestDiffMarkerHasAccentFgAndBold pins the contract that the leading '+' /
// '-' rune of a diff cell is rendered with the theme's DiffPlus / DiffMinus
// foreground AND bold. Without this, the marker inherits the terminal
// default fg (off-white) and gets visually swallowed by syntax-highlighted
// code on the row, making +/- hard to spot at a glance.
func TestDiffMarkerHasAccentFgAndBold(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	m.state.SelectedFile = "test.go"

	// DiffPlus = #3fb950 -> SGR foreground "38;2;63;185;80"; bold = "1".
	plusOut := m.styledDiffCell("+x", m.theme.DiffPlusBg)
	if !strings.Contains(plusOut, "38;2;63;185;80") {
		t.Errorf("'+' marker missing DiffPlus fg SGR (38;2;63;185;80): %q", plusOut)
	}
	if !strings.Contains(plusOut, "\x1b[1") && !strings.Contains(plusOut, ";1m") {
		t.Errorf("'+' marker missing bold SGR: %q", plusOut)
	}

	// DiffMinus = #f85149 -> SGR foreground "38;2;248;81;73".
	minusOut := m.styledDiffCell("-x", m.theme.DiffMinusBg)
	if !strings.Contains(minusOut, "38;2;248;81;73") {
		t.Errorf("'-' marker missing DiffMinus fg SGR (38;2;248;81;73): %q", minusOut)
	}
	if !strings.Contains(minusOut, "\x1b[1") && !strings.Contains(minusOut, ";1m") {
		t.Errorf("'-' marker missing bold SGR: %q", minusOut)
	}

	// Context cells (leading space) must NOT pick up a bold marker — the
	// space inherits the surrounding bg/fg with no extra weight.
	ctxOut := m.styledDiffCell(" x", "")
	if strings.Contains(ctxOut, "38;2;63;185;80") || strings.Contains(ctxOut, "38;2;248;81;73") {
		t.Errorf("context cell unexpectedly picked up +/- marker color: %q", ctxOut)
	}
}

// TestStyledDiffCellNeverEmitsNewline pins the contract that styledDiffCell
// output is single-line. Chroma's line-oriented lexers (most notably the
// JavaScript / TSX lexers) auto-append a trailing `\n` to their input so
// regex anchors match, and that synthesized newline rides into one of the
// emitted Whitespace tokens. Without sanitization, the `\n` ends up inside
// a rendered diff cell — when the row is concatenated and printed, the
// newline breaks the cell across two terminal rows, fragmenting the split
// layout into the stripe pattern observed in PR #1's diff view.
//
// We test multiple lexer family / cell-content shapes because the auto-
// newline behavior is per-lexer; the JS family was the failing case but
// the contract should hold for every lexer the user can hit.
func TestStyledDiffCellNeverEmitsNewline(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	cases := []struct {
		file    string
		cell    string
		comment string
	}{
		{"test.mjs", "+ short" + strings.Repeat(" ", 23), "JS short content padded to 30"},
		{"test.mjs", "+" + strings.Repeat(" ", 29), "JS empty + only whitespace"},
		{"test.mjs", "+   await s.type('?')" + strings.Repeat(" ", 9), "JS realistic line padded"},
		{"test.go", "+ func main() { }" + strings.Repeat(" ", 13), "Go content padded"},
		{"test.py", "+ def hi(): pass" + strings.Repeat(" ", 14), "Python content padded"},
		{"test.tsx", "- const x = 1" + strings.Repeat(" ", 17), "TSX deletion padded"},
		{"test.txt", " plain context" + strings.Repeat(" ", 16), "plain text context"},
	}
	m := NewModel(nil, nil)
	for _, tc := range cases {
		m.state.SelectedFile = tc.file
		got := m.styledDiffCell(tc.cell, m.theme.DiffPlusBg)
		stripped := stripSGR(got)
		if strings.ContainsAny(stripped, "\n\r") {
			t.Errorf("%s: styledDiffCell output contained newline; stripped=%q",
				tc.comment, stripped)
		}
	}
}

// stripSGR is a minimal SGR-stripper used only by the test. It mirrors
// ansi.Strip's behaviour for the CSI sequences chroma + lipgloss emit but
// avoids pulling in the package just for one test.
func stripSGR(s string) string {
	var sb strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
			continue
		}
		sb.WriteByte(s[i])
		i++
	}
	return sb.String()
}

// TestUnifiedAdditionGetsBgAndSyntax pins the contract that in unified mode,
// '+' rows pass through styledDiffCell so they receive both a row-wide bg
// (DiffPlusBg) and per-token syntax highlighting — symmetric with how '-'
// rows already get DiffMinusBg + syntax. Previously the unified renderer
// always called colorDiffCell with isRight=false, which routed '+' lines to
// `return cell` (plain), so additions appeared with no bg and no syntax
// while deletions were correctly tinted red.
func TestUnifiedAdditionGetsBgAndSyntax(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	m.state.SelectedFile = "test.go"
	m.state.DiffViewMode = model.DiffViewUnified
	m.paneWidthDiff = 80

	// Use distinct idx values; rowCache is keyed on (mode, idx, halfW, commented)
	// and these two calls would otherwise alias on idx=0.
	plusRows := m.renderUnifiedBufferLine("+func main() { return }", 0, -1, false)
	if len(plusRows) == 0 {
		t.Fatalf("expected at least one row for '+' line")
	}
	minusRows := m.renderUnifiedBufferLine("-func main() { return }", 1, -1, false)
	if len(minusRows) == 0 {
		t.Fatalf("expected at least one row for '-' line")
	}

	// '-' is the known-good baseline: it must carry SGR sequences (syntax + bg).
	if !strings.Contains(minusRows[0], "\x1b[") {
		t.Fatalf("baseline '-' row missing SGR; styling pipeline is broken: %q", minusRows[0])
	}
	// The '+' row must also carry SGR — same styling depth as '-'.
	if !strings.Contains(plusRows[0], "\x1b[") {
		t.Errorf("unified '+' row missing SGR (no bg / no syntax): %q", plusRows[0])
	}
}
