package tui

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestBuildEditorCmd_NoTmux pins that without $TMUX in the environment
// the editor invocation goes through the canonical `sh -c <shellCmd>`
// path used since the original compose flow. The popup width / height
// parameters are ignored on this branch — there is no popup to size.
func TestBuildEditorCmd_NoTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	shellCmd := "vim '/tmp/foo.md'"
	cmd := buildEditorCmd(shellCmd, 70, 40)
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

// TestBuildEditorCmd_TmuxPopupDefault pins that with $TMUX set and zero
// width / height (the "no override" signal that reva.toml's [editor]
// block uses), the popup falls back to the built-in 50% × 50% default.
// This is the size the user gets out of the box: roomy enough for a
// few-paragraph comment while leaving the surrounding reva frame
// clearly visible underneath, so the popup feels like a transient
// overlay rather than a full editor swap.
func TestBuildEditorCmd_TmuxPopupDefault(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")
	shellCmd := "vim '/tmp/foo.md'"
	cmd := buildEditorCmd(shellCmd, 0, 0)
	if !strings.HasSuffix(cmd.Args[0], "tmux") {
		t.Errorf("argv[0] with TMUX must be tmux; got %q", cmd.Args[0])
	}
	want := []string{"display-popup", "-E", "-w", "50%", "-h", "50%", shellCmd}
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

// TestBuildEditorCmd_TmuxPopupCustom pins that valid in-range
// percentages from reva.toml's [editor] block are emitted verbatim into
// the tmux command line. The user controls the popup geometry from a
// single config knob.
func TestBuildEditorCmd_TmuxPopupCustom(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")
	shellCmd := "vim '/tmp/foo.md'"
	cmd := buildEditorCmd(shellCmd, 70, 40)
	want := []string{"display-popup", "-E", "-w", "70%", "-h", "40%", shellCmd}
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

// TestBuildEditorCmd_TmuxPopupOutOfRangeFallsBack pins that values
// outside the [editorPopupPercentMin, editorPopupPercentMax] = [20, 95]
// range are rejected and the popup falls back to the 50/50 default,
// mirroring the comments_width_percent contract. Out-of-range here
// means a typo, not user intent — the safe behavior is to ignore the
// bad value rather than render a popup so narrow it can't fit a single
// line, or so wide it eats the whole frame.
func TestBuildEditorCmd_TmuxPopupOutOfRangeFallsBack(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")
	cases := []struct {
		name string
		w, h int
	}{
		{"width too low", 5, 50},
		{"width too high", 99, 50},
		{"height too low", 50, 0},
		{"height too high", 50, 100},
		{"both negative", -10, -10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := buildEditorCmd("vim '/tmp/foo.md'", tc.w, tc.h)
			want := []string{"-w", "50%", "-h", "50%"}
			got := strings.Join(cmd.Args, " ")
			for i := 0; i < len(want); i += 2 {
				flag, val := want[i], want[i+1]
				idx := -1
				for j, a := range cmd.Args {
					if a == flag {
						idx = j
						break
					}
				}
				if idx < 0 || idx+1 >= len(cmd.Args) || cmd.Args[idx+1] != val {
					t.Errorf("out-of-range (%d,%d) must fall back to %s %s; got %s", tc.w, tc.h, flag, val, got)
				}
			}
		})
	}
}

// TestBuildEditorCmd_TmuxEmptyValueFallsBack pins that an explicitly
// empty TMUX env var is treated the same as unset — this matches what
// shells do when TMUX is unset by `unset TMUX` (it disappears) versus
// `TMUX=` (set to empty). Both must take the sh -c path.
func TestBuildEditorCmd_TmuxEmptyValueFallsBack(t *testing.T) {
	t.Setenv("TMUX", "")
	cmd := buildEditorCmd("vim '/tmp/foo.md'", 0, 0)
	if !strings.HasSuffix(cmd.Args[0], "sh") {
		t.Errorf("empty TMUX must take the sh -c branch; got argv[0]=%q", cmd.Args[0])
	}
}

// TestRunEditorOverlay_RoundTrip pins the tmux popup branch's contract:
// the editor process is run inline via cmd.Run() (no tea.ExecProcess),
// and after exit the tempfile body is read back, the file is removed,
// and a composeBodyMsg is emitted. This is the path that keeps reva's
// alt-screen painted underneath the popup — releasing the terminal via
// tea.ExecProcess (as the non-tmux branch does) would clobber the paint
// and re-introduce the full-screen swap.
func TestRunEditorOverlay_RoundTrip(t *testing.T) {
	f, err := os.CreateTemp("", "test-overlay-*.md")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	want := "hello from overlay\n"
	if _, err := f.WriteString(want); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	tmpPath := f.Name()

	// `true` exits 0 instantly without touching the tempfile, simulating
	// an editor that quit cleanly after the user saved.
	teaCmd := runEditorOverlay(exec.Command("true"), tmpPath)
	msg := teaCmd()

	body, ok := msg.(composeBodyMsg)
	if !ok {
		t.Fatalf("expected composeBodyMsg; got %T", msg)
	}
	if body.err != nil {
		t.Fatalf("unexpected err: %v", body.err)
	}
	if body.body != want {
		t.Errorf("body mismatch: got %q want %q", body.body, want)
	}
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("tempfile must be removed after readback; stat err = %v", err)
	}
}

// TestRunEditorOverlay_EditorFailurePropagates pins that a non-zero
// editor exit surfaces as composeBodyMsg.err and the tempfile is still
// cleaned up. Without cleanup, repeated cancel/retry cycles would
// accumulate stale gh-reva-compose-*.md files in $TMPDIR.
func TestRunEditorOverlay_EditorFailurePropagates(t *testing.T) {
	f, err := os.CreateTemp("", "test-overlay-fail-*.md")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	tmpPath := f.Name()

	teaCmd := runEditorOverlay(exec.Command("false"), tmpPath)
	msg := teaCmd()

	body, ok := msg.(composeBodyMsg)
	if !ok {
		t.Fatalf("expected composeBodyMsg; got %T", msg)
	}
	if body.err == nil {
		t.Errorf("expected err from non-zero exit; got nil")
	}
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("tempfile must be removed even on editor failure; stat err = %v", err)
	}
}
