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

func NewAppState() *AppState {
	return &AppState{
		FocusedPane:  PaneFiles,
		DiffViewMode: DiffViewSplit,
		FoldedDirs:   map[string]bool{},
		DiffCache:    map[string]string{},
		Loading:      map[string]bool{},
	}
}
