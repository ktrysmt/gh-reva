package tui

import (
	"strings"
	"testing"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// TestLexerForLine_AllViewPerFile pins that in the Files "All" view
// (SelectedFile == AllFilesPath) the Diff pane picks a chroma lexer
// per file section of the concatenated diff — driven by each file's
// `+++ b/<path>` header — instead of falling back to plaintext for the
// whole buffer. Without this, the AllFilesPath sentinel (which matches
// no extension) routes every line through lexers.Fallback and syntax
// highlighting silently disappears in All view.
func TestLexerForLine_AllViewPerFile(t *testing.T) {
	m := NewModel(nil, nil)
	m.state.PR = &model.PR{HeadSHA: "headsha"}
	m.state.SelectedFile = model.AllFilesPath
	m.state.SelectedRange = model.CommitRange{Kind: model.RangeWholePR}

	goDiff := "--- a/src/greeting.go\n+++ b/src/greeting.go\n@@ -1,1 +1,2 @@\n package src\n+func Hello() {}\n"
	pyDiff := "--- a/app/main.py\n+++ b/app/main.py\n@@ -1,1 +1,2 @@\n import os\n+def hi(): pass\n"
	m.state.DiffCache[diffKey("", model.AllFilesPath)] = goDiff + pyDiff

	lines := m.patchLines()
	if len(lines) == 0 {
		t.Fatal("expected concat patch lines")
	}

	// Walk the buffer; assert content lines resolve to the lexer of the
	// file section they belong to.
	var sawGo, sawPy bool
	cur := ""
	for i, ln := range lines {
		if strings.HasPrefix(ln, "+++ ") {
			cur = strings.TrimPrefix(strings.TrimSpace(ln[4:]), "b/")
			continue
		}
		if strings.HasPrefix(ln, "---") || strings.HasPrefix(ln, "@@") || ln == "" {
			continue
		}
		name := strings.ToLower(m.lexerForLine(i).Config().Name)
		switch {
		case strings.HasSuffix(cur, ".go"):
			if name != "go" {
				t.Errorf("line %d (%q) in .go section: expected go lexer, got %q", i, ln, name)
			}
			sawGo = true
		case strings.HasSuffix(cur, ".py"):
			if name != "python" {
				t.Errorf("line %d (%q) in .py section: expected python lexer, got %q", i, ln, name)
			}
			sawPy = true
		}
	}
	if !sawGo || !sawPy {
		t.Fatalf("did not exercise both sections (go=%v py=%v)", sawGo, sawPy)
	}
}
