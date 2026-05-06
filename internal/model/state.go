package model

type AppState struct {
	PR *PR

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

	// SubmitReview is non-nil while the submit-review modal is open
	// (key `R`) or while submitPullRequestReview is in flight. The
	// modal asks the user to pick an event (approve / comment /
	// request_changes) for their pending review. handleKey absorbs all
	// keystrokes while SubmitReview != nil.
	SubmitReview *SubmitReviewState

	HelpOpen bool

	// DiffPendingPrefix holds a pane-scoped key prefix awaiting completion
	// (vim-style). Currently only `g` is recorded — a follow-up `g` completes
	// `gg` (gotoTop); any other key cancels the prefix and dispatches as
	// usual. The slot is forward-compatible with future `gd` / `gh` / `gb`
	// style mappings inside Diff. Cleared on focus change, visual entry/exit,
	// help open, and any other key that resolves the sequence.
	DiffPendingPrefix string

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

type LoadStage int

const (
	LoadStagePR LoadStage = iota
	LoadStageCommits
	LoadStageFiles
	LoadStageComments
	LoadStageDiffs
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
// untouched (split⇄unified). Pane is the pane the modal is showing.
type ModalState struct {
	Pane PaneID
}

// ComposeKind tags which GraphQL mutation a ComposeState resolves to:
//
//   - ComposeInline → addPullRequestReviewThread on the PR's pending
//     review (created on demand). Path / CommitSHA / Line / Side
//     (and StartLine / StartSide for ranges) describe the anchor.
//   - ComposeReply → addPullRequestReviewThreadReply on the parent
//     thread. ParentThreadID is the GraphQL node ID of the thread
//     under the cursor; ParentDBID is the integer DB id of the root
//     comment, kept for InReplyTo round-tripping.
type ComposeKind int

const (
	ComposeInline ComposeKind = iota
	ComposeReply
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

// SubmitEvent is the GitHub PullRequestReviewEvent the user picks
// when finalizing their pending review. Only the three submission
// values are exposed (DISMISS is for dismissing other reviewers'
// reviews via a separate mutation, not relevant here).
type SubmitEvent string

const (
	SubmitApprove        SubmitEvent = "APPROVE"
	SubmitComment        SubmitEvent = "COMMENT"
	SubmitRequestChanges SubmitEvent = "REQUEST_CHANGES"
)

// SubmitReviewStatus tracks the submit-review modal lifecycle:
// Choosing → user picks event; Submitting → GraphQL submit in flight;
// Failed → submit returned an error, modal stays open with retry.
// On success the modal closes and PR.Comments is refetched so the
// just-submitted comments lose their Pending flag.
type SubmitReviewStatus int

const (
	SubmitChoosing SubmitReviewStatus = iota
	SubmitSubmitting
	SubmitFailed
)

// SubmitReviewState is the modal state for the "finish your review"
// flow. PendingCount is captured at modal-open time so the user sees
// what they are about to submit even if they background the modal.
type SubmitReviewState struct {
	Status       SubmitReviewStatus
	PendingCount int
	Event        SubmitEvent
	ErrMsg       string
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
	ParentDBID     int64

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

