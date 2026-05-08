package model

type AppState struct {
	PR *PR

	// ViewerLogin is the authenticated GitHub user's login, populated
	// during the load sequence via Client.ViewerLogin. Used by the
	// Comments-pane Enter dispatch to gate the "edit own comment" path
	// (vs the "reply only on others" hint). Empty until the first
	// successful viewer fetch — the Comments pane treats empty as
	// "ownership unknown, fall back to reply" so an early Enter before
	// the viewer arrives degrades safely instead of misrouting to edit.
	ViewerLogin string

	FocusedPane PaneID

	SelectedFile  string
	SelectedRange CommitRange
	DiffViewMode  DiffViewMode

	FilesCursor    int
	CommitsCursor  int
	DiffCursor     DiffCursor
	DiffViewport   DiffViewport
	CommentsCursor int

	FilesTreeMode   bool
	FoldedDirs      map[string]bool
	FilesViewIndex  []FilesRow

	// CommentsHidden hides the right (Comments) pane and lets the Diff
	// column take its width. Toggled by Ctrl+E. Hiding while focus is on
	// Comments shifts FocusedPane to Diff (handled in keys.go); Tab /
	// Shift+Tab skip Comments while it is hidden so the cycle stays
	// consistent. Diff Enter on a row carrying threads auto-reveals the
	// pane before the modal handoff so the close-restore-focus contract
	// does not strand focus on an invisible pane.
	CommentsHidden bool

	Visual *VisualState

	Modal *ModalState

	// Compose drives the PR-comment input flow (Diff Enter / Comments
	// Enter). Non-nil while the user is composing, submitting, or viewing
	// a submission failure; nil otherwise. The handleKey dispatcher in
	// keys.go absorbs keystrokes when Compose != nil && UseTextarea so
	// the textarea owns input; the $EDITOR path returns to nil control
	// the moment tea.ExecProcess exits and the resulting message is
	// processed.
	Compose *ComposeState

	// PendingConfirm holds a built-but-not-yet-started Compose payload
	// while the user is shown a `[y]es / [n]o` prompt. The actual editor
	// launch is deferred until the user presses `y`; `n` / `Esc` / `q` /
	// `Ctrl+C` discard the payload. While PendingConfirm is non-nil the
	// keystroke router routes every key through handleKeyConfirm so
	// background panes are frozen and the prompt cannot be missed.
	// Compose stays nil during the confirm step — only on `y` does the
	// payload move into Compose and the editor / textarea start.
	PendingConfirm *PendingConfirm

	HelpOpen bool

	// Notice is a transient single-line message shown in the status bar
	// (replacing the per-pane context, suffix dropped). Set by handlers
	// that need to surface a soft warning — e.g. "Comments Enter on a
	// foreign user's comment is reply-only" — and cleared by the next
	// keystroke at the top of handleKey. Empty string means "no notice".
	Notice string

	// PendingPrefix holds a global key prefix awaiting completion (vim-style).
	// Currently only `g` is recorded — a follow-up `g` completes `gg` (gotoTop)
	// in whichever pane currently has focus; any other key cancels the prefix
	// and dispatches as usual. The slot is forward-compatible with future
	// `gd` / `gh` / `gb` style mappings. Cleared on focus change, visual
	// entry/exit, help open, and any other key that resolves the sequence.
	PendingPrefix string

	// Search drives the global `/` substring search. Editing collects the
	// query rune-by-rune (incremental); Active is post-Enter and exposes
	// `n` / `N` to cycle. Each search is scoped to the pane that was
	// focused when `/` was pressed (TargetPane); the saved cursor fields
	// let Esc restore the pre-search state on cancel.
	Search *SearchState

	DiffCache map[string]string
	Loading   map[string]bool

	LoadStage LoadStage
	LoadFrame int
}

type DiffCursor struct {
	Line int
}

type DiffViewport struct {
	Top    int
	Height int
}

// LoadStage tracks whether the PR data is still being fetched. The
// pre-parallel-load enum had per-API stages (metadata / commits / files
// / comments / diffs) so the loader could update the spinner caption
// after each tea.Sequence step. loadPRCmd now fans those reads out
// concurrently and emits a single PRLoadedMsg, so the only useful
// distinction left is "still loading" vs "done"; LoadStagePR is
// retained as the default value (loadingView callers in tests still
// pass it) and LoadStageDone gates SpinnerTickMsg's re-tick.
type LoadStage int

const (
	LoadStagePR LoadStage = iota
	LoadStageDone
)

// FilesRow describes one rendered row in the Files pane. When TreeMode is
// false, every row is a Kind=FilesRowFile entry. When TreeMode is true, dirs
// (Kind=FilesRowDir) intersperse the files and may be folded.
type FilesRow struct {
	Kind  FilesRowKind
	Depth int
	// File: full path. Dir: dir path (without trailing slash).
	Path  string
	// File: index into PR.Files. Dir: -1.
	FileIndex int
}

type FilesRowKind int

const (
	FilesRowFile FilesRowKind = iota
	FilesRowDir
)

type VisualState struct {
	OriginPane PaneID
	Anchor     int
	AnchorLine int
	Linewise   bool
}

// ModalState drives the centered "zoom" modal opened by `<space>` in the
// Files / Commits / Comments panes. While Modal is non-nil, the active
// pane's content is also rendered inside a centered popup at a wider
// budget (paths unwrapped, comments wrap relaxed). j/k inside the modal
// goes through the regular pane handlers, so navigation propagates to the
// underlying main state and the modal closes onto the same row. Tab,
// Shift-Tab, Esc, and `?` all close the modal; Diff `<space>` is
// untouched (split⇄unified).
//
//   - Pane    : the pane the modal is showing.
//   - Origin  : the pane the user was on when the modal opened. Most close
//     gestures (space, q, Esc, Ctrl+C) restore focus to Origin so the
//     user lands back where they started — relevant when the modal was
//     opened via a handoff (Diff Enter on a commented row → Comments
//     modal with Origin=Diff): without Origin restore, focus would
//     linger on Comments after close. Tab / Shift-Tab close the modal
//     too but advance focus from Origin instead of restoring to it.
type ModalState struct {
	Pane   PaneID
	Origin PaneID
}

// ComposeKind tags which GraphQL mutation a ComposeState resolves to:
//
//   - ComposeInline → addPullRequestReviewThread on the PR's pending
//     review (created on demand). Path / CommitSHA / Line / Side
//     (and StartLine / StartSide for ranges) describe the anchor.
//   - ComposeReply → addPullRequestReviewThreadReply on the parent
//     thread. ParentThreadID is the GraphQL node ID of the thread
//     under the cursor — the only field needed by the reply mutation.
//   - ComposeEdit → updatePullRequestReviewComment on a single
//     existing comment (Pending or public, viewer-authored only).
//     EditCommentNodeID is the comment's GraphQL node ID. The pre-edit
//     body is preloaded into the editor / textarea so the user starts
//     from the existing text instead of a blank buffer.
type ComposeKind int

const (
	ComposeInline ComposeKind = iota
	ComposeReply
	ComposeEdit
)

// ComposeStatus is the lifecycle of one Compose attempt:
//   - Editing: body is being collected (editor or textarea).
//   - Submitting: the GraphQL POST is in flight.
//   - Failed: the POST returned an error; Body and ErrMsg are
//     preserved so Ctrl+S retries without re-typing.
//
// A successful POST clears Compose entirely (no Succeeded state);
// the returned ReviewComment is appended to PR.Comments with
// Pending=true and the user is back in normal navigation.
type ComposeStatus int

const (
	ComposeEditing ComposeStatus = iota
	ComposeSubmitting
	ComposeFailed
)

// PendingConfirm is the parking state for a built ComposeState while
// the user is shown a `[y]es / [n]o` prompt. Holding the built payload
// here (instead of in Compose) keeps the global Compose absorber from
// engaging — that absorber routes every key through the textarea, which
// would intercept the `y` / `n` we need for the confirm dispatch. On
// `y` the payload moves into AppState.Compose and the editor / textarea
// starts; on `n` / `Esc` / `q` / `Ctrl+C` the payload is discarded.
//
// Kind duplicates Compose.Kind so the status-bar prompt can pick the
// right label without dereferencing Compose (e.g. "start new comment?"
// vs "post reply?" vs "edit comment?").
type PendingConfirm struct {
	Kind    ComposeKind
	Compose *ComposeState
}

// SearchStatus is the lifecycle of a `/` search session.
//
//   - SearchEditing: query is being typed. Every keystroke updates the
//     Query and recomputes Matches, jumping the cursor to the first match
//     (vim-style incsearch).
//   - SearchActive: query committed via Enter. `n` / `N` cycle through
//     Matches; further `/` re-enters Editing.
type SearchStatus int

const (
	SearchEditing SearchStatus = iota
	SearchActive
)

// SearchMatch carries one matched row in TargetPane's content. Index is a
// pane-local row index (Files: row idx; Commits: cursor idx 0..len(commits);
// Diff: buffer line idx; Comments: flatComments idx).
type SearchMatch struct {
	Index int
}

// SearchState drives the global `/` search. Saved cursor fields capture
// the pre-search position so Esc can restore it on cancel.
type SearchState struct {
	Status     SearchStatus
	Query      string
	TargetPane PaneID

	Matches   []SearchMatch
	CursorIdx int

	SavedFilesCursor     int
	SavedCommitsCursor   int
	SavedDiffCursor      DiffCursor
	SavedDiffViewportTop int
	SavedCommentsCursor  int
	SavedSelectedFile    string
	SavedSelectedRange   CommitRange
	SavedFocusedPane     PaneID
}

// ComposeState is the in-flight state of a comment-input session.
type ComposeState struct {
	Kind        ComposeKind
	Status      ComposeStatus
	UseTextarea bool

	// Inline target (Kind == ComposeInline).
	Path      string
	CommitSHA string
	Line      int
	Side      string
	StartLine *int
	StartSide string

	// Reply target (Kind == ComposeReply).
	ParentThreadID string

	// Edit target (Kind == ComposeEdit). Comment NodeID is the GraphQL
	// identity of the comment whose body is being rewritten.
	EditCommentNodeID string

	Body   string
	ErrMsg string
}

func NewAppState() *AppState {
	return &AppState{
		FocusedPane:  PaneFiles,
		DiffViewMode: DiffViewSplit,
		FoldedDirs:   map[string]bool{},
		DiffCache:    map[string]string{},
		Loading:      map[string]bool{},
	}
}

