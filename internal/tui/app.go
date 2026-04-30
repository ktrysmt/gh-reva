package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ktrysmt/gh-rv/internal/api"
	"github.com/ktrysmt/gh-rv/internal/model"
)

type Model struct {
	client api.Client
	target *api.Target
	state  *model.AppState
	width  int
	height int
	err    error

	// Per-pane render budgets, set by View() before delegating to the
	// pane renderers. Each pane uses these for width-aware wrapping
	// (Comments) or viewport sizing (Diff).
	paneWidthFiles     int
	paneHeightFiles    int
	paneWidthCommits   int
	paneHeightCommits  int
	paneWidthDiff      int
	paneHeightDiff     int
	paneWidthComments  int
	paneHeightComments int
}

func (m Model) Err() error { return m.err }

// SetDiffHeight pins the Diff viewport height. Used by the test-only
// --diff-height flag to make scroll assertions deterministic regardless of
// terminal size.
func (m *Model) SetDiffHeight(h int) { m.state.DiffViewport.Height = h }

func NewModel(client api.Client, target *api.Target) Model {
	return Model{
		client: client,
		target: target,
		state:  model.NewAppState(),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(loadPRCmd(m.client, m.target), spinnerTickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case LoadStageMsg:
		m.state.LoadStage = msg.Stage
		return m, nil
	case SpinnerTickMsg:
		m.state.LoadFrame++
		if m.state.LoadStage == model.LoadStageDone {
			return m, nil
		}
		return m, spinnerTickCmd()
	case PRLoadedMsg:
		m.state.PR = msg.PR
		m.state.DiffCache = msg.Diffs
		if len(msg.PR.Files) > 0 {
			m.state.SelectedFile = msg.PR.Files[0].Path
		}
		m.state.LoadStage = model.LoadStageDone
		return m, nil
	case ScrollDiffToLineMsg:
		patch := m.currentPatch()
		if patch == "" {
			return m, nil
		}
		bufIdx := bufferIndexForNewLine(patch, msg.NewLine)
		if bufIdx >= 0 {
			totalLines := strings.Count(patch, "\n") + 1
			m.scrollDiffToLine(bufIdx, totalLines)
		}
		return m, nil
	case ErrMsg:
		m.err = msg.Err
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) View() string {
	if m.state.PR == nil {
		return loadingView(m.state.LoadFrame, m.state.LoadStage)
	}
	// Reserve a row for the visual indicator when active so it does not
	// disappear under the column rendering.
	bodyHeight := m.height
	if m.state.Visual != nil && bodyHeight > 0 {
		bodyHeight--
	}
	if m.width <= 0 || bodyHeight <= 0 {
		// Window size not received yet — fall back to a stacked render
		// (used by smoke tests and the very first frame).
		body := strings.Join([]string{
			m.filesView(),
			m.commitsView(),
			m.diffView(),
			m.commentsView(),
		}, "\n\n")
		if m.state.Visual != nil {
			return body + "\n-- VISUAL --"
		}
		return body
	}

	leftW, midW, rightW := splitColumnWidths(m.width)
	topH, bottomH := splitColumnHeights(bodyHeight)

	// Each pane renders as: top border + title row + ├──┤ divider + content
	// rows + bottom border. Inner content budget is therefore outer width − 2
	// and outer height − 4.
	innerLeftW := atLeast(leftW-2, 1)
	innerMidW := atLeast(midW-2, 1)
	innerRightW := atLeast(rightW-2, 1)
	innerTopH := atLeast(topH-4, 1)
	innerBottomH := atLeast(bottomH-4, 1)
	innerBodyH := atLeast(bodyHeight-4, 1)

	m.paneWidthFiles = innerLeftW
	m.paneHeightFiles = innerTopH
	m.paneWidthCommits = innerLeftW
	m.paneHeightCommits = innerBottomH
	m.paneWidthDiff = innerMidW
	m.paneHeightDiff = innerBodyH
	m.paneWidthComments = innerRightW
	m.paneHeightComments = innerBodyH

	files := boxFromPaneView(m.filesView(), leftW, topH)
	commits := boxFromPaneView(m.commitsView(), leftW, bottomH)
	leftCol := lipgloss.JoinVertical(lipgloss.Left, files, commits)
	diffCol := boxFromPaneView(m.diffView(), midW, bodyHeight)
	commentsCol := boxFromPaneView(m.commentsView(), rightW, bodyHeight)
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, diffCol, commentsCol)

	if m.state.Visual != nil {
		return body + "\n-- VISUAL --"
	}
	return body
}

// splitColumnWidths divides total terminal width across the three columns.
// Targets roughly 30 / 60 / remainder for left / right / middle so the
// Comments column has room to display typical comment bodies without
// aggressive wrap.
func splitColumnWidths(total int) (left, mid, right int) {
	if total >= 130 {
		// Border consumes 2 cols per pane; bump outer widths so inner widths
		// (used for content) match the pre-border targets.
		left = 42
		right = 57
		mid = total - left - right
		return
	}
	if total >= 80 {
		left = total / 4
		if left < 22 {
			left = 22
		}
		if left > 38 {
			left = 38
		}
		right = total * 2 / 5
		if right < 28 {
			right = 28
		}
		mid = total - left - right
		if mid < 25 {
			mid = 25
			over := left + mid + right - total
			right -= over
			if right < 22 {
				right = 22
				left = total - mid - right
			}
		}
		return
	}
	// Degenerate (<80 cols): keep something sensible.
	left = total / 4
	mid = total / 2
	right = total - left - mid
	if right < 1 {
		right = 1
	}
	return
}

// splitColumnHeights divides the body height between Files (top) and Commits
// (bottom). Top gets the larger half so file lists are easier to scan.
func splitColumnHeights(total int) (top, bottom int) {
	if total < 4 {
		return total, 0
	}
	top = (total + 1) / 2
	bottom = total - top
	return
}

func atLeast(n, floor int) int {
	if n < floor {
		return floor
	}
	return n
}

// boxFromPaneView lifts a pane renderer's "title\nbody" output into a
// bordered box with a horizontal divider under the title row. width and
// height are outer dimensions.
//
//	┌────────┐
//	│ title  │
//	├────────┤
//	│ body…  │
//	└────────┘
func boxFromPaneView(view string, width, height int) string {
	title, body := splitTitleBody(view)
	return renderPaneBox(title, body, width, height)
}

func splitTitleBody(s string) (string, string) {
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

func renderPaneBox(title, body string, width, height int) string {
	innerW := atLeast(width-2, 1)
	contentRows := atLeast(height-4, 0)
	bar := strings.Repeat("─", innerW)

	var sb strings.Builder
	sb.WriteString("┌" + bar + "┐\n")
	sb.WriteString("│" + padTrunc(title, innerW) + "│\n")
	sb.WriteString("├" + bar + "┤\n")

	bodyLines := strings.Split(body, "\n")
	for i := 0; i < contentRows; i++ {
		line := ""
		if i < len(bodyLines) {
			line = bodyLines[i]
		}
		sb.WriteString("│" + padTrunc(line, innerW) + "│\n")
	}
	sb.WriteString("└" + bar + "┘")
	return sb.String()
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func loadingView(frame int, stage model.LoadStage) string {
	glyph := spinnerFrames[frame%len(spinnerFrames)]
	return fmt.Sprintf("%s Loading PR (%s)...", glyph, stageLabel(stage))
}

func stageLabel(s model.LoadStage) string {
	switch s {
	case model.LoadStagePR:
		return "metadata"
	case model.LoadStageCommits:
		return "commits"
	case model.LoadStageFiles:
		return "files"
	case model.LoadStageComments:
		return "comments"
	case model.LoadStageDiffs:
		return "diffs"
	default:
		return "ready"
	}
}

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return SpinnerTickMsg{}
	})
}

// loadPRCmd loads PR data in stages via tea.Sequence. Each stage emits a
// LoadStageMsg so the spinner label can update; the final stage assembles
// accumulated data and emits PRLoadedMsg. A per-launch accumulator (closed
// over by every cmd) carries data between stages.
func loadPRCmd(c api.Client, t *api.Target) tea.Cmd {
	acc := &loadAccumulator{}
	ctx := context.Background()
	return tea.Sequence(
		stageMsgCmd(model.LoadStagePR),
		func() tea.Msg {
			pr, err := c.GetPR(ctx, t.Owner, t.Repo, t.Number)
			if err != nil {
				return ErrMsg{Err: err}
			}
			acc.pr = pr
			return LoadStageMsg{Stage: model.LoadStageCommits}
		},
		func() tea.Msg {
			commits, err := c.ListCommits(ctx, t.Owner, t.Repo, t.Number)
			if err != nil {
				return ErrMsg{Err: err}
			}
			acc.commits = commits
			return LoadStageMsg{Stage: model.LoadStageFiles}
		},
		func() tea.Msg {
			files, err := c.ListFiles(ctx, t.Owner, t.Repo, t.Number)
			if err != nil {
				return ErrMsg{Err: err}
			}
			acc.files = files
			return LoadStageMsg{Stage: model.LoadStageComments}
		},
		func() tea.Msg {
			comments, err := c.ListComments(ctx, t.Owner, t.Repo, t.Number)
			if err != nil {
				return ErrMsg{Err: err}
			}
			acc.comments = comments
			return LoadStageMsg{Stage: model.LoadStageDiffs}
		},
		func() tea.Msg {
			diffs := map[string]string{}
			for _, f := range acc.files {
				d, err := c.GetFileDiff(ctx, t.Owner, t.Repo, t.Number, "", f.Path)
				if err == nil && d != "" {
					diffs[diffKey("", f.Path)] = d
				}
			}
			for _, com := range acc.commits {
				for path := range com.ChangedFiles {
					d, err := c.GetFileDiff(ctx, t.Owner, t.Repo, t.Number, com.SHA, path)
					if err == nil && d != "" {
						diffs[diffKey(com.SHA, path)] = d
					}
				}
			}
			acc.pr.Commits = acc.commits
			acc.pr.Files = acc.files
			acc.pr.Comments = acc.comments
			return PRLoadedMsg{PR: acc.pr, Diffs: diffs}
		},
	)
}

type loadAccumulator struct {
	pr       *model.PR
	commits  []*model.Commit
	files    []*model.FileEntry
	comments []*model.ReviewComment
}

func stageMsgCmd(s model.LoadStage) tea.Cmd {
	return func() tea.Msg { return LoadStageMsg{Stage: s} }
}

func diffKey(sha, path string) string {
	return sha + "::" + path
}

func (m Model) currentPatch() string {
	if m.state.PR == nil || m.state.SelectedFile == "" {
		return ""
	}
	sha := ""
	if m.state.SelectedRange.Kind == model.RangeSingleCommit {
		sha = m.state.SelectedRange.SHA
	}
	return m.state.DiffCache[diffKey(sha, m.state.SelectedFile)]
}

func (m Model) effectiveDiffViewMode() string {
	if m.state.DiffViewMode == model.DiffViewUnified {
		return "unified"
	}
	if m.width > 0 && m.width < 100 {
		return "unified"
	}
	return "split"
}
