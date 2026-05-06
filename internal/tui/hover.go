package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// hoverMaxLines caps the popup body so a pathologically long commit
// message cannot eat the screen. 12 rows fits a typical "subject + bullet
// list" body without dominating the layout.
const hoverMaxLines = 12

// hoverEligible answers whether the popup should be drawn given the
// current focus / mode. The popup is toggled on / off explicitly via
// `<space>` in Files / Commits; visual mode suppresses it because the
// visual banner already carries focus + a row-wide bg highlight.
func (m Model) hoverEligible() bool {
	if !m.state.Hover.Show {
		return false
	}
	if m.state.Visual != nil {
		return false
	}
	return m.state.FocusedPane == model.PaneFiles || m.state.FocusedPane == model.PaneCommits
}

// hoverLines returns the popup body. Files = single-line `<path>` (with
// `(N comments)` suffix when the cursor file carries threads); for tree
// rows the path is resolved through `filesTreeRows()` so dir rows surface
// the dir path rather than misindexing into PR.Files. Commits =
// "<sha> <subject>" followed by every body line of the commit message,
// preserving blank lines so bullet-list / paragraph-style descriptions
// render verbatim.
func (m Model) hoverLines() []string {
	if m.state == nil || m.state.PR == nil {
		return nil
	}
	switch m.state.FocusedPane {
	case model.PaneFiles:
		if m.state.FilesTreeMode {
			rows := m.filesTreeRows()
			idx := m.state.FilesCursor
			if idx < 0 || idx >= len(rows) {
				return nil
			}
			r := rows[idx]
			switch r.Kind {
			case model.FilesRowDir:
				return []string{r.Path + "/"}
			case model.FilesRowFile:
				if r.FileIndex < 0 || r.FileIndex >= len(m.state.PR.Files) {
					return nil
				}
				return []string{filesHoverLine(m.state.PR.Files[r.FileIndex])}
			}
			return nil
		}
		idx := m.state.FilesCursor
		files := m.state.PR.Files
		if idx < 0 || idx >= len(files) {
			return nil
		}
		return []string{filesHoverLine(files[idx])}
	case model.PaneCommits:
		// Index 0 is the synthetic "All commits" row — there is no per-commit
		// summary to surface, so the popup is suppressed.
		commits := m.visibleCommits()
		idx := m.state.CommitsCursor
		if idx <= 0 || idx > len(commits) {
			return nil
		}
		c := commits[idx-1]
		subject, body := splitCommitMessage(c.Message)
		// Color the SHA so the popup mirrors the Commits row's syntax
		// highlighting; the subject stays uncolored to match the row.
		out := []string{fg(shortSHA(c.SHA), m.theme.CommitSHA) + " " + subject}
		out = append(out, body...)
		return out
	}
	return nil
}

// filesHoverLine formats one Files-pane popup body row. The popup mirrors
// the row's path and appends an explicit `(N comments)` count when the
// file carries threads — narrow row content (`(N)`) gets clobbered by
// long paths, the popup has the room to spell it out.
func filesHoverLine(f *model.FileEntry) string {
	if f == nil {
		return ""
	}
	if f.CommentCount <= 0 {
		return f.Path
	}
	word := "comments"
	if f.CommentCount == 1 {
		word = "comment"
	}
	return fmt.Sprintf("%s (%d %s)", f.Path, f.CommentCount, word)
}

// splitCommitMessage divides a git commit message into its subject (first
// line) and body lines. The conventional blank separator between subject
// and body is dropped; subsequent blank lines inside the body are kept so
// paragraph structure survives.
func splitCommitMessage(msg string) (subject string, body []string) {
	msg = strings.TrimRight(msg, "\n")
	idx := strings.IndexByte(msg, '\n')
	if idx < 0 {
		return msg, nil
	}
	subject = msg[:idx]
	rest := strings.TrimLeft(msg[idx+1:], "\n")
	if rest == "" {
		return subject, nil
	}
	return subject, strings.Split(rest, "\n")
}

// hoverCursorRow returns the screen row index of the focused pane's
// cursor, or -1 when the cursor is not visible (off-screen, wrong pane,
// degenerate window). The math mirrors the View() column layout:
// pane top row + 1 (top border) + 1 (title) + 1 (divider) + intra-pane
// cursor index. Files / Commits do not scroll, so a cursor beyond the
// pane's content rows is simply hidden by the box renderer; we bail in
// that case so the popup never points at empty space.
func (m Model) hoverCursorRow() int {
	if m.height <= 0 || m.width <= 0 {
		return -1
	}
	bodyHeight := m.height
	if m.state.Visual != nil {
		bodyHeight--
	}
	topH, bottomH := splitColumnHeights(bodyHeight)
	switch m.state.FocusedPane {
	case model.PaneFiles:
		row := 3 + m.state.FilesCursor
		if row >= topH-1 {
			return -1
		}
		return row
	case model.PaneCommits:
		row := topH + 3 + m.state.CommitsCursor
		if row >= topH+bottomH-1 {
			return -1
		}
		return row
	}
	return -1
}

// hoverLayout decides where to draw the popup and how many body lines to
// show. Position priority is: above the cursor (the natural reading
// direction once the user has pressed j to land on a row), then below
// (if there is no room above), then "best effort" — pick the side with
// more vertical space and shrink the popup to fit. The horizontal anchor
// is the column where the cursor row's path / subject text begins, so
// the popup hovers directly above the text it mirrors rather than across
// the rest of the screen.
func (m Model) hoverLayout() (lines []string, top, left, width int, ok bool) {
	rawLines := m.hoverLines()
	if len(rawLines) == 0 {
		return nil, 0, 0, 0, false
	}
	cursorRow := m.hoverCursorRow()
	if cursorRow < 0 {
		return nil, 0, 0, 0, false
	}
	left = m.hoverAnchorCol()
	if left < 0 || left >= m.width {
		return nil, 0, 0, 0, false
	}

	contentW := 0
	for _, ln := range rawLines {
		if w := lipgloss.Width(ln); w > contentW {
			contentW = w
		}
	}
	if contentW < 1 {
		contentW = 1
	}
	width = contentW + 2
	if max := m.width - left; width > max {
		width = max
	}
	if width < 4 {
		return nil, 0, 0, 0, false
	}

	spaceAbove := cursorRow
	spaceBelow := m.height - cursorRow - 1
	if m.state.Visual != nil {
		spaceBelow--
	}

	want := len(rawLines)
	if want > hoverMaxLines {
		want = hoverMaxLines
	}
	contentN := want
	height := contentN + 2

	// Prefer above when it fits.
	if height <= spaceAbove {
		top = cursorRow - height
		return rawLines[:contentN], top, left, width, true
	}
	if height <= spaceBelow {
		top = cursorRow + 1
		return rawLines[:contentN], top, left, width, true
	}

	// Neither side fits the full popup — pick the larger side and
	// shrink content to fit. We still need at least one content row
	// (so spaceN-2 >= 1) for the popup to be useful.
	pickAbove := spaceAbove >= spaceBelow
	avail := spaceAbove
	if !pickAbove {
		avail = spaceBelow
	}
	contentN = avail - 2
	if contentN < 1 {
		return nil, 0, 0, 0, false
	}
	height = contentN + 2
	if pickAbove {
		top = cursorRow - height
	} else {
		top = cursorRow + 1
	}
	return rawLines[:contentN], top, left, width, true
}

// hoverAnchorCol returns the absolute screen column where the popup's
// left border (`│` / `┌` / `└`) should land. Both Files and Commits live
// in the leftmost column so the leading `│` is at screen col 0; the
// anchor is therefore measured from screen col 0.
//
//	Files flat: │(1) + cursor(2) + ` `(1) + status(1) + ` `(1) → col 6
//	            (popup left lands at the path column)
//	Files tree: same prefix + 2*depth indent + marker(2)/(3)
//	Commits:    │(1) + cursor(2) + annotation(4) → col 7
//	            (popup left lands at the SHA column so the popup body's
//	            `<sha> <subject>` lines up with the row below)
func (m Model) hoverAnchorCol() int {
	switch m.state.FocusedPane {
	case model.PaneFiles:
		if m.state.FilesTreeMode {
			rows := m.filesTreeRows()
			idx := m.state.FilesCursor
			if idx < 0 || idx >= len(rows) {
				return 6
			}
			r := rows[idx]
			base := 1 + 2 + 2*r.Depth
			if r.Kind == model.FilesRowDir {
				return base + 2
			}
			return base + 3
		}
		return 6
	case model.PaneCommits:
		// Border + cursor(2) + annotation(4) = 7 — anchored at the SHA
		// column so the popup body's `<sha> <subject>` lines up exactly
		// with the Commits row below it.
		return 7
	}
	return 0
}

// renderHoverPopupBlock builds the bordered tooltip box. width is the
// outer width including borders; inner content area is width-2.
func (m Model) renderHoverPopupBlock(lines []string, width int) string {
	if len(lines) == 0 {
		return ""
	}
	innerW := atLeast(width-2, 1)
	bar := strings.Repeat("─", innerW)
	border := m.theme.PaneBorderActive
	side := fg("│", border)
	var sb strings.Builder
	sb.WriteString(fg("┌"+bar+"┐", border) + "\n")
	for _, ln := range lines {
		sb.WriteString(side + padTrunc(ln, innerW) + side + "\n")
	}
	sb.WriteString(fg("└"+bar+"┘", border))
	return sb.String()
}

// overlayHover splices the popup over the body at the rectangle returned
// by hoverLayout. Each popup row replaces visible columns
// [left .. left+width) of the corresponding body row; the prefix is
// preserved (active pane chrome) and the suffix is dropped (the popup
// extends to the screen's right edge so nothing significant lives past
// it).
func (m Model) overlayHover(body string) string {
	if !m.hoverEligible() {
		return body
	}
	lines, top, left, width, ok := m.hoverLayout()
	if !ok {
		return body
	}
	popup := m.renderHoverPopupBlock(lines, width)
	if popup == "" {
		return body
	}
	popupRows := strings.Split(popup, "\n")
	bodyRows := strings.Split(body, "\n")
	for i, pr := range popupRows {
		row := top + i
		if row < 0 || row >= len(bodyRows) {
			continue
		}
		bodyRows[row] = spliceMid(bodyRows[row], pr, left, width)
	}
	return strings.Join(bodyRows, "\n")
}

// spliceMid returns line with visible columns [col, col+width) replaced
// by replacement, preserving both the prefix [0, col) and the suffix
// [col+width, end). The prefix is padded with plain spaces when the
// original line was shorter than col cells so the popup never collides
// with leftover content. SGR codes are kept intact in both prefix and
// suffix via ansi.Truncate / ansi.TruncateLeft.
func spliceMid(line, replacement string, col, width int) string {
	leftPart := ""
	if col > 0 {
		leftPart = ansi.Truncate(line, col, "")
		if w := lipgloss.Width(leftPart); w < col {
			leftPart += strings.Repeat(" ", col-w)
		}
	}
	rightPart := ansi.TruncateLeft(line, col+width, "")
	return leftPart + replacement + rightPart
}
