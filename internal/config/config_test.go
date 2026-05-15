package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad_EmptyPathReturnsZeroConfig pins that an empty path is the
// "no config requested" signal and returns a zero-value Config without
// touching the filesystem. Callers can lean on the returned non-nil
// pointer being safe to dereference even when the user never set
// --config and no default file exists.
func TestLoad_EmptyPathReturnsZeroConfig(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\"): %v", err)
	}
	if cfg == nil {
		t.Fatal("Load(\"\") returned nil")
	}
	if got := len(cfg.Syntax.Extensions); got != 0 {
		t.Errorf("expected zero extensions; got %d", got)
	}
}

// TestLoad_MissingExplicitPathErrors pins that an explicitly-given path
// that doesn't exist is a hard error — the user typed --config <path>
// and we owe them a failure if the file isn't there. Compare with the
// implicit-search path, which silently tolerates absence.
func TestLoad_MissingExplicitPathErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.toml")
	if _, err := Load(missing); err == nil {
		t.Errorf("Load(missing) must error; got nil")
	}
}

// TestLoad_ValidExtensionsTable pins the canonical schema:
//
//	[syntax.extensions]
//	".j2" = "yaml+jinja2"
func TestLoad_ValidExtensionsTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reva.toml")
	body := `[syntax.extensions]
".j2" = "yaml+jinja2"
".html.j2" = "html+jinja2"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%s): %v", path, err)
	}
	if got := cfg.Syntax.Extensions[".j2"]; got != "yaml+jinja2" {
		t.Errorf(`Extensions[".j2"] = %q; want "yaml+jinja2"`, got)
	}
	if got := cfg.Syntax.Extensions[".html.j2"]; got != "html+jinja2" {
		t.Errorf(`Extensions[".html.j2"] = %q; want "html+jinja2"`, got)
	}
}

// TestLoad_LayoutCommentsWidthPercent pins the [layout] section schema.
// An integer in [10, 70] is honored verbatim; out-of-range / unset / zero
// surfaces as 0 so the consumer can apply the built-in default. The 10–70
// clamp lives at the consumer (Model.SetCommentsWidthPercent), not the
// loader — the loader stays a thin TOML→struct adapter.
func TestLoad_LayoutCommentsWidthPercent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reva.toml")
	body := `[layout]
comments_width_percent = 42
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%s): %v", path, err)
	}
	if got := cfg.Layout.CommentsWidthPercent; got != 42 {
		t.Errorf("Layout.CommentsWidthPercent = %d; want 42", got)
	}
}

// TestLoad_EmptyConfigZeroesLayout pins the "unset means zero" contract
// that the consumer pivots on to fall back to the built-in default.
func TestLoad_EmptyConfigZeroesLayout(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\"): %v", err)
	}
	if got := cfg.Layout.CommentsWidthPercent; got != 0 {
		t.Errorf("zero-value Layout.CommentsWidthPercent = %d; want 0", got)
	}
}

// TestResolvePath_ExplicitWins pins that --config takes precedence over
// XDG / HOME defaults — user intent always wins.
func TestResolvePath_ExplicitWins(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/should/not/be/checked")
	t.Setenv("HOME", "/should/not/be/checked/either")
	got := ResolvePath("/explicit/reva.toml")
	if got != "/explicit/reva.toml" {
		t.Errorf("ResolvePath: got %q, want explicit verbatim", got)
	}
}

// TestResolvePath_PrefersXDG pins the implicit-search ladder:
// $XDG_CONFIG_HOME/reva.toml wins over $HOME/.config/reva.toml when
// both exist.
func TestResolvePath_PrefersXDG(t *testing.T) {
	xdg := t.TempDir()
	home := t.TempDir()
	xdgPath := filepath.Join(xdg, "reva.toml")
	homePath := filepath.Join(home, ".config", "reva.toml")
	if err := os.MkdirAll(filepath.Dir(homePath), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{xdgPath, homePath} {
		if err := os.WriteFile(p, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("HOME", home)
	got := ResolvePath("")
	if got != xdgPath {
		t.Errorf("ResolvePath: got %q, want %q (XDG should win)", got, xdgPath)
	}
}

// TestResolvePath_FallsBackToHome pins that without XDG_CONFIG_HOME but
// with $HOME/.config/reva.toml present, the latter is chosen.
func TestResolvePath_FallsBackToHome(t *testing.T) {
	home := t.TempDir()
	homePath := filepath.Join(home, ".config", "reva.toml")
	if err := os.MkdirAll(filepath.Dir(homePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(homePath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)
	got := ResolvePath("")
	if got != homePath {
		t.Errorf("ResolvePath: got %q, want %q ($HOME/.config fallback)", got, homePath)
	}
}

// TestResolvePath_MissingReturnsEmpty pins that with neither default
// path populated, ResolvePath returns the empty string — the signal
// for "no config file, run with defaults".
func TestResolvePath_MissingReturnsEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // empty dir
	t.Setenv("HOME", t.TempDir())
	got := ResolvePath("")
	if got != "" {
		t.Errorf("ResolvePath: got %q, want empty", got)
	}
}
