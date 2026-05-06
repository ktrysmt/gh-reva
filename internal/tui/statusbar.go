package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ktrysmt/gh-reva/internal/model"
)

// Per-context status bar hints. Kept as bare top-level constants (not a
// map) so a `git grep` for any of these strings lands on the canonical
// definition. Format: `key:label` separated by single spaces; multiple
// alternatives in one slot use `/` (e.g. `j/k`, `H/M/L`).
const (
	hintFilesFlat = "j/k:move space:zoom t:tree"
	hintFilesTree = "j/k:move enter:fold space:zoom t:tree"
	hintCommits   = "j/k:move space:zoom"
	hintDiff      = "j/k:move H/M/L:viewport gg/G:top/bottom space:split enter:comment"
	hintComments  = "j/k:move space:zoom enter:reply"
	hintVisual    = "-- VISUAL --  y:yank esc/ctrl+c:cancel"
	hintModal     = "space/esc/q/ctrl+c:close"
	hintHelp      = "?/esc/q:close"

	// Compose state hints. Editing covers the textarea fallback and
	// the brief Editing→Submitting transition for the $EDITOR path.
	// Submitting reflects the in-flight GraphQL POST; Failed lets the
	// user retry without re-typing.
	hintComposeEditing    = "ctrl+s:save  esc:cancel"
	hintComposeExternal   = "editing in $EDITOR — finish there to continue"
	hintComposeSubmitting = "posting to GitHub…"
	hintComposeFailed     = "ctrl+s:retry  esc:cancel"

	// Submit-review modal hints. Choose phase prompts the event;
	// Submitting + Failed mirror the compose lifecycle.
	hintSubmitChoosing   = "a:approve  c:comment  r:request-changes  esc:cancel"
	hintSubmitSubmitting = "submitting review…"
	hintSubmitFailed     = "ctrl+s:retry  esc:cancel"

	// statusCommonSuffix is the always-visible right-flushed group shown
	// in normal (non-visual / non-modal / non-help) mode. Truncation rule
	// in composeStatusBar drops it whole when it does not fit alongside
	// the context, rather than splitting it mid-token.
	statusCommonSuffix = "tab:focus J/K:file R:submit ?:help q:quit"
)

// statusBar returns the bottom-row hint string already padded to m.width.
// Returns an empty string when the terminal is too small for a meaningful
// bar; callers should skip emitting the trailing newline in that case so
// the body retains its full height.
func (m Model) statusBar() string {
	if m.width <= 0 || m.height <= 1 {
		return ""
	}
	context, suffix := m.statusBarContent()
	return composeStatusBar(context, suffix, m.width, m.theme.DiffLineNumber)
}

// statusBarContent picks the context hint and (optionally) common suffix
// based on global mode flags. Compose / visual / modal / help all replace
// the context AND drop the suffix — those modes have their own narrow
// keymap surface, so showing the normal-mode suffix would mislead.
func (m Model) statusBarContent() (string, string) {
	if sr := m.state.SubmitReview; sr != nil {
		switch sr.Status {
		case model.SubmitSubmitting:
			return hintSubmitSubmitting, ""
		case model.SubmitFailed:
			return hintSubmitFailed, ""
		default:
			return hintSubmitChoosing, ""
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
	if m.state.Visual != nil {
		return hintVisual, ""
	}
	if m.state.Modal != nil {
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
//	" <context>   <middle pad>   <suffix> "
//
// Truncation: if the combined " context   suffix " does not fit in width,
// the suffix is dropped entirely (no half-truncated suffix — partial
// keymap hints would mislead the reader). If the context still overflows
// after dropping the suffix, the context itself is truncated with `…`
// via ansi.Truncate so any SGR runs stay balanced.
func composeStatusBar(context, suffix string, width int, color lipgloss.Color) string {
	if width <= 0 {
		return ""
	}
	const sidePad = 1
	const minGap = 3
	contextW := lipgloss.Width(context)
	suffixW := lipgloss.Width(suffix)

	// Try the full layout first.
	if suffix != "" && sidePad+contextW+minGap+suffixW+sidePad <= width {
		gap := width - sidePad - contextW - suffixW - sidePad
		bar := strings.Repeat(" ", sidePad) + context + strings.Repeat(" ", gap) + suffix + strings.Repeat(" ", sidePad)
		return fg(bar, color)
	}

	// Suffix dropped (or empty). Pad the context to fill width.
	innerMax := width - 2*sidePad
	if innerMax < 1 {
		return strings.Repeat(" ", width)
	}
	if contextW > innerMax {
		// `…` is 1 cell wide; reserve 1 cell for it.
		context = ansi.Truncate(context, innerMax-1, "") + "…"
		contextW = lipgloss.Width(context)
	}
	bar := strings.Repeat(" ", sidePad) + context + strings.Repeat(" ", innerMax-contextW) + strings.Repeat(" ", sidePad)
	return fg(bar, color)
}
