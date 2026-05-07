package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// statusBarRows is the on-screen footprint of the status bar: one
// content row (per-pane keymap + URL) plus one blank row of breathing
// space below. View() reserves this many rows from m.height before
// computing the body layout. The bar carries no border glyphs — the
// blank row replaces what used to be the bottom border, and the body's
// own bottom border serves as the visual separator above.
const statusBarRows = 2

// Per-context status bar hints. Kept as bare top-level constants (not a
// map) so a `git grep` for any of these strings lands on the canonical
// definition. Format: `key:label` separated by single spaces; multiple
// alternatives in one slot use `/` (e.g. `j/k`, `H/M/L`).
const (
	hintFilesFlat     = "j/k:move /:search space:zoom t:tree"
	hintFilesTree     = "j/k:move /:search enter:fold t:tree"
	hintCommits       = "j/k:move /:search space:zoom"
	hintDiff          = "j/k:move H/M/L:viewport gg/G:top/bottom /:search space:split enter:comment"
	hintComments      = "j/k:move space:zoom enter:edit r:reply"
	hintVisual        = "-- VISUAL --  y:yank esc/ctrl+c:cancel"
	hintModal         = "space/esc/q/ctrl+c:close"
	hintModalComments = "enter:edit r:reply space/esc/q/ctrl+c:close"
	hintHelp          = "?/esc/q:close"

	// Search hints. Editing is the typing phase; Active is post-Enter
	// where n/N cycle and a fresh `/` re-enters Editing.
	hintSearchEditing = "type query  enter:confirm  esc:cancel"
	hintSearchActive  = "n:next  N:prev  /:edit  esc:clear"

	// Compose state hints. Editing covers the textarea fallback and
	// the brief Editing→Submitting transition for the $EDITOR path.
	// Submitting reflects the in-flight GraphQL POST; Failed lets the
	// user retry without re-typing.
	hintComposeEditing    = "ctrl+s:save  esc:cancel"
	hintComposeExternal   = "editing in $EDITOR — finish there to continue"
	hintComposeSubmitting = "posting to GitHub…"
	hintComposeFailed     = "ctrl+s:retry  esc:cancel"

	// Confirm prompts: shown while a built compose payload is parked in
	// PendingConfirm awaiting `[y]es / [n]o`. Each compose kind gets its
	// own verb so the user sees what they are about to commit.
	hintConfirmInline = "start new comment? [y]es [n]o"
	hintConfirmReply  = "post reply? [y]es [n]o"
	hintConfirmEdit   = "edit comment? [y]es [n]o"

	// statusCommonSuffix is the navigation hint group appended to the
	// per-pane context in normal mode. It lives on the LEFT (joined to
	// the context with two spaces) so the right side is reserved for
	// the PR URL — the renderer in composeStatusBar drops the suffix
	// (not the URL) when the bar gets tight, since `?:help`/`q:quit`
	// remain discoverable via convention while a missing URL would
	// silently strand the user without a way to copy/share the PR
	// reference.
	statusCommonSuffix = "tab/shift+tab:pane J/K:file ctrl+e:comments ?:help q:quit"
)

// statusBar returns the 2-row borderless status block (keymap / URL
// content row + a blank row of breathing space below) joined by "\n".
// Returns an empty string when the terminal is too small to fit the
// bar plus at least one body row above it; callers should skip
// emitting the trailing newline in that case so the body retains its
// full height. composeStatusBar gets the full m.width budget — there
// are no side `│` glyphs to subtract.
func (m Model) statusBar() string {
	if m.width <= 0 || m.height <= statusBarRows {
		return ""
	}
	context, suffix := m.statusBarContent()
	left := context
	if suffix != "" {
		left = context + "  " + suffix
	}
	leftMin := context
	var urls []string
	if m.target != nil {
		urls = m.target.PRShortForms()
	}
	body := composeStatusBar(left, leftMin, urls, m.width, m.theme.PaneTitle)
	blank := strings.Repeat(" ", m.width)
	return body + "\n" + blank
}

// statusBarContent picks the context hint and (optionally) common suffix
// based on global mode flags. Compose / visual / modal / help all replace
// the context AND drop the suffix — those modes have their own narrow
// keymap surface, so showing the normal-mode suffix would mislead.
func (m Model) statusBarContent() (string, string) {
	// Transient notice (e.g. "cannot edit comments by other users")
	// takes precedence over the per-pane keymap so the user cannot miss
	// it. Cleared by handleKey on the next keystroke; see #notice in
	// state.go.
	if m.state.Notice != "" {
		return m.state.Notice, ""
	}
	// PendingConfirm is checked ahead of Compose because the parked
	// payload has been moved out of m.state.Compose into PendingConfirm
	// while the prompt is up — Compose stays nil until the user
	// presses `y`. Suffix is dropped so the prompt fills the slot
	// without competing with `?:help`/`q:quit` hints.
	if pc := m.state.PendingConfirm; pc != nil {
		switch pc.Kind {
		case model.ComposeReply:
			return hintConfirmReply, ""
		case model.ComposeEdit:
			return hintConfirmEdit, ""
		default:
			return hintConfirmInline, ""
		}
	}
	if cs := m.state.Compose; cs != nil {
		switch cs.Status {
		case model.ComposeSubmitting:
			return hintComposeSubmitting, ""
		case model.ComposeFailed:
			return hintComposeFailed, ""
		default:
			if cs.UseTextarea {
				return hintComposeEditing, ""
			}
			return hintComposeExternal, ""
		}
	}
	if m.state.HelpOpen {
		return hintHelp, ""
	}
	// Search Editing replaces the context with the live query so the user
	// sees what they typed; Active swaps to the n/N hint set. Suffix is
	// dropped in both so the prompt does not compete with `?:help` etc.
	if s := m.state.Search; s != nil {
		if s.Status == model.SearchEditing {
			return "/" + s.Query + "_", ""
		}
		count := len(s.Matches)
		idx := s.CursorIdx + 1
		if count == 0 {
			idx = 0
		}
		return hintSearchActive + "  [" + strconv.Itoa(idx) + "/" + strconv.Itoa(count) + "] /" + s.Query, ""
	}
	if m.state.Visual != nil {
		return hintVisual, ""
	}
	if m.state.Modal != nil {
		// The Comments modal is more than a read-only zoom view — Enter
		// edits the cursor comment and `r` replies, so the per-pane
		// keymap stays meaningful inside the zoom. Surface those hints
		// alongside the close gesture set; other panes' modals don't
		// expose any pane action and keep the close-only hint.
		if m.state.Modal.Pane == model.PaneComments {
			return hintModalComments, ""
		}
		return hintModal, ""
	}
	switch m.state.FocusedPane {
	case model.PaneFiles:
		if m.state.FilesTreeMode {
			return hintFilesTree, statusCommonSuffix
		}
		return hintFilesFlat, statusCommonSuffix
	case model.PaneCommits:
		return hintCommits, statusCommonSuffix
	case model.PaneDiff:
		return hintDiff, statusCommonSuffix
	case model.PaneComments:
		return hintComments, statusCommonSuffix
	}
	return "", statusCommonSuffix
}

// composeStatusBar assembles the bar:
//
//	" <left>     <middle pad>     <url> "
//
// `leftFull` is the context + common suffix; `leftMin` is the context
// alone (used when the combined left + URL does not fit even at the
// shortest URL form). `urls` is the URL ladder from longest to
// shortest — composeStatusBar picks the longest URL form that fits.
//
// Priority order (highest to lowest):
//
//  1. Show context.
//  2. Show URL (shrink through the ladder before sacrificing other elements).
//  3. Show common suffix (drop it before dropping the URL or the context).
//
// Behaviour summary:
//
//   - Both fit: full layout with longest-fitting URL form.
//   - Only context + URL fit: drop the suffix.
//   - Even shortest URL does not fit alongside context: drop the URL,
//     left-pad context across the whole bar, truncate with `…` if context
//     itself overflows.
func composeStatusBar(leftFull, leftMin string, urls []string, width int, color lipgloss.Color) string {
	if width <= 0 {
		return ""
	}
	const sidePad = 2
	const minGap = 3
	leftFullW := lipgloss.Width(leftFull)
	leftMinW := lipgloss.Width(leftMin)

	// Pass 1: keep the suffix; pick the longest URL form that fits.
	if leftFull != leftMin {
		for _, u := range urls {
			uw := lipgloss.Width(u)
			if sidePad+leftFullW+minGap+uw+sidePad <= width {
				return renderBar(leftFull, leftFullW, u, uw, width, sidePad, color)
			}
		}
	}
	// Pass 2: drop the suffix; pick the longest URL form that fits.
	for _, u := range urls {
		uw := lipgloss.Width(u)
		if sidePad+leftMinW+minGap+uw+sidePad <= width {
			return renderBar(leftMin, leftMinW, u, uw, width, sidePad, color)
		}
	}
	// No URL fits even at shortest. Drop URL; pad the context-only bar.
	innerMax := width - 2*sidePad
	if innerMax < 1 {
		return strings.Repeat(" ", width)
	}
	if leftMinW > innerMax {
		leftMin = ansi.Truncate(leftMin, innerMax-1, "") + "…"
		leftMinW = lipgloss.Width(leftMin)
	}
	bar := strings.Repeat(" ", sidePad) + leftMin + strings.Repeat(" ", innerMax-leftMinW) + strings.Repeat(" ", sidePad)
	return fg(bar, color)
}

func renderBar(left string, leftW int, url string, urlW, width, sidePad int, color lipgloss.Color) string {
	gap := width - sidePad - leftW - urlW - sidePad
	bar := strings.Repeat(" ", sidePad) + left + strings.Repeat(" ", gap) + url + strings.Repeat(" ", sidePad)
	return fg(bar, color)
}
