package model

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
