package model

import "time"

type PR struct {
	Owner    string           `json:"owner"`
	Repo     string           `json:"repo"`
	Number   int              `json:"number"`
	Title    string           `json:"title"`
	BaseSHA  string           `json:"base_sha"`
	HeadSHA  string           `json:"head_sha"`
	Commits  []*Commit        `json:"commits,omitempty"`
	Files    []*FileEntry     `json:"files,omitempty"`
	Comments []*ReviewComment `json:"comments,omitempty"`
}

type Commit struct {
	SHA          string                `json:"sha"`
	ShortSHA     string                `json:"short_sha"`
	Message      string                `json:"message"`
	Author       string                `json:"author"`
	Date         time.Time             `json:"date"`
	ChangedFiles map[string]ChangeKind `json:"changed_files,omitempty"`
}

type FileEntry struct {
	Path         string     `json:"path"`
	Status       ChangeKind `json:"status"`
	Additions    int        `json:"additions"`
	Deletions    int        `json:"deletions"`
	CommentCount int        `json:"comment_count"`
}

type ReviewComment struct {
	ID               int64     `json:"id"`
	Path             string    `json:"path"`
	CommitID         string    `json:"commit_id"`
	OriginalCommitID string    `json:"original_commit_id"`
	Line             int       `json:"line"`
	OriginalLine     int       `json:"original_line"`
	DiffHunk         string    `json:"diff_hunk"`
	InReplyTo        int64     `json:"in_reply_to"`
	User             string    `json:"user"`
	CreatedAt        time.Time `json:"created_at"`
	Body             string    `json:"body"`
	Outdated         bool      `json:"outdated"`
}
