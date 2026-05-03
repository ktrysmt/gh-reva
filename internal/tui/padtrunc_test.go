package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestPadTruncASCIIExact(t *testing.T) {
	if got := padTrunc("hello", 5); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestPadTruncASCIIPad(t *testing.T) {
	if got := padTrunc("hi", 5); got != "hi   " {
		t.Errorf("got %q, want %q", got, "hi   ")
	}
}

func TestPadTruncASCIITruncate(t *testing.T) {
	if got := padTrunc("helloworld", 5); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestPadTruncSGRWithinWidth(t *testing.T) {
	// Colored content already at exact width must round-trip unchanged.
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Render("hi")
	got := padTrunc(red, 2)
	if w := lipgloss.Width(got); w != 2 {
		t.Errorf("width = %d, want 2; got=%q", w, got)
	}
}

func TestPadTruncSGROverflow(t *testing.T) {
	// Regression: a colored Commits row that is wider than the pane must
	// still come out at exactly innerW visible cells. Previously the SGR
	// branch returned the over-width string untouched and the right border
	// got pushed off-screen.
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Render("Include guessed host in telemetry")
	got := padTrunc("  [M] b0929b5 "+red, 40)
	if w := lipgloss.Width(got); w != 40 {
		t.Errorf("visible width = %d, want 40; got=%q", w, got)
	}
}

func TestPadTruncSGRPad(t *testing.T) {
	// Colored content shorter than width must be right-padded with plain
	// spaces, not by extending the SGR run.
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Render("hi")
	got := padTrunc(red, 5)
	if w := lipgloss.Width(got); w != 5 {
		t.Errorf("width = %d, want 5; got=%q", w, got)
	}
}
