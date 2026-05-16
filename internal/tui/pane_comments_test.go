package tui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// commentsModelFixture builds a Model with a synthetic patch + two anchored
// comment threads on src/foo.go for use by the tests below. Buffer layout:
//
//	0: @@ -1,3 +1,5 @@
//	1:  line1                (newLine 1)
//	2: +addedLine2           (newLine 2) ← thread T1 (root + reply)
//	3: +addedLine3           (newLine 3)
//	4:  line4                (newLine 4) ← thread T2 (root only)
//	5:  line5                (newLine 5)
func commentsModelFixture(t *testing.T) Model {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	// Use time.Local so the renderer's CreatedAt.Local().Format(...) round-trip
	// reproduces the literal hh:mm we assert on, regardless of host TZ.
	t1Root := &model.ReviewComment{
		ID: 1, Path: "src/foo.go", CommitID: "abcdef0123456",
		Line: 2, User: "carol",
		CreatedAt: time.Date(2024, 1, 15, 13, 0, 0, 0, time.Local),
		Body:      "Consider extracting this into a helper function for clarity",
	}
	t1Reply := &model.ReviewComment{
		ID: 2, Path: "src/foo.go", CommitID: "abcdef0123456",
		Line: 2, InReplyTo: 1, User: "alice",
		CreatedAt: time.Date(2024, 1, 15, 14, 30, 0, 0, time.Local),
		Body:      "Good point, will refactor",
	}
	t2Root := &model.ReviewComment{
		ID: 3, Path: "src/foo.go", CommitID: "abcdef0123456",
		Line: 4, User: "dave",
		CreatedAt: time.Date(2024, 1, 16, 9, 15, 0, 0, time.Local),
		Body:      "Nit",
	}
	m.state.PR = &model.PR{
		Owner: "o", Repo: "r", Number: 1,
		Files: []*model.FileEntry{{Path: "src/foo.go", Status: model.ChangeModified}},
		Comments: []*model.ReviewComment{t1Root, t1Reply, t2Root},
	}
	m.state.SelectedFile = "src/foo.go"
	m.state.DiffCache[diffKey("", "src/foo.go")] = strings.Join([]string{
		"@@ -1,3 +1,5 @@",
		" line1",
		"+addedLine2",
		"+addedLine3",
		" line4",
		" line5",
	}, "\n")
	m.paneWidthComments = 50
	return m
}

// commentsLeftSideFixture builds a Model whose patch contains both `-`
// (deleted) and `+` (added) lines, plus comments anchored on each side.
// Buffer layout:
//
//	0: @@ -1,3 +1,3 @@
//	1:  line1                (oldLine 1, newLine 1)
//	2: -removed_line2        (oldLine 2)        ← thread T_LEFT (Side="LEFT", Line=2)
//	3: +added_line2          (newLine 2)        ← thread T_RIGHT (Side="RIGHT", Line=2)
//	4:  line3                (oldLine 3, newLine 3)
//
// Used to pin that LEFT-side comments anchor on the `-` buffer row, not
// silently fall through `newLineNumbers` (which returns 0 for `-` lines)
// and disappear from both ◆ marker and Comments column.
func commentsLeftSideFixture(t *testing.T) Model {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := NewModel(nil, nil)
	leftComment := &model.ReviewComment{
		ID: 10, Path: "src/foo.go", CommitID: "abcdef0123456",
		Line: 2, Side: "LEFT", User: "carol",
		CreatedAt: time.Date(2024, 1, 15, 13, 0, 0, 0, time.Local),
		Body:      "deleted-line note",
	}
	rightComment := &model.ReviewComment{
		ID: 11, Path: "src/foo.go", CommitID: "abcdef0123456",
		Line: 2, Side: "RIGHT", User: "dave",
		CreatedAt: time.Date(2024, 1, 16, 9, 15, 0, 0, time.Local),
		Body:      "added-line note",
	}
	m.state.PR = &model.PR{
		Owner: "o", Repo: "r", Number: 1,
		Files:    []*model.FileEntry{{Path: "src/foo.go", Status: model.ChangeModified}},
		Comments: []*model.ReviewComment{leftComment, rightComment},
	}
	m.state.SelectedFile = "src/foo.go"
	m.state.DiffCache[diffKey("", "src/foo.go")] = strings.Join([]string{
		"@@ -1,3 +1,3 @@",
		" line1",
		"-removed_line2",
		"+added_line2",
		" line3",
	}, "\n")
	m.paneWidthComments = 50
	return m
}

// TestCommentsViewShowsLeftSideThreadOnDeletedLine pins that a comment
// posted on a `-` (deleted) line anchors on that buffer row. The comment
// is stored with Side="LEFT" and Line=<old-file-line-number>; the display
// pipeline must consult the OLD-file line mapping for LEFT comments
// instead of treating Line as a new-file line number (which silently
// drops the comment because newLineNumbers returns 0 for `-` rows).
func TestCommentsViewShowsLeftSideThreadOnDeletedLine(t *testing.T) {
	m := commentsLeftSideFixture(t)
	m.state.DiffCursor.Line = 2 // the `-removed_line2` buffer row
	// j/k auto-skip never lands a RIGHT cursor on a `-` row, so the only
	// reachable real-world state for a cursor at buffer 2 is Side=LEFT.
	// Pin that explicitly so the Side filter does not hide the LEFT
	// thread we are asserting on.
	m.state.DiffCursor.Side = model.DiffSideLeft

	got := m.commentsView()
	if !strings.Contains(got, "carol") {
		t.Errorf("LEFT-side comment (carol) must anchor on the `-` row at buffer line 2:\n%s", got)
	}
	if !strings.Contains(got, "deleted-line note") {
		t.Errorf("LEFT-side comment body must render at the `-` row:\n%s", got)
	}
	if strings.Contains(got, "dave") || strings.Contains(got, "added-line note") {
		t.Errorf("RIGHT-side comment must NOT leak onto the `-` buffer row:\n%s", got)
	}
}

// TestCommentsViewShowsRightSideThreadOnAddedLine is the symmetric pin:
// the RIGHT-side comment anchors on the `+` buffer row, not the `-` row.
func TestCommentsViewShowsRightSideThreadOnAddedLine(t *testing.T) {
	m := commentsLeftSideFixture(t)
	m.state.DiffCursor.Line = 3 // the `+added_line2` buffer row

	got := m.commentsView()
	if !strings.Contains(got, "dave") {
		t.Errorf("RIGHT-side comment (dave) must anchor on the `+` row at buffer line 3:\n%s", got)
	}
	if strings.Contains(got, "carol") {
		t.Errorf("LEFT-side comment must NOT leak onto the `+` buffer row:\n%s", got)
	}
}

// TestCommentsView_FilterBySideOnContextRow pins that the Comments
// column shows ONLY threads matching the cursor's Side. Both threads
// anchor on the same context buffer line (` line1` exists on both
// sides at oldLine=1 / newLine=1, so buffer index 1 is shared) but with
// opposite Side values; flipping cursor.Side flips which one renders.
func TestCommentsView_FilterBySideOnContextRow(t *testing.T) {
	m := commentsModelFixture(t)
	leftComment := &model.ReviewComment{
		ID: 100, Path: "src/foo.go", CommitID: "abcdef0123456",
		Line: 1, Side: "LEFT", User: "leftie",
		CreatedAt: time.Date(2024, 2, 1, 10, 0, 0, 0, time.Local),
		Body:      "before-side note",
	}
	rightComment := &model.ReviewComment{
		ID: 101, Path: "src/foo.go", CommitID: "abcdef0123456",
		Line: 1, Side: "RIGHT", User: "rightie",
		CreatedAt: time.Date(2024, 2, 1, 11, 0, 0, 0, time.Local),
		Body:      "after-side note",
	}
	m.state.PR.Comments = []*model.ReviewComment{leftComment, rightComment}
	m.state.DiffCursor.Line = 1

	m.state.DiffCursor.Side = model.DiffSideLeft
	gotLeft := m.commentsView()
	if !strings.Contains(gotLeft, "leftie") {
		t.Errorf("LEFT cursor must show LEFT-side thread:\n%s", gotLeft)
	}
	if strings.Contains(gotLeft, "rightie") {
		t.Errorf("LEFT cursor must NOT show RIGHT-side thread:\n%s", gotLeft)
	}

	m.state.DiffCursor.Side = model.DiffSideRight
	gotRight := m.commentsView()
	if !strings.Contains(gotRight, "rightie") {
		t.Errorf("RIGHT cursor must show RIGHT-side thread:\n%s", gotRight)
	}
	if strings.Contains(gotRight, "leftie") {
		t.Errorf("RIGHT cursor must NOT show LEFT-side thread:\n%s", gotRight)
	}
}

// TestCommentLineMarkersIncludesLeftAndRightAnchors pins that the ◆ marker
// covers both the `-` row (LEFT comment) and the `+` row (RIGHT comment).
// Without side-aware mapping, the LEFT comment's Line=2 would either find
// the `+` row (because newLine 2 happens to land there in this fixture)
// or no row at all, and the `-` row would never get a marker.
func TestCommentLineMarkersIncludesLeftAndRightAnchors(t *testing.T) {
	m := commentsLeftSideFixture(t)
	got := m.commentLineMarkers()
	if got.Left[2] != '◆' {
		t.Errorf("Left map must place ◆ at buffer index 2 (the `-` row anchored by LEFT comment); got Left=%#v", got.Left)
	}
	if got.Right[3] != '◆' {
		t.Errorf("Right map must place ◆ at buffer index 3 (the `+` row anchored by RIGHT comment); got Right=%#v", got.Right)
	}
	if _, leaked := got.Right[2]; leaked {
		t.Errorf("Right map must not leak the LEFT-side ◆ onto buffer index 2: got Right=%#v", got.Right)
	}
	if _, leaked := got.Left[3]; leaked {
		t.Errorf("Left map must not leak the RIGHT-side ◆ onto buffer index 3: got Left=%#v", got.Left)
	}
}

// TestCommentsViewEmptyWhenCursorNotOnAnchor pins the contract that an
// off-anchor row inside the file body (context / + / -) shows the
// placeholder instead of the previous "all anchored threads" listing.
// Buffer idx 1 is ` line1` (context row, no thread anchored).
func TestCommentsViewEmptyWhenCursorNotOnAnchor(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.DiffCursor.Line = 1 // context row inside body, no anchor

	got := m.commentsView()
	if !strings.Contains(got, "(no comment at cursor)") {
		t.Errorf("expected placeholder when cursor is off-anchor, got:\n%s", got)
	}
	if strings.Contains(got, "carol") || strings.Contains(got, "dave") {
		t.Errorf("comments must not leak when cursor is off-anchor:\n%s", got)
	}
}

// TestCommentsView_MetaRow_ShowsAllFileThreads pins the file-overview
// exception: a Diff cursor parked on a metadata row (`---`, `+++`, `@@`)
// has no real line number, so the per-cursor + per-Side filter is
// bypassed and the Comments column lists every thread for the file.
// commentsModelFixture has its `@@` hunk header at buffer idx 0 with
// two threads (carol+alice on RIGHT, dave on RIGHT). Both must surface
// regardless of cursor.Side.
func TestCommentsView_MetaRow_ShowsAllFileThreads(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.DiffCursor.Line = 0 // `@@ -1,3 +1,5 @@` hunk header

	for _, side := range []model.DiffSide{model.DiffSideRight, model.DiffSideLeft} {
		m.state.DiffCursor.Side = side
		got := m.commentsView()
		if !strings.Contains(got, "carol") {
			t.Errorf("Side=%s: carol thread must appear on hunk-header overview:\n%s", side, got)
		}
		if !strings.Contains(got, "dave") {
			t.Errorf("Side=%s: dave thread must appear on hunk-header overview:\n%s", side, got)
		}
	}
}

// TestCommentsView_MetaRow_BothSidesInOverview pins that the meta-row
// overview ignores the per-Side filter — LEFT-side and RIGHT-side
// threads both appear when the cursor sits on a header/hunk row.
func TestCommentsView_MetaRow_BothSidesInOverview(t *testing.T) {
	m := commentsLeftSideFixture(t)
	m.state.DiffCursor.Line = 0 // `@@ -1,3 +1,3 @@` hunk header
	m.state.DiffCursor.Side = model.DiffSideRight
	m.paneWidthComments = 50

	got := m.commentsView()
	if !strings.Contains(got, "carol") {
		t.Errorf("LEFT-side thread (carol) must appear on the hunk-header overview:\n%s", got)
	}
	if !strings.Contains(got, "dave") {
		t.Errorf("RIGHT-side thread (dave) must appear on the hunk-header overview:\n%s", got)
	}
}

// TestThreadsForCursor_FileHeaderRow_ReturnsAll pins that the same
// short-circuit applies to `---` / `+++` file-header rows, not just
// `@@` hunks.
func TestThreadsForCursor_FileHeaderRow_ReturnsAll(t *testing.T) {
	m := commentsModelFixture(t)
	// Replace the fixture patch with one that opens with `---` / `+++`
	// so idx 0 / 1 are file headers (kind 'h') and idx 2 is the `@@`.
	m.state.DiffCache[diffKey("", "src/foo.go")] = strings.Join([]string{
		"--- a/src/foo.go",
		"+++ b/src/foo.go",
		"@@ -1,3 +1,5 @@",
		" line1",
		"+addedLine2",
		"+addedLine3",
		" line4",
		" line5",
	}, "\n")
	// Comments' Line=2/4 still point at NEW lines 2/4; that's buffer
	// idx 4 / 6 in this patch, but threadsForView (not cursor-bound)
	// returns them regardless. The buffer line numbers below only
	// matter for the meta-row short-circuit assertion.
	m.invalidatePatchInfoCache(model.ExpandKey{Path: "src/foo.go", RangeKind: model.RangeWholePR})
	m.threadsCache = &threadsViewCache{}

	for _, idx := range []int{0, 1, 2} { // ---, +++, @@
		m.state.DiffCursor.Line = idx
		threads := m.threadsForCursor()
		if len(threads) == 0 {
			t.Errorf("cursor at meta-row idx %d must return all file threads, got 0", idx)
		}
	}
}

// TestCommentsViewShowsOnlyCursorAnchorThreads pins that exactly the threads
// anchored at the Diff cursor's buffer line are visible — no others.
func TestCommentsViewShowsOnlyCursorAnchorThreads(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.DiffCursor.Line = 2 // ◆ for thread T1 (carol root + alice reply)

	got := m.commentsView()
	if !strings.Contains(got, "carol") {
		t.Errorf("T1 root (carol) should be visible at cursor line 2:\n%s", got)
	}
	if !strings.Contains(got, "alice") {
		t.Errorf("T1 reply (alice) should be visible at cursor line 2:\n%s", got)
	}
	if strings.Contains(got, "dave") {
		t.Errorf("T2 (dave) is anchored at line 4 and must NOT leak to line 2:\n%s", got)
	}

	m.state.DiffCursor.Line = 4 // ◆ for thread T2
	got = m.commentsView()
	if !strings.Contains(got, "dave") {
		t.Errorf("T2 (dave) should be visible at cursor line 4:\n%s", got)
	}
	if strings.Contains(got, "carol") || strings.Contains(got, "alice") {
		t.Errorf("T1 must not leak to line 4:\n%s", got)
	}
}

// TestCommentsTagsPendingOnRootAndReply pins the contract that a Pending
// comment carries a `[pending]` tag in its header row regardless of
// depth (root vs reply). The tag is the user-visible signal that a
// draft has not yet been submitted via the R-key flow; missing it on
// either depth would let the user accidentally submit-then-publish a
// reply they thought was still local.
func TestCommentsTagsPendingOnRootAndReply(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.PR.Comments[0].Pending = true // root (carol)
	m.state.PR.Comments[1].Pending = true // reply (alice)
	m.state.DiffCursor.Line = 2

	got := m.commentsView()
	rootIdx := strings.Index(got, "carol")
	replyIdx := strings.Index(got, "alice")
	if rootIdx < 0 || replyIdx < 0 {
		t.Fatalf("both root and reply must be visible:\n%s", got)
	}
	rootRow := got[rootIdx:replyIdx]
	if !strings.Contains(rootRow, "[pending]") {
		t.Errorf("root header must carry [pending]:\n%s", rootRow)
	}
	replyRow := got[replyIdx:]
	if !strings.Contains(replyRow, "[pending]") {
		t.Errorf("reply header must carry [pending]:\n%s", replyRow)
	}
}

// TestCommentsHeaderUsesNewFormat pins the header shape:
//
//	<name>: <yyyy-mm-dd hh:mm> <hash>
//
// (no trailing " — " body separator) and that the body is on its own row.
func TestCommentsHeaderUsesNewFormat(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.DiffCursor.Line = 2

	got := m.commentsView()
	// Use the local-TZ formatted timestamp so the assertion is robust against
	// the test machine's TZ. The fixture stores CreatedAt with a synthetic
	// fixed zone, but Format(local) honors that.
	wantHeader := "carol: 2024-01-15 13:00 abcdef0"
	if !strings.Contains(got, wantHeader) {
		t.Errorf("expected header %q in:\n%s", wantHeader, got)
	}
	// Ensure the body is NOT on the same line as the header (new format puts
	// it on the next row).
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "carol:") && strings.Contains(line, "Consider extracting") {
			t.Errorf("header and body must be on separate rows; combined row: %q", line)
		}
	}
}

// TestCommentsBodyIndentedAndWrapped pins that the body is indented past the
// header and wraps within the Comments column width. The fixture body is
// long enough to require a wrap at paneWidthComments=50.
func TestCommentsBodyIndentedAndWrapped(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.DiffCursor.Line = 2

	got := m.commentsView()
	lines := strings.Split(got, "\n")

	// Find the body row(s) that follow the carol header.
	headerIdx := -1
	for i, l := range lines {
		if strings.Contains(l, "carol:") {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		t.Fatalf("carol header not found:\n%s", got)
	}
	if headerIdx+1 >= len(lines) {
		t.Fatalf("expected body row after header; got truncated output:\n%s", got)
	}
	bodyRow := lines[headerIdx+1]
	// Body row must start with whitespace (indentation past the header).
	if !strings.HasPrefix(bodyRow, "  ") {
		t.Errorf("body row must be indented past the header column; got %q", bodyRow)
	}
	// The body fixture is "Consider extracting this into a helper function for clarity"
	// — at 50-col pane width with body indent, this should wrap onto >= 2 rows.
	// We verify the head and tail land on distinct rows.
	headOnRow := strings.Contains(bodyRow, "Consider")
	if !headOnRow {
		t.Errorf("first body row should start with 'Consider'; got %q", bodyRow)
	}
	tailFound := false
	for i := headerIdx + 2; i < len(lines); i++ {
		if strings.Contains(lines[i], "clarity") {
			tailFound = true
			// Continuation rows must keep the same indent as the body.
			if !strings.HasPrefix(lines[i], "  ") {
				t.Errorf("body continuation row %d must keep the body indent; got %q", i, lines[i])
			}
			break
		}
	}
	if !tailFound {
		t.Errorf("expected body wrap with 'clarity' on a later row; output:\n%s", got)
	}
}

// TestCommentsBodyWrapsLongCJKString pins that a body containing a long
// run of wide (CJK) characters with no whitespace is wrapped so each
// row's display width fits inside paneWidthComments. wrapText measures
// in rune count, but lipgloss.Width measures display cells (CJK = 2),
// so rune-only wrap would overflow visually and renderPaneBox's padTrunc
// would silently cut each row mid-content — which is what the user
// reported as "改行なしで長い文章は折り返されない / 途中で切れる".
func TestCommentsBodyWrapsLongCJKString(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.DiffCursor.Line = 4 // dave's anchor (T2)

	// 60 wide chars × 2 cells = 120 cells, well past paneWidthComments=50.
	m.state.PR.Comments[2].Body = strings.Repeat("あ", 60)

	got := m.commentsView()
	bodyRows := []string{}
	for _, row := range strings.Split(got, "\n") {
		if strings.Contains(row, "あ") {
			bodyRows = append(bodyRows, row)
		}
	}
	if len(bodyRows) < 2 {
		t.Fatalf("expected the long CJK body to wrap onto >=2 rows; got %d:\n%s", len(bodyRows), got)
	}
	total := 0
	for _, r := range bodyRows {
		total += strings.Count(r, "あ")
		if w := lipgloss.Width(r); w > m.paneWidthComments {
			t.Errorf("CJK body row exceeds paneWidthComments=%d (display width %d): %q",
				m.paneWidthComments, w, r)
		}
	}
	if total != 60 {
		t.Errorf("full CJK body must reach the pane (60 chars); got %d total:\n%s", total, got)
	}
}

// TestCommentsBodyWrapsLongUnbrokenString pins that a body containing a
// very long string with no whitespace (URL, identifier, etc.) is wrapped
// onto multiple rows so the full content stays inside the column.
func TestCommentsBodyWrapsLongUnbrokenString(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.DiffCursor.Line = 4 // dave's anchor (T2)

	// Replace dave's body with a long unbroken string. 200 chars of 'x' is
	// well past paneWidthComments=50.
	long := strings.Repeat("x", 200)
	m.state.PR.Comments[2].Body = long

	got := m.commentsView()
	bodyRows := []string{}
	for _, row := range strings.Split(got, "\n") {
		if strings.Contains(row, "x") {
			bodyRows = append(bodyRows, row)
		}
	}
	if len(bodyRows) < 2 {
		t.Fatalf("expected the long body to wrap onto >=2 rows; got %d row(s):\n%s", len(bodyRows), got)
	}
	totalX := 0
	for _, r := range bodyRows {
		totalX += strings.Count(r, "x")
		if w := utf8.RuneCountInString(r); w > m.paneWidthComments {
			t.Errorf("body row exceeds paneWidthComments=%d (got width %d): %q",
				m.paneWidthComments, w, r)
		}
	}
	if totalX != 200 {
		t.Errorf("full body must reach the pane (200 'x' chars); got %d total across rows:\n%s",
			totalX, got)
	}
}

// TestCommentsBodyKeepsAsciiCjkWordTogether reproduces the user-reported
// case where `wrapText` splits on a whitespace whose right side is CJK,
// stranding the leading ASCII fragment ("slack") on its own row when the
// following CJK word is too long to fit alongside it.
//
// Real fixture (PR DatachainDoC/doc-github#345 comment id 3055362231)
// body shape:
//
//	slackコマンドの後にスペースがないと、…なってしまってた\nslack コマンドの後すぐに…修正
//	                                                       ^ half-width space
//
// At a 55-cell Comments column (bodyWidth=51), the second source line
// is split into ["slack", "コマンドの後すぐに…修正"] by the default
// `strings.Fields` rule. word2 (48 cells) plus a sep can't fit after
// word1 on the same row, so word1 is flushed alone — that's the
// "slack stranded on its own row" symptom.
//
// Desired behavior: an ASCII↔CJK whitespace stays inside the running
// word, so the whole "slack コマンドの…修正" segment becomes one
// (long) word that `hardBreak` can wrap mid-CJK without isolating the
// "slack" prefix.
func TestCommentsBodyKeepsAsciiCjkWordTogether(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.DiffCursor.Line = 4
	m.paneWidthComments = 55 // bodyWidth = 51

	m.state.PR.Comments[2].Body = "slackコマンドの後にスペースがないと、コマンド部分が削除されない仕様になってしまってた\nslack コマンドの後すぐに改行しても削除されるように修正"

	got := m.commentsView()
	rows := strings.Split(got, "\n")

	for i, r := range rows {
		// Strip the body leader (cursor area + indent) before comparing.
		trimmed := strings.TrimLeft(r, " ")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "slack" {
			t.Errorf("row %d contains only \"slack\" — ASCII fragment was stranded by whitespace word split:\n%s", i, got)
		}
	}
}

// source body is rendered as a row break — matching GitHub PR comment
// rendering, which converts soft line breaks to <br>. Each source line
// must occupy its own row (no soft-break collapse). The previous
// "merge into one paragraph" approach broke the user's two-sentence
// body by gluing the second sentence to the tail of the first.
func TestCommentsBodyHonorsSourceLineBreaks(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.DiffCursor.Line = 4 // dave's anchor (T2)
	m.paneWidthComments = 80

	m.state.PR.Comments[2].Body = "slackコマンドの後にスペースがないと、コマンド部分が削除されない仕様になってしまってた\nslackコマンドの後すぐに改行しても削除されるように修正"

	got := m.commentsView()
	rows := strings.Split(got, "\n")

	// Line 1 wraps over 2 rows at this width — assert the *tail* of line 1
	// ("しまってた") and the *head* of line 2 ("slackコマンドの後すぐに改行")
	// land on adjacent rows, with no blank between and no merge.
	tail := -1
	head := -1
	for i, r := range rows {
		if tail < 0 && strings.Contains(r, "しまってた") {
			tail = i
		}
		if head < 0 && strings.Contains(r, "改行しても削除されるように修正") {
			head = i
		}
	}
	if tail < 0 || head < 0 {
		t.Fatalf("both source lines must reach the rendered output:\n%s", got)
	}
	if head != tail+1 {
		t.Errorf("source line 2 must render on the row immediately after line 1's last row (no merge / no extra blank); got tail row %d / head row %d:\n%s",
			tail, head, got)
	}
	// Tail of line 1 ("…まってた") must NOT be glued onto the start of line 2.
	for _, r := range rows {
		if strings.Contains(r, "まってたslack") {
			t.Errorf("line 1 tail must not be merged with line 2 head; row: %q\nfull:\n%s", r, got)
		}
	}
}

// TestCommentsBodyDoesNotCollapseAsciiSoftBreak pins that an ASCII↔ASCII
// soft break stays as two rows — i.e. "Hello\nworld" renders as two
// rows ("Hello", "world"), not as one merged "Hello world" row. This
// matches GitHub's `<br>`-on-soft-break rendering for PR comments.
func TestCommentsBodyDoesNotCollapseAsciiSoftBreak(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.DiffCursor.Line = 4
	m.paneWidthComments = 60

	m.state.PR.Comments[2].Body = "Hello\nworld is fun"

	got := m.commentsView()
	if strings.Contains(got, "Hello world is fun") {
		t.Errorf("ASCII↔ASCII soft break must NOT merge into one row; got:\n%s", got)
	}
	rows := strings.Split(got, "\n")
	helloRow, worldRow := -1, -1
	for i, r := range rows {
		if helloRow < 0 && strings.Contains(r, "Hello") && !strings.Contains(r, "world") {
			helloRow = i
		}
		if worldRow < 0 && strings.Contains(r, "world is fun") {
			worldRow = i
		}
	}
	if helloRow < 0 || worldRow < 0 || worldRow != helloRow+1 {
		t.Errorf("expected adjacent rows: 'Hello' then 'world is fun'; got rows %d / %d:\n%s",
			helloRow, worldRow, got)
	}
}

// TestCommentsBodyPreservesCodeFenceLineBreaks pins that line breaks
// inside ```...``` fences are kept verbatim — GFM treats fenced code as
// pre-formatted, so the soft-break collapse must NOT fire there. Reported
// regression: a fenced quote of @-mentions and a `───` divider rendered
// as a single soft-joined row.
func TestCommentsBodyPreservesCodeFenceLineBreaks(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.DiffCursor.Line = 4
	m.paneWidthComments = 80

	m.state.PR.Comments[2].Body = "こんな感じになるイメージ\n\nもっといいデザインあったら、コメントお願いします 🙏\n```\n@Ryota Fuwa レビューお願いします\n───\nfrom: @Takehiro Arima\n```"

	got := m.commentsView()
	rows := strings.Split(got, "\n")

	findRow := func(needle string) int {
		for i, r := range rows {
			if strings.Contains(r, needle) {
				return i
			}
		}
		return -1
	}
	mention := findRow("@Ryota Fuwa")
	divider := findRow("───")
	footer := findRow("from: @Takehiro Arima")
	if mention < 0 || divider < 0 || footer < 0 {
		t.Fatalf("all fenced code lines must appear; got:\n%s", got)
	}
	if mention >= divider {
		t.Errorf("mention must come before divider (rows %d, %d):\n%s", mention, divider, got)
	}
	if divider >= footer {
		t.Errorf("divider must come before footer (rows %d, %d):\n%s", divider, footer, got)
	}
	// Mention / divider / footer must be on separate rows (no soft-break
	// collapse inside the fence).
	if divider-mention != 1 {
		t.Errorf("mention and divider must be adjacent rows inside fence (got %d, %d):\n%s", mention, divider, got)
	}
	if footer-divider != 1 {
		t.Errorf("divider and footer must be adjacent rows inside fence (got %d, %d):\n%s", divider, footer, got)
	}
	// Both fence markers must be present on their own rows.
	fences := 0
	for _, r := range rows {
		trim := strings.TrimSpace(r)
		if trim == "```" {
			fences++
		}
	}
	if fences != 2 {
		t.Errorf("expected 2 fence-marker rows on their own; got %d. full output:\n%s", fences, got)
	}
}

// TestCommentsSoftBreakRenderingSnapshot is a human-eye sanity check:
// it logs the rendered Comments view for the user-reported scenarios so
// regressions are visible in test output. Run with `-v` to inspect.
func TestCommentsSoftBreakRenderingSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("snapshot is informational; run with -v to inspect")
	}

	cases := []struct {
		name string
		body string
	}{
		{
			// Real fixture from PR DatachainDoC/doc-github#345 comment 3055362231.
			// `slack` ASCII prefix + half-width space + long CJK trailing word.
			name: "slack-space-CJK at narrow column (real fixture)",
			body: "slackコマンドの後にスペースがないと、コマンド部分が削除されない仕様になってしまってた\nslack コマンドの後すぐに改行しても削除されるように修正",
		},
		{
			name: "fenced code block with mentions",
			body: "こんな感じになるイメージ\n\nもっといいデザインあったら、コメントお願いします 🙏\n```\n@Ryota Fuwa レビューお願いします\n───\nfrom: @Takehiro Arima\n```",
		},
	}
	for _, tc := range cases {
		// Render at two widths so both wrap regimes are visible: 55 cells
		// reproduces the user-reported narrow column (the 'slack' stranding
		// case), 80 cells matches the wider real-PR snapshot.
		for _, w := range []int{55, 80} {
			m := commentsModelFixture(t)
			m.state.DiffCursor.Line = 4
			m.paneWidthComments = w
			m.state.PR.Comments[2].Body = tc.body
			t.Logf("[%s @ paneWidthComments=%d]\n%s", tc.name, w, m.commentsView())
		}
	}
}

// TestCommentsBodyHandlesEmoji pins that emoji bodies — ZWJ-joined
// sequences, regional-indicator flags, VS16 presentation variants, and
// skin-tone modifiers — produce wrapped rows whose display width stays
// inside the Comments column. Reported regression: emoji bodies push
// the modal box's right border out of column because `runewidth.StringWidth`
// (used by wrapText) under-reports flag and VS16 emoji as 1 cell while
// the terminal (and lipgloss / uniseg) renders them as 2.
//
// The flag-only body is the sharpest probe of the bug: regional-indicator
// pairs render as a single 2-cell glyph in every modern terminal, but
// runewidth.StringWidth sums each codepoint as 1, so a 36-flag body
// reports as 36 cells while lipgloss.Width / uniseg.StringWidth report
// the true 72. wrapText's early `<= width` exit then leaves the row
// unwrapped, and renderPaneBox's padTrunc trusts the same broken
// arithmetic — the trailing │ ends up in the wrong column on screen.
func TestCommentsBodyHandlesEmoji(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.DiffCursor.Line = 4 // dave's anchor (T2)
	m.paneWidthComments = 40

	// 36 regional-indicator flag emoji. True display = 72 cells; runewidth
	// thinks 36, so wrapText's early exit fires at bodyWidth=36 and the
	// whole row passes through unwrapped if the bug is present.
	m.state.PR.Comments[2].Body = strings.Repeat("\U0001F1EF\U0001F1F5", 36)

	got := m.commentsView()
	for _, row := range strings.Split(got, "\n") {
		if !strings.Contains(row, "\U0001F1EF") {
			continue
		}
		if w := lipgloss.Width(row); w > m.paneWidthComments {
			t.Errorf("emoji body row exceeds paneWidthComments=%d (display width %d): %q",
				m.paneWidthComments, w, row)
		}
	}
}

// TestCommentsBodyHandlesMixedEmoji exercises the wider grapheme-cluster
// taxonomy: ZWJ-joined family glyph, skin-tone modifier, VS16 heart,
// single-codepoint emoji. Each cluster must consume exactly its rendered
// width when wrapText decides where to break, so that a long mixed-emoji
// body wraps without overflowing the column.
func TestCommentsBodyHandlesMixedEmoji(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.DiffCursor.Line = 4
	m.paneWidthComments = 30

	emoji := "\U0001F1EF\U0001F1F5❤️\U0001F468‍\U0001F4BB\U0001F44B\U0001F3FD\U0001F389"
	m.state.PR.Comments[2].Body = strings.Repeat(emoji, 8)

	got := m.commentsView()
	for _, row := range strings.Split(got, "\n") {
		if !strings.ContainsAny(row, "\U0001F1EF❤\U0001F468\U0001F44B\U0001F389") {
			continue
		}
		if w := lipgloss.Width(row); w > m.paneWidthComments {
			t.Errorf("mixed-emoji row exceeds paneWidthComments=%d (display width %d): %q",
				m.paneWidthComments, w, row)
		}
	}
}

// TestCommentsBodyKeepsParagraphBreak pins that a blank line between
// content (\n\n) survives the soft-break collapse — true paragraphs stay
// on separate rows.
func TestCommentsBodyKeepsParagraphBreak(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.DiffCursor.Line = 4
	m.paneWidthComments = 60

	m.state.PR.Comments[2].Body = "first paragraph\n\nsecond paragraph"

	got := m.commentsView()
	rows := strings.Split(got, "\n")
	firstIdx, secondIdx := -1, -1
	for i, r := range rows {
		if strings.Contains(r, "first paragraph") {
			firstIdx = i
		}
		if strings.Contains(r, "second paragraph") {
			secondIdx = i
		}
	}
	if firstIdx < 0 || secondIdx < 0 {
		t.Fatalf("both paragraphs must appear; got:\n%s", got)
	}
	if secondIdx-firstIdx < 2 {
		t.Errorf("a blank row must separate paragraphs (firstIdx=%d, secondIdx=%d); got:\n%s",
			firstIdx, secondIdx, got)
	}
}

// Range tag (`R<start>-<end>` for same-side, `L<start>-R<end>` for
// mixed) sits in the Comments column header between the commit hash
// and the trailing `#<id>` / `[pending]` / `[outdated]` slots. Multi-
// line range comments used to be conveyed by ┌ / │ glyphs running down
// the diff gutter; those collided with neighbouring ◆ anchors and the
// markerRank precedence forced the diamond to win the slot, hiding the
// range shape. The text tag carries the same information without
// fighting for gutter columns.
func TestCommentsHeaderShowsSameSideRange(t *testing.T) {
	m := commentsModelFixture(t)
	// Promote dave (T2, single-line at newLine 4) into a same-side range
	// RIGHT 2 → 4. Cursor on the end-anchor buffer index (4).
	root := m.state.PR.Comments[2]
	root.Side = "RIGHT"
	root.StartLine = 2
	root.StartSide = "RIGHT"
	m.state.DiffCursor.Line = 4
	m.state.DiffCursor.Side = model.DiffSideRight

	got := m.commentsView()
	if !strings.Contains(got, "R2-R4") {
		t.Errorf("expected header to carry range tag R2-R4; got:\n%s", got)
	}
}

func TestCommentsHeaderShowsMixedSideRange(t *testing.T) {
	m := commentsLeftSideFixture(t)
	// Promote the RIGHT comment (id=11) into a mixed-side range:
	// LEFT oldLine 2 → RIGHT newLine 2. End is RIGHT → anchor lives on
	// buffer 3 (the `+added_line2` row).
	rc := m.state.PR.Comments[1]
	rc.StartLine = 2
	rc.StartSide = "LEFT"
	m.state.DiffCursor.Line = 3
	m.state.DiffCursor.Side = model.DiffSideRight

	got := m.commentsView()
	if !strings.Contains(got, "L2-R2") {
		t.Errorf("expected mixed-side range tag L2-R2; got:\n%s", got)
	}
}

func TestCommentsHeaderOmitsRangeForSingleLine(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.DiffCursor.Line = 4 // dave (T2) is single-line — no StartLine
	m.state.DiffCursor.Side = model.DiffSideRight

	got := m.commentsView()
	if strings.Contains(got, "R4-") || strings.Contains(got, "-R4") {
		t.Errorf("single-line comment must NOT carry a range tag; got:\n%s", got)
	}
}

func TestCommentsHeaderUsesOriginalStartLineFallback(t *testing.T) {
	m := commentsModelFixture(t)
	root := m.state.PR.Comments[2]
	root.Side = "RIGHT"
	// Outdated comment shape: live StartLine zeroed, historical start
	// preserved in OriginalStartLine — the renderer should fall back to
	// the original the same way `Line` falls back to `OriginalLine`.
	root.StartLine = 0
	root.OriginalStartLine = 2
	root.StartSide = "RIGHT"
	m.state.DiffCursor.Line = 4
	m.state.DiffCursor.Side = model.DiffSideRight

	got := m.commentsView()
	if !strings.Contains(got, "R2-R4") {
		t.Errorf("range tag must fall back to OriginalStartLine; got:\n%s", got)
	}
}

// TestCommentsReplyDeeperIndent pins that the reply header is indented
// further than the root header, matching the user's format example.
func TestCommentsReplyDeeperIndent(t *testing.T) {
	m := commentsModelFixture(t)
	m.state.DiffCursor.Line = 2

	got := m.commentsView()
	lines := strings.Split(got, "\n")
	rootIdx, replyIdx := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "carol:") && rootIdx < 0 {
			rootIdx = i
		}
		if strings.Contains(l, "alice:") && replyIdx < 0 {
			replyIdx = i
		}
	}
	if rootIdx < 0 || replyIdx < 0 {
		t.Fatalf("expected both root (carol) and reply (alice):\n%s", got)
	}
	indentOf := func(l string) int {
		// Strip the styledCursor prefix (always 2 cells) before measuring.
		// Cursor isn't applied in Ascii color profile, so it's literally "  " or "> ".
		stripped := strings.TrimPrefix(l, "> ")
		stripped = strings.TrimPrefix(stripped, "  ")
		n := 0
		for _, r := range stripped {
			if r != ' ' {
				break
			}
			n++
		}
		return n
	}
	if indentOf(lines[replyIdx]) <= indentOf(lines[rootIdx]) {
		t.Errorf("reply header must be indented deeper than root header; root=%q reply=%q",
			lines[rootIdx], lines[replyIdx])
	}
}
