// Package config loads gh-reva's optional user configuration. The single
// source of truth is `reva.toml`, looked up via the XDG Base Directory
// spec and overridable with the --config flag. Absent / unset means
// "use defaults"; the package never returns nil.
package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the in-memory shape of reva.toml. Add fields with their own
// TOML tag and zero-value semantics; nothing here may be required for
// gh-reva to run.
type Config struct {
	Syntax SyntaxConfig `toml:"syntax"`
	Layout LayoutConfig `toml:"layout"`
}

// SyntaxConfig holds syntax-highlight-related overrides. Today only
// extension → chroma-lexer mappings live here; future fields can be
// added under the same [syntax.*] table.
type SyntaxConfig struct {
	// Extensions maps a file extension (with leading dot, e.g. ".j2") to
	// a chroma lexer name or alias. Lookups are exact (case-sensitive)
	// and longest-match-first at the call site so ".html.j2" can shadow
	// ".j2" cleanly.
	Extensions map[string]string `toml:"extensions"`
}

// LayoutConfig holds layout / pane-sizing overrides. Fields are int
// percentages; zero means "use the built-in default". The consumer
// owns the valid range and falls back to the default on out-of-range
// values — the loader stays a thin TOML→struct adapter.
type LayoutConfig struct {
	// CommentsWidthPercent overrides the Comments column's share of the
	// total terminal width. Honored when in [10, 70]; out-of-range or
	// zero falls back to the built-in default. The Comments column gets
	// `total * pct / 100` and the Diff column absorbs whatever Files
	// doesn't claim. Subject to the existing narrow-terminal floors so
	// Diff and Files never collapse below readable widths.
	CommentsWidthPercent int `toml:"comments_width_percent"`
}

// Load reads `path` and unmarshals into a Config. An empty path yields
// the zero-value Config without touching the filesystem — the signal
// for "no config requested". A missing or unreadable file when path is
// non-empty surfaces as an error so the user notices a typo on
// --config; the implicit-search caller funnels through ResolvePath
// first, which only returns paths that exist.
func Load(path string) (*Config, error) {
	cfg := &Config{Syntax: SyntaxConfig{Extensions: map[string]string{}}}
	if path == "" {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}
	if cfg.Syntax.Extensions == nil {
		cfg.Syntax.Extensions = map[string]string{}
	}
	return cfg, nil
}

// ResolvePath returns the path of the config file to load. Priority:
//
//  1. `explicit` (verbatim) — the --config flag wins unconditionally.
//  2. $XDG_CONFIG_HOME/reva.toml if it exists.
//  3. $HOME/.config/reva.toml if it exists.
//
// Returns "" when no candidate exists; callers feed the empty string to
// Load() to get a zero-value Config.
func ResolvePath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		p := filepath.Join(x, "reva.toml")
		if fileExists(p) {
			return p
		}
	}
	home := os.Getenv("HOME")
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = h
		}
	}
	if home != "" {
		p := filepath.Join(home, ".config", "reva.toml")
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
