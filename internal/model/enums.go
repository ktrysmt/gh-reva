package model

// AllFilesPath is the synthetic SelectedFile value that means "no single
// file — render the concatenated diff of every file". It sits at cursor
// index 0 of the Files pane as the symmetric counterpart of the "All
// commits" virtual row in the Commits pane. The literal value uses
// reserved NUL bytes so it cannot collide with any real path on any
// filesystem GitHub serves.
const AllFilesPath = "\x00ALL_FILES\x00"

type ChangeKind int

const (
	ChangeAdded ChangeKind = iota
	ChangeModified
	ChangeDeleted
	ChangeRenamed
)

type CommitRangeKind int

const (
	RangeWholePR CommitRangeKind = iota
	RangeSingleCommit
)

type CommitRange struct {
	Kind CommitRangeKind
	SHA  string
}

type DiffViewMode int

const (
	DiffViewSplit DiffViewMode = iota
	DiffViewUnified
)

type PaneID int

const (
	PaneFiles PaneID = iota + 1
	PaneCommits
	PaneDiff
	PaneComments
)
