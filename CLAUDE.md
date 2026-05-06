# CLAUDE.md — gh-reva development conventions

`gh` extension that opens a vim-like 4-pane TUI for reviewing GitHub PRs.
Built on `bubbletea` + `lipgloss`. Single-purpose CLI; no shared infrastructure.

This file is authoritative for development. Read it once at the start of any
session that touches this repo, and update it when an invariant changes.

---

## 1. Build / test commands

```sh
# Repo root: /Users/dew/workspace/gh-reva

# Go
go build -o gh-reva .            # produce ./gh-reva binary at repo root
go vet ./...
go test ./...                  # internal/api (ghclient errors), internal/theme (registry round-trip), internal/tui (pane / loading / padtrunc / syntax)

# Manual TUI
./gh-reva --fixture testdata/sample-pr.json
./gh-reva --fixture testdata/large-pr.json
./gh-reva --fixture testdata/sample-pr.json --slow-load 500ms

# E2E (cd e2e first)
pnpm install
pnpm test                      # full suite; pretest hook auto-rebuilds gh-reva
pnpm run test:smoke
node --test --test-force-exit --test-timeout=20000 \
     --test-name-pattern='F2|F11' tests/05_pane_diff.test.mjs   # targeted

# Large fixture regeneration
go run testdata/gen_large_fixture.go testdata/large-pr.json
```

`go build -o gh-reva .` (NOT `go build ./...`) is required — the latter does
not produce a usable binary at repo root. Targeted `node --test` skips the
pretest hook, so rebuild manually.

### Hidden flags (E2E only)
- `--fixture <path>` — load PR data from JSON instead of GitHub
- `--simulate-error <kind>` — `unauth` | `not_found` | `rate_limit` (any other kind falls back to `errors.New("simulated error: <kind>")`)
- `--diff-height N` — pin Diff viewport height for deterministic scroll tests
- `--slow-load <duration>` — inject per-API sleep in fixtureClient (spinner observation)

### User-facing flags
- `--theme <name>` — color theme; default `gruvbox`. Accepts any chroma styles registry name (74) plus `builtin-dark`. `GH_REVA_THEME` env var works as fallback. `theme.Resolve("")` is wired to the `defaultThemeName` constant in `internal/theme/theme.go` — change the constant if you want a different empty-name default.
- `--no-color` — disable color output. Also honors `NO_COLOR` / `CLICOLOR` (`termenv.EnvNoColor`).
- `--list-themes` — print every accepted name on stdout and exit 0; no API access.

---

## 2. Workflow discipline

### TDD is mandatory
1. Write the failing test(s) first.
2. Run targeted test, confirm failure (with the actual assertion error matching the missing behavior — not a timeout / build break).
3. Implement.
4. Run targeted test, confirm pass.
5. Run full e2e (`pnpm test`), confirm no regressions.
6. If unrelated tests fail under the new behavior, update them in the same change. Never leave the suite red.

Skipping the failing-test-first step is forbidden — even when the
implementation seems trivial, it surfaces incorrect assertions and missing
edge cases (we caught several this way: D3c visual-mode gating, F8b enter
fallback, F2d tab alignment).

### Decision-first vs. action-first
For requirements with non-trivial design space (which key to bind, which
fallback semantics, which visual marker), present 2–3 concrete options
with tradeoffs and ask the user to pick **before** writing tests.
Examples in repo history:
- B-vs-A-vs-C contract pick for Commits Enter behavior (B = j/k auto-select, Enter = focus only)
- A-vs-B-vs-C marker glyph for Diff comments (B = gutter column)

For straightforward asks ("add a border around panes"), proceed directly with TDD.

### Risky operations require confirmation
Confirm before:
- `git push`, `git reset --hard`, force push (any push to main / master is forbidden without explicit user direction)
- Deleting fixture files or test snapshots
- Adding new top-level Go dependencies
- Renaming branches

The user is the only one who runs `git commit` unless they explicitly ask
the assistant to commit.

---

## 3. Architecture

```
gh-reva/
├── cmd/root.go                     # CLI entry, flags (incl hidden)
├── main.go
├── internal/
│   ├── api/                        # GitHub client (go-gh) + fixture mode
│   │   ├── client.go               # Client interface (read + pending POST + submit)
│   │   ├── pr.go                   # ghClient: GetPR / ListCommits / ListFiles
│   │   ├── diff.go                 # ghClient: GetFileDiff (PR-wide and per-commit)
│   │   ├── paginate.go             # ghClient: Link-header pagination helper (REST only)
│   │   ├── resolve.go              # ghClient: ResolveCurrentBranchPR + ParseTargetArg
│   │   ├── graphql_comments.go     # ghClient: GraphQL ListComments + reviewThread mapping
│   │   ├── graphql_post.go         # ghClient: ensurePendingReview + thread / reply / submit mutations
│   │   ├── fixture.go              # fixtureClient (loads testdata/*.json) + WithSlowLoad + in-memory POST/submit
│   │   ├── error_client.go         # error injection (--simulate-error)
│   │   └── ghclient_errors_test.go # httptest 401 / 404 / 429 / pagination
│   ├── clipboard/
│   ├── diff/                       # patch parsing (sourcegraph/go-diff) + side resolver
│   │   ├── parse.go
│   │   ├── render_split.go
│   │   ├── render_unified.go
│   │   └── side.go                 # ResolveAnchor / ResolveRange (Compose anchor lookup)
│   ├── model/                      # AppState + value types
│   ├── theme/                      # color palette resolution
│   │   ├── theme.go                # Theme struct, Resolve, ListThemes
│   │   ├── builtin.go              # builtin-dark fallback palette
│   │   ├── chroma.go               # chroma styles → Theme adapter
│   │   └── theme_test.go           # registry round-trip + chroma name fixup
│   └── tui/
│       ├── app.go                  # Model, View(), layout, loadPRCmd, renderPaneBox
│       ├── keys.go                 # global key dispatch (Tab, q, v, J, K)
│       ├── messages.go             # tea.Msg types
│       ├── styles.go               # paneTitle / fitPaneTitle / wrapText / indent / styled* helpers
│       ├── colors.go               # fg / fgBold / bgRow lipgloss wrappers
│       ├── syntax.go               # styledDiffCell + chroma lexer detect + token cache
│       ├── pane_files.go           # filesView + j/k auto-select + advanceFile
│       ├── pane_commits.go         # commitsView + j/k auto-select
│       ├── pane_diff.go            # diffView + split rendering + ◆ gutter + tabs + diffLineKind / colorDiffCell
│       ├── pane_comments.go        # commentsView + word wrap + diff auto-scroll
│       ├── files_tree.go           # tree mode rendering
│       ├── visual.go               # visual mode + yank
│       ├── modal.go                # `<space>` zoom modal (Files/Commits/Comments)
│       ├── compose.go              # Pending-comment compose orchestration (state machine, $EDITOR cmd, POST cmd)
│       ├── compose_test.go         # compose state-machine assertions
│       ├── textarea.go             # in-app textarea fallback + compose modal rendering
│       ├── submit.go               # submit-review modal (R key, approve/comment/request_changes)
│       ├── submit_test.go          # submit modal state-machine assertions
│       └── diffmap.go              # newLineNumbers / commentThreadIndexForDiffLine
├── testdata/
│   ├── sample-pr.json              # default fixture (5 files, 3 commits, 4 comments)
│   ├── large-pr.json               # 60 commits / 120 files / 122 KB (J3)
│   ├── wrap-pr.json                # single long-bodied comment (G11)
│   └── gen_large_fixture.go        # //go:build ignore generator
└── e2e/
    ├── helpers/launch.mjs          # launchReva / paneText / countSelectedRows
    └── tests/                      # node:test + tuistory
```

### Receiver conventions
- Mutating helpers: pointer receiver `(m *Model)`. Examples: `selectFile`, `autoSelectFlat`, `autoSelectTree`, `autoSelectCommit`, `advanceFile`, `scrollDiffIntoView`, `scrollDiffToLine`, `syncDiffToCursorComment`.
- Pure queries / renderers: value receiver `(m Model)`. Examples: `filesView`, `diffView`, `visibleCommits`, `commentLineSet`, `splitLayout`, `effectiveDiffViewMode`.
- `handleKey*` are value receivers and return updated `Model`. They may invoke pointer-receiver helpers via Go auto-addressing.
- `m.state` is `*model.AppState`; mutations through `m.state.X = Y` propagate regardless of receiver kind.

### Single source of truth
- `model.AppState` owns all mutable state. No globals beyond constants.
- `m.state.SelectedFile` drives the entire app: `visibleCommits` filters by it; `commentsForView` filters by it; Diff cache keys on `(SelectedRange.SHA, SelectedFile)`.
- Per-pane render budgets (`paneWidthFiles`, `paneHeightDiff`, ...) are set by `View()` from layout math, then read by pane renderers.

---

## 4. TUI invariants

These are load-bearing — breaking any of them breaks at least one e2e test
and several break the user's mental model.

### Layout
1. **3-column bordered layout**: Files+Commits in left column stacked vertically; Diff fills middle; Comments fills right. Each pane is its own `┌─┐ │ ├─┤ │ └─┘` box with a horizontal divider under the title.
2. **Pane box structure**: 4 + N rows — top border `┌─┐` / title `│…│` / divider `├─┤` / N content rows / bottom border `└─┘`. Inner width = outer − 2; inner height = outer − 4.
3. **`splitColumnWidths`** has three branches keyed on terminal width:
   - **total ≥ 130**: `left = 42`, `right = 57`, `mid = total − 99`. Inner targets Files/Commits = 40, Comments = 55, Diff = remainder. This is the canonical layout that all e2e tests assume.
   - **80 ≤ total < 130**: proportional — `left = clamp(total/4, 22, 38)`, `right = max(total*2/5, 28)`, `mid = total − left − right` with a `mid ≥ 25` floor (Diff steals from `right` when needed; `right` is itself clamped at 22). Used so the layout degrades gracefully on a narrower-than-default terminal.
   - **total < 80**: degenerate fallback — `left = total/4`, `mid = total/2`, `right = remainder`. No floor; rendering is best-effort. Tests do not pin this branch.
4. **Active pane**: `▶ ` prefix on its title row. Exactly one pane has it.
5. **Cursor row**: `> ` prefix in Files / Commits / Diff / Comments. Visual-range rows also carry `> `.
6. Status bar (bottom row): 1 row at the bottom of the screen is always reserved once the PR is loaded — `bodyHeight = m.height - 1` whenever `m.height > 1`, and the body layout (column widths, pane box heights) is computed from `bodyHeight`. The bar is rendered by `internal/tui/statusbar.go::statusBar` and emitted after `body + "\n"` at the end of `View()`. Layout: `<leading space><left><middle padding><url><trailing space>` where `<left>` is the per-mode keymap (per-pane context joined to the common suffix with two spaces in normal mode), and `<url>` is the PR URL right-flushed via a longest-fitting ladder from `api.Target.PRShortForms`:
   1. `https://<host>/<owner>/<repo>/pull/<n>` — full URL (uses `Target.Host`; defaults to `github.com`).
   2. `<owner>/<repo>/pulls/<n>` — host-stripped, REST-endpoint shape.
   3. `<owner>/<repo>/<n>` — pulls segment dropped.
   4. `<repo>/<n>` — owner dropped.

   Per-pane / per-mode context strings (selected by `(*Model).statusBarContent`):
   - Files (flat): `j/k:move space:zoom t:tree`
   - Files (tree): `j/k:move enter:fold space:zoom t:tree`
   - Commits: `j/k:move space:zoom`
   - Diff (split or unified): `j/k:move H/M/L:viewport gg/G:top/bottom space:split enter:comment` (the `ctrl+f`/`ctrl+b` page-move shortcuts are still bound, just dropped from the hint to keep room for `enter:comment`; the previous `space:split/unified` was shortened to `space:split` for the same reason)
   - Comments: `j/k:move space:zoom enter:reply`
   - In compose (`Compose != nil`): Editing-textarea → `ctrl+s:save  esc:cancel`; Editing-external → `editing in $EDITOR — finish there to continue` (rarely visible — bubbletea is suspended during `tea.ExecProcess`); Submitting → `posting to GitHub…`; Failed → `ctrl+s:retry  esc:cancel`. All four replace context AND drop suffix.
   - In submit review (`SubmitReview != nil`): Choosing → `a:approve  c:comment  r:request-changes  esc:cancel`; Submitting → `submitting review…`; Failed → `ctrl+s:retry  esc:cancel`. All replace context AND drop suffix.
   - In help modal (`HelpOpen`): `?/esc/q:close` (replaces context AND drops suffix)
   - In zoom modal (`Modal != nil`): `space/esc/q/ctrl+c:close` (replaces context AND drops suffix; J/K is intentionally omitted to keep the bar short — they still work for file scrubbing per §4 #42; Ctrl+C closes the modal symmetrically with q, see §4 #42 for the rationale)
   - In visual mode (`Visual != nil`): `-- VISUAL --  y:yank esc/ctrl+c:cancel` (replaces context AND drops suffix; absorbs the previous standalone `-- VISUAL --` banner — do not also re-emit the banner above the status bar)

   Common suffix (normal mode only, joined to the per-pane context by two spaces): `tab:focus J/K:file R:submit ?:help q:quit` (the `R:submit` slot opens the submit-review modal — see §4 Submit).

   Truncation priority (highest to lowest): context > URL (shrink through ladder) > common suffix. Pass 1 of `composeStatusBar` keeps the suffix and picks the longest URL form that fits alongside `context + "  " + suffix`; pass 2 drops the suffix and re-tries with `context` alone. If even the shortest URL (`<repo>/<n>`) does not fit alongside the context, the URL is dropped entirely and the context is left-padded across the bar; if the context still overflows, it is truncated with a trailing `…` via `ansi.Truncate`. The suffix is never half-truncated mid-token — it is either fully present or fully absent. Color: `theme.DiffLineNumber` is applied to the whole bar as a single fg span; the bar carries no background. The bar is suppressed when `m.width <= 0` or `m.height <= 1` (degenerate / pre-WindowSize) and during the loading splash (invariant #7 owns the screen).
7. **Loading view**: pre-PR shows the splash logo (10 rows of `▓`/`░`/`█` glyphs sourced from `logo.md`, embedded as `logoArt` in `internal/tui/app.go`) + a single blank gap + `<spinner> Loading PR (<stage>)...` (no boxes). The whole block is centered horizontally per-row (each row's lead is `(m.width - lipgloss.Width(row)) / 2`) and vertically as a unit (`topPad = (m.height - len(rows)) / 2`). Stages: `metadata → commits → files → comments → diffs → ready`. Before `tea.WindowSizeMsg` arrives (`m.width <= 0`), the spinner line falls back to top-left and the logo is suppressed so the very first frame still emits text. Logo glyph coloring uses `theme.LogoShade1` (█, brightest) / `LogoShade2` (▓, mid) / `LogoShade3` (░, dimmest); `renderLogo` coalesces same-shade runs into one SGR span to bound escape overhead. The status bar (#6) is suppressed during loading — the splash owns the entire screen; `View()` returns from the loading branch before the status bar is appended.

### Diff pane
8. Split mode layout (first row of a buffer line): `<cursor 2><marker 2><oldLn 4><sp 1><leftCell halfW><sp 1>│<sp 1><newLn 4><sp 1><rightCell halfW>`. Fixed overhead = 17. `halfW = (paneWidthDiff − 17) / 2`. Degrades to unified when `halfW < 8` (structural fallback only).
9. Tab expansion: `expandTabs(line, 4)` is applied before wrap/pad. Without it, terminal-side tab expansion shifts `│`.
10. `◆` gutter marker appears in the marker slot (cols 2–3) on the **first display row** of a buffer line that carries an anchored review comment. Continuation rows (from wrap) leave the slot blank.
11. Split row distribution: header (`---`/`+++`/`@@`) and context lines render on both sides; `-` only on left; `+` only on right.
12. Wrap is always on. Buffer line ↔ display row is 1:N. `DiffCursor.Line` indexes the raw patch buffer; cursor `>` and `◆` markers appear only on the first display row of each buffer line. Continuation rows in unified are indented 5 cols (cursor 2 + marker 2 + diff-marker 1) so wrapped content aligns past the `+`/`-`/space marker. In split, continuation rows leave cursor / marker / oldLn / newLn columns blank, prefix each cell with 1 blank to align past the diff marker, and re-draw `│` at the same column.
13. `fitPaneTitle` preserves the `[mode]` suffix at narrow widths. Label shrinks with `…`.
14. Diff Enter opens the pending-comment compose flow (`(*Model).startComposeInline` in `internal/tui/compose.go`). On a `+`/`-`/context buffer line, Enter creates a `model.ComposeState{Kind: ComposeInline}` from `internal/diff.ResolveAnchor` (Path = SelectedFile, CommitSHA = `PR.HeadSHA`, Line + Side from the patch row). On a header (`---`/`+++`) or hunk (`@@`) row, Enter is a no-op (`buildComposeInline` returns false). In Diff visual mode, Enter consumes the visual range — `internal/diff.ResolveRange` produces a `(start_line, start_side) → (line, side)` tuple and the visual state is cleared; ranges are normalized by buffer-position so the buffer-earlier endpoint always becomes start (mixed-side ranges supported, see §4 #27d). Body collection picks `$VISUAL` then `$EDITOR` (POSIX convention) and runs via `tea.ExecProcess` against a `gh-reva-compose-*.md` tempfile invoked through `sh -c "$EDITOR <quoted-path>"` (matches `git commit` / `crontab -e` convention so editors with shell-style argv like `EDITOR='code --wait'` work). If neither var is set, `ComposeState.UseTextarea` is set and the in-app textarea (`textarea.go::handleKeyTextarea`) collects the body. Empty body (after `strings.TrimSpace`) and editor errors cancel without a POST. Non-empty body transitions `ComposeStatus` to `ComposeSubmitting` and fires `submitComposeCmd`, which POSTs via GraphQL `addPullRequestReviewThread` (or `addPullRequestReviewThreadReply` for replies) into the user's pending review on this PR — the pending review is created on demand by `ghClient.ensurePendingReview` (`reviews(states: [PENDING], first: 50)` filtered by `viewer.login` → `addPullRequestReview` if no viewer-owned PENDING exists; cached per PR). On 200, the returned `*ReviewComment` (Pending=true because `pullRequestReview.state == PENDING`) is optimistically appended to `state.PR.Comments` and `FileEntry.CommentCount` is bumped; `applyComposeSubmitted` then queues `refreshCommentsCmd` so the canonical comment list is re-fetched (mirrors the SubmitPendingReview flow, keeps state idempotent regardless of partial mutation responses). On error, status flips to `ComposeFailed` and Body / ErrMsg are preserved so Ctrl+S retries without re-typing. The Comments pane header tags Pending entries with `[pending]` (colored via `theme.CommentPending`). The whole flow is locked in by `internal/tui/compose_test.go` and `e2e/tests/18_compose.test.mjs`.
14b. **`gg` is a true two-key sequence**, not a single-`g` shortcut. The first `g` records `AppState.DiffPendingPrefix = "g"` and returns without view change; the next `g` clears pending and runs gotoTop; any non-`g` key clears pending AND falls through to its normal dispatch (so `g` then `k` moves the cursor up by one — it does NOT jump to top). The pending slot is global state on `AppState` (forward-compatible with future `gd` / `gh` / `gb` Diff-pane mappings sharing the same dispatch) and is explicitly cleared by every keystroke that takes the user out of the Diff key context: `tab`, `shift+tab`, `J`, `K`, `v`, `?` in `internal/tui/keys.go`, plus `esc` / `y` (visual exit) in `internal/tui/visual.go`. The `case "g":` branch in `handleKeyDiff` was removed; the prefix dispatch lives at the top of the handler before the main switch. Earlier the implementation accepted a single `g` for gotoTop with a self-acknowledged "Phase 1 cuts a corner" comment; the new state machine makes vim-correct semantics the contract and keeps the door open for further `g`-prefix maps without per-mapping ad-hoc state. Locked down by e2e F4d (single g no-op) / F4e (g + non-g cancel) / F4f (focus-change clears pending) in `e2e/tests/05_pane_diff.test.mjs`.

### Commits pane
15. **`visibleCommits` is auto-filtered by `SelectedFile`**. No manual override; the previous `space` toggle and `CommitFilterFile` field were removed. `SelectedFile` is set on load (`app.go::Update PRLoadedMsg` assigns `PR.Files[0].Path` whenever the PR has any files), so in live UX the filter is always engaged from the first frame; the `SelectedFile == ""` branch in `visibleCommits` (returns all commits) is kept only as a safety net for the pre-PRLoadedMsg frame and tests that simulate it. The Commits cursor starts at idx 0 (the synthetic "All commits" row → `RangeWholePR`), so the initial Diff still shows the whole-PR diff of `PR.Files[0]` rather than a single commit's slice.
15a. **Cursor index 0 is the synthetic "All commits" row** representing `RangeWholePR`. It is rendered above the real commits by `commitsView` via `allCommitsRow`, and is the only path back to the whole-PR diff from inside the Commits pane (k past the top lands on it). The cursor space is therefore `[0, len(visibleCommits)]` — `handleKeyCommits` caps `j` at `< len(commits)` (one past the previous bound) and `autoSelectCommit` switches `SelectedRange` to `RangeWholePR` when `idx == 0` and to `RangeSingleCommit{commits[idx-1].SHA}` otherwise. Label form: `All commits (N)` when no file filter is active OR when the filter resolves to M == N (every commit touches the selected file); `All commits (M of N)` only when M < N. The annotation slot mirrors the file's PR-level `Status` (`[A]/[M]/[D]/[R]`) when filtered, blank otherwise. The label is rendered bold via `fgBold(label, "")` to set the row apart from real commits without an extra divider. `selectFile` resets `CommitsCursor = 0` so any file change (including `Shift+J/K`) returns to the All commits row. Visual yank skips this row — `yankString` for Commits iterates the cursor space `len(commits) + 1` and `continue`s on `i == 0`, so the clipboard never includes the `All commits` label. Label rule + behavior contract is locked in by `internal/tui/pane_commits_test.go::TestAllCommitsRowLabel`.
16. **`j/k` in Commits auto-selects** the cursor row. The cursor space is `[0, len(visibleCommits)]`: idx 0 maps to `RangeWholePR` (the synthetic "All commits" row described in #15a), idx 1..N maps to `RangeSingleCommit{commits[idx-1].SHA}`. Visual mode gates this so multi-row yank does not mutate the working slice.
17. **Enter on Commits is a no-op**. The cursor commit is already auto-selected by j/k, and the Diff pane reflects that selection live; pressing Tab is the only way to shift focus to Diff.
18. **`[A]/[M]/[D]/[R]` annotation** decorates each commit row that touches `SelectedFile`.

### Files pane
19. **`j/k` in Files auto-selects** the cursor file → `selectFile(path)`. Visual mode gates this. `selectFile` resets `DiffCursor`, `DiffViewport.Top`, `CommitsCursor`, `CommentsCursor` only when the path changes.
20. **Tree mode** (`t` toggles): dirs render `v <name>/` (expanded) or `> <name>/` (folded); files show basename + status + comment count.
21. **`autoSelectTree` skips `selectFile` on dir rows** so folding/unfolding does not clobber Diff.
22. **`remapCursorOnTreeToggle`** preserves the conceptual cursor position when toggling flat ⇄ tree.
22b. **Enter is bound only to dir fold/unfold** in tree mode. Enter on a file row (flat or tree) is a no-op — j/k auto-select drives Diff/Comments sync; Tab moves focus.

### Comments pane
23. **Diff-cursor coupling**: `commentsView` shows ONLY the threads anchored at the Diff cursor's current buffer line (the rows the Diff pane decorates with `◆`). When the cursor is not on a `◆` row — including the initial state — the column reads `(no comment at cursor)`. The visible-thread set is computed by `threadsForCursor`: it maps `DiffCursor.Line` through `patchNewLineNumbers` to a new-file line, then keeps every thread where any comment's `commentNewLine` matches that line. Multiple threads on the same line all render. `flatComments` (and therefore Comments-pane j/k navigation + visual yank) is scoped to `threadsForCursor` so the cursor index never drifts past the visible content.
23b. **Render shape**: each entry is a header row plus indented body rows. Header = `<name>: <yyyy-mm-dd hh:mm> <hash>[ [pending]| [outdated]]` where the timestamp is rendered in local TZ via `CreatedAt.Local().Format("2006-01-02 15:04")` and `<hash>` is `shortSHA(CommitID)` (falling back to `OriginalCommitID`). The status tag slot carries `[pending]` (yellow, `theme.CommentPending`) for local drafts created via the compose flow, OR `[outdated]` (red, `theme.CommentOutdated`) for upstream comments whose anchor moved off the patch — the two are mutually exclusive (a pending comment cannot be outdated by definition), and pending takes precedence in the renderer. Body rows are indented by `2 + 2*(depth+1)` cols (root body = 4, reply body = 6, including the 2-col cursor area). Replies use `depth=1` so their header sits 2 cols deeper than the root header. Entries are separated by a single blank row; the cursor `>` glyph appears on the header row only. Body rendering (`renderCommentBody`) honors source line breaks the way GitHub PR comments do: every single `\n` in `c.Body` is a row break (one source line → one rendered row), and a run of 2+ consecutive `\n`s emits exactly one extra blank row to mark the paragraph boundary. Leading and trailing blank lines are elided. Each source line is then wrapped at `paneWidthComments − bodyLeader` cols via `wrapText` (cell-width measured); over-wide lines flow onto multiple rows but stay glued to their source line (no merge with the next). Fenced code blocks need no special handling — every `\n` inside `` ``` … ``` `` is already a row break under this rule, so the fence markers and code lines render on their own rows. Soft-break collapsing was tried earlier and rejected: it merged distinct source sentences into one paragraph, which mismatched both GitHub's `<br>`-on-soft-break web UI and the user's mental model of "the line I typed should be its own line."
23c. **Word-boundary rule for wrap**: `wrapText` calls `splitWrapWords` (in `internal/tui/styles.go`) instead of `strings.Fields` so a whitespace splits the input into separate words ONLY when both adjacent runes are ASCII word runes (letters / digits / ASCII punctuation). If either side is non-ASCII (CJK, emoji, etc.), the whitespace is collapsed to a single space and stays inside the running word. Without this rule, a body like `slack コマンドの後すぐに…` splits into `["slack", "コマンドの後すぐに…"]`; the long CJK trailing word can't fit alongside `slack` in a narrow column, so wrap flushes `slack` alone and strands an ASCII fragment on its own row (real bug observed on PR `DatachainDoC/doc-github#345` comment id 3055362231). With the rule, the whole `slack コマンドの…` segment is one (long) word that `hardBreak` can split mid-CJK, keeping `slack` glued to the start of the wrap output. ASCII↔ASCII whitespace (`Hello world`) still acts as a word boundary, so plain English wrap behaves unchanged.
24. **Cursor movement (`j/k`) auto-scrolls Diff** to the buffer line of the cursored comment via `syncDiffToCursorComment`. `h/l` and `backspace` are unbound — there is no thread fold/unfold (every reply is always visible) and Tab is the only focus mover.
24b. **Comments Enter starts a reply** to the thread the cursor is sitting on, via `(*Model).startComposeReply` → `buildComposeReply`. `threadIdentityForCursor` walks the flat list backing `flatComments` (`[root, replies..., next root, replies...]`) and reports the cursor's containing thread — both its GraphQL node ID (for the `addPullRequestReviewThreadReply` mutation) and the integer DB id of the root comment. When no thread is visible (cursor not on a `◆` row) `buildComposeReply` returns false and Enter is a no-op. The body-collection / POST / status-bar lifecycle is identical to inline (see §4 Diff #14); the only difference is the GraphQL mutation routed in `submitComposeCmd`.
25. **HEAD vs single-commit visibility**: HEAD/WholePR view hides outdated comments (`c.Outdated`); single-commit view shows comments anchored to that SHA via `CommitID` or `OriginalCommitID`.
25b. **Threads are always expanded.** `flatComments` and `commentsView` walk every reply; the previous `state.ThreadFolded` map and `flatIndexForThread` / `threadRootIDForCursor` / `clampCommentsCursor` helpers were removed with the keymap cleanup.

### Visual mode + yank shapes
26. **`v` enters**, `y` yanks and exits, `Esc` exits without yanking.
27. **Yank shape per pane** (clipboard contents):
    - Files: path (or paths joined by `\n` for visual range)
    - Commits: `<short_sha> <subject>`
    - Comments: `<user> @ <date>\n<body>`
    - Diff: line content (visual range = lines joined by `\n`)

### Compose (pending PR comment input)
The compose flow mirrors GitHub's web-UI "Start a review → add inline comments → Finish your review" pattern. Comments saved by compose are POSTed to GitHub immediately as part of the user's pending (draft) review; submitting the pending review with an event (Approve / Comment / Request changes) flips them to public.
27a. **`AppState.Compose *ComposeState`** is the fourth global overlay state, peer to `Visual` / `Modal` / `HelpOpen`. Non-nil while a comment is being authored, posted, or recovering from a POST failure. The `handleKey` dispatcher in `internal/tui/keys.go` checks `Compose != nil` first (after the Submit-review modal — see §4 Submit) and routes every keystroke to `handleKeyTextarea`, so background panes stay frozen while compose is active.
27b. **Lifecycle states (`ComposeStatus`)**:
   - `ComposeEditing` — body collection in progress. Two sub-paths:
     - `UseTextarea = false` (default when `$VISUAL` or `$EDITOR` is set): `tea.ExecProcess` opens the editor on a `gh-reva-compose-*.md` tempfile. Bubbletea is suspended; no overlay is drawn. On exit the file is read and emitted as `composeBodyMsg`; the tempfile is deleted in the same callback.
     - `UseTextarea = true` (no editor configured): the `overlayCompose` modal is drawn over the body. Keys append runes to `Compose.Body`; Enter inserts `\n`; Backspace drops one rune; Tab inserts `\t`; Ctrl+S emits `composeBodyMsg{body: Compose.Body}`; Esc / Ctrl+C clears Compose.
   - `ComposeSubmitting` — `submitComposeCmd` is in flight (POSTs to GitHub via GraphQL). Status bar reads `posting to GitHub…`. Modal stays up; the only key honoured is Esc / Ctrl+C, which detaches the in-flight response (it is dropped on arrival because Compose==nil).
   - `ComposeFailed` — POST returned an error. `ErrMsg` carries the message, `Body` is preserved verbatim. Ctrl+S retries (`retryComposeSubmit`) without re-asking for body; Esc cancels.
27c. **Inline (Kind = `ComposeInline`)** anchors to a Diff buffer line via `internal/diff.ResolveAnchor`. `Path` = `state.SelectedFile`, `CommitSHA` = `state.PR.HeadSHA` (always — comments anchor to the PR's head, not the currently-selected single-commit slice; mirrors GitHub web). Header / hunk rows are rejected by `buildComposeInline` (Enter is a no-op there).
27d. **Multi-line range** is opened by entering Diff visual mode (`v`), moving cursor to the other endpoint, then Enter. `internal/diff.ResolveRange` collapses anchor + cursor into a `(start_line, start_side) → (line, side)` tuple. Same-side ranges are normalized so `start_line <= line`. Mixed-side ranges (anchor on `-`, cursor on `+`) are accepted as-is. Single-line ranges (anchor == cursor) drop the `start_*` fields. `buildComposeInline` clears `state.Visual` on success.
27e. **Reply (Kind = `ComposeReply`)** captures the cursor thread's GraphQL node ID via `threadIdentityForCursor` so the reply mutation routes via `addPullRequestReviewThreadReply`. `ParentDBID` is also stored for downstream UI work (rendering "in reply to …" hints later); the mutation itself only needs the thread ID.
27f. Pending review session. `ghClient.ensurePendingReview` returns the GraphQL node ID of the user's pending review on this PR. First call: `reviews(states: [PENDING], first: 50)` query alongside `viewer { login }` — iterate the response and pick the first review whose `author.login == viewer.login`; otherwise `addPullRequestReview` (event omitted) creates a fresh pending review. The earlier `viewerLatestReview` shortcut returned the most-recent review regardless of state, so a viewer who had previously submitted (APPROVED / COMMENTED / CHANGES_REQUESTED) on the same PR would skip past their existing PENDING draft and the next `addPullRequestReview` call would 422 on GitHub's "one pending review per user per PR" constraint. The state-filtered + author-matched query avoids that miss. The ID is cached on `ghClient.pendingReviewID[n]` and reused for every subsequent compose POST until `SubmitPendingReview` clears the entry.
27g. **POST mutations** are routed by `submitComposeCmd`:
   - Inline → `addPullRequestReviewThread` with `pullRequestReviewId`, `path`, `line`, `side` (and `startLine` / `startSide` for ranges), `subjectType: LINE`. Response carries the new thread + its first comment.
   - Reply → `addPullRequestReviewThreadReply` with `pullRequestReviewId` + `pullRequestReviewThreadId`. Response carries the new comment under the same thread.
   On success, `convertGQLComment` shapes the response into a `model.ReviewComment` with `Pending: true` (because `pullRequestReview.state == PENDING`); `applyComposeSubmitted` appends it to `state.PR.Comments` and bumps `FileEntry.CommentCount`. The Comments pane header renders the `[pending]` tag (colored via `theme.CommentPending`).
27h. **Status bar contexts** (see §4 #6): Editing-textarea → `ctrl+s:save  esc:cancel`; Editing-external → `editing in $EDITOR — finish there to continue`; Submitting → `posting to GitHub…`; Failed → `ctrl+s:retry  esc:cancel`. All four replace the context AND drop the suffix.

### Submit review
The submit-review modal finalizes the pending review (and all its inline comments) with a chosen event. Mirrors GitHub web's "Finish your review" dialog.
27i. **Key `R`** opens the modal from any pane (`startSubmitReview`); the `handleKey` dispatcher routes to `handleKeySubmit` while `state.SubmitReview != nil`. `R` takes precedence over Compose: opening the submit modal while Compose is active is impossible by construction (Compose absorbs every keystroke including `R` until dismissed). The modal title is `Submit review`; the body shows `<n> pending comment(s)` (computed by `pendingCommentCount` from `PR.Comments`) and three labeled lines: `[a] approve`, `[c] comment`, `[r] request changes`.
27j. **Lifecycle states (`SubmitReviewStatus`)**:
   - `SubmitChoosing` — initial state. Pressing `a`, `c`, or `r` sets `Event` and transitions to Submitting; Esc / Ctrl+C cancels.
   - `SubmitSubmitting` — `submitReviewCmd` is in flight (`submitPullRequestReview` GraphQL mutation with the chosen event). Esc / Ctrl+C detaches; the response (when it arrives) finds `SubmitReview==nil` and is dropped.
   - `SubmitFailed` — mutation returned an error; ErrMsg shown. Ctrl+S retries with the same event; Esc cancels.
27k. **`SubmitEvent` enum values** (`model.SubmitEvent`):
   - `SubmitApprove` → `APPROVE`
   - `SubmitComment` → `COMMENT`
   - `SubmitRequestChanges` → `REQUEST_CHANGES`
   These map 1:1 to GitHub's `PullRequestReviewEvent` enum. `DISMISS` is intentionally excluded — it targets dismissing other users' reviews via a different mutation, not the submit flow.
27l. **Post-submit refresh.** Success clears `SubmitReview` and queues `refreshCommentsCmd`, which re-runs `Client.ListComments` and emits `commentsRefreshedMsg`. `applyCommentsRefreshed` swaps in the new comment list and recomputes per-file CommentCount; the just-submitted comments lose their Pending flag (the GraphQL listing now reports `pullRequestReview.state != PENDING` for them). The pending review ID cache on `ghClient` is also cleared so the next compose POST starts a fresh draft. Failure to refetch (network blip) is silently tolerated — the previous list stays visible; the user can hit `R`-Esc-anything to refresh later.
27m. **Status bar contexts**: Choosing → `a:approve  c:comment  r:request-changes  esc:cancel`; Submitting → `submitting review…`; Failed → `ctrl+s:retry  esc:cancel`. All replace context AND drop the suffix.

### Global keys
28. **Tab / Shift-Tab cycle focus** through Files → Commits → Diff → Comments. They are the only keys that move focus between panes.
29. **Enter is the comment-input gesture in Diff and Comments; not a focus mover.** Backspace is unbound everywhere. Enter bindings:
    - Files (tree mode, dir row): fold/unfold the directory.
    - Files (file row, flat or tree): no-op (j/k auto-select drives Diff/Comments).
    - Commits: no-op (cursor commit is already auto-selected).
    - Diff: open inline-comment compose at the cursor row (or Diff visual range — see §4 Diff #14). Header / hunk rows are no-op.
    - Comments: reply to the thread under the cursor (see §4 Comments #24b).
    Visual mode preserves the Diff-Enter compose gesture (range comment); Enter in any other pane while visual is active is inert (`visual.go::handleKeyVisual`). Compose itself absorbs every keystroke while `state.Compose != nil` via the top-level guard in `internal/tui/keys.go::handleKey`, so background panes cannot receive input while the textarea / submitting / failed overlay is up.
30. **Shift+J / Shift+K** advance to next/prev file from any pane via `advanceFile(forward bool)`. Focus is preserved.

### Color theming
31. **Theme palette is the single source of truth for color**. `internal/theme.Theme` holds 28 `lipgloss.Color` fields plus `SyntaxStyle *chroma.Style`, covering pane chrome, diff lines (fg + near-black bg), status badges, comment metadata (incl. `CommentPending` for local-draft tags), the spinner, and the splash logo's three shade ramps (`LogoShade1/2/3`). `Resolve(name)` accepts `"builtin-dark"`, any chroma styles registry name, or `""`; the empty name routes through the `defaultThemeName` constant (currently `"gruvbox"`). Unknown names error.
32. **Chroma adapter (`internal/theme/chroma.go`)** maps chroma tokens to UI roles: `GenericInserted`/`GenericDeleted` → diff add/del fg + status badges; `GenericSubheading` → hunk header + status modified + `LogoShade2`; `GenericHeading` → file header + status renamed; `GenericStrong` → active border / pane title / commit author + cursor `> ` + `LogoShade1`; `GenericEmph` → comment anchor; `LineNumbers` → numbers / SHA / inactive border (with `Brighten(-0.3 / -0.4)` for separators, and `Brighten(-0.2)` for `LogoShade3`). Missing tokens fall back to `builtinDark()`. The chroma style itself is stored in `Theme.SyntaxStyle` for token-level fg in diff content. Two invariants override the per-token mapping: (a) `DiffPlusBg` / `DiffMinusBg` are hard-coded to `#0d3b13` / `#3b0d0d` for every theme so the +/- distinction stays unambiguous regardless of palette; and (b) `GenericInserted`/`GenericDeleted` are read through `pickAccent`, which prefers `StyleEntry.Background` when `StyleEntry.Colour` equals the editor background (gruvbox-style inversion convention) — without this fallback `DiffPlus` / `DiffMinus` / `StatusAdded` / `StatusDeleted` would collapse to the editor base (`#282828` for gruvbox) and render invisibly. Cursor `> ` shares its source with `PaneTitleActive` (`GenericStrong`) so the focus accent is internally consistent.
33. **`m.theme` is non-nil after `NewModel`**. `Model` constructor seeds the empty-name default (currently `gruvbox` via `defaultThemeName`); `cmd/root.go` overrides via `Model.SetTheme` after `theme.Resolve`. Renderers must dereference safely.
34. **Renderer color application uses `internal/tui/colors.go`** helpers — `fg`, `fgBold`, `bgRow`. They no-op when the color is the zero value. Apply color AFTER `padTrunc` / cell assembly, never before, so width math stays driven by visible cells.
35. **`padTrunc` is SGR-aware** via `lipgloss.Width` for measurement and `ansi.Truncate` for over-width truncation (preserves SGR run integrity). Right-pads with plain spaces.
36. **Pane border / title coloring** lives in `app.go::renderPaneBox`. The active pane's border + title use `PaneBorderActive` + `PaneTitleActive` (Bold); the rest use `PaneBorderInactive` + `PaneTitle`.
37. **Visual-mode rows get a row-wide background** via `bgRow(row, theme.VisualRangeBg)` after the row has been padded to `paneWidthDiff`. The bg ends inside the pane; pane borders stay border-colored.
38. **Diff cells use bg-for-change + per-token syntax fg** (`internal/tui/syntax.go::styledDiffCell`). `+` rows get `DiffPlusBg` row-wide AND chroma syntax-highlighted fg per token; `-` rows likewise with `DiffMinusBg`. Context rows also run through `styledDiffCell` with `bg=""` so they get the same per-token fg on the terminal default bg — visual parity with the changed rows. File / hunk headers stay flat-fg (they are not source code). The cell's leading marker (`+`/`-`/space) is excluded from the chroma lexer (parses as a syntax error in most languages) and re-emitted under the same bg. The `+` / `-` rune itself is rendered bold with `theme.DiffPlus` / `theme.DiffMinus` foreground (uniform `#3fb950` / `#f85149` across themes — same intent as the uniform bg) so the marker reads at a glance against syntax-colored content; the continuation / context space marker leaves the fg untouched. The marker fgs are theme-uniform constants and therefore do NOT participate in the `syntaxCache` key. Tokenizing context rows is bounded by `rowCache` (per-buffer-line) and `syntaxCache` (per-(lexer, bg, cell)) — first render pays once, subsequent frames hit cache. Earlier versions left context flat-fg to spare the e2e render budget; the cache pair removed that constraint.
39. **Syntax-token results are cached** via `Model.syntaxCache` (a `*sync.Map`) keyed on `lexer.Name + style.Name + bg + cell`. Without it, even just changed-line tokenization races the parser idle deadline on Tab redraws. The cache pointer is shared across Model copies (Bubbletea returns new Models each Update).
39b. **Per-patch derived data is bundled in `Model.patchLinesC` → `*patchInfo`** keyed on `diffKey(sha, path)`. `patchInfo` carries `lines` (always), `specs` (lazy, split mode only), `newNums` (lazy, comment line mapping). Previously each render re-ran `strings.Split(patch)` + `parseDiffSpecs` + `newLineNumbers`; the bundle eliminates O(buffer) work per render. `parseDiffSpecs`, `newLineNumbers`, `bufferIndexForNewLine` accept `[]string` rather than the raw patch string so they reuse the cached split. Renderers that need raw lines call `m.patchLines()`; `m.patchSpecs()` and `m.patchNewLineNumbers()` lazily populate the secondary fields.
39c. **Per-buffer-line render output is cached** via `Model.rowCache` (a `*diffRowCache` with `map[string][]string`). `renderSplitBufferLine` and `renderUnifiedBufferLine` only cache when the row is NOT the cursor and NOT in visual range; the cursor row recomputes every keystroke (correct), 28/30 visible rows hit cache (fast). Key includes mode (`s`/`u`), `lineIdx`, `halfW`, `commented`. Width / patch identity changes invalidate via `m.invalidateRowCacheIfStale()` (called once at the top of `diffView`). Without this cache, split-mode `j`-hold visibly stalls (each frame redoes ~30 rows of tokenize + concat + padTrunc).
39d. **Diff-renderer perf rule**: do not call `strings.Split(patch, "\n")` or `parseDiffSpecs(patch)` directly from any hot path. Always go through `m.patchLines() / m.patchSpecs() / m.patchNewLineNumbers()`. New caches that share fate with the patch should also key on `diffKey(sha, path)` and reset via the `invalidateRowCacheIfStale` pattern (key + paneWidthDiff + halfW).
40. **`waitReady` defaults to 10s** in `e2e/helpers/launch.mjs` to absorb chroma's `styles` + `lexers` init cost (~500ms cold) plus first-frame tokenization. Tests that need a tighter signal can pass `{ timeout: ... }` explicitly.
41. **`session.press` / `session.type` are wrapped with a 120ms settle** in `launchReva`. bubbletea's Update→View pipeline is async and ghostty's parser needs a beat to drain SGR-laden output before subsequent `text()` reads see the post-keystroke screen. Don't reach for `session.press` directly inside helpers — go through the wrapped session returned by `launchReva`.
42. **Pane modal (`<space>` zoom)** is gated by `model.ModalState{Pane}` and toggled by `<space>` in the Files / Commits / Comments panes. `m.state.Modal == nil` is closed; non-nil holds the `PaneID` whose contents the modal is showing. Toggle is `(*Model).toggleModal(pane)` in `internal/tui/modal.go`: a second `<space>` from the same pane closes. Diff `<space>` is unchanged — it still toggles split⇄unified (separate code path in `pane_diff.go`, never touches `state.Modal`). The modal closes implicitly when focus moves (`tab`, `shift+tab`), the help modal opens (`?`), `esc` is pressed, `q` is pressed, or `Ctrl+C` is pressed — all four single-key dismiss gestures behave identically (close first, no app exit). Both `q` and `Ctrl+C` only quit the app when the modal is already closed, the dual-purpose convention shared with the Help modal: a stray dismiss gesture in a zoom view does not drop the user out of the program. (The earlier contract had `Ctrl+C` quit unconditionally as a "force exit" backstop; the symmetric-dismiss model was adopted because the asymmetry surprised users in interactive review — `q` and `Ctrl+C` are interchangeable elsewhere in vim and most TUIs.) `J` / `K` (advance file) leave the modal open by design so users can scrub through files inside Commits / Comments modals. Visual mode (`v` / `y` / `esc`) is allowed inside the modal — the comment-input modal mentioned in §4 Diff #14 is a separate Phase 2 feature that opens `$EDITOR`, distinct from this read-only zoom view, so allowing visual selection here does not collide with that plan. Layout (`modalLayout`): width = max content row + 3 (1-col leading-space pad + 2 border cols), capped to `m.width - 4`; height = body rows + 4 (top border + title + divider + bottom border), capped to `m.height - 2`; centered both axes. Content (`modalContent`) reuses the regular pane renderers — Files / Commits because their row format is width-independent (no wrap), Comments by mutating the local `m.paneWidthComments` to `min(m.width - 10, commentsModalWrapMax = 80)` before calling `commentsView()` (Model is a value receiver, so the wider budget never leaks back to the body rendered behind the modal). Title row carries the bare pane name with a single leading space (`│ Files`); the regular pane title rows always carry `▶ ` (active) or `  ` (inactive), so the single-space form is the unique signature for e2e detection (`13_modal.test.mjs::MODAL_TITLE_RE = /│ (Files\|Commits\|Comments)\s+│/`). Overlay (`overlayModal`) splices via `spliceMid`, shared with the Help overlay (declared in `modal.go` for both consumers; uses `ansi.Truncate` + `ansi.TruncateLeft` to preserve SGR run integrity). j / k navigation inside the modal goes through the regular pane handlers (no separate routing) so cursor / `SelectedFile` / `SelectedRange` / `CommentsCursor` updates propagate to the underlying main state — closing the modal leaves the main UI on the same row the user landed on inside the zoom view. The previous hover-popup machinery (`HoverState`, `hover.go`, `e2e/tests/13_hover.test.mjs`) was deleted at the same time; do not reintroduce a "show full path on hover" overlay — the modal is now the sole zoom affordance.

---

## 5. E2E test conventions

### Helpers (`e2e/helpers/launch.mjs`)
- `launchReva({ args, fixture, cols, rows, env })` — spawn gh-reva with default fixture.
- `waitReady(session, { timeout = 5000 })` — wait for `Files` text after PR load.
- `quit(session)` — send `q`, then close.
- `activePaneLabel(session)` — return the single active pane name; throw if 0 or > 1.
- **`paneText(screen, label)`** — extract the pane's column slice. Required when asserting on cursor markers (`^>`) in non-leftmost panes — borders place `│` at col 0 of every line, and cross-column content satisfies the wrong row otherwise. Trailing border `│` chars are stripped automatically.
- **`countSelectedRows(screen, label)`** — count rows in the pane's slice that begin with `> `. Used by visual-yank tests.

### Patterns

**`describe + before + screen capture`** — for read-only observation tests grouped by initial state. Capture screen once, run many `test()` blocks against it. Saves ~1 s per shared launch (was ~5 s before the TERM=tmux-256color fix described below).
- Currently used: B1+B2+B6, D1+D2, E1+E1b+E2, G1+G2+G3+G4.

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
  test('B', () => { /* assert against screen */ })
})
```

**`describe + before/after + shared session`** — for navigation tests that begin and end at Files focus without mutating cursors / SelectedFile. Tests run sequentially in the same session.
- Currently used: C1+C4+C5+C7.

**Independent `test()` blocks** — for tests that mutate state (visual mode, file selection, single-commit drill). Each launches its own session.

### Substring rules
- **Prefer short, contiguous substrings** (≤ ~20 chars) for column-wrap safety. `Implement Hello function` may wrap mid-string at narrow Comments widths; use `Implement Hello` or the SHA.
- **Anchor on column slice via `paneText`** when checking cursor rows (`/^>...content/m`). Borders break full-screen `^` anchors.
- **Substring negation** (`!includes`) usually works on raw screen because absent text is absent everywhere.

### Fixture choice
- Default tests → `testdata/sample-pr.json` (small, fast).
- Long-comment wrap tests → `testdata/wrap-pr.json`.
- Performance / large-PR tests → `testdata/large-pr.json` + responsiveness assertion.
- Add a new fixture rather than extending `sample-pr.json` when the test needs unusual content (avoids cross-test pollution).

---

## 6. Common pitfalls

- **Forgot to rebuild binary**: `go build -o gh-reva .` (NOT `go build ./...`). The `pretest` hook of `pnpm test` does this automatically; targeted `node --test` does not.
- **`^>` regex on raw screen**: borders place `│` at col 0 of each row. Use `paneText(screen, label)` slice instead.
- **Long substring assertions**: column wrap will split words across rows. Shorten or normalize before checking.
- **bubbletea startup ~1 s blank**: first `s.text()` after launch can be empty. Always use `waitReady` before reading.
- **tuistory cannot reliably emit CSI Z**: shift-tab tests are skipped (C2). Document inline and verify by inspection.
- **Do not re-introduce `lipgloss.Border()`**: we render boxes manually via `renderPaneBox` in `app.go` because lipgloss cannot produce the title-bar divider. Touch only `renderPaneBox` for box visual changes.
- **Tabs in Diff content**: split mode requires `expandTabs(line, 4)` before wrap/pad. Without it, terminal-side tab expansion shifts `│`.
- **CJK / wide chars in Comments**: `wrapText` (in `internal/tui/styles.go`) measures with `runewidth.StringWidth` / `runewidth.RuneWidth` so CJK and emoji are accounted for as 2 cells. The accumulator and the hard-break helper both use cell width — a single CJK rune that does not fit the remaining budget rolls to the next chunk. Don't reintroduce `utf8.RuneCountInString` here: rune count and display width diverge for any non-ASCII fixture, and `renderPaneBox`'s per-row `padTrunc` will silently truncate any over-wide row produced upstream.
- **Diff wrap is always on**: there is no toggle. A buffer line that exceeds the cell width is split into multiple display rows with cursor / `◆` rendered only on the first row, and continuation rows indented past the diff marker. `DiffViewport.Top` is a buffer-line index; `diffViewportHeight()` is in display rows; `displayRowsBetween` is the bridge.
- **Color SGR doesn't reach tuistory's `text()`**: ghostty parses ANSI into cell state, so substring assertions stay color-agnostic. The A9 smoke test guards against raw `\x1b` bytes leaking into the rendered text — keep it in place when adding new renderers.
- **Chroma case quirk**: registry key `rpgle` resolves to a Style whose `Name` is `RPGLE`. `theme.Resolve` canonicalizes on the registry key; do not rely on `Style.Name` matching the user-supplied name.
- **Bubbletea v1 has no color profile option**: `lipgloss.SetColorProfile(termenv.Ascii)` and `SetHasDarkBackground(true)` must be called BEFORE `tea.NewProgram`. `cmd/root.go` does this; new entry points must replicate.
- **Chroma init is eager**: importing `github.com/alecthomas/chroma/v2/styles` parses all 74 embedded XMLs at package init; `chroma/v2/lexers` registers ~250 lexers. Combined cold-start cost is ~500ms. Don't import these from hot-path packages — the theme module is the gateway.
- **Diff syntax highlighting needs the cache**: `Model.syntaxCache` is the only thing keeping diff rendering snappy. Don't accidentally drop the pointer when restructuring `Model` (e.g. via `NewModel` rewrites) — without the cache, e2e starts intermittently failing on `waitReady`.
- **`Model` has 3 caches that must propagate across Bubbletea's value-copied Updates**:
  - `syntaxCache` — pointer (`*syntaxCache`); the wrapped `sync.Map` is shared by pointer identity.
  - `rowCache` — pointer (`*diffRowCache`); the wrapped `map[string][]string` is shared by pointer identity.
  - `patchLinesC` — **struct value** (`patchLinesCache`), but its only field `cache` is a `map[string]*patchInfo`. Maps in Go are reference types: copying the struct duplicates the header, but every copy points at the same underlying hash table. So the struct-value embedding is safe **only because** that field is a map — replacing it with a slice / scalar would silently break cache propagation.
  Do not switch `Model` to struct embedding that re-allocates these fields, do not change `NewModel` to deep-copy them, and do not turn `patchLinesC.cache` into a non-reference type. All three failure modes look identical at the type checker but cause every render to miss the cache, and j/k repeat lag returns.
- **`s.press` / `s.type` are auto-settled in tests**: `launchReva` wraps the tuistory session so a 120ms wait fires after every keystroke. Don't add manual `await sleep(N)` after presses; if a test still races, the right fix is `await s.waitForText(<expected post-state>)` rather than upping the global settle.
- **`launchReva` forces TERM=tmux-256color via `sh -c`**: bubbletea v1's `tea_init.go` calls `lipgloss.HasDarkBackground()` at package import, which makes termenv send OSC 11 + DSR queries to stdout and block up to `termenv.OSCTimeout` (5 s) waiting for a terminal that does not exist behind the PTY. termenv's `termStatusReport` short-circuits when `TERM` starts with `screen` / `tmux`, so we set `TERM=tmux-256color` (and keep `COLORTERM=truecolor` so the rendered profile stays TrueColor). Tuistory's `session.js` hard-codes `TERM: 'xterm-truecolor'` AFTER spreading `options.env`, so the value cannot be passed through the `env:` field — `launchReva` instead spawns `/bin/sh -c "TERM=tmux-256color COLORTERM=truecolor exec gh-reva …"` so the child process re-applies the right `TERM` immediately before exec. Removing this wrapper restores the 5 s per-launch idle wait that previously dominated the suite (606 s → 26 s after the fix).
- **Don't import `chroma/v2/styles` or `chroma/v2/lexers` outside `internal/theme` and `internal/tui/syntax.go`**. Both packages run heavy `init()` work (~500ms cold). The theme module is the single gateway; non-theme code asks `m.theme.SyntaxStyle` / `m.currentLexer()`.

---

## 7. Output conventions

These follow the global CLAUDE.md but are emphasized here:

- **Chat replies**: Japanese, neutral professional register. No slang, no emojis, no self-deprecating hedges.
- **Code identifiers, log/error messages, comments, PR templates**: English.
- **CLAUDE.md, prompts, agent instructions, skill definitions**: English.
- **Cite file locations**: `path:line` when discussing code (e.g. `internal/tui/pane_diff.go:144`).
- **Cite evidence URLs** at end of any research-based reply.
- **Never use emojis** unless the user asks.

---

## 8. Commit conventions

- Commit only when explicitly requested by the user.
- Never push to main / master; never force-push. Tag pushes are allowed when explicitly requested as part of a release (see §9).
- Subject ≤ 70 chars; body explains the why if non-obvious.
- Trailer: `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.
- Stage by name when feasible. `git add -A` is allowed for the initial commit and when `.gitignore` is known to be correct, but not for arbitrary staging.

---

## 9. Release procedure

Releases are driven entirely by the `v*` tag pushed to `origin`. The
`release.yml` workflow runs goreleaser which reads the version from
`{{.Version}}` (= the tag) and produces per-OS-arch binaries with the
`gh-reva_<os>-<arch>` name template (the hyphen is required — gh CLI's
`gh extension install` matches assets by `strings.HasSuffix(name, "<os>-<arch>")`,
so `_` in that slot breaks the install path; documented in
`.goreleaser.yaml:20-25`). There is NO `version.go` to bump and NO
changelog to update — the tag is the single source of truth.

### Steps for a patch / minor / major release

Run from the repo root. Replace `vX.Y.Z` with the actual version.

1. **Pre-flight checks** (must all pass before tagging):
   ```sh
   git status                       # working tree must be clean OR contain only the release-bound diff
   go vet ./...
   go test ./...                    # internal/api + theme + tui packages
   (cd e2e && pnpm test)            # full e2e (pretest hook auto-rebuilds gh-reva)
   git log --oneline $(git describe --tags --abbrev=0)..HEAD   # confirm what's shipping
   ```
2. **Pick the next version** by reading `git tag --sort=-v:refname | head -1` and applying SemVer:
   - patch (bug fixes only): `vMAJOR.MINOR.(PATCH+1)`
   - minor (new features, no breaking change): `vMAJOR.(MINOR+1).0`
   - major (breaking change): `v(MAJOR+1).0.0`
3. **Commit any pending work** with the standard `type(scope): subject` Conventional-Commits style. One commit per logical feature is preferred; a single composite commit is acceptable for tightly-coupled changes that touch overlapping files.
4. **Bump the e2e workspace version** in `e2e/package.json` to match the new tag's `MAJOR.MINOR.PATCH` (no `v` prefix). Past convention: `chore(release): bump e2e workspace to vX.Y.Z` as a separate commit. The e2e workspace version has no functional effect on the release — it's a lockstep marker so the workspace and the binary share an identifier.
5. **Create the annotated tag** at HEAD:
   ```sh
   git tag -a vX.Y.Z -m "vX.Y.Z"
   ```
   Use `-a` (annotated), not lightweight, so `git describe` works correctly.
6. **Push master + tag** in one atomic step:
   ```sh
   git push origin master vX.Y.Z
   ```
7. **Watch the workflow**:
   ```sh
   gh run watch --exit-status                                      # or:
   gh run list --workflow=release.yml --limit 1
   ```
   On success, the GitHub release page exposes `gh-reva_<os>-<arch>` binaries + `checksums.txt`. If the workflow fails, fix forward and re-tag with the next patch (e.g. vX.Y.Z+1) — never delete and re-push the same tag, because users who already pulled it would silently get a different artifact.
8. **Smoke-verify the release**:
   ```sh
   gh release view vX.Y.Z
   gh extension install ktrysmt/gh-reva --force                    # installs the freshly published binary
   gh reva --version                                               # should print vX.Y.Z (commit, date)
   ```

### Things that REQUIRE explicit user authorization (don't autonomously do)

- Any push to `main` / `master` (release flow only).
- Tag creation + push.
- `gh release edit` / `gh release delete` (post-publish edits).

The user saying "release してほしい" / "release まで進めて" / "patch +1 で release" counts as explicit authorization for the full §9 sequence; partial requests like "commit して" do not authorize tagging or pushing.
