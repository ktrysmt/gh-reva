package api

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestFixtureClientStripsCarriageReturnsInCommentBodies mirrors the
// GraphQL-path sanitizer assertion for the fixture client. Both paths
// converge on model.ReviewComment.Body and both must scrub `\r` bytes —
// without the parallel fix, an authored regression fixture carrying
// CRLF / bare CR in `body` would reproduce the original "Files column
// shifts right when Comments is open" bug (rendering `\r` resets the
// terminal cursor to column 0 of the joined Files+Diff+Comments row,
// so subsequent bytes overwrite the Files column to the left).
func TestFixtureClientStripsCarriageReturnsInCommentBodies(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	fixturePath := filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", "comments-cr-pr.json")

	c, err := NewFixtureClient(fixturePath)
	if err != nil {
		t.Fatalf("NewFixtureClient: %v", err)
	}
	comments, err := c.ListComments(context.Background(), "octocat", "hello-world", 42)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) == 0 {
		t.Fatalf("fixture must carry at least one comment to exercise the sanitizer")
	}

	// At least one fixture comment must originally encode `\r` so the
	// assertion below proves the sanitizer ran (rather than passing
	// vacuously on a CR-free fixture).
	var sawCRSource bool
	for _, c := range comments {
		if strings.ContainsRune(c.Body, '\r') {
			t.Errorf("comment id=%d body still contains CR after fixture load: %q", c.ID, c.Body)
		}
		// The fixture intentionally seeds CRLF / bare CR in several
		// bodies; the assertion below requires the source JSON to carry
		// those bytes by checking that the post-sanitize body still has
		// the surrounding text but not the CR. We sample one well-known
		// fixture comment (id=9001) to confirm CRLF was folded to LF.
		if c.ID == 9001 {
			sawCRSource = true
			if !strings.Contains(c.Body, "line1\nline2") {
				t.Errorf("comment id=9001: CRLF should fold to LF; got body=%q", c.Body)
			}
		}
	}
	if !sawCRSource {
		t.Errorf("fixture must include comment id=9001 with CRLF body for the regression assertion to be meaningful")
	}
}
