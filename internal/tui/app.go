package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
