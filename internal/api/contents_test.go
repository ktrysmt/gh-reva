package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "fx.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return p
}

// fixtureClient.GetFileContents returns the file content as a slice of
// NEW-side lines for a given (ref, path). The fixture JSON stores file
// content under "file_contents": {"<ref>::<path>": "L1\nL2\n..."}.
func TestFixtureGetFileContents_Found(t *testing.T) {
	body := `{
		"pr": {"owner": "o", "repo": "r", "number": 1, "head_sha": "headsha"},
		"file_contents": {
			"headsha::foo.go": "line1\nline2\nline3"
		}
	}`
	c, err := NewFixtureClient(writeFixture(t, body))
	if err != nil {
		t.Fatalf("new fixture: %v", err)
	}
	lines, err := c.GetFileContents(context.Background(), "o", "r", 1, "headsha", "foo.go")
	if err != nil {
		t.Fatalf("GetFileContents: %v", err)
	}
	want := []string{"line1", "line2", "line3"}
	if len(lines) != len(want) {
		t.Fatalf("len: got %d want %d (%q)", len(lines), len(want), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("lines[%d]: got %q want %q", i, lines[i], want[i])
		}
	}
}

func TestFixtureGetFileContents_NotFound(t *testing.T) {
	body := `{
		"pr": {"owner": "o", "repo": "r", "number": 1},
		"file_contents": {}
	}`
	c, err := NewFixtureClient(writeFixture(t, body))
	if err != nil {
		t.Fatalf("new fixture: %v", err)
	}
	lines, err := c.GetFileContents(context.Background(), "o", "r", 1, "any", "missing.go")
	if err == nil {
		t.Fatalf("expected error for missing file, got lines=%q", lines)
	}
}

// Trailing newline must not produce a phantom empty line at EOF — the
// trim mirrors how Expand walks `len(fileLines)` to detect the EOF gap.
func TestFixtureGetFileContents_TrailingNewlineTrimmed(t *testing.T) {
	body := `{
		"pr": {"owner": "o", "repo": "r", "number": 1, "head_sha": "h"},
		"file_contents": {"h::foo.go": "a\nb\n"}
	}`
	c, err := NewFixtureClient(writeFixture(t, body))
	if err != nil {
		t.Fatalf("new fixture: %v", err)
	}
	lines, _ := c.GetFileContents(context.Background(), "o", "r", 1, "h", "foo.go")
	if len(lines) != 2 || lines[0] != "a" || lines[1] != "b" {
		t.Fatalf("trailing nl: got %q", lines)
	}
}
