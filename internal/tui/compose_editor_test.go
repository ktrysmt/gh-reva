package tui

import (
	"strings"
	"testing"
)

// TestBuildEditorCmd_NoTmux pins that without $TMUX in the environment
// the editor invocation goes through the canonical `sh -c <shellCmd>`
// path used since the original compose flow.
func TestBuildEditorCmd_NoTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	shellCmd := "vim '/tmp/foo.md'"
	cmd := buildEditorCmd(shellCmd)
	if got, want := cmd.Args[0], "sh"; !strings.HasSuffix(got, want) {
		t.Errorf("argv[0] without TMUX must be sh; got %q", got)
	}
	if cmd.Args[1] != "-c" {
		t.Errorf("argv[1] must be -c; got %q", cmd.Args[1])
	}
	if cmd.Args[2] != shellCmd {
		t.Errorf("argv[2] must carry the editor invocation verbatim; got %q", cmd.Args[2])
	}
}

// TestBuildEditorCmd_TmuxPopup pins that with $TMUX set the editor is
// launched via `tmux display-popup -E -w 80% -h 80% <shellCmd>` so the
// composer floats over the gh-reva TUI instead of replacing it. The user
// can finish in the popup and gh-reva's frame stays painted underneath
// the popup, eliminating the full-screen swap that the bare $EDITOR
// path performs.
func TestBuildEditorCmd_TmuxPopup(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")
	shellCmd := "vim '/tmp/foo.md'"
	cmd := buildEditorCmd(shellCmd)
	if !strings.HasSuffix(cmd.Args[0], "tmux") {
		t.Errorf("argv[0] with TMUX must be tmux; got %q", cmd.Args[0])
	}
	want := []string{"display-popup", "-E", "-w", "80%", "-h", "80%", shellCmd}
	got := cmd.Args[1:]
	if len(got) != len(want) {
		t.Fatalf("argv tail length mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argv[%d]: got %q want %q", i+1, got[i], want[i])
		}
	}
}

// TestBuildEditorCmd_TmuxEmptyValueFallsBack pins that an explicitly
// empty TMUX env var is treated the same as unset — this matches what
// shells do when TMUX is unset by `unset TMUX` (it disappears) versus
// `TMUX=` (set to empty). Both must take the sh -c path.
func TestBuildEditorCmd_TmuxEmptyValueFallsBack(t *testing.T) {
	t.Setenv("TMUX", "")
	cmd := buildEditorCmd("vim '/tmp/foo.md'")
	if !strings.HasSuffix(cmd.Args[0], "sh") {
		t.Errorf("empty TMUX must take the sh -c branch; got argv[0]=%q", cmd.Args[0])
	}
}
