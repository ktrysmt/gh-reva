package api

import (
	"strings"
	"testing"
)

// TestConvertGQLCommentStripsCarriageReturns pins that comment bodies
// returned by GitHub's GraphQL API have CRLF line endings normalized to
// LF and bare CRs removed before the body reaches model.ReviewComment.
//
// Why this matters: a comment body containing a `\r` byte renders as
// "carriage return" in the terminal, moving the cursor back to column 0
// of the current line. In the joined `<Files><Diff><Comments>` layout,
// the cursor reset lands inside the Files column and subsequent bytes
// overwrite it — making the Files column visually corrupt whenever the
// Comments column is open. Stripping at the API boundary covers every
// downstream consumer (renderer, clipboard yank, edit composer).
func TestConvertGQLCommentStripsCarriageReturns(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "CRLF normalized to LF",
			in:   "line1\r\nline2\r\nline3",
			want: "line1\nline2\nline3",
		},
		{
			name: "bare CR removed",
			in:   "before\rafter",
			want: "beforeafter",
		},
		{
			name: "mixed CRLF and bare CR",
			in:   "line1\r\nline2\rline3\r\n",
			want: "line1\nline2line3\n",
		},
		{
			name: "no CR untouched",
			in:   "plain\nbody",
			want: "plain\nbody",
		},
		{
			name: "japanese body with CRLF",
			in:   "推奨**ARN ではなく**\r\n\r\nシークレット名を使用",
			want: "推奨**ARN ではなく**\n\nシークレット名を使用",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gc := gqlReviewComment{Body: tc.in}
			thread := gqlReviewThread{}
			rc := convertGQLComment(gc, thread)
			if rc.Body != tc.want {
				t.Errorf("Body mismatch\n got: %q\nwant: %q", rc.Body, tc.want)
			}
			if strings.ContainsRune(rc.Body, '\r') {
				t.Errorf("Body still contains CR: %q", rc.Body)
			}
		})
	}
}
