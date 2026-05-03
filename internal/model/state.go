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

	ThreadFolded map[int64]bool

	FilesTreeMode   bool
	FoldedDirs      map[string]bool
	FilesViewIndex  []FilesRow

	Visual *VisualState

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

func NewAppState() *AppState {
	return &AppState{
		FocusedPane:  PaneFiles,
		DiffViewMode: DiffViewSplit,
		ThreadFolded: map[int64]bool{},
		FoldedDirs:   map[string]bool{},
		DiffCache:    map[string]string{},
		Loading:      map[string]bool{},
	}
}
