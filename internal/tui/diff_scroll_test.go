package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// Pressing j on a long, wrapping diff must scroll the viewport once the
// cursor moves below the visible area. Regression: handleKey did not
// re-run measureLayout, so paneWidthDiff was 0 at Update time and
// scrollDiffIntoView fell back to a 1:1 buffer→display mapping that
// ignored wrap. The cursor moved off-screen visually but Top stayed at
// 0 — the user-facing symptom was "j/k doesn't scroll, only ctrl+f
// does".
func TestDiffJ_ScrollsViewport_UnderWrap(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	m.state.PR = &model.PR{
		Owner: "o", Repo: "r", Number: 1,
		Files: []*model.FileEntry{{Path: "x.go"}},
	}
	m.state.SelectedFile = "x.go"
	m.state.FocusedPane = model.PaneDiff
	m.state.DiffCursor.Side = model.DiffSideRight
	m.state.DiffViewport.Height = 10

	// Long content that wraps multiple display rows per buffer line so the
	// 1:1 fallback is provably wrong.
	long := strings.Repeat("Aaaaa Bbbbb Cccc Ddddd ", 8)
	patch := []string{"@@ -1,1 +1,30 @@"}
	for i := 0; i < 30; i++ {
		patch = append(patch, "+"+long+strconv.Itoa(i))
	}
	m.state.DiffCache[diffKey("", "x.go")] = strings.Join(patch, "\n")

	// Drive Update directly so handleKey's measureLayout fires (mirroring
	// the live tea.KeyMsg path).
	m.width = 200
	m.height = 50

	var mt tea.Model = m
	for i := 0; i < 8; i++ {
		mt, _ = mt.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}
	final := mt.(Model)
	if final.state.DiffCursor.Line != 8 {
		t.Fatalf("cursor=%d, want 8", final.state.DiffCursor.Line)
	}
	if final.state.DiffViewport.Top == 0 {
		t.Fatalf("DiffViewport.Top stayed 0 after 8 j's on wrapping diff; expected scroll")
	}
}
