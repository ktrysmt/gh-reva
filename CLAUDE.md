# CLAUDE.md — gh-reva development conventions

`gh` extension that opens a vim-like 4-pane TUI for reviewing GitHub PRs.
Built on `bubbletea` + `lipgloss`. Single-purpose CLI; no shared infrastructure.

This file is authoritative. Read once at session start; update when an
invariant changes. Cited source files are the source of truth for detail.

---

## 1. Build / test commands

```sh
# Repo root: /Users/dew/workspace/gh-reva
go build -o gh-reva .          # produces ./gh-reva (NOT `go build ./...`)
go vet ./... && go test ./...

# Manual TUI
./gh-reva --fixture testdata/sample-pr.json
./gh-reva --fixture testdata/sample-pr.json --slow-load 500ms

# E2E (cd e2e first)
pnpm install && pnpm test       # full suite; pretest auto-rebuilds gh-reva
node --test --test-force-exit --test-timeout=20000 \
     --test-name-pattern='F2|F11' tests/05_pane_diff.test.mjs
                                # targeted; skips pretest — rebuild manually

# Large fixture regeneration
go run testdata/gen_large_fixture.go testdata/large-pr.json
```

### Hidden flags (E2E only)
- `--fixture <path>` — load PR data from JSON
- `--simulate-error <kind>` — `unauth` | `not_found` | `rate_limit` | other
- `--diff-height N` — pin Diff viewport height
- `--slow-load <duration>` — per-API sleep in fixtureClient

### User-facing flags
- `--theme <name>` — default `gruvbox`. Any chroma styles registry name (74) plus `builtin-dark`. `GH_REVA_THEME` env fallback. Empty → `defaultThemeName` in `internal/theme/theme.go`.
- `--no-color` — honors `NO_COLOR` / `CLICOLOR` (`termenv.EnvNoColor`).
- `--list-themes` — print accepted names and exit 0.
- `--config <path>` — load `reva.toml`. Defaults: `$XDG_CONFIG_HOME/reva.toml` → `$HOME/.config/reva.toml`. Schemas:
  - `[syntax.extensions]` — filename suffix (e.g. `.j2`) → chroma lexer name. Authoritative: `internal/config/config.go` + `internal/tui/syntax.go::lexerFromOverride` (longest-suffix-match wins; unknown lexer names fall back to chroma's default extension matcher).
  - `[layout].comments_width_percent` — integer percentage of total terminal width allocated to the Comments column. Honored in `[commentsWidthPercentMin, commentsWidthPercentMax]` = `[10, 70]`; zero / out-of-range falls back to `defaultCommentsWidthPercent = 25`. Files keeps its fixed 42-col width on wide terminals; the Diff column absorbs the remainder. Subject to a `mid ≥ 25` floor (mirrors the 80–130 branch) so Diff cannot collapse below readability even under aggressive overrides. Authoritative: `internal/config/config.go::LayoutConfig` + `internal/tui/app.go::splitColumnWidths` + `internal/tui/app.go::Model.SetCommentsWidthPercent` (called from `cmd/root.go`).
  - `[editor].popup_width_percent` / `popup_height_percent` — integer percentages of terminal width / height used for `tmux display-popup -w <PCT>% -h <PCT>%` when reva launches an external editor from inside tmux (composer flow). Honored when each value is in `[editorPopupPercentMin, editorPopupPercentMax]` = `[20, 95]`; zero / out-of-range falls back to `defaultEditorPopupPercent = 50` per dimension. Has no effect outside tmux — the bare `$EDITOR` path takes the whole terminal regardless. Authoritative: `internal/config/config.go::EditorConfig` + `internal/tui/compose.go::buildEditorCmd` + `internal/tui/compose.go::resolveEditorPopupPercent` + `internal/tui/app.go::Model.SetEditorPopupSize` (called from `cmd/root.go`).
  Explicit `--config` missing target = hard error; implicit search tolerates absence.

---

## 2. Workflow discipline

### TDD is mandatory
1. Failing test(s) first.
2. Targeted run; confirm failure shows the assertion mismatch (not timeout / build break).
3. Implement.
4. Targeted run; confirm pass.
5. Full e2e (`pnpm test`); confirm no regressions.
6. Unrelated tests failing under new behavior → update in the same change. Never leave the suite red.

Skipping step 1 is forbidden — even for trivial changes.

### Decision-first vs. action-first
Non-trivial design space (key bindings, fallback semantics, visual markers) → present 2–3 options with tradeoffs and ask before writing tests. Straightforward asks → proceed with TDD.

### Risky operations require confirmation
Confirm before: `git push`, `git reset --hard`, force push (push to main / master forbidden without explicit direction); deleting fixtures / snapshots; adding top-level Go deps; renaming branches. User runs `git commit` unless explicitly delegated.

---

## 3. Architecture

```
gh-reva/
├── cmd/root.go                 # CLI entry, flags
├── main.go
├── internal/
│   ├── config/                 # reva.toml loader (XDG ladder + --config)
│   ├── api/                    # GitHub client (go-gh) + fixture
│   │   ├── client.go           # Client iface (read + pending POST + submit)
│   │   ├── pr.go               # GetPR / ListCommits / ListFiles
│   │   ├── diff.go             # GetFileDiff (PR-wide and per-commit)
│   │   ├── contents.go         # GetFileContents (file body for context expand)
│   │   ├── paginate.go         # Link-header pagination (REST)
│   │   ├── resolve.go          # ResolveCurrentBranchPR + ParseTargetArg
│   │   ├── graphql_comments.go # ListComments + reviewThread mapping
│   │   ├── graphql_post.go     # ensurePendingReview + thread/reply/edit/submit
│   │   ├── fixture.go          # testdata/*.json + WithSlowLoad + in-mem POST
│   │   └── error_client.go     # --simulate-error
│   ├── clipboard/
│   ├── diff/                   # patch parsing + side resolver + context expand
│   │   ├── side.go             # ResolveAnchor / ResolveRange (raw patch)
│   │   └── expand.go           # Expand + ParseSpecsAug (synthetic `···` rows)
│   ├── model/                  # AppState + value types
│   ├── theme/                  # color palette
│   │   ├── theme.go            # Theme, Resolve, ListThemes
│   │   ├── builtin.go          # builtin-dark fallback palette
│   │   └── chroma.go           # chroma styles → Theme adapter
│   └── tui/
│       ├── app.go              # Model, View(), layout, loadPRCmd, renderPaneBox
│       ├── keys.go             # global key dispatch
│       ├── messages.go         # tea.Msg types
│       ├── styles.go           # paneTitle / fitPaneTitle / wrapText / styled*
│       ├── colors.go           # fg / fgBold / bgRow lipgloss wrappers
│       ├── syntax.go           # styledDiffCell + chroma lexer + token cache
│       ├── statusbar.go        # composeStatusBar (authoritative)
│       ├── splash.go           # 3 splash layouts × 3 ASCII REVA arts
│       ├── pane_files.go       # filesView + advanceFile
│       ├── pane_commits.go     # commitsView + auto-select + allCommitsRow
│       ├── pane_diff.go        # diffView + split + ◆ gutter + tabs
│       ├── pane_comments.go    # commentsView + word wrap + diff auto-scroll
│       ├── files_tree.go       # tree mode rendering
│       ├── visual.go           # visual mode + yank
│       ├── modal.go            # `<space>` zoom modal
│       ├── compose.go          # Pending-comment compose state machine
│       ├── textarea.go         # in-app textarea fallback + compose modal
│       ├── refresh.go          # refreshCommentsCmd / mergeRefreshedComments
│       ├── search.go           # `/` incsearch + handlePendingG
│       ├── expand.go           # Context expand routing + prefetch + fileContentsLoadedMsg
│       └── diffmap.go          # newLineNumbers / commentThreadIndexForDiffLine
├── testdata/
│   ├── sample-pr.json          # default (5 files, 3 commits, 4 comments)
│   ├── large-pr.json           # 60 commits / 120 files / 122 KB
│   ├── wrap-pr.json            # single long-bodied comment
│   ├── expand-pr.json          # BOF/Mid/EOF gaps + file_contents (context expand e2e)
│   └── gen_large_fixture.go    # //go:build ignore
└── e2e/
    ├── helpers/launch.mjs      # launchReva / paneText / countSelectedRows
    └── tests/                  # node:test + tuistory
```

### Receiver conventions
- Mutating helpers — pointer `(m *Model)`: `selectFile`, `advanceFile`, `scrollDiffIntoView`.
- Pure queries / renderers — value `(m Model)`: `filesView`, `diffView`, `visibleCommits`.
- `handleKey*` — value receivers; mutate via Go auto-addressing.
- `m.state` — `*model.AppState`; mutations propagate regardless of receiver kind.

### Single source of truth
- `model.AppState` — all mutable state. No globals beyond constants.
- `m.state.SelectedFile` — drives `visibleCommits`, `commentsForView`, Diff cache key.
- Per-pane render budgets (`paneWidthFiles`, `paneHeightDiff`, …): populated by `(*Model).measureLayout`, called once at the top of `Update` (before the type switch) and once by `View()` for the first frame. Persistence rides on `Update`'s value-receiver return — Bubbletea stores the returned Model, so paneWidth* survive between messages without per-handler re-measure. View's measurements never persist (string-only return).

---

## 4. TUI invariants

Load-bearing — breaking any of them breaks at least one e2e test. Keep numbering stable; other items reference these indices.

### Layout
1. 3-column bordered layout: Files+Commits stacked left; Diff middle; Comments right. Each pane is its own `┌─┐ │ ├─┤ │ └─┘` box with a divider under the title.
2. Pane box: 4 + N rows (top / title / divider / N content / bottom). Inner width = outer − 2; inner height = outer − 4.
3. `splitColumnWidths(total, commentsHidden, commentsPct)` branches by terminal width. `commentsPct` is the requested Comments share (0..100) and falls back to `defaultCommentsWidthPercent = 25` when 0 or outside `[10, 70]`. Honored overrides come from `Model.commentsWidthPercent`, set by `Model.SetCommentsWidthPercent` from `cfg.Layout.CommentsWidthPercent` (reva.toml `[layout]`).
   - total ≥ 130: `left = 42`, `right = total * commentsPct / 100` (floored at 28 to keep Comments readable), `mid = total − left − right` (floored at 25; overflow stolen from right). At default 25%: 130 → right=32 mid=56; 160 → right=40 mid=78; 200 → right=50 mid=108.
   - 80 ≤ total < 130: proportional with `mid ≥ 25` floor (Diff steals from `right`); also honors `commentsPct` for the right slot before the floor.
   - total < 80: degenerate; tests do not pin.
   The e2e default `cols=160` still works for every assertion, but the Comments header's `#<id>` slot (#23b) is dropped at render time when it would push the trailing `[pending]` / `[outdated]` tag past the column edge — keeping the critical state tag visible takes priority over the reference id. `internal/tui/mouse_test.go::mouseModelFixture` pins `m.commentsWidthPercent = 41` to preserve the legacy `right=57 mid=41` frame the 30+ mouse tests baked specific (x, y) coords against; changing the default doesn't cascade through them.
4. Active pane: `▶ ` prefix on its title row. Exactly one.
5. Cursor row: `> ` prefix in Files / Commits / Diff / Comments. Visual-range rows also `> `.
6. Status bar (`internal/tui/statusbar.go`): 2-row borderless block — content + blank, `statusBarRows = 2`, `bodyHeight = m.height - statusBarRows`. Suppressed when `m.width <= 0`, `m.height <= statusBarRows`, or during loading splash. URL from `api.Target.PRShortForms` (4-step ladder). Per-pane context: mode-selected (normal / compose / help / modal / visual); suffix `tab/shift+tab:pane J/K:file ctrl+e:comments ?:help q:quit` joined only in normal. Diff context built by `(Model).diffHint`: `[A]` (RIGHT) or `[B]` (LEFT) prepended to `hintDiffKeys`. `AppState.Notice` replaces context for one keystroke. Pane bottom borders sit above the bar's content row — visual separator.
7. Loading view: splash + blank gap + optional version+blank + `<spinner> Loading PR…`. Centered. Pre-`tea.WindowSizeMsg` → top-left fallback. Status bar suppressed.
   - `loadPRCmd`: errgroup fan-out over `GetPR` / `ListCommits` / `ListFiles` / `ListComments` / `ViewerLogin`; wall time = slowest leg (typically `ListCommits` at `api.commitDetailConcurrency = 8`).
   - Comment counts derived in assembler from comments list (outdated excluded, mirroring #25); `api.ListFiles` no longer round-trips through `ListComments`.
   - `ViewerLogin` failure swallowed (`""`); Comments Enter falls back to reply-only (#24b).
   - `model.LoadStage = {LoadStagePR, LoadStageDone}`; Done gates `SpinnerTickMsg` re-tick.
   - Pinned by `internal/tui/load_test.go` + `internal/api/parallel_test.go`.
7a. Splash (`internal/tui/splash.go`): 3 layouts × 3 ASCII REVA arts. `chooseSplashLayout` / `chooseSplashArt` picked once in `NewModel` (no flicker). Random by default; pinnable via `GH_REVA_SPLASH_LAYOUT` (1/2/3) + `GH_REVA_SPLASH_ART` (0/1/2). Layouts: `1` dome + `reva vX.Y.Z` + spinner; `2` art + `vX.Y.Z` + spinner; `3` art ▌ dome + `vX.Y.Z` + spinner. Version from `cmd/root.go::SetVersion`; empty suppresses.

### Diff pane
8. Split row layout: `<Lcursor 2><Lmarker 2><oldLn 4><sp 1><leftCell halfW><sp 1>│<sp 1><Rmarker 2><Rcursor 2><newLn 4><sp 1><rightCell halfW>`. Overhead = 21; `halfW = (paneWidthDiff − 21) / 2`. Degrades to unified when `halfW < 8` (split engages at `paneWidthDiff ≥ 37`). Per-side cursor / marker columns required for h/l Side switching — a single cursor column can't indicate which physical lane is parked. `splitColumnWidths` ladder unchanged; at total 130–135 the layout still splits, but the overhead may push narrow terminals into the early-unified fallback.
9. Tab expansion: `expandTabs(line, 4)` before wrap/pad. Without it, terminal-side tab expansion shifts `│`.
10. Gutter markers per-side. `commentLineMarkers` (`internal/tui/diffmap.go`) returns `sideMarkers{Left, Right}`; Side captured from `root.Side` at classification time so the same buffer index never collides across columns.
    - Column placement: LEFT-side → Lmarker col (cols 2–3, immediately left of `oldLn`); RIGHT-side → Rmarker col (immediately right of `│`, just before `Rcursor` / `newLn`).
    - Glyph set: every thread paints exactly one glyph at the end-anchor row — `◆` (unresolved, `markerAnchor`) or `✓` (resolved, `markerResolved`). Multi-line range comments do NOT paint intermediate rows; the upper edge of the range is conveyed by the `R<start>-<end>` (or `L<start>-R<end>`) tag in the Comments header (#23b). Previous ┌/│ glyphs were retired — the 2-col gutter could not host both edges, and markerRank precedence forced ◆ to win whenever a neighbouring thread's anchor landed on a middle row, so range shape would silently vanish under overlap. `✓` colored with `theme.CommentResolved` (green semantic via `Model.markerColor`).
    - Continuation rows always blank both per-side gutters — the anchor glyph belongs to the first display row only.
    - Mixed-side ranges (`StartSide=LEFT`, `Side=RIGHT`) place ◆ on END's column following `root.Side`. The Comments header tag retains both prefixes (`L5-R10`) so the LEFT endpoint stays discoverable even though the gutter only marks one column.
    - Replies ignored — only the thread root carries the range / anchor.
    - Overlap precedence on same Side: `◆ (2) > ✓ (1)` (`markerRank`). Unresolved beats resolved so an unresolved concern at the same buffer index never gets hidden behind a resolved ✓. LEFT and RIGHT at the same buffer row coexist (per-side maps).
    - Unified mode: `foldMarker(left, right)` collapses to one glyph by rank.
    - Range data: `model.ReviewComment.{StartLine, OriginalStartLine, StartSide}`. Populator: `internal/api/graphql_comments.go::convertGQLComment`. `OriginalStartLine` is the outdated-fallback (mirrors `OriginalLine` for `Line`). Consumed by `pane_comments.go::formatRangeTag` for the header label, not by `commentLineMarkers`.
11. Split row distribution: header (`---`/`+++`/`@@`) and context render both sides; `-` left only; `+` right only.
12. Wrap always on; buffer line ↔ display row is 1:N. `DiffCursor.Line` indexes raw patch buffer; `>` and `◆` appear only on the first display row. Cursor `>` follows active Side: RIGHT (default) → `Rcursor` col; LEFT → `Lcursor` col; opposite stays blank. Continuation rows: unified indents 5 cols; split blanks both cursor / line-number cols, prefixes each cell with 1 blank, redraws inter-half `│`. Per-side gutter blank on every continuation row (no range middle to keep connected; #10).
13. `fitPaneTitle` preserves the `[mode]` suffix at narrow widths; label shrinks with `…`.
14. Diff Enter (in order; first match wins):
    - Synthetic `···` row → `handleEnterOnSynthetic` expands the gap (#14d). Short-circuits compose / focus handoff.
    - No comments at cursor → `startComposeInline` queues inline compose confirm. Anchor = `Model.resolveAnchorAug` over the augmented buffer (Path = SelectedFile, CommitSHA = `PR.HeadSHA`, Line + Side). Header / hunk / synthetic rows rejected. In Diff visual mode, Enter consumes the visual range via `resolveRangeAug`; mixed-side ranges supported (#27d). Editor / textarea launch held until confirm (#27j).
    - Comments at cursor (`threadsForCursor()` non-empty) → `focusCommentsAtCursor` shifts focus to Comments (`CommentsCursor = 0`, `Modal = nil`). Hidden Comments column (#30c) auto-reveals first. User acts via Comments keymap (Enter = edit own / `r` = reply / Space = zoom modal). Adding another thread on the same line: intentionally not exposed.
14b. `gg` — true two-key sequence: first `g` sets `AppState.PendingPrefix = "g"`; next `g` runs gotoTop; any non-`g` key clears pending AND falls through. Slot is global — every pane calls `handlePendingG` (`search.go`). `G` is the symmetric gotoBottom. Per-pane semantics: Files moves `FilesCursor` only (#19); Commits auto-selects; Diff honors per-side filter (walks to FIRST / LAST row on `DiffCursor.Side`; LEFT skips `+`, RIGHT skips `-`; headers / hunks / context exist on both).

14c. Diff per-column UX (`internal/tui/diffmap.go::lineExistsOnSide`):
   - `DiffCursor.Side` (`model.DiffSide`, "LEFT"/"RIGHT", default RIGHT) drives every Side decision: cursor visual position (#12), ◆ marker placement (#10), Comments filter (#23), Compose anchor Side (#27c).
   - `j/k` auto-skip: RIGHT skips `-` rows, LEFT skips `+` rows. Headers / hunks / context / synthetic (#14d) never skipped. `nextSideLine` returns -1 when no further row exists → `j/k` no-op (cursor stays). Wheel uses the same path.
   - `h` → LEFT, `l` → RIGHT. If cursor row absent on the new Side, `nearestSideLine` repositions the cursor (prefers upward — `h` from `+` lands on the `-` or context above, matching "the line this `+` replaced"). Idempotent when Side matches.
   - `h/l` in unified mode (`<space>` toggle): no-op + `state.Notice = "h/l: split mode only"`. Side preserved internally; split → unified → split round-trips don't reset column.
   - `h/l` during Diff visual range: no-op + `state.Notice = "side locked in visual (esc to leave)"`. Anchor + cursor share Side by construction (auto-skip never crosses); mid-range switch would strand an endpoint.
   - `selectFile` (#19), `autoSelectCommit`, `RangeWholePR` reset → `DiffCursor = model.DiffCursor{Side: DiffSideRight}` so every context switch lands on a known column. Empty-string Side would freeze j/k entirely.
   - Mouse click in Diff sets Side from inner col: `< halfW+10` → LEFT, `> halfW+10` → RIGHT (divider `│` preserves Side). After Side change, `switchSide` repositions cursor to nearest same-side row if click row is opposite-side. Wheel preserves Side.

14d. Context expand (synthetic `···` rows). Implemented in `internal/diff/expand.go` + `internal/tui/expand.go`; renderer slot in `pane_diff.go::renderSynthBufferLine`.
   - Hidden regions surface as a buffer row reading `··· N lines hidden  (enter: expand)`. Three gap kinds: `GapKindBOF` (above first hunk), `GapKindMid` (between consecutive hunks, indexed 0..N-2), `GapKindEOF` (below last hunk). EOF requires file lines (file length unknown otherwise). Render shape per mode (`pane_diff.go::renderSynthBufferLine`):
     - Unified: one full-width row, `> ` at col 0 follows cursor / visual membership.
     - Split: mirror the standard split geometry (#8) — same `··· …` body painted on BOTH halves, `│` divider between them, per-side line-number / marker columns blank. `> ` follows `DiffCursor.Side` (Lcursor / Rcursor), matching the rule used by regular split rows; a single-cell body would strand the cursor on the left column and read as "cursor jumped" when Side=RIGHT. The body label degrades through 5 tiers (`pane_diff.go::synthLabel`): `··· N lines hidden  (enter: expand)` → `··· N lines hidden (enter)` → `··· N lines hidden` → `··· N hidden` → `···`, so the hidden-count signal survives even at the split-engage threshold (halfW=8).
   - Emission gated on `FileLines != nil` in `diff.Expand`. Without prefetched file contents the augmented buffer equals the raw patch — preserves backward compat for tests / fixtures that don't carry `file_contents`. Production prefetches on file selection (see below) so synthetic rows surface within a frame of `PRLoadedMsg`.
   - Synthetic line in the buffer = the literal sentinel string `diff.SyntheticLine` (`\x01SYNTH`). Real diff content never starts with `\x01`, so prefix-matching call sites (`lineExistsOnSide`, `splitDiffLine`, `diffLineKind`) detect it without colliding. `parseDiffSpecs` is replaced by `diff.ParseSpecsAug(lines, gaps)` which carries the gap-end → `OldEnd+1 / NewEnd+1` line-number jump so expanded-context rows below a synthetic report the correct OLD/NEW pair.
   - Expanded context rows (` <file content>`) are emitted by `diff.Expand` at their canonical buffer position — between the relevant hunks (Mid), between the file headers and the first `@@` (BOF), or after the last hunk's body (EOF). They behave exactly like normal context rows for line-number bookkeeping, cursor navigation, comment anchoring (compose works on them).
   - Enter routing (`pane_diff.go::handleKeyDiff` Enter branch, ahead of `threadsForCursor` / `startComposeInline`):
     - Cursor on synthetic → `handleEnterOnSynthetic(idx)`. BOF / EOF grow by 20 (counter += 2 × `expandUnit`); Mid grows symmetrically (10 / 10 — `InterAbove[i]` and `InterBelow[i]` each += `expandUnit`). Counters cap inside `diff.Expand` at the gap size, so repeat presses are idempotent once the gap closes.
     - Cursor on header / hunk / `+` / `-` / context → existing semantics (compose-confirm or focus-handoff).
   - State: `AppState.ExpandedContext map[ExpandKey]*ExpandState` (per `(path, range.Kind, range.SHA)`). `AppState.FileContents map[FileContentsKey][]string` keyed on `(ref, path)`; ref = `PR.HeadSHA` for `RangeWholePR`, commit SHA for `RangeSingleCommit`. Both maps initialized in `NewAppState`.
   - Prefetch: `Update`'s outer wrapper runs `maybePrefetchFileContents` after each `updateInner`. Fires when `SelectedFile` / range changed since last frame AND `FileContents[(ref, path)]` not cached. Skips `AllFilesPath` and nil client / target (tests). The `prefetchedRef` / `prefetchedPath` fields on `Model` debounce repeat fetches.
   - Cache invalidation: `invalidatePatchInfoCache(ExpandKey)` (#39) drops the `patchLinesC` entry, resets `rowCache`, marks `threadsCache.valid = false`. Called by `handleEnterOnSynthetic` after the counter mutation and by `applyFileContentsLoaded` after `FileContents` populates (the latter goes through `invalidatePatchInfoCacheForRef` which drops both the WholePR and the single-commit slot under that ref).
   - Compose / anchor pipeline switched from raw-patch `diff.ResolveAnchor` / `ResolveRange` to augmented-buffer `Model.resolveAnchorAug` / `Model.resolveRangeAug`. They consume `m.patchSpecs()` so synthetic rows reject (`Kind == 's'`) and expanded-context rows report the correct `OldLn`/`NewLn`. Standalone `diff.ResolveAnchor` / `ResolveRange` stay for raw-patch unit-test use.
   - Search / yank exclude synthetic: `collectDiffMatches` skips `SyntheticLine`; visual yank for Diff also skips it. Without these, search would phantom-match the sentinel byte and yank would leak `\x01` into the clipboard.
   - API: `Client.GetFileContents(ctx, owner, repo, n, ref, path) ([]string, error)`. ghClient hits `GET /repos/{owner}/{repo}/contents/{path}?ref={ref}` (JSON+base64) and caches by `(ref, path)`. fixtureClient reads from `fixtureData.FileContents` keyed `"<ref>::<path>"` and splits on `\n`, trimming trailing newline.

### Commits pane
15. `visibleCommits` auto-filtered by `SelectedFile`. Set on load (`PR.Files[0].Path`) so the filter is always engaged. AllFilesPath bypasses the filter (see #19a).
15a. Cursor index 0 is the synthetic "All commits" row representing `RangeWholePR`. Cursor space `[0, len(visibleCommits)]`: idx 0 → `RangeWholePR`, idx 1..N → `RangeSingleCommit{commits[idx-1].SHA}`. Label: `All commits (N)` identity, `All commits (M of N)` filtered. Bold; `selectFile` resets `CommitsCursor = 0`. Visual yank skips this row. Annotation slot is fixed to the synthetic `[*]` marker (`m.allRowMarker()`, themed via `theme.DiffLineNumber`), regardless of file selection — mirroring file `[<status>]` there made the row look like a real-commit annotation column-wise; `[*]` keeps it visually distinct as a virtual row. Under `SelectedFile == model.AllFilesPath` (#19a) the filter is dropped → label always `All commits (N)`.
16. `j/k` in Commits auto-selects the cursor row. Visual mode gates this so multi-row yank does not mutate `SelectedRange`.
17. Enter on Commits is a no-op (cursor commit is already auto-selected).
18. `[A]/[M]/[D]/[R]` annotates each commit row that touches `SelectedFile`. Suppressed when `SelectedFile == model.AllFilesPath` — the cross-file browse has no per-file status to display.

### Files pane
19. `j/k` in Files moves `FilesCursor` only — no Diff re-render per keystroke (sluggish). Deliberate selection gestures: `Enter` (commit) or `Shift+J/K` (`advanceFile`, any pane) → `selectFile(path)`. `selectFile` resets `DiffCursor`, `DiffViewport.Top`, `CommitsCursor`, `CommentsCursor` only when path changes. Incsearch (`/`) auto-select retained — typing expects the cursor to follow.
19a. Cursor index 0 is the synthetic "[*] All (N files)" row, symmetric to the Commits pane's "[*] All commits" row (#15a). Cursor space `[0, len(PR.Files)]`: idx 0 → `selectAllFiles()` (sets `SelectedFile = model.AllFilesPath`); idx 1..N → `selectFile(PR.Files[i-1].Path)`. Loader lands on cursor 0 — the [*] row — and sets `SelectedFile = AllFilesPath` so the splash hands off to a PR-wide overview (concat diff across every file); users drill into a single file via `j` + `Enter` (or `Shift+J` to advance without losing focus). FilesCursor and SelectedFile are kept in sync at boot so the cursor never sits on [*] while the Diff/Commits/Comments columns reflect a single file. Under AllFilesPath:
   - `visibleCommits` returns the full commit list unfiltered (#15) so the user can walk the entire PR history.
   - `commitsView` / `allCommitsRow` drop the per-commit `[A/M/D/R]` annotation and the `(M of N)` filtered count (#15a, #18).
   - `patchInfo` reads the pre-built concatenated diff under `diffKey(sha, AllFilesPath)` — `loadPRCmd` builds these in PR.Files order alongside the per-file entries (one for `sha=""` / WholePR + one per single-commit SHA over its touched files).
   - `commentsView` renders the placeholder `(no file selected)\nComments disabled in All view` (two lines so the message fits inside the narrowed Comments column at the 25% default); `commentsForView` / `threadsForCursor` return empty so no `◆` markers appear in Diff.
   - `buildComposeInline` short-circuits with `state.Notice = "comments unavailable in All view (select a file first)"` so Diff Enter / visual+Enter / Comments `r` are blocked.
   - Visual entry on Files idx 0 is forbidden (`handleKey "v"` returns early with `state.Notice = "visual unavailable on the All row"`); Files yank skips idx 0 in both flat and tree modes (symmetric to the Commits-pane skip for All commits).
   - `advanceFile` (Shift+J/K) skips the All row — that gesture is "next file diff", not "browse mode entry". Reach All via Tab to Files + `k` / `gg`.
   - Diff title renders `Diff: All files (N)` (or `Diff: All files (N) @ <sha>` for a single commit) instead of leaking the AllFilesPath sentinel (which contains NUL bytes).
20. Tree mode (`t` toggles): dirs render `v <name>/` (expanded) or `> <name>/` (folded); files show basename + bracketed status `[A]/[M]/[D]/[R]` + comment count. Tree row 0 is the FilesRowAll entry — rendered as `[*] All (N files)` (bold; the `[*]` slot mirrors flat mode and the Commits-pane All row).
21. `autoSelectTree` skips `selectFile` on dir rows so a search-driven cursor jump onto a dir does not clobber Diff. `FilesRowAll` rows route to `selectAllFiles`.
22. `remapCursorOnTreeToggle` preserves the conceptual cursor position when toggling flat ⇄ tree. The All row is shared between modes (flat idx 0 ⇄ tree idx 0).
22b. Enter on a file row (flat or tree) — commit gesture: `selectFile(path)` + `FocusedPane = PaneDiff`. Enter on the All row (flat idx 0 / tree FilesRowAll) — `selectAllFiles()` + `FocusedPane = PaneDiff`. Tree dir rows fold / unfold, focus stays. Files zoom modal Enter = same commit gestures (All / file / dir); modal-Enter path lives in `keys.go::handleKey` because it must clear `Modal` first.

### Comments pane
23. Diff-cursor coupling: `commentsView` shows ONLY threads anchored at the Diff cursor's buffer line AND matching `DiffCursor.Side` (`◆` rows on the active column). Off `◆` → `(no comment at cursor)`, `<space>` no-op. Visible set: `threadsForCursor` filters by buffer index (`commentBufferIndex`) then by `root.Side` (`threadOnSide`). `flatComments` scoped to `threadsForCursor` so cursor never drifts past visible content. Empty `root.Side` → RIGHT (legacy, mirroring GitHub default). Exception — file-overview short-circuit: when the cursor sits on a file-metadata row (`---` / `+++` file headers, `@@` hunk header — kinds `h` / `@` per `diffLineKind`; synthetic `···` rows kind `s` are excluded because their Enter binding owns the gesture), `threadsForCursor` bypasses both the buffer-index and the Side filter and returns the full `threadsForView()` list. Meta rows carry no real file line number so the "thread at this exact anchor" contract has nothing to match; falling back to a file-wide overview lets the user skim every comment from the headers without first finding a `◆` row, and lets Diff Enter on a meta row route to the focus-handoff path (`pane_diff.go::handleKeyDiff`) so Tab+browse works. The placeholder `(no comment at cursor)` is still shown on body rows (`+` / `-` / context) that have no anchored thread.
23b. Render shape: header + indented body.
   - Header: `[ [resolved] ]<name>: <yyyy-mm-dd hh:mm> <hash>[ <range>][ #<id>][ [pending]| [outdated]]` (`CreatedAt.Local()`, `<hash>` = `shortSHA(CommitID)`, `<range>` = `pane_comments.go::formatRangeTag` for multi-line range comments — `R<start>-R<end>` for same-side, `L<start>-R<end>` for mixed-side; empty for single-line and replies — `#<id>` = `ReviewComment.ID` = REST `databaseId`). The `<range>` slot replaces the previous ┌/│ gutter visual (#10): drawing the range shape in 2 cols collided with neighbouring ◆ anchors under markerRank, so the upper edge moved to the header where it cannot conflict. Both endpoints carry their Side prefix so same vs. mixed Side reads at a glance without consulting the underlying comment. The `#<id>` slot is omitted when `ID == 0` (pre-POST drafts before `convertGQLComment` stamps the id). Narrow-width degradation: when the rendered header width (via `lipgloss.Width`) exceeds `paneWidthComments` the `#<id>` slot drops first; if the header still overflows the `<range>` slot drops too, leaving the critical `[pending]`/`[outdated]` state tag intact.
   - `[resolved]` (green, `theme.CommentResolved`) sits at the line head (immediately after cursor / depth indent, before author) so resolved threads can be skimmed at a glance. Driven by `ReviewComment.Resolved` (mirrors GraphQL `PullRequestReviewThread.isResolved`; propagated onto every comment in the thread by `convertGQLComment`).
   - `[pending]` (yellow) / `[outdated]` (red) mutually exclusive at the trailing slot; pending wins. `[resolved]` can co-exist with `[outdated]` (resolved-but-stale); `[resolved]` + `[pending]` is unreachable in practice (drafts have no thread to resolve yet).
   - Body indent: `2 + 2*(depth+1)`.
   - Body line-break mirrors GitHub web: every `\n` → row break; 2+ consecutive `\n`s → one extra blank row.
   - Per-source-line wrap: `paneWidthComments − bodyLeader` via `wrapText`.
23c. Word-boundary rule: `wrapText` → `splitWrapWords` (`styles.go`) splits on whitespace ONLY when both adjacent runes are ASCII word runes. CJK / emoji on either side keeps whitespace inside the running word; `hardBreak` then splits mid-CJK.
24. Cursor movement (`j/k`) auto-scrolls Diff to the cursored comment via `syncDiffToCursorComment`. `h/l` and `backspace` are unbound. `j/k/G/gg` ALSO clamp `AppState.CommentsTop` via `scrollCommentsIntoView` so the cursored header stays inside the pane's viewport — without this, long threads (many replies or wrapped bodies) overflow the box and renderPaneBox silently clips the bottom rows; `j` past the visible edge would advance the logical cursor while the rendered window stayed frozen. The viewport window is anchored on the header row, not the body — a comment taller than the pane still surfaces "where the cursor is" and the body clips below (mirrors how Diff handles a wrapped line taller than its window). `CommentsTop` resets to 0 alongside every `CommentsCursor = 0` site (Diff Enter handoff `focusCommentsAtCursor`, file selection `selectFile` / `selectAllFiles`, commit selection `autoSelectCommit`, Files-modal-Enter, Commits-modal-Enter, Comments `gg`). Mouse wheel and click in Comments call `scrollCommentsIntoView` after mutating the cursor; `commentIndexAtDisplayRow` adds `CommentsTop` to the pane-relative row before walking the layout so clicks on a scrolled-down column resolve to the correct comment. Shared layout helper: `commentsLayout()` returns `(rows, headerAt)` and is consumed by `commentsView` (slice from `CommentsTop`), `scrollCommentsIntoView` (clamp around `headerAt[CommentsCursor]`), and indirectly by the hit-test walker; keeping a single source of truth prevents the scroll math and the render from drifting. Viewport height fallback ladder mirrors `diffViewportHeight()`: `paneHeightComments → height-16 → 5`.
24b. Comments Enter / `r` split:
    - Enter → `startComposeEdit` opens an in-place body edit on the cursor comment when `User == AppState.ViewerLogin`. Body preloaded; saved body POSTs via `updatePullRequestReviewComment`. Foreign user → `state.Notice = "cannot edit comments by other users (press r to reply)"`.
    - `r` → `startComposeReply` replies to the thread under the cursor via `addPullRequestReviewThreadReply`. No-op when cursor is not on a `◆` row.

    Both share the body / POST / status-bar lifecycle of inline compose (#14, #27). Inside the Comments zoom modal, the same handler runs.
25. HEAD vs single-commit visibility: HEAD/WholePR view hides outdated (`c.Outdated`); single-commit view shows comments anchored to that SHA via `CommitID` or `OriginalCommitID`.
25b. Threads always expanded. `flatComments` and `commentsView` walk every reply.

### Visual mode + yank shapes
26. `v` enters, `y` yanks and exits, `Esc` exits without yanking.
27. Yank shapes:
    - Files: path (or paths joined by `\n` for visual range)
    - Commits: `<short_sha> <subject>`
    - Comments: `<user> @ <date>\n<body>`
    - Diff: line content with the leading column (`+`/`-`/` `) stripped on add/del/context rows so a mixed-range paste keeps consistent indentation; headers (`---`/`+++`) and hunks (`@@`) verbatim; synthetic `···` rows skipped. Visual range = rows joined by `\n`. Helper: `internal/tui/visual.go::stripDiffPrefix`.

### Compose (pending PR comment input)
The compose flow POSTs into the user's pending (draft) review. Submission to public is intentionally NOT exposed; users finalize via web UI or `gh api graphql`.

27a. `AppState.Compose *ComposeState` is a global overlay state, peer to `Visual` / `Modal` / `HelpOpen`. While non-nil, `handleKey` routes every keystroke to `handleKeyTextarea`; background panes frozen.
27b. Lifecycle (`ComposeStatus`):
   - `ComposeEditing` — body collection.
     - Default `UseTextarea = false`: runs `$EDITOR <tempfile>` on `gh-reva-compose-*.md` (so `EDITOR='code --wait'` works); tempfile pre-populated for `ComposeEdit`.
     - vim-family detection (`internal/tui/compose.go::startInsertFlag`): first whitespace token of `$EDITOR` / `$VISUAL` after stripping leading dir + `.exe`. Match set: vim, nvim, vi, gvim, mvim. Match → inject `+startinsert` before the file arg so the buffer opens in Insert mode. Non-match → unchanged.
     - Dispatch by `$TMUX`:
       - Unset → `tea.ExecProcess(sh -c "$EDITOR <tempfile>")` releases the alt-screen; editor takes the whole terminal.
       - Set → `runEditorOverlay` calls `cmd.Run()` in a `tea.Cmd` goroutine on `tmux display-popup -E -w 80% -h 80% <shellCmd>`. `tea.ExecProcess` bypassed because the popup overlays reva's pane via tmux server; releasing alt-screen would redraw the shell underneath. `-E` blocks the client and closes the popup on any editor exit code (`:q!` returns cleanly).
     - Tests force-empty `TMUX` in `e2e/helpers/launch.mjs`.
     - `UseTextarea = true`: `overlayCompose` modal collects rune-by-rune; Ctrl+S saves, Esc / Ctrl+C cancels.
   - `ComposeSubmitting` — `submitComposeCmd` in flight. Status bar: `posting to GitHub…`. Esc / Ctrl+C detaches.
   - `ComposeFailed` — POST errored. `Body` + `ErrMsg` preserved; Ctrl+S retries, Esc cancels.
27c. Inline (`Kind = ComposeInline`): anchored via `ResolveAnchor`. `Path = state.SelectedFile`; `CommitSHA = state.PR.HeadSHA` always (comments anchor to PR head, mirroring web). Header / hunk rows rejected. Side resolution: `+` → RIGHT (fixed), `-` → LEFT (fixed), CONTEXT → `DiffCursor.Side` (user's h/l choice). Override fires only on context rows because `+`/`-` exist on one column and `lineExistsOnSide` / auto-skip already pin `cursor.Side`. Line number recomputed via `lineForSide` after Side override → LEFT context anchor reports `OldLine`, not `NewLine`.
27d. Multi-line range: enter Diff visual, move cursor, Enter. `ResolveRange` collapses anchor + cursor into `(start_line, start_side) → (line, side)` normalized by buffer position. Mixed-side ranges accepted as-is. Single-line ranges drop `start_*` fields.
27e. Reply (`Kind = ComposeReply`) captures the cursor thread's GraphQL node ID via `threadIdentityForCursor` → `addPullRequestReviewThreadReply`.
27e2. Edit (`Kind = ComposeEdit`) captures comment node ID via `buildComposeEdit` and pre-loads the body. Mutation `updatePullRequestReviewComment`; gated by `User == ViewerLogin`. Anchor stitched back from cached comment list (response carries only the comment row).
27f. Pending review session: `ghClient.ensurePendingReview` queries `reviews(states: [PENDING], first: 50)` filtered by `viewer.login` (NOT `viewerLatestReview` — that one hides a PENDING draft when a non-PENDING review by the same viewer is more recent, which 422s the next `addPullRequestReview` under GitHub's "one pending per user per PR" rule). Falls back to `addPullRequestReview` if none owned. Cached on `pendingReviewID[n]` for process lifetime. `viewerLogin` exposed via `Client.ViewerLogin(ctx)` for the edit gate.
27g. POST routing (`submitComposeCmd`): Inline → `addPullRequestReviewThread`; Reply → `addPullRequestReviewThreadReply`; Edit → `updatePullRequestReviewComment`. Success path: `convertGQLComment` → `model.ReviewComment` (`Pending` from review state); `applyComposeSubmitted` appends (Inline / Reply) or replaces by NodeID (Edit) AND drops `m.threadsCache.valid` so the cached thread tree rebuilds. Header tags Pending entries `[pending]` (`theme.CommentPending`).
27h. Status-bar contexts handled in `internal/tui/statusbar.go` (#6).
27i. Post-compose refresh: `applyComposeSubmitted` queues `refreshCommentsCmd` → re-runs `Client.ListComments`. `mergeRefreshedComments(local, refreshed)` preserves locally-known Pending whose NodeID is absent from the refresh (`pullRequest.reviewThreads` has eventual-consistency lag; a naive REPLACE drops the just-posted draft). Edit POSTs flip body in place by NodeID. `CommentCount` recomputed. `applyCommentsRefreshed` also drops `m.threadsCache.valid`. Refetch failure tolerated silently. Success clears `CommentsHidden` (#30c) so a draft posted from Diff while the column was hidden becomes visible; failure leaves the toggle alone.
27j. Confirm gate. Entry points (Diff Enter, Diff visual range Enter, Comments Enter on own, Comments `r`) park the built `ComposeState` in `AppState.PendingConfirm` — editor not started yet.
   - Guard in `keys.go::handleKey` routes every keystroke through `handleKeyConfirm` while `PendingConfirm != nil` (sits ahead of Compose / Help / Visual absorbers).
     - `y` / `Enter` → `confirmComposeStart` (PendingConfirm cleared, payload → `Compose`, Visual cleared for inline ranges, body collection begins).
     - `n` / `Esc` / `q` / `Ctrl+C` → `cancelComposeConfirm` (payload discarded; Visual preserved so the user can refine).
   - Render: centered confirm modal (`internal/tui/confirm.go::overlayConfirm`), layered above every other overlay (zoom modal, Help, compose textarea).
     - Title: action verb (`Start new comment?` / `Post reply?` / `Edit comment?`).
     - Body: target subject — Inline `<path>:<line> <SIDE>` (or `<path>:<start>-<line> <SIDE>` for ranges); Reply `<path>:<line> by <root.User>` (from PR.Comments lookup); Edit `<path>:<line> <SIDE>` (from NodeID lookup).
     - Footer: `[y]es   [n]o`.
   - Status bar intentionally NOT mirroring the prompt — focused-pane hint stays visible so URL / keymap context survives the confirm step.
   - Foreign-author Comments Enter short-circuits inside `buildComposeEdit` with a `state.Notice` — no confirm queued.
   - `buildComposeInline` deliberately does NOT clear `Visual`; that mutation lives in `confirmComposeStart` so the highlight survives the y/n prompt.

### Search (global `/`)
27k. `AppState.Search *SearchState` — global overlay, peer to Compose / Visual / Modal / PendingConfirm. Two phases: `SearchEditing` (incsearch input collection), `SearchActive` (post-Enter; n/N cycles). State machine: `internal/tui/search.go`. `keys.go` slots the Editing absorber between Compose and Help. Active falls through to normal dispatch — n/N intercepted before per-pane handlers; everything else (j/k etc.) works with the cursor parked on the current match.
27l. Lifecycle:
   - `/` (normal) → `startSearch` snapshots cursor state into `SearchState.Saved*` (FilesCursor, CommitsCursor, DiffCursor, DiffViewport.Top, CommentsCursor, SelectedFile, SelectedRange, FocusedPane); `Status = SearchEditing`; `TargetPane = m.state.FocusedPane`.
   - `/` (Active) → re-enter Editing, saved snapshot retained (vim convention).
   - `/` (Comments) → silent no-op. Disabled until modal-vs-flat UX decided. Comments hint omits `/:search`.
   - Printable rune → `recomputeSearch` (smart-case literal substring) + `applySearchCursor` (jumps live cursor to nearest match ≥ saved position). Files / Commits auto-select via the j/k path; Diff calls `scrollDiffIntoView`.
   - Backspace → drops one rune; empty query → `cancelSearch`.
   - Enter → `commitSearch`. Empty / no-match → `cancelSearch` + `state.Notice = "no match: <query>"`.
   - Esc / Ctrl+C (Editing) → `cancelSearch` restores every Saved* field.
   - Esc / Ctrl+C / Tab / Shift+Tab (Active) → clears `state.Search` (no Saved* restore). Tab / Shift+Tab additionally advance focus.
27m. Per-pane match providers (`search.go`): Files matches `FileEntry.Path` (or `FilesRow.Path` in tree mode); Commits matches `Commit.Message` / `SHA` / `ShortSHA` and emits cursor `i+1` to skip the "All commits" row; Diff matches each `patchLines()` entry by buffer index; Comments has `collectCommentMatches` but `/` rejected for now. Smart-case via `smartCaseFold`: lowercase → fold; any uppercase → exact.
27n. Match highlight: `theme.SearchMatchBg` (theme-uniform muted dark yellow, `#574b00`). Files / Commits / Files-tree dirs wrap each occurrence via `Model.searchHighlight` → `highlightMatches` (byte-indexed; CJK / mixed bodies stay correct because `strings.ToLower` preserves byte length on non-ASCII runes). Commits short-SHA rendered without highlight to avoid lipgloss SGR-nesting collisions; cursor `>` carries the signal for sha-only matches. Diff applies `bgRow(_, theme.SearchMatchBg)` per buffer line in `searchMatchLines()` — visual-range bg wins. Match-highlighted Diff rows skip `rowCache` (match set drifts per keystroke).
27o. Status-bar contexts: `hintSearchEditing` replaced by live `/<query>_` prompt; Active shows `n:next  N:prev  /:edit  esc:clear  [idx/count] /<query>`. Suffix dropped in both. Per-pane normal hints carry `/:search` EXCEPT `hintComments`.

### Global keys
28. Tab / Shift-Tab cycle Files → Commits → Diff → Comments. Only keys that move focus across panes.
29. Enter is the commit / focus-handoff / compose-entry gesture; never quits. Backspace unbound everywhere. Per-pane:
    - Files file row (flat or tree) → #22b. Tree dir row → fold/unfold.
    - Files / Commits zoom modal → close modal AND shift FocusedPane to PaneDiff. Files modal also commits `selectFile`.
    - Commits (normal pane) → no-op (#17).
    - Diff uncommented row → inline compose (#14, #27d). Header / hunk rows no-op.
    - Diff commented row → focus to Comments (#14); auto-reveals column if hidden.
    - Comments → #24b.

    While `state.Compose != nil`, `handleKey` absorbs every keystroke.
30. Shift+J / Shift+K advance to next/prev file from any pane via `advanceFile(forward bool)`. Focus preserved.
30b. `gg` / `G` work in every pane (#14b). `/` opens search scoped to the focused pane (#27k–n). `n` / `N` cycle while Search is Active.
30c. `Ctrl+E` toggles `AppState.CommentsHidden`. Hidden: right column width 0, saved width added to Diff via `splitColumnWidths(total, hidden, commentsPct)`. Stacked-fallback (`m.width<=0` or `bodyHeight<8`) drops Comments from the join. Hide while `FocusedPane == PaneComments` → focus → Diff. Tab / Shift+Tab skip Comments while hidden. `focusCommentsAtCursor` (Diff Enter handoff #14) auto-reveals first.
30c2. `1` / `2` / `3` / `4` jump focus directly to Files / Commits / Diff / Comments. Implementation: `keys.go::jumpToPane`. Cleanup contract mirrors Tab — `PendingPrefix` cleared, Active search dropped (n/N stops intercepting), open zoom modal closed via `closeModal()`. Visual selection is also cleared (jump is a positive navigation gesture, not a cancel — leaving an anchor on the prior pane would strand the next visual entry). Jumping to Comments while `CommentsHidden == true` auto-reveals the column (`CommentsHidden = false` + `tea.ClearScreen` returned, same redraw rationale as `Ctrl+E` #30c). Status bar: `1-4:jump` slot inside `statusCommonSuffix`. Help modal: `1 / 2 / 3 / 4` row in the Global section (`help.go::helpSections`). Absorber ladder unchanged: `Compose`, `PendingConfirm`, `Search.SearchEditing`, `HelpOpen` swallow first; Visual routes 1/2/3/4 through the same `jumpToPane` ahead of the per-pane handoff so `v` + digit cancels visual and lands on the target.

### Mouse
30d. Mouse capture: `tea.WithMouseCellMotion()` in `cmd/root.go`. Cell motion sufficient (no in-app drag selection). Standard terminals honor `Shift`-held drag as terminal-side text selection while tracking is active → copy/paste works without `--no-mouse`. `Update` routes `tea.MouseMsg` through `(Model).handleMouse`; only `MouseActionPress` dispatches.
30e. Hit testing (`internal/tui/mouse.go::paneAt`): `(x, y)` → pane + content row. Pane outer rect: row 0 = top border, 1 = title, 2 = divider, [3, h-1) = content, h-1 = bottom border. Borders / dividers / side bars / status-bar reject. Title hit → `OnTitle=true`; content hit → `ContentRow / ContentCol`. Stacked fallback (`m.width<=0` or `bodyHeight<8`) and loading phase (`PR == nil`) reject.
30f. Per-pane click (`handleMouseClick`): sets `FocusedPane = hit.Pane`, then dispatches when `OnTitle == false`.
   - Files: `mouseClickFiles` moves `FilesCursor` AND `selectFile(path)` — j/k stays cursor-only (#19), but a click is deliberate one-shot. Tree-mode dir rows fold / unfold instead. Focus stays on Files.
   - Commits: `mouseClickCommits` sets `CommitsCursor` + `autoSelectCommit` (mirrors j/k #16).
   - Diff: `mouseClickDiff` resolves buffer line via `bufferLineAtDiffDisplayRow` (wrap-aware reverse of `displayRowsForLine`); calls `scrollDiffIntoView`.
   - Comments: `mouseClickComments` resolves flat-comment index via `commentIndexAtDisplayRow`; calls `syncDiffToCursorComment`.
30g. Wheel (`MouseButtonWheelUp` / `Down` → `handleMouseWheel(hit, ±1)`): focus unchanged. Files: cursor only. Commits: cursor + `autoSelectCommit`. Diff: `DiffCursor.Line` ± 1 + `scrollDiffIntoView`. Comments: `CommentsCursor` ± 1 + `syncDiffToCursorComment`.
30h. Absorbed layers: `Compose != nil` / `PendingConfirm != nil` / `HelpOpen` / `Modal != nil` / `Search.Status == SearchEditing` short-circuit `handleMouse` to no-op. SearchActive falls through (n/N + wheel both live so wheel can re-position the cursor on a match).
30i. Layout measurement: `(*Model).measureLayout` populates `paneWidth* / paneHeight*` from the current terminal size. Called once at the top of `Update` (before the type switch — covers KeyMsg / MouseMsg / ScrollDiffToLineMsg etc. uniformly) and once at the start of `View()` for the first frame. Persistence rides on `Update`'s value-receiver return: Bubbletea stores the returned Model so paneWidth* survive between messages. View's mutations never persist (string-only return). Stacked fallback (`m.width<=0` or `bodyHeight<8`) skips assignment.

### Color theming
31. `internal/theme.Theme` is the single source of truth — 28 `lipgloss.Color` fields plus `SyntaxStyle *chroma.Style`. `Resolve(name)` accepts `"builtin-dark"`, any chroma registry name, or `""` (→ `defaultThemeName`). Unknown names error.
32. Chroma adapter (chroma token → UI role) in `internal/theme/chroma.go`. Two overrides: `DiffPlusBg` / `DiffMinusBg` are hard-coded (`#172319` / `#23171a`, muted dark green / muted dark red); `GenericInserted` / `GenericDeleted` go through `pickAccent`, which prefers `StyleEntry.Background` when `StyleEntry.Colour` equals editor background (gruvbox-style inversion). Without these the +/- distinction collapses on inversion themes.
33. `m.theme` is non-nil after `NewModel` (constructor seeds `defaultThemeName`). `cmd/root.go` overrides via `Model.SetTheme`.
34. Color application via `internal/tui/colors.go` (`fg`, `fgBold`, `bgRow`); no-op on zero-value colors. Apply AFTER `padTrunc` / cell assembly so width math stays driven by visible cells.
35. `padTrunc` is SGR-aware (`lipgloss.Width` to measure, `ansi.Truncate` over-width). Right-pads with plain spaces.
36. Pane border / title coloring in `app.go::renderPaneBox`. Active uses `PaneBorderActive` + `PaneTitleActive` (Bold); inactive uses `PaneBorderInactive` + `PaneTitle`.
37. Visual-mode rows in Diff carry NO row-wide bg — range membership is signaled by the `> ` glyph alone (mirrors Files / Commits / Comments). In split mode the bg used to leak onto the opposite lane even though h/l is locked (#14c) and j/k auto-skips opposite-side rows, which falsely implied both columns were selected; the previous `bgRow(row, theme.VisualRangeBg)` calls in `renderUnifiedBufferLine` / `renderSplitBufferLine` are gone. `inVisual` still drives `> ` on continuation rows (#12) and row-cache exclusion (#39) — only the bg call is dropped. SearchMatchBg in the same code path is independent and unchanged.
38. Diff cells: bg-for-change + per-token syntax fg (`syntax.go::styledDiffCell`). `+` rows: `DiffPlusBg` row-wide + chroma fg per token; `-` similar with `DiffMinusBg`. Context rows pass `bg=""`. File / hunk headers stay flat-fg. Leading marker excluded from lexer and re-emitted bold under same bg with theme-uniform `theme.DiffPlus` / `theme.DiffMinus` (`#3fb950` / `#f85149`); marker fgs are NOT in the `syntaxCache` key.
39. `Model` caches that must propagate across Bubbletea's value-copied Updates:
   - `syntaxCache` — `*syncMap` keyed on `lexer.Name + style.Name + bg + cell`. Pointer identity shared.
   - `rowCache` — `*diffRowCache` (`map[string][]string`) keyed on split `(s, lineIdx, halfW, leftMarker, rightMarker)` or unified `(u, lineIdx, 0, marker)`. cursorSide intentionally NOT keyed — affects only cursor / visual rows, both of which take the no-cache path; including it would over-invalidate non-cursor rows on every h/l. Width / patch identity changes invalidate via `m.invalidateRowCacheIfStale()`. Skips cursor + visual-range + match-bg rows.
   - `patchLinesC` — struct value (`patchLinesCache`); `cache` is `map[string]*patchInfo` keyed on `diffKey(sha, path)`. Maps are reference types so struct-value embedding propagates — replacing with slice / scalar breaks propagation. `patchInfo` carries: `lines`, `specs`, `newNums`, `oldNums` (all eager now — `lines` and `specs` come from `diff.Expand` + `diff.ParseSpecsAug` in one pass; numbers derive lazily from `specs`), plus `gaps map[int]diff.GapInfo` (#14d synthetic row metadata) and `markers *sideMarkers` + `markersGen uint64` (commentLineMarkers cache; see #39a). Invalidation: `m.invalidatePatchInfoCache(ExpandKey)` drops the slot (called by `handleEnterOnSynthetic` after Expand-state mutates); `m.invalidatePatchInfoCacheForRef(ref, path)` drops both the WholePR and single-commit slot (called by `applyFileContentsLoaded` after a prefetch lands).
   - `threadsCache` — `*threadsViewCache`: single-entry memo of `threadsForView()`. Key: `(SelectedFile, SelectedRange.Kind, SelectedRange.SHA)`. Mutation sites for `m.state.PR.Comments` MUST set `m.threadsCache.valid = false`: `applyComposeSubmitted` (`compose.go`) and `applyCommentsRefreshed` (`refresh.go`) are the only two; new sites do the same. Each successful rebuild bumps `gen` (uint64).
   - `AppState.ExpandedContext map[ExpandKey]*ExpandState` (#14d) — per-(file, range) counters for revealed gap regions. Mutations only via `handleEnterOnSynthetic`. Reading happens inside `patchInfo()` before the `diff.Expand` call.
   - `AppState.FileContents map[FileContentsKey][]string` (#14d) — NEW-side file body per (ref, path). Populated by `applyFileContentsLoaded` after `fetchFileContentsCmd`. Read by `patchInfo()` to drive synthetic emission in `diff.Expand`.

   Hot-path rule: never call `strings.Split(patch, "\n")` or `parseDiffSpecs(patch)` directly. Go through `m.patchLines() / m.patchSpecs() / m.patchNewLineNumbers() / m.commentLineMarkers() / m.threadsForView()`. For compose anchoring on the augmented buffer use `m.resolveAnchorAug / m.resolveRangeAug` (NOT `diff.ResolveAnchor` — that walks the raw patch and would mis-index past synthetic / expanded-context rows).

39a. `commentLineMarkers` caches its `sideMarkers` result on `patchInfo.markers` keyed by `markersGen == m.threadsCache.gen`. Call `m.threadsForView()` BEFORE reading the markers cache so `gen` reflects the latest invalidation — `commentLineMarkers` does this internally; new callers must keep the order if they re-implement.
40. `waitReady` defaults to 10s in `e2e/helpers/launch.mjs` to absorb chroma init + first-frame tokenization.
41. `session.press` / `session.type` are wrapped with a 120ms settle in `launchReva`. Don't reach for `session.press` in helpers — use the wrapped session.
42. Pane modal (`<space>` zoom): gated by `model.ModalState{Pane, Origin}`. Toggled by `<space>` in Files / Commits / Comments; Diff `<space>` is split⇄unified. Closes on tab / shift+tab / `?` / esc / `q` / Ctrl+C; `q` and Ctrl+C quit only when modal closed. `J` / `K` leave modal open by design. Visual mode allowed inside; Comments Enter / `r` work in Comments modal. Title row uses single leading space (`│ Files`) — distinct from regular pane titles (`▶ ` / `  `); the e2e detection signature.
42b. Modal focus restore: `ModalState.Origin` records the opener. `toggleModal` reads `m.state.FocusedPane` at open. Close routes through `closeModal` → `FocusedPane = Origin` then clears `Modal`. Tab / Shift+Tab / `?` (Help) also call `closeModal` first. Files-modal-Enter / Commits-modal-Enter bypass `closeModal` and explicitly set `FocusedPane = PaneDiff`.

---

## 5. E2E test conventions

### Helpers (`e2e/helpers/launch.mjs`)
- `launchReva({ args, fixture, cols, rows, env })` — spawn gh-reva with default fixture.
- `waitReady(session, { timeout = 5000 })` — wait for `Files` text after PR load.
- `quit(session)` — send `q`, then close.
- `activePaneLabel(session)` — return the single active pane name; throw if 0 or > 1.
- `paneText(screen, label)` — extract the pane's column slice. Required when asserting cursor markers (`^>`) in non-leftmost panes — borders place `│` at col 0. Trailing `│` stripped.
- `countSelectedRows(screen, label)` — count `> ` rows in the pane's slice.

### Patterns
- Read-only observation: `describe + before + screen capture` (capture once, many `test()` blocks).
- Navigation tests beginning + ending at Files focus: `describe + before/after + shared session`.
- State mutation (visual, file selection, single-commit drill): independent `test()` blocks.

### Substring rules
- Prefer short, contiguous substrings (≤ ~20 chars) for column-wrap safety.
- Anchor on column slice via `paneText` for cursor rows. Borders break full-screen `^` anchors.
- Substring negation (`!includes`) usually works on raw screen.

### Fixture choice
- Default → `testdata/sample-pr.json`.
- Long-comment wrap → `testdata/wrap-pr.json`.
- Performance / large-PR → `testdata/large-pr.json` + responsiveness assertion.
- Add a new fixture rather than extending `sample-pr.json` for unusual content.

---

## 6. Common pitfalls

- Forgot to rebuild: `go build -o gh-reva .` (pretest handles `pnpm test`; targeted `node --test` does not).
- `^>` regex on raw screen: borders place `│` at col 0. Use `paneText`.
- Long substring assertions: column wrap splits words. Shorten or normalize.
- bubbletea startup ~1s blank: first `s.text()` after launch can be empty. Use `waitReady`.
- tuistory cannot reliably emit CSI Z: shift-tab tests are skipped (C2). Verify by inspection.
- Do not reintroduce `lipgloss.Border()`: boxes are rendered manually via `renderPaneBox`.
- Tabs in Diff: see §4 #9.
- CJK / wide chars in Comments: `wrapText` measures with `runewidth.StringWidth` / `runewidth.RuneWidth`. Do not reintroduce `utf8.RuneCountInString`.
- Diff wrap: see §4 #12. Helpers: `diffViewportHeight()` (display rows), `displayRowsBetween` (buffer ↔ display).
- Color SGR doesn't reach tuistory's `text()`: ghostty parses ANSI into cell state. The A9 smoke test guards against raw `\x1b` leaking.
- Chroma case quirk: registry key `rpgle` resolves to a Style whose `Name` is `RPGLE`. `theme.Resolve` canonicalizes on the registry key.
- Bubbletea v1 has no color profile option: `lipgloss.SetColorProfile(termenv.Ascii)` and `SetHasDarkBackground(true)` must be called BEFORE `tea.NewProgram`. `cmd/root.go` does this; new entry points must replicate.
- Chroma init is eager (~500ms cold). Don't import `chroma/v2/styles` or `chroma/v2/lexers` outside `internal/theme` and `internal/tui/syntax.go`.
- Cache pointer identity: see §4 #39. Dropping `Model.syntaxCache` makes e2e fail on `waitReady`; deep-copying caches in `NewModel` or making `patchLinesC.cache` non-reference breaks propagation.
- rowCache key over-keying: fields affecting only cursor / visual rows (already cache-skipped) belong OUT of the key — extra keying turns h/l / cursor moves into full invalidation. Minimal split key: `(s, lineIdx, halfW, leftMarker, rightMarker)`.
- `m.state.PR.Comments` mutation sites must drop `m.threadsCache.valid = false`. Threads cache feeds `commentLineMarkers` via `gen` → stale flag stales gutter markers too.
- `measureLayout` lives ONLY at top of `Update` (pre-type-switch) + start of `View()`. Per-handler re-measure is redundant and was the source of the "j/k doesn't scroll on wrapping diffs" bug.
- `s.press` / `s.type` are auto-settled (120ms) in tests via `launchReva`. Don't add manual `await sleep(N)`; use `await s.waitForText(<expected>)` if a test still races.
- `launchReva` forces `TERM=tmux-256color` via `sh -c`. Why: bubbletea v1's `tea_init.go` calls `lipgloss.HasDarkBackground()` at package import → termenv sends OSC 11 + DSR queries (blocks up to 5s); termenv short-circuits when `TERM` starts with `screen` / `tmux`. Tuistory's `session.js` hard-codes `TERM: 'xterm-truecolor'`, so the value can't pass via `env:` — the wrapper re-applies `TERM` before `exec`.

---

## 7. Output / commit conventions

- Chat replies: Japanese, neutral professional. No slang, no emojis, no self-deprecating hedges.
- Code identifiers, log/error messages, comments, PR templates: English.
- CLAUDE.md, prompts, agent instructions, skill definitions: English.
- Cite file locations with `path:line` (e.g. `internal/tui/pane_diff.go:144`).
- Cite evidence URLs at the end of any research-based reply.

### Commit
- Commit only when explicitly requested.
- Never push to main / master; never force-push. Tag pushes allowed when explicitly requested as part of a release (§8).
- Subject ≤ 70 chars; body explains the why if non-obvious.
- Trailer: `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.
- Stage by name when feasible. `git add -A` allowed for the initial commit and when `.gitignore` is known correct.

---

## 8. Release procedure

Driven by `v*` tag pushed to `origin`. `release.yml` runs goreleaser; version from `{{.Version}}` (= the tag); produces `gh-reva_<os>-<arch>` binaries. Hyphen required — gh CLI matches assets by `strings.HasSuffix(name, "<os>-<arch>")`, so `_` in that slot breaks `gh extension install` (see `.goreleaser.yaml:20-25`). NO `version.go` to bump and NO changelog — the tag is the single source of truth.

### Steps for a patch / minor / major release

Run from repo root. Replace `vX.Y.Z`.

1. Pre-flight (must all pass before tagging):
   ```sh
   git status
   go vet ./... && go test ./...
   (cd e2e && pnpm test)
   git log --oneline $(git describe --tags --abbrev=0)..HEAD
   ```
2. Pick next version from `git tag --sort=-v:refname | head -1` and apply SemVer.
3. Commit pending work with Conventional Commits style.
4. Bump `e2e/package.json` version to match (no `v` prefix). Convention: `chore(release): bump e2e workspace to vX.Y.Z` as a separate commit.
5. Annotated tag at HEAD: `git tag -a vX.Y.Z -m "vX.Y.Z"` (use `-a`, not lightweight, so `git describe` works).
6. Push master + tag atomically: `git push origin master vX.Y.Z`.
7. Watch: `gh run watch --exit-status`. On failure, fix forward and re-tag with the next patch — never delete and re-push the same tag.
8. Smoke-verify:
   ```sh
   gh release view vX.Y.Z
   gh extension install ktrysmt/gh-reva --force
   gh reva --version
   ```

### Things that REQUIRE explicit user authorization
- Any push to `main` / `master` (release flow only).
- Tag creation + push.
- `gh release edit` / `gh release delete`.

User saying "release してほしい" / "release まで進めて" / "patch +1 で release" counts as explicit authorization for the full §8 sequence; partial requests like "commit して" do not authorize tagging or pushing.
