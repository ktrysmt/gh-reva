# CLAUDE.md — gh-reva development conventions

`gh` extension that opens a vim-like 4-pane TUI for reviewing GitHub PRs.
Built on `bubbletea` + `lipgloss`. Single-purpose CLI; no shared infrastructure.

This file is authoritative. Read once at session start; update when an
invariant changes. For detail not pinned here, the cited source files are
the source of truth.

---

## 1. Build / test commands

```sh
# Repo root: /Users/dew/workspace/gh-reva

# Go
go build -o gh-reva .          # produces ./gh-reva (NOT `go build ./...`)
go vet ./...
go test ./...

# Manual TUI
./gh-reva --fixture testdata/sample-pr.json
./gh-reva --fixture testdata/large-pr.json
./gh-reva --fixture testdata/sample-pr.json --slow-load 500ms

# E2E (cd e2e first)
pnpm install
pnpm test                      # full suite; pretest auto-rebuilds gh-reva
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
- `--config <path>` — load `reva.toml`. Defaults to `$XDG_CONFIG_HOME/reva.toml`, then `$HOME/.config/reva.toml`. Schema today: `[syntax.extensions]` table mapping a filename suffix (e.g. `".j2"`) to a chroma lexer name or alias (`yaml`, `jinja`, …). An explicit `--config` whose target does not exist is a hard error; implicit search silently tolerates absence. `internal/config/config.go` is authoritative; lexer override applied in `internal/tui/syntax.go::lexerFromOverride` (longest-suffix-match wins; unknown lexer names fall back to chroma's default extension matcher).

---

## 2. Workflow discipline

### TDD is mandatory
1. Write the failing test(s) first.
2. Run targeted, confirm failure with the actual assertion mismatch (not a timeout / build break).
3. Implement.
4. Run targeted, confirm pass.
5. Run full e2e (`pnpm test`), confirm no regressions.
6. If unrelated tests fail under the new behavior, update them in the same change. Never leave the suite red.

Skipping the failing-test-first step is forbidden — even for trivial changes; it surfaces wrong assertions and missed edges.

### Decision-first vs. action-first
For non-trivial design space (key bindings, fallback semantics, visual markers), present 2–3 options with tradeoffs and ask the user to pick before writing tests. For straightforward asks, proceed directly with TDD.

### Risky operations require confirmation
Confirm before: `git push`, `git reset --hard`, force push (push to main / master forbidden without explicit direction); deleting fixtures / snapshots; adding top-level Go deps; renaming branches.

The user runs `git commit` unless they explicitly delegate it.

---

## 3. Architecture

```
gh-reva/
├── cmd/root.go                     # CLI entry, flags
├── main.go
├── internal/
│   ├── config/                     # reva.toml loader (XDG ladder + --config)
│   │   ├── config.go
│   │   └── config_test.go
│   ├── api/                        # GitHub client (go-gh) + fixture
│   │   ├── client.go               # Client iface (read + pending POST + submit)
│   │   ├── pr.go                   # GetPR / ListCommits / ListFiles
│   │   ├── diff.go                 # GetFileDiff (PR-wide and per-commit)
│   │   ├── paginate.go             # Link-header pagination (REST)
│   │   ├── resolve.go              # ResolveCurrentBranchPR + ParseTargetArg
│   │   ├── graphql_comments.go     # ListComments + reviewThread mapping
│   │   ├── graphql_post.go         # ensurePendingReview + thread/reply/edit/submit
│   │   ├── fixture.go              # loads testdata/*.json + WithSlowLoad + in-mem POST
│   │   ├── error_client.go         # --simulate-error
│   │   └── ghclient_errors_test.go # httptest 401 / 404 / 429 / pagination
│   ├── clipboard/
│   ├── diff/                       # patch parsing + side resolver
│   │   ├── parse.go
│   │   ├── render_split.go
│   │   ├── render_unified.go
│   │   └── side.go                 # ResolveAnchor / ResolveRange (Compose anchor)
│   ├── model/                      # AppState + value types
│   ├── theme/                      # color palette
│   │   ├── theme.go                # Theme, Resolve, ListThemes
│   │   ├── builtin.go              # builtin-dark fallback palette
│   │   ├── chroma.go               # chroma styles → Theme adapter
│   │   └── theme_test.go
│   └── tui/
│       ├── app.go                  # Model, View(), layout, loadPRCmd, renderPaneBox
│       ├── keys.go                 # global key dispatch
│       ├── messages.go             # tea.Msg types
│       ├── styles.go               # paneTitle / fitPaneTitle / wrapText / styled* helpers
│       ├── colors.go               # fg / fgBold / bgRow lipgloss wrappers
│       ├── syntax.go               # styledDiffCell + chroma lexer + token cache
│       ├── statusbar.go            # composeStatusBar (authoritative for the bar)
│       ├── splash.go               # 3 splash layouts + 3 ASCII REVA designs
│       ├── pane_files.go           # filesView + j/k auto-select + advanceFile
│       ├── pane_commits.go         # commitsView + j/k auto-select + allCommitsRow
│       ├── pane_diff.go            # diffView + split + ◆ gutter + tabs
│       ├── pane_comments.go        # commentsView + word wrap + diff auto-scroll
│       ├── files_tree.go           # tree mode rendering
│       ├── visual.go               # visual mode + yank
│       ├── modal.go                # `<space>` zoom modal
│       ├── compose.go              # Pending-comment compose state machine + $EDITOR + POST
│       ├── compose_test.go
│       ├── textarea.go             # in-app textarea fallback + compose modal rendering
│       ├── refresh.go              # refreshCommentsCmd / mergeRefreshedComments
│       ├── refresh_test.go
│       └── diffmap.go              # newLineNumbers / commentThreadIndexForDiffLine
├── testdata/
│   ├── sample-pr.json              # default (5 files, 3 commits, 4 comments)
│   ├── large-pr.json               # 60 commits / 120 files / 122 KB
│   ├── wrap-pr.json                # single long-bodied comment
│   └── gen_large_fixture.go        # //go:build ignore
└── e2e/
    ├── helpers/launch.mjs          # launchReva / paneText / countSelectedRows
    └── tests/                      # node:test + tuistory
```

### Receiver conventions
- Mutating helpers: pointer `(m *Model)`. E.g. `selectFile`, `advanceFile`, `scrollDiffIntoView`.
- Pure queries / renderers: value `(m Model)`. E.g. `filesView`, `diffView`, `visibleCommits`.
- `handleKey*` are value receivers; mutate via Go auto-addressing.
- `m.state` is `*model.AppState`; mutations propagate regardless of receiver kind.

### Single source of truth
- `model.AppState` owns all mutable state. No globals beyond constants.
- `m.state.SelectedFile` drives the app: `visibleCommits`, `commentsForView`, and Diff cache all key on it.
- Per-pane render budgets (`paneWidthFiles`, `paneHeightDiff`, …) set by `View()`, read by pane renderers.

---

## 4. TUI invariants

Load-bearing — breaking any of them breaks at least one e2e test, and several break the user's mental model. Keep numbering stable; other items reference these indices.

### Layout
1. 3-column bordered layout: Files+Commits stacked left; Diff middle; Comments right. Each pane is its own `┌─┐ │ ├─┤ │ └─┘` box with a divider under the title.
2. Pane box: 4 + N rows (top / title / divider / N content / bottom). Inner width = outer − 2; inner height = outer − 4.
3. `splitColumnWidths` branches by terminal width:
   - total ≥ 130: `left = 42`, `right = 57`, `mid = total − 99`. Canonical layout that all e2e tests assume.
   - 80 ≤ total < 130: proportional with `mid ≥ 25` floor (Diff steals from `right`).
   - total < 80: degenerate; tests do not pin.
4. Active pane: `▶ ` prefix on its title row. Exactly one.
5. Cursor row: `> ` prefix in Files / Commits / Diff / Comments. Visual-range rows also `> `.
6. Status bar: 2-row borderless block at bottom — content row (per-pane keymap + URL) + blank, always reserved (`statusBarRows = 2`). `bodyHeight = m.height - statusBarRows`. All layout / URL ladder / per-pane strings / truncation / color sourcing live in `internal/tui/statusbar.go::composeStatusBar` / `statusBar` / `(*Model).statusBarContent` — that file is authoritative. Suppressed when `m.width <= 0`, `m.height <= statusBarRows`, or during loading splash. URL from `api.Target.PRShortForms` (4-step ladder); per-pane context is mode-selected (normal / compose / help / modal / visual) and joins suffix `tab/shift+tab:pane J/K:file ctrl+e:comments ?:help q:quit` only in normal. Transient `AppState.Notice` replaces context, clears on next keystroke. Pane bottom borders sit directly above the bar's content row — they double as the visual separator.
7. Loading view: splash + blank gap + (optional version line + blank) + `<spinner> Loading PR (<stage>)…`. Centered both axes. Stages: `metadata → commits → files → comments → diffs → ready`. Pre-`tea.WindowSizeMsg` falls back to top-left text. Status bar suppressed during loading.
7a. Splash variants in `internal/tui/splash.go`: 3 layouts × 3 ASCII REVA arts. `chooseSplashLayout` / `chooseSplashArt` are called once in `NewModel` and held for the program's lifetime (no flicker). Random by default; pinnable via `GH_REVA_SPLASH_LAYOUT` (1/2/3) and `GH_REVA_SPLASH_ART` (0/1/2). Layouts: 1 = dome + `reva vX.Y.Z` + spinner; 2 = ASCII art + `vX.Y.Z` + spinner (no `reva` prefix — art names the tool); 3 = art beside dome (3-col gap, vertical center pad on the shorter block) + `vX.Y.Z` + spinner. ASCII art rows pre-padded by `init()`. Version from `cmd/root.go::SetVersion(version)`; empty version suppresses the line. Tests: `loading_test.go::TestLoadingView_Layout{Dome,Ascii,DomeAscii}`, `e2e/tests/09_errors.test.mjs::J1b/J1c` (latter pin layout 1).

### Diff pane
8. Split row layout: `<cursor 2><marker 2><oldLn 4><sp 1><leftCell halfW><sp 1>│<sp 1><newLn 4><sp 1><rightCell halfW>`. Overhead = 17. `halfW = (paneWidthDiff − 17) / 2`. Degrades to unified when `halfW < 8`.
9. Tab expansion: `expandTabs(line, 4)` before wrap/pad. Without it, terminal-side tab expansion shifts `│`.
10. `◆` gutter marker in cols 2–3 on the first display row of any buffer line that carries a review comment. Continuation rows leave it blank.
11. Split row distribution: header (`---`/`+++`/`@@`) and context render both sides; `-` left only; `+` right only.
12. Wrap always on. Buffer line ↔ display row is 1:N. `DiffCursor.Line` indexes raw patch buffer; cursor `>` and `◆` only on first display row. Continuation rows: unified indents 5 cols; split leaves cursor / marker / line-number columns blank, prefixes each cell with 1 blank, redraws `│`.
13. `fitPaneTitle` preserves the `[mode]` suffix at narrow widths; label shrinks with `…`.
14. Diff Enter:
    - Cursor row has NO existing comments → queues inline compose confirm via `(*Model).startComposeInline`. Anchor = `internal/diff.ResolveAnchor` (Path = SelectedFile, CommitSHA = `PR.HeadSHA`, Line + Side). Header / hunk rows rejected. In Diff visual mode, Enter consumes the visual range via `internal/diff.ResolveRange`; mixed-side ranges supported (#27d). Editor / textarea launch held until confirm (#27j).
    - Cursor row HAS comments (`threadsForCursor()` non-empty) → `(*Model).focusCommentsAtCursor` shifts focus to the Comments pane (`CommentsCursor = 0`, `Modal = nil`). When the Comments column is hidden via Ctrl+E (#30c), it auto-reveals first so focus never lands on an invisible pane. The user acts via the Comments-pane keymap (Enter = edit own / `r` = reply / Space = open zoom modal). Adding another thread on the same line is intentionally not exposed. The previous Comments-zoom-modal handoff was retired once Ctrl+E gave the column a stable visibility gesture; the modal had been an extra layer of UI without earning its keystroke.

    Body collection, `$EDITOR`, GraphQL, refresh: §4 #27. Locked by `compose_test.go::TestHandleKeyDiff_EnterOnCommentedRow*` and `e2e/tests/05_pane_diff.test.mjs::F7` / `e2e/tests/13_modal.test.mjs::F-modal-9`.
14b. `gg` is a true two-key sequence. First `g` records `AppState.PendingPrefix = "g"`; next `g` runs gotoTop. Any non-`g` key clears pending AND falls through to its normal dispatch (`g` then `k` moves up by one). Pending cleared by every keystroke that exits the pane key context (`tab`, `shift+tab`, `J`, `K`, `v`, `?`, `esc`, `y`, `/`). The slot is global — Files / Commits / Diff / Comments each call `(*Model).handlePendingG` (in `search.go`) at the top of their handler with a pane-local gotoTop closure, so `gg` works in every pane. `G` is the symmetric gotoBottom in every pane (Files: last file row; Files-tree: last tree row; Commits: last commit row index `len(commits)`; Diff: last buffer line; Comments: last flatComments row). Files gg/G move `FilesCursor` only — they no longer call `selectFile` (#19); Commits gg/G still auto-select via the existing per-pane logic. Locked by F4d / F4e / F4f in `e2e/tests/05_pane_diff.test.mjs`, P1 / P2 / P3 in `e2e/tests/20_search.test.mjs`, and `internal/tui/search_test.go::TestFiles_ggGotoTop` / `TestFiles_GGotoBottom` / `TestCommits_ggGotoTop` / `TestCommits_GGotoBottom` / `TestDiff_GGotoBottom` / `TestComments_ggGotoTop` / `TestPendingPrefix_ClearedOnTab`.

### Commits pane
15. `visibleCommits` auto-filtered by `SelectedFile`. Set on load (`PR.Files[0].Path`) so the filter is always engaged in live UX. The `SelectedFile == ""` branch is a safety net for pre-`PRLoadedMsg` and tests.
15a. Cursor index 0 is the synthetic "All commits" row representing `RangeWholePR`. Cursor space `[0, len(visibleCommits)]`: idx 0 → `RangeWholePR`, idx 1..N → `RangeSingleCommit{commits[idx-1].SHA}`. Label: `All commits (N)` identity, `All commits (M of N)` filtered. Annotation slot mirrors file's PR-level Status when filtered. Bold; `selectFile` resets `CommitsCursor = 0`. Visual yank skips this row. Pinned by `pane_commits_test.go::TestAllCommitsRowLabel`.
16. `j/k` in Commits auto-selects the cursor row (#15a). Visual mode gates this so multi-row yank does not mutate `SelectedRange`.
17. Enter on Commits is a no-op (cursor commit is already auto-selected).
18. `[A]/[M]/[D]/[R]` annotates each commit row that touches `SelectedFile`.

### Files pane
19. `j/k` in Files moves `FilesCursor` only — `SelectedFile` does not change on every keystroke. The per-keystroke Diff re-render felt sluggish during navigation; `Enter` (commit) or `Shift+J/K` (`advanceFile`, works from any pane) is the deliberate gesture that calls `selectFile(path)`. `selectFile` resets `DiffCursor`, `DiffViewport.Top`, `CommitsCursor`, `CommentsCursor` only when path changes. The incsearch (`/`) auto-select retained — search has always been a direct file-selection gesture; users typing a query expect the cursor to follow. `autoSelectFlat` / `autoSelectTree` survive solely as the search-side helper. Locked by `pane_toggle_test.go::TestFiles_jKDoesNotChangeSelectedFile` / `TestFiles_ggGCursorOnly` and e2e `D3b`.
20. Tree mode (`t` toggles): dirs render `v <name>/` (expanded) or `> <name>/` (folded); files show basename + status + comment count.
21. `autoSelectTree` skips `selectFile` on dir rows so a search-driven cursor jump onto a dir does not clobber Diff. Same helper used by the (search-only) auto-select path.
22. `remapCursorOnTreeToggle` preserves the conceptual cursor position when toggling flat ⇄ tree.
22b. Enter on a file row (flat or tree) is the commit gesture: it calls `selectFile(path)` and shifts `FocusedPane = PaneDiff`. Tree-mode dir rows still fold/unfold and keep focus on Files. Inside the Files zoom modal, `Enter` performs the same commit (selectFile + close modal + Diff focus); the modal-Enter path lives in `keys.go::handleKey` rather than `pane_files.go` because it needs to clear `Modal` first. Locked by `pane_toggle_test.go::TestFiles_EnterShiftsFocusToDiffAndSelects` / `TestFiles_EnterTreeDirStillFolds` and e2e `D6` / `F-modal-2`.

### Comments pane
23. Diff-cursor coupling: `commentsView` shows ONLY threads anchored at the Diff cursor's current buffer line (the `◆` rows). When cursor is not on a `◆` row, column reads `(no comment at cursor)` and `<space>` is a no-op (zooming a placeholder is noise; reserve the gesture for actual content). Visible-thread set computed by `threadsForCursor` (maps `DiffCursor.Line` through `patchNewLineNumbers`). `flatComments` is scoped to `threadsForCursor` so cursor index never drifts past visible content. Locked by `pane_toggle_test.go::TestComments_SpaceNoopWhenNoThread` / `TestComments_SpaceOpensModalWhenThreadVisible` and e2e `F-modal-5b`.
23b. Render shape: header + indented body. Header = `<name>: <yyyy-mm-dd hh:mm> <hash>[ [pending]| [outdated]]` (`CreatedAt.Local()`, `<hash>` = `shortSHA(CommitID)`). `[pending]` (yellow) and `[outdated]` (red) are mutually exclusive; pending wins. Body indent = `2 + 2*(depth+1)`. Body line-break mirrors GitHub web: every `\n` is a row break; 2+ consecutive `\n`s emit one extra blank row. Each source line wraps at `paneWidthComments − bodyLeader` via `wrapText`. Detail in `pane_comments.go::renderCommentBody`.
23c. Word-boundary rule: `wrapText` calls `splitWrapWords` (in `styles.go`), which splits on whitespace ONLY when both adjacent runes are ASCII word runes. CJK / emoji on either side keeps whitespace inside the running word — `hardBreak` then splits mid-CJK. Without this, ASCII fragments get stranded ahead of long CJK words on narrow columns.
24. Cursor movement (`j/k`) auto-scrolls Diff to the cursored comment via `syncDiffToCursorComment`. `h/l` and `backspace` are unbound.
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
    - Diff: line content (visual range = lines joined by `\n`)

### Compose (pending PR comment input)
The compose flow POSTs into the user's pending (draft) review. Submission to public is intentionally NOT exposed; users finalize via web UI or `gh api graphql`.

27a. `AppState.Compose *ComposeState` is a global overlay state, peer to `Visual` / `Modal` / `HelpOpen`. While non-nil, `handleKey` routes every keystroke to `handleKeyTextarea`; background panes frozen.
27b. Lifecycle (`ComposeStatus`):
   - `ComposeEditing` — body collection. `UseTextarea = false` (default with `$VISUAL` / `$EDITOR`): `tea.ExecProcess` opens editor on `gh-reva-compose-*.md` tempfile via `sh -c "$EDITOR <quoted-path>"` (so `EDITOR='code --wait'` works). Tempfile pre-populated for `ComposeEdit`. When `$TMUX` is non-empty, `buildEditorCmd` wraps the same shell command in `tmux display-popup -E -w 80% -h 80% <shellCmd>` so the editor floats over the gh-reva TUI instead of swapping the screen out — `-E` closes the popup whenever the editor exits (zero or non-zero), so vim `:q!` still returns control. Tests force-empty `TMUX` in `e2e/helpers/launch.mjs` so the popup branch is locked by `compose_editor_test.go::TestBuildEditorCmd_*` and never sneaks into PTY-based e2e. `UseTextarea = true` (no editor): `overlayCompose` modal collects rune-by-rune; Ctrl+S saves; Esc / Ctrl+C cancels.
   - `ComposeSubmitting` — `submitComposeCmd` in flight. Status bar: `posting to GitHub…`. Esc / Ctrl+C detaches.
   - `ComposeFailed` — POST errored. `Body` and `ErrMsg` preserved; Ctrl+S retries without re-typing; Esc cancels.
27c. Inline (`Kind = ComposeInline`) anchors via `ResolveAnchor`. `Path = state.SelectedFile`, `CommitSHA = state.PR.HeadSHA` (always — comments anchor to PR head, mirroring web). Header / hunk rows rejected.
27d. Multi-line range: enter Diff visual, move cursor, Enter. `ResolveRange` collapses anchor + cursor into `(start_line, start_side) → (line, side)` normalized by buffer position. Mixed-side ranges accepted as-is. Single-line ranges drop `start_*` fields.
27e. Reply (`Kind = ComposeReply`) captures the cursor thread's GraphQL node ID via `threadIdentityForCursor` → `addPullRequestReviewThreadReply`.
27e2. Edit (`Kind = ComposeEdit`) captures comment node ID via `buildComposeEdit` and pre-loads the body. Mutation `updatePullRequestReviewComment`; gated by `User == ViewerLogin`. Anchor stitched back from cached comment list (response carries only the comment row).
27f. Pending review session: `ghClient.ensurePendingReview` queries `reviews(states: [PENDING], first: 50)` filtered by `viewer.login` (NOT `viewerLatestReview` — that one can hide a PENDING draft when a non-PENDING review by the same viewer is more recent, which 422s the next `addPullRequestReview` on GitHub's "one pending review per user per PR" rule). Falls back to `addPullRequestReview` if no viewer-owned PENDING. Cached on `pendingReviewID[n]` for process lifetime. `viewerLogin` from the same query exposed via `Client.ViewerLogin(ctx)` for the edit gate.
27g. POST mutations routed by `submitComposeCmd`:
   - Inline → `addPullRequestReviewThread` (`pullRequestReviewId`, `path`, `line`, `side`; `startLine` / `startSide` for ranges; `subjectType: LINE`).
   - Reply → `addPullRequestReviewThreadReply` (`pullRequestReviewId` + `pullRequestReviewThreadId`).
   - Edit → `updatePullRequestReviewComment` (`pullRequestReviewCommentId` + `body`).

   On success, `convertGQLComment` shapes into `model.ReviewComment` (with `Pending` from review state). `applyComposeSubmitted` appends (Inline / Reply) or replaces in place by NodeID (Edit). Header tags Pending entries `[pending]` (`theme.CommentPending`).
27h. Status-bar contexts handled in `internal/tui/statusbar.go` (#6).
27i. Post-compose refresh: `applyComposeSubmitted` queues `refreshCommentsCmd`, which re-runs `Client.ListComments`. `mergeRefreshedComments(local, refreshed)` preserves any locally-known Pending whose NodeID is NOT in the refresh — `pullRequest.reviewThreads` has eventual-consistency lag; a naive REPLACE silently drops the just-posted draft. Edit POSTs flip body in place by NodeID. `CommentCount` recomputed. Refetch failure tolerated silently.
27j. Confirm gate: every entry point (Diff Enter, Diff visual range Enter, Comments Enter on own, Comments `r`) parks the built `ComposeState` in `AppState.PendingConfirm` instead of starting the editor immediately. While `PendingConfirm != nil`, the top-level guard in `keys.go::handleKey` routes every keystroke through `handleKeyConfirm` (sits ahead of Compose / Help / Visual absorbers): `y` → `confirmComposeStart` (PendingConfirm clears, payload moves into `Compose`, Visual cleared for inline ranges, body collection begins); `n` / `Esc` / `q` / `Ctrl+C` → `cancelComposeConfirm` (payload discarded; Visual stays so the user can refine). Status-bar contexts: `start new comment? [y]es [n]o`, `post reply? [y]es [n]o`, `edit comment? [y]es [n]o` (`hintConfirm{Inline,Reply,Edit}`). Foreign-author Comments Enter still short-circuits inside `buildComposeEdit` and surfaces `state.Notice` — no confirm queued. `buildComposeInline` no longer clears `Visual`; that mutation moved to `confirmComposeStart` so the highlight stays during the prompt. Locked by `compose_test.go::TestStartCompose*_QueuesPendingConfirm` / `TestHandleKey_PendingConfirm*` and `e2e/tests/18_compose.test.mjs::C1/C2/C3/C5/C5b/C6/C6b`.

### Search (global `/`)
27k. `AppState.Search *SearchState` is a global overlay state, peer to Compose / Visual / Modal / PendingConfirm. Two phases: `SearchEditing` (incsearch input collection) and `SearchActive` (post-Enter; n/N cycles). `internal/tui/search.go` owns the state machine; `keys.go` slots the Editing absorber between the Compose absorber and the Help absorber so query keystrokes don't leak into pane navigation. `Active` falls through to normal dispatch — n/N are intercepted before per-pane handlers; everything else (j/k, etc.) keeps working with the cursor parked on the current match.

27l. Lifecycle:
   - `/` (normal mode) → `startSearch` snapshots cursor state into `SearchState.Saved*` (FilesCursor, CommitsCursor, DiffCursor, DiffViewport.Top, CommentsCursor, SelectedFile, SelectedRange, FocusedPane), sets `Status = SearchEditing`, scopes `TargetPane = m.state.FocusedPane`.
   - `/` (Active) → re-enters Editing without dropping the saved snapshot so the user can refine the query (vim convention).
   - `/` (Comments pane) → silent no-op. Comments search is intentionally disabled until the modal-vs-flat UX is decided. The hint set in Comments omits `/:search` to match.
   - Each printable rune → `recomputeSearch` (smart-case literal substring) + `applySearchCursor` (jumps the live cursor to the nearest match ≥ saved position). Files / Commits search auto-selects via the same path j/k uses; Diff calls `scrollDiffIntoView`.
   - `Backspace` → drops one rune; on empty query, `cancelSearch`.
   - `Enter` → `commitSearch`. Empty / no-match → `cancelSearch` + `state.Notice = "no match: <query>"` so the user gets feedback without a stuck Active session.
   - `Esc` / `Ctrl+C` (Editing) → `cancelSearch` restores every Saved* field.
   - `Esc` / `Ctrl+C` / `Tab` / `Shift+Tab` (Active) → clears `state.Search` (no Saved* restore; the user already committed). Tab / Shift+Tab additionally advance the focused pane after clearing — moving focus implies the user has navigated to the row they were after, so n/N should stop intercepting and the highlight should drop.

27m. Per-pane match providers (in `search.go`): Files matches against `FileEntry.Path` (or `FilesRow.Path` in tree mode); Commits matches against `Commit.Message` / `SHA` / `ShortSHA` and emits cursor index `i+1` to skip the synthetic "All commits" row; Diff matches each `patchLines()` entry by buffer index. Comments has a provider in place (`collectCommentMatches`) but the Comments pane currently rejects `/` so the provider is unreachable from the UI. Smart-case via `smartCaseFold`: lowercase query → fold; any uppercase → exact.

27n. Match highlight: `theme.SearchMatchBg` (theme-uniform muted dark yellow, `#574b00`). Files / Commits / Files-tree dirs wrap each occurrence of the query inside the visible row text via `Model.searchHighlight` → `highlightMatches` (byte-indexed; CJK / mixed bodies stay correct because strings.ToLower preserves byte length on non-ASCII runes). Commits short-SHA is rendered without highlight to avoid lipgloss SGR-nesting collisions with the CommitSHA fg color; the cursor `>` glyph carries the visual signal for sha-only matches. Diff applies `bgRow(_, theme.SearchMatchBg)` to every buffer line whose index is in `searchMatchLines()` — visual-range bg still wins when both fire. Match-highlighted Diff rows skip `rowCache` because the match set drifts per keystroke. Locked by `internal/tui/search_test.go::TestHighlight_*` / `TestFilesView_HighlightsMatchedPath` and `e2e/tests/20_search.test.mjs::P11`.

27o. Status-bar contexts: `hintSearchEditing` is replaced by the live `/<query>_` prompt; Active shows `n:next  N:prev  /:edit  esc:clear  [idx/count] /<query>`. Suffix dropped in both. Per-pane normal hints carry `/:search` EXCEPT `hintComments` (search disabled there). Locked by `internal/tui/search_test.go::TestSearch*` and `e2e/tests/20_search.test.mjs::P1..P15`.

### Global keys
28. Tab / Shift-Tab cycle Files → Commits → Diff → Comments. Only keys that move focus across panes.
29. Enter is the commit / focus-handoff / compose-entry gesture; never quits. Backspace unbound everywhere. Bindings:
    - Files (tree, dir): fold/unfold.
    - Files (file row, flat or tree): commit `selectFile(path)` and shift `FocusedPane = PaneDiff` (#22b).
    - Files / Commits zoom modal: close modal AND shift FocusedPane to PaneDiff (CommentsCursor reset). Files modal also commits `selectFile` for the cursor file (peer to the per-pane Enter); tree-mode dir rows still fold/unfold.
    - Commits (normal pane): no-op.
    - Diff (uncommented row): inline compose at cursor (or visual range — #14, #27d). Header / hunk rows no-op.
    - Diff (row with comments): focus shift to Comments pane (#14) — auto-reveals the column if hidden.
    - Comments: edit own in place; foreign → Notice. Replies use `r`. See #24b.

    Visual mode preserves Diff-Enter compose; Enter elsewhere with visual active is inert. While `state.Compose != nil`, `handleKey` absorbs every keystroke.
30. Shift+J / Shift+K advance to next/prev file from any pane via `advanceFile(forward bool)`. Focus preserved.
30b. `gg` / `G` work in every pane (#14b). `/` opens search scoped to the focused pane (#27k–n). `n` / `N` cycle while Search is Active.
30c. `Ctrl+E` toggles `AppState.CommentsHidden`. Hidden state collapses the right column to width 0 and adds the saved width to the middle (Diff) column via `splitColumnWidths(total, hidden)` — Files / Commits keep their familiar widths so the layout transition is local to the right side. Layout fallback (m.width<=0 or bodyHeight<8) drops Comments from the stacked join. Hiding while `FocusedPane == PaneComments` shifts focus to Diff so keystrokes don't strand on an invisible target; revealing keeps focus where it is. `Tab` / `Shift+Tab` skip Comments while hidden so the cycle stays Files → Commits → Diff → Files. `focusCommentsAtCursor` (Diff Enter handoff, #14) auto-reveals before shifting focus so the user always lands on a visible pane. Locked by `internal/tui/pane_toggle_test.go` and `e2e/tests/21_pane_toggle.test.mjs::T1..T5`.

### Color theming
31. `internal/theme.Theme` is the single source of truth — 28 `lipgloss.Color` fields plus `SyntaxStyle *chroma.Style`. `Resolve(name)` accepts `"builtin-dark"`, any chroma registry name, or `""` (→ `defaultThemeName`). Unknown names error.
32. Chroma adapter (chroma token → UI role) in `internal/theme/chroma.go`. Two overrides: `DiffPlusBg` / `DiffMinusBg` are hard-coded (`#0d3b13` / `#3b0d0d`); `GenericInserted` / `GenericDeleted` go through `pickAccent`, which prefers `StyleEntry.Background` when `StyleEntry.Colour` equals editor background (gruvbox-style inversion). Without these the +/- distinction collapses on inversion themes.
33. `m.theme` is non-nil after `NewModel` (constructor seeds `defaultThemeName`). `cmd/root.go` overrides via `Model.SetTheme`.
34. Color application via `internal/tui/colors.go` (`fg`, `fgBold`, `bgRow`); no-op on zero-value colors. Apply AFTER `padTrunc` / cell assembly so width math stays driven by visible cells.
35. `padTrunc` is SGR-aware (`lipgloss.Width` to measure, `ansi.Truncate` over-width). Right-pads with plain spaces.
36. Pane border / title coloring in `app.go::renderPaneBox`. Active uses `PaneBorderActive` + `PaneTitleActive` (Bold); inactive uses `PaneBorderInactive` + `PaneTitle`.
37. Visual-mode rows get row-wide bg via `bgRow(row, theme.VisualRangeBg)` after padding to `paneWidthDiff`. Bg ends inside the pane; borders stay border-colored.
38. Diff cells: bg-for-change + per-token syntax fg (`syntax.go::styledDiffCell`). `+` rows: `DiffPlusBg` row-wide + chroma fg per token; `-` similar with `DiffMinusBg`. Context rows pass `bg=""`. File / hunk headers stay flat-fg. Leading marker excluded from lexer and re-emitted bold under same bg with theme-uniform `theme.DiffPlus` / `theme.DiffMinus` (`#3fb950` / `#f85149`); marker fgs are NOT in the `syntaxCache` key.
39. `Model` has 3 caches that must propagate across Bubbletea's value-copied Updates:
   - `syntaxCache` — `*syncMap` keyed on `lexer.Name + style.Name + bg + cell`. Pointer identity shared.
   - `rowCache` — `*diffRowCache` (`map[string][]string`) keyed on `(mode, lineIdx, halfW, commented)`. Width / patch identity changes invalidate via `m.invalidateRowCacheIfStale()`. Skips cursor + visual-range rows.
   - `patchLinesC` — struct value (`patchLinesCache`); `cache` field is `map[string]*patchInfo` keyed on `diffKey(sha, path)`. Maps are reference types, so the struct value embedding works — replacing it with a slice / scalar breaks propagation.

   Hot-path rule: never call `strings.Split(patch, "\n")` or `parseDiffSpecs(patch)` directly. Go through `m.patchLines() / m.patchSpecs() / m.patchNewLineNumbers()`. New caches sharing fate with the patch should also key on `diffKey(sha, path)` and use the `invalidateRowCacheIfStale` pattern.
40. `waitReady` defaults to 10s in `e2e/helpers/launch.mjs` to absorb chroma init + first-frame tokenization.
41. `session.press` / `session.type` are wrapped with a 120ms settle in `launchReva`. Don't reach for `session.press` in helpers — use the wrapped session.
42. Pane modal (`<space>` zoom): gated by `model.ModalState{Pane, Origin}`, toggled by `<space>` in Files / Commits / Comments. Diff `<space>` is unchanged (split⇄unified). Modal closes on `tab`, `shift+tab`, `?`, `esc`, `q`, `Ctrl+C`; `q` and `Ctrl+C` quit only when modal is closed (symmetric dismiss). `J` / `K` leave the modal open by design. Visual mode allowed inside; Comments-pane Enter / `r` keep working in the Comments modal. Title row uses single leading space (`│ Files`) — distinct from regular pane titles (`▶ ` / `  `); that's the e2e detection signature (`13_modal.test.mjs::MODAL_TITLE_RE`). j / k inside the modal go through the regular pane handlers so cursor / state propagates. There is no "hover popup" overlay — modal is the sole zoom affordance. Layout in `modal.go::modalLayout` / `modalContent` / `overlayModal`.
42b. Modal focus restore: `ModalState.Origin` records the opener pane. `toggleModal` (the only Modal-opener path now) reads `m.state.FocusedPane` at open time. Close gestures (`<space>`, `q`, `Esc`, `Ctrl+C`) route through `(*Model).closeModal`, which restores `FocusedPane = Origin` before clearing `Modal`. `Tab` / `Shift+Tab` also call `closeModal` first so next/prev reads the user's pre-modal pane. `?` (Help) likewise calls `closeModal` so post-Help close lands on the opener. Files-modal-Enter / Commits-modal-Enter intentionally bypass `closeModal` and explicitly set `FocusedPane = PaneDiff` (the user picked a row to inspect). Locked by `compose_test.go::TestSpaceClose_*` / `TestQClose_RestoresFocusToOrigin` / `TestEscClose_*` and `e2e/tests/13_modal.test.mjs::F-modal-10`.

---

## 5. E2E test conventions

### Helpers (`e2e/helpers/launch.mjs`)
- `launchReva({ args, fixture, cols, rows, env })` — spawn gh-reva with default fixture.
- `waitReady(session, { timeout = 5000 })` — wait for `Files` text after PR load.
- `quit(session)` — send `q`, then close.
- `activePaneLabel(session)` — return the single active pane name; throw if 0 or > 1.
- `paneText(screen, label)` — extract the pane's column slice. Required when asserting cursor markers (`^>`) in non-leftmost panes — borders place `│` at col 0, and cross-column content satisfies the wrong row otherwise. Trailing `│` stripped.
- `countSelectedRows(screen, label)` — count `> ` rows in the pane's slice.

### Patterns

`describe + before + screen capture` — for read-only observation tests. Capture screen once, run many `test()` blocks against it.

```js
describe('group', () => {
  let screen
  before(async () => {
    const s = await launchReva()
    await waitReady(s)
    screen = await s.text()
    await quit(s)
  })
  test('A', () => { /* assert against screen */ })
})
```

`describe + before/after + shared session` — for navigation tests that begin and end at Files focus without mutating cursors / SelectedFile.

Independent `test()` blocks — for tests that mutate state (visual, file selection, single-commit drill).

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
- Long substring assertions: column wrap splits words across rows. Shorten or normalize.
- bubbletea startup ~1 s blank: first `s.text()` after launch can be empty. Use `waitReady`.
- tuistory cannot reliably emit CSI Z: shift-tab tests are skipped (C2). Verify by inspection.
- Do not reintroduce `lipgloss.Border()`: boxes are rendered manually via `renderPaneBox`. Touch only that for box visual changes.
- Tabs in Diff: split mode requires `expandTabs(line, 4)` before wrap/pad.
- CJK / wide chars in Comments: `wrapText` measures with `runewidth.StringWidth` / `runewidth.RuneWidth`. Do not reintroduce `utf8.RuneCountInString` — rune count and display width diverge for non-ASCII fixtures.
- Diff wrap is always on (no toggle). `DiffViewport.Top` is buffer-line index; `diffViewportHeight()` is in display rows; `displayRowsBetween` bridges them.
- Color SGR doesn't reach tuistory's `text()`: ghostty parses ANSI into cell state. The A9 smoke test guards against raw `\x1b` leaking into rendered text.
- Chroma case quirk: registry key `rpgle` resolves to a Style whose `Name` is `RPGLE`. `theme.Resolve` canonicalizes on the registry key.
- Bubbletea v1 has no color profile option: `lipgloss.SetColorProfile(termenv.Ascii)` and `SetHasDarkBackground(true)` must be called BEFORE `tea.NewProgram`. `cmd/root.go` does this; new entry points must replicate.
- Chroma init is eager (~500ms cold). Don't import `chroma/v2/styles` or `chroma/v2/lexers` outside `internal/theme` and `internal/tui/syntax.go` — `theme` is the single gateway.
- Diff syntax highlighting needs `Model.syntaxCache`. Don't drop the pointer when restructuring `Model` — without it, e2e fails on `waitReady`.
- Cache pointer identity (#39): `syntaxCache`, `rowCache`, `patchLinesC.cache` rely on Bubbletea Model copies sharing the same underlying map / sync.Map. Don't deep-copy them in `NewModel`; don't change `patchLinesC.cache` to a non-reference type.
- `s.press` / `s.type` are auto-settled (120ms) in tests via `launchReva`. Don't add manual `await sleep(N)`; use `await s.waitForText(<expected>)` if a test still races.
- `launchReva` forces `TERM=tmux-256color` via `sh -c`: bubbletea v1's `tea_init.go` calls `lipgloss.HasDarkBackground()` at package import, which makes termenv send OSC 11 + DSR queries and block up to 5s waiting for a real terminal. termenv short-circuits when `TERM` starts with `screen` / `tmux`. Tuistory's `session.js` hard-codes `TERM: 'xterm-truecolor'`, so the value cannot pass via `env:` — the wrapper re-applies `TERM` before `exec`. Removing it restores 5s/launch idle wait.

---

## 7. Output conventions

- Chat replies: Japanese, neutral professional. No slang, no emojis, no self-deprecating hedges.
- Code identifiers, log/error messages, comments, PR templates: English.
- CLAUDE.md, prompts, agent instructions, skill definitions: English.
- Cite file locations with `path:line` (e.g. `internal/tui/pane_diff.go:144`).
- Cite evidence URLs at the end of any research-based reply.

---

## 8. Commit conventions

- Commit only when explicitly requested.
- Never push to main / master; never force-push. Tag pushes allowed when explicitly requested as part of a release (§9).
- Subject ≤ 70 chars; body explains the why if non-obvious.
- Trailer: `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.
- Stage by name when feasible. `git add -A` allowed for the initial commit and when `.gitignore` is known correct, not for arbitrary staging.

---

## 9. Release procedure

Releases are driven by the `v*` tag pushed to `origin`. `release.yml` runs goreleaser, which reads the version from `{{.Version}}` (= the tag) and produces `gh-reva_<os>-<arch>` binaries (the hyphen is required — gh CLI matches assets by `strings.HasSuffix(name, "<os>-<arch>")`, so `_` in that slot breaks `gh extension install`; documented in `.goreleaser.yaml:20-25`). NO `version.go` to bump and NO changelog — the tag is the single source of truth.

### Steps for a patch / minor / major release

Run from repo root. Replace `vX.Y.Z`.

1. Pre-flight (must all pass before tagging):
   ```sh
   git status
   go vet ./...
   go test ./...
   (cd e2e && pnpm test)
   git log --oneline $(git describe --tags --abbrev=0)..HEAD
   ```
2. Pick next version from `git tag --sort=-v:refname | head -1` and apply SemVer (patch / minor / major).
3. Commit pending work with Conventional Commits style.
4. Bump `e2e/package.json` version to match (no `v` prefix). Convention: `chore(release): bump e2e workspace to vX.Y.Z` as a separate commit. Lockstep marker only.
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

User saying "release してほしい" / "release まで進めて" / "patch +1 で release" counts as explicit authorization for the full §9 sequence; partial requests like "commit して" do not authorize tagging or pushing.
