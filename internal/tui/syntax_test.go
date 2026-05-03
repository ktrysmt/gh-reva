package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
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
