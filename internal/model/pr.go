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
	NodeID           string    `json:"node_id,omitempty"`
	ThreadID         string    `json:"thread_id,omitempty"`
	Path             string    `json:"path"`
	CommitID         string    `json:"commit_id"`
	OriginalCommitID string    `json:"original_commit_id"`
	Line             int       `json:"line"`
	OriginalLine     int       `json:"original_line"`
	Side             string    `json:"side,omitempty"`
	// StartLine / OriginalStartLine / StartSide describe the upper edge
	// of a multi-line range comment. Zero values mean "single-line"
	// (the comment anchors only at Line / Side). When the live
	// StartLine resolves to 0 on an outdated comment, the renderer falls
	// back to OriginalStartLine the same way Line falls back to
	// OriginalLine.
	StartLine         int    `json:"start_line,omitempty"`
	OriginalStartLine int    `json:"original_start_line,omitempty"`
	StartSide         string `json:"start_side,omitempty"`
	DiffHunk         string    `json:"diff_hunk"`
	InReplyTo        int64     `json:"in_reply_to"`
	User             string    `json:"user"`
	CreatedAt        time.Time `json:"created_at"`
	Body             string    `json:"body"`
	Outdated         bool      `json:"outdated"`
	// Pending marks a comment whose containing review has not been
	// submitted yet — i.e. `pullRequestReview.state == PENDING` per
	// GitHub's GraphQL schema. POSTing via the compose flow returns
	// the comment with Pending=true; SubmitPendingReview flips the
	// review's state which makes the comment public on the next
	// ListComments refetch.
	Pending          bool      `json:"pending,omitempty"`
	// Resolved mirrors the GraphQL `PullRequestReviewThread.isResolved`
	// signal — a thread the author or reviewer has explicitly marked
	// as resolved on GitHub. The flag is thread-level on the API but
	// gets propagated onto every comment in the thread during
	// convertGQLComment so the renderer can decide per-row without
	// re-walking the thread. Resolved threads get a `[resolved]`
	// header tag in the Comments column and swap the Diff gutter
	// glyph from ◆ to ✓.
	Resolved         bool      `json:"resolved,omitempty"`
}
