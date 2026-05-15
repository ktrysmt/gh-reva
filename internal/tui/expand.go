package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ktrysmt/gh-reva/internal/api"
	"github.com/ktrysmt/gh-reva/internal/diff"
	"github.com/ktrysmt/gh-reva/internal/model"
)

// expandUnit is the symmetric-half expand step for inter-hunk gaps (and
// half of the total step for BOF / EOF). The exposed total is 20 lines
// per Enter — Mid splits into 10 above + 10 below; BOF grows BOFBelow by
// 20; EOF grows EOFAbove by 20.
const expandUnit = 10

// fileContentsLoadedMsg is dispatched when a background fetch of file
// contents returns. The Update handler stores Lines under
// FileContents[(Ref, Path)] and invalidates the patchInfo cache so the
// next render shows synthetic rows. Err non-nil = fetch failed; the
// status bar surfaces "context unavailable" and the synthetic stays
// hidden for that file.
type fileContentsLoadedMsg struct {
	Ref   string
	Path  string
	Lines []string
	Err   error
}

// currentExpandKey reads the (file, range) key the Diff pane is
// currently showing. Used by Enter-on-synthetic and the prefetch
// trigger to address the right ExpandedContext slot.
func (m Model) currentExpandKey() model.ExpandKey {
	return model.ExpandKey{
		Path:      m.state.SelectedFile,
		RangeKind: m.state.SelectedRange.Kind,
		RangeSHA:  m.state.SelectedRange.SHA,
	}
}

// currentFileRef returns the SHA at which the active file's content
// should be fetched. WholePR → PR head; single-commit → the commit's SHA.
func (m Model) currentFileRef() string {
	if m.state.SelectedRange.Kind == model.RangeSingleCommit {
		return m.state.SelectedRange.SHA
	}
	if m.state.PR != nil {
		return m.state.PR.HeadSHA
	}
	return ""
}

// handleEnterOnSynthetic is invoked from handleKeyDiff when the cursor
// sits on a `···` row. Applies the expansion delta to ExpandedContext
// immediately (so the next render shows revealed lines) and returns nil
// when FileContents is already cached. If not cached, queues a fetch
// Cmd; the resulting fileContentsLoadedMsg later invalidates patchInfo
// so the augmented buffer can rebuild with both FileContents and the
// applied expansion.
func (m *Model) handleEnterOnSynthetic(bufIdx int) tea.Cmd {
	gaps := m.patchGaps()
	gap, ok := gaps[bufIdx]
	if !ok {
		return nil
	}
	ek := m.currentExpandKey()
	m.applyExpand(ek, gap)
	m.invalidatePatchInfoCache(ek)

	ref := m.currentFileRef()
	path := m.state.SelectedFile
	if path == "" || path == model.AllFilesPath {
		return nil
	}
	if m.state.FileContents != nil {
		if _, cached := m.state.FileContents[model.FileContentsKey{Ref: ref, Path: path}]; cached {
			return nil
		}
	}
	if m.client == nil || m.target == nil {
		return nil
	}
	m.state.Notice = "fetching file contents…"
	return fetchFileContentsCmd(m.client, m.target, ref, path)
}

// invalidatePatchInfoCacheForRef invalidates the patchInfo cache for any
// (file, range) pair whose FileContents key matches (ref, path). Called
// when a fetch completes so the newly-available file lines flow into
// the next render of the affected diff view.
func (m Model) invalidatePatchInfoCacheForRef(ref, path string) {
	if m.patchLinesC.cache == nil {
		return
	}
	// patchInfo cache is keyed by diffKey(sha, path). The arrived
	// FileContents may match either the WholePR view (sha == "") or a
	// single-commit view (sha == ref). Drop both candidates.
	delete(m.patchLinesC.cache, diffKey("", path))
	delete(m.patchLinesC.cache, diffKey(ref, path))
	if m.rowCache != nil {
		m.rowCache.reset(m.rowCache.patchKey, m.rowCache.width, m.rowCache.halfW)
	}
	if m.threadsCache != nil {
		m.threadsCache.valid = false
	}
}

// applyExpand grows the ExpandedContext counters for the gap under the
// cursor. BOF / EOF receive a single counter (20 each); Mid splits 10/10
// across InterAbove / InterBelow. Repeat presses keep growing past the
// gap size — diff.Expand caps internally at gap boundaries so the
// counters going above the gap is harmless.
func (m *Model) applyExpand(ek model.ExpandKey, gap diff.GapInfo) {
	es := m.state.ExpandedContext[ek]
	if es == nil {
		es = &model.ExpandState{}
		m.state.ExpandedContext[ek] = es
	}
	if es.InterAbove == nil {
		es.InterAbove = map[int]int{}
	}
	if es.InterBelow == nil {
		es.InterBelow = map[int]int{}
	}
	switch gap.ID.Kind {
	case diff.GapKindBOF:
		es.BOFBelow += 2 * expandUnit
	case diff.GapKindEOF:
		es.EOFAbove += 2 * expandUnit
	case diff.GapKindMid:
		es.InterAbove[gap.ID.Index] += expandUnit
		es.InterBelow[gap.ID.Index] += expandUnit
	}
}

// fetchFileContentsCmd returns a tea.Cmd that fetches file content
// from the API client at the given (ref, path). The result lands on
// Update via fileContentsLoadedMsg — successful fetches populate
// FileContents and invalidate the patchInfo cache; errors set a Notice
// and leave the synthetic row absent (no FileContents → no synthetic).
func fetchFileContentsCmd(c api.Client, t *api.Target, ref, path string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		lines, err := c.GetFileContents(ctx, t.Owner, t.Repo, t.Number, ref, path)
		if err != nil {
			return fileContentsLoadedMsg{Ref: ref, Path: path, Err: err}
		}
		return fileContentsLoadedMsg{Ref: ref, Path: path, Lines: lines}
	}
}
