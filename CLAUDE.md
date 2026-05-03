# CLAUDE.md — gh-rv development conventions

`gh` extension that opens a vim-like 4-pane TUI for reviewing GitHub PRs.
Built on `bubbletea` + `lipgloss`. Single-purpose CLI; no shared infrastructure.

This file is authoritative for development. Read it once at the start of any
session that touches this repo, and update it when an invariant changes.

---

## 1. Build / test commands

```sh
# Repo root: /Users/dew/workspace/gh-rv

# Go
go build -o gh-rv .            # produce ./gh-rv binary at repo root
go vet ./...
go test ./...                  # currently only internal/api ghclient_errors_test

# Manual TUI
./gh-rv --fixture testdata/sample-pr.json
./gh-rv --fixture testdata/large-pr.json
./gh-rv --fixture testdata/sample-pr.json --slow-load 500ms

# E2E (cd e2e first)
pnpm install
pnpm test                      # full suite; pretest hook auto-rebuilds gh-rv
pnpm run test:smoke
node --test --test-force-exit --test-timeout=20000 \
     --test-name-pattern='F2|F11' tests/05_pane_diff.test.mjs   # targeted

# Large fixture regeneration
go run testdata/gen_large_fixture.go testdata/large-pr.json
```

`go build -o gh-rv .` (NOT `go build ./...`) is required — the latter does
not produce a usable binary at repo root. Targeted `node --test` skips the
pretest hook, so rebuild manually.

### Hidden flags (E2E only)
- `--fixture <path>` — load PR data from JSON instead of GitHub
- `--simulate-error <kind>` — `not_found` | `rate_limit` | `network` | `unauthorized`
- `--diff-height N` — pin Diff viewport height for deterministic scroll tests
- `--slow-load <duration>` — inject per-API sleep in fixtureClient (spinner observation)

### User-facing flags
- `--theme <name>` — color theme; default `builtin-dark`. Accepts any chroma styles registry name (74) plus `builtin-dark`. `GH_RV_THEME` env var works as fallback.
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
gh-rv/
├── cmd/root.go                     # CLI entry, flags (incl hidden)
├── main.go
├── internal/
│   ├── api/                        # GitHub client (go-gh) + fixture mode
│   │   ├── client.go               # Client interface
│   │   ├── ghclient_*.go           # real client (gh REST API)
│   │   ├── fixture.go              # fixtureClient (loads testdata/*.json)
│   │   ├── error_client.go         # error injection (--simulate-error)
│   │   └── ghclient_errors_test.go # httptest 401 / 404 / 429 / pagination
│   ├── clipboard/
│   ├── diff/                       # patch parsing (sourcegraph/go-diff)
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
│       ├── hover.go                # cursor-row tooltip (Files/Commits)
│       └── diffmap.go              # newLineNumbers / commentThreadIndexForDiffLine
├── testdata/
│   ├── sample-pr.json              # default fixture (5 files, 3 commits, 4 comments)
│   ├── large-pr.json               # 60 commits / 120 files / 122 KB (J3)
│   ├── wrap-pr.json                # single long-bodied comment (G11)
│   └── gen_large_fixture.go        # //go:build ignore generator
└── e2e/
    ├── helpers/launch.mjs          # launchGhRv / paneText / countSelectedRows
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
3. **`splitColumnWidths` (total ≥ 130)**: left = 42, right = 57, mid = total − 99. Inner targets Files/Commits = 40, Comments = 55, Diff = remainder.
4. **Active pane**: `▶ ` prefix on its title row. Exactly one pane has it.
5. **Cursor row**: `> ` prefix in Files / Commits / Diff / Comments. Visual-range rows also carry `> `.
6. **Visual mode indicator**: `-- VISUAL --` on its own row at the bottom. 1 row reserved when `state.Visual != nil`.
7. **Loading view**: pre-PR uses `<spinner> Loading PR (<stage>)...` (no boxes). Stages: `metadata → commits → files → comments → diffs → ready`.

### Diff pane
8. Split mode layout (first row of a buffer line): `<cursor 2><marker 2><oldLn 4><sp 1><leftCell halfW><sp 1>│<sp 1><newLn 4><sp 1><rightCell halfW>`. Fixed overhead = 17. `halfW = (paneWidthDiff − 17) / 2`. Degrades to unified when `halfW < 8` (structural fallback only).
9. Tab expansion: `expandTabs(line, 4)` is applied before wrap/pad. Without it, terminal-side tab expansion shifts `│`.
10. `◆` gutter marker appears in the marker slot (cols 2–3) on the **first display row** of a buffer line that carries an anchored review comment. Continuation rows (from wrap) leave the slot blank.
11. Split row distribution: header (`---`/`+++`/`@@`) and context lines render on both sides; `-` only on left; `+` only on right.
12. Wrap is always on. Buffer line ↔ display row is 1:N. `DiffCursor.Line` indexes the raw patch buffer; cursor `>` and `◆` markers appear only on the first display row of each buffer line. Continuation rows in unified are indented 5 cols (cursor 2 + marker 2 + diff-marker 1) so wrapped content aligns past the `+`/`-`/space marker. In split, continuation rows leave cursor / marker / oldLn / newLn columns blank, prefix each cell with 1 blank to align past the diff marker, and re-draw `│` at the same column.
13. `fitPaneTitle` preserves the `[mode]` suffix at narrow widths. Label shrinks with `…`.
14. Diff Enter on an anchored row focuses Comments and selects that thread. Non-anchored Enter is a no-op (Phase 2 will open a comment-input modal). Anchor lookup is by buffer line, regardless of which display row the cursor sits on.

### Commits pane
15. **`visibleCommits` is auto-filtered by `SelectedFile`**. No manual override; the previous `space` toggle and `CommitFilterFile` field were removed. Without `SelectedFile`, all commits show.
16. **`j/k` in Commits auto-selects** the cursor commit → `SelectedRange = SingleCommit`. Visual mode gates this.
17. **Enter on Commits is focus-shift only** (does NOT change `SelectedRange`). Single-commit drill is driven by `j/k` followed by Enter; Enter without prior `j/k` keeps the WholePR view set by Files Enter.
18. **`[A]/[M]/[D]/[R]` annotation** decorates each commit row that touches `SelectedFile`.

### Files pane
19. **`j/k` in Files auto-selects** the cursor file → `selectFile(path)`. Visual mode gates this. `selectFile` resets `DiffCursor`, `DiffViewport.Top`, `CommitsCursor`, `CommentsCursor` only when the path changes.
20. **Tree mode** (`t` toggles): dirs render `v <name>/` (expanded) or `> <name>/` (folded); files show basename + status + comment count.
21. **`autoSelectTree` skips `selectFile` on dir rows** so folding/unfolding does not clobber Diff.
22. **`remapCursorOnTreeToggle`** preserves the conceptual cursor position when toggling flat ⇄ tree.

### Comments pane
23. **Word-wrap**: `renderCommentRow` wraps the body at `paneWidthComments − headWidth` cols. Continuation rows are indented by `headWidth` spaces so the body column lines up.
24. **Cursor movement (`j/k/h/l/backspace`) auto-scrolls Diff** to the buffer line of the cursored comment via `syncDiffToCursorComment`.
25. **HEAD vs single-commit visibility**: HEAD/WholePR view hides outdated comments (`c.Outdated`); single-commit view shows comments anchored to that SHA via `CommitID` or `OriginalCommitID`.

### Visual mode + yank shapes
26. **`v` enters**, `y` yanks and exits, `Esc` exits without yanking.
27. **Yank shape per pane** (clipboard contents):
    - Files: path (or paths joined by `\n` for visual range)
    - Commits: `<short_sha> <subject>`
    - Comments: `<user> @ <date>\n<body>`
    - Diff: line content (visual range = lines joined by `\n`)

### Global keys
28. **Tab / Shift-Tab** cycle focus through Files → Commits → Diff → Comments.
29. **Backspace** moves focus one step backward in the drill chain.
30. **Shift+J / Shift+K** advance to next/prev file from any pane via `advanceFile(forward bool)`. Focus is preserved.

### Color theming
31. **Theme palette is the single source of truth for color**. `internal/theme.Theme` holds 24 `lipgloss.Color` fields plus `SyntaxStyle *chroma.Style`, covering pane chrome, diff lines (fg + near-black bg), status badges, comment metadata, and the spinner. `Resolve(name)` returns `builtin-dark` for "" / "builtin-dark", any chroma styles registry name, or an error.
32. **Chroma adapter (`internal/theme/chroma.go`)** maps chroma tokens to UI roles: `GenericInserted`/`GenericDeleted` → diff add/del fg + status; `Brighten(-0.85)` of those → `DiffPlusBg`/`DiffMinusBg` (near-black hue); `GenericSubheading` → hunk header + status modified; `GenericHeading` → file header + status renamed; `GenericStrong` → active border / pane title / commit author; `GenericEmph` → comment anchor; `LineNumbers` → numbers / SHA / inactive border (with `Brighten(-0.3 / -0.4)` for separators). Missing tokens fall back to `builtinDark()`. The chroma style itself is stored in `Theme.SyntaxStyle` for token-level fg in diff content.
33. **`m.theme` is non-nil after `NewModel`**. `Model` constructor seeds `builtin-dark` (whose `SyntaxStyle = github-dark`); `cmd/root.go` overrides via `Model.SetTheme` after `theme.Resolve`. Renderers must dereference safely.
34. **Renderer color application uses `internal/tui/colors.go`** helpers — `fg`, `fgBold`, `bgRow`. They no-op when the color is the zero value. Apply color AFTER `padTrunc` / cell assembly, never before, so width math stays driven by visible cells.
35. **`padTrunc` is SGR-aware** via `lipgloss.Width` for measurement and `ansi.Truncate` for over-width truncation (preserves SGR run integrity). Right-pads with plain spaces.
36. **Pane border / title coloring** lives in `app.go::renderPaneBox`. The active pane's border + title use `PaneBorderActive` + `PaneTitleActive` (Bold); the rest use `PaneBorderInactive` + `PaneTitle`.
37. **Visual-mode rows get a row-wide background** via `bgRow(row, theme.VisualRangeBg)` after the row has been padded to `paneWidthDiff`. The bg ends inside the pane; pane borders stay border-colored.
38. **Diff cells use bg-for-change + per-token syntax fg** (`internal/tui/syntax.go::styledDiffCell`). `+` rows get `DiffPlusBg` row-wide AND chroma syntax-highlighted fg per token; `-` rows likewise with `DiffMinusBg`. Context rows also run through `styledDiffCell` with `bg=""` so they get the same per-token fg on the terminal default bg — visual parity with the changed rows. File / hunk headers stay flat-fg (they are not source code). The cell's leading marker (`+`/`-`/space) is excluded from the chroma lexer (parses as a syntax error in most languages) and re-emitted under the same bg. Tokenizing context rows is bounded by `rowCache` (per-buffer-line) and `syntaxCache` (per-(lexer, bg, cell)) — first render pays once, subsequent frames hit cache. Earlier versions left context flat-fg to spare the e2e render budget; the cache pair removed that constraint.
39. **Syntax-token results are cached** via `Model.syntaxCache` (a `*sync.Map`) keyed on `lexer.Name + style.Name + bg + cell`. Without it, even just changed-line tokenization races the parser idle deadline on Tab redraws. The cache pointer is shared across Model copies (Bubbletea returns new Models each Update).
39b. **Per-patch derived data is bundled in `Model.patchLinesC` → `*patchInfo`** keyed on `diffKey(sha, path)`. `patchInfo` carries `lines` (always), `specs` (lazy, split mode only), `newNums` (lazy, comment line mapping). Previously each render re-ran `strings.Split(patch)` + `parseDiffSpecs` + `newLineNumbers`; the bundle eliminates O(buffer) work per render. `parseDiffSpecs`, `newLineNumbers`, `bufferIndexForNewLine` accept `[]string` rather than the raw patch string so they reuse the cached split. Renderers that need raw lines call `m.patchLines()`; `m.patchSpecs()` and `m.patchNewLineNumbers()` lazily populate the secondary fields.
39c. **Per-buffer-line render output is cached** via `Model.rowCache` (a `*diffRowCache` with `map[string][]string`). `renderSplitBufferLine` and `renderUnifiedBufferLine` only cache when the row is NOT the cursor and NOT in visual range; the cursor row recomputes every keystroke (correct), 28/30 visible rows hit cache (fast). Key includes mode (`s`/`u`), `lineIdx`, `halfW`, `commented`. Width / patch identity changes invalidate via `m.invalidateRowCacheIfStale()` (called once at the top of `diffView`). Without this cache, split-mode `j`-hold visibly stalls (each frame redoes ~30 rows of tokenize + concat + padTrunc).
39d. **Diff-renderer perf rule**: do not call `strings.Split(patch, "\n")` or `parseDiffSpecs(patch)` directly from any hot path. Always go through `m.patchLines() / m.patchSpecs() / m.patchNewLineNumbers()`. New caches that share fate with the patch should also key on `diffKey(sha, path)` and reset via the `invalidateRowCacheIfStale` pattern (key + paneWidthDiff + halfW).
40. **`waitReady` defaults to 10s** in `e2e/helpers/launch.mjs` to absorb chroma's `styles` + `lexers` init cost (~500ms cold) plus first-frame tokenization. Tests that need a tighter signal can pass `{ timeout: ... }` explicitly.
41. **`session.press` / `session.type` are wrapped with a 120ms settle** in `launchGhRv`. bubbletea's Update→View pipeline is async and ghostty's parser needs a beat to drain SGR-laden output before subsequent `text()` reads see the post-keystroke screen. Don't reach for `session.press` directly inside helpers — go through the wrapped session returned by `launchGhRv`.
42. **Hover popup is gated by `model.HoverState{Gen, Show}` plus `Model.hoverDelay`**. Every `tea.KeyMsg` increments `Gen` and forces `Show=false`; eligible panes (Files / Commits, non-Visual) then schedule a `tea.Tick(hoverDelay)` returning `HoverTickMsg{Gen}`. The handler only flips `Show=true` when `msg.Gen == m.state.Hover.Gen`, so a later keystroke (which has bumped `Gen`) implicitly cancels the in-flight tick. `PRLoadedMsg` arms the initial popup so a freshly-opened PR shows the cursor's full path / subject after the delay without requiring a key. `--hover-delay 0` disables the popup; e2e settle (120ms) is longer than tight delays (~80ms) so a press-then-capture sequence sees the popup again on the new cursor row. The overlay is anchored to the right of the active pane (left = `splitColumnWidths(...).left`) and positioned vertically by `hover.go::hoverLayout`: prefer above the cursor row, fall back to below when the cursor is too close to the top, and clamp content rows to fit if neither side has room. Multi-line popups expand row count to match `c.Message`'s body lines (capped at `hoverMaxLines = 12`). Reserved height was rejected because layout would shift on every show / hide cycle; the overlay simply splices via `spliceColumn` (preserves prefix, drops suffix past `left+width`).
43. **`launchGhRv` injects `--hover-delay 0` by default** in `e2e/helpers/launch.mjs` so substring / equality assertions across the suite stay deterministic. Tests that exercise the popup (13_hover.test.mjs) pass their own `--hover-delay` value and the explicit one wins (the helper checks `args.includes('--hover-delay')`). Don't drop this default without auditing the suite for "screen before vs after a no-op key" comparisons (e.g. D4) and "Diff title visible" substring checks (e.g. F1, J3) — both classes break when the popup is allowed to overlay the Diff / Comments title row.

---

## 5. E2E test conventions

### Helpers (`e2e/helpers/launch.mjs`)
- `launchGhRv({ args, fixture, cols, rows, env })` — spawn gh-rv with default fixture.
- `waitReady(session, { timeout = 5000 })` — wait for `Files` text after PR load.
- `quit(session)` — send `q`, then close.
- `activePaneLabel(session)` — return the single active pane name; throw if 0 or > 1.
- **`paneText(screen, label)`** — extract the pane's column slice. Required when asserting on cursor markers (`^>`) in non-leftmost panes — borders place `│` at col 0 of every line, and cross-column content satisfies the wrong row otherwise. Trailing border `│` chars are stripped automatically.
- **`countSelectedRows(screen, label)`** — count rows in the pane's slice that begin with `> `. Used by visual-yank tests.

### Patterns

**`describe + before + screen capture`** — for read-only observation tests grouped by initial state. Capture screen once, run many `test()` blocks against it. Saves ~5 s per launch.
- Currently used: B1+B2+B6, D1+D2, E1+E1b+E2, G1+G2+G3+G4.

```js
describe('group', () => {
  let screen
  before(async () => {
    const s = await launchGhRv()
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

- **Forgot to rebuild binary**: `go build -o gh-rv .` (NOT `go build ./...`). The `pretest` hook of `pnpm test` does this automatically; targeted `node --test` does not.
- **`^>` regex on raw screen**: borders place `│` at col 0 of each row. Use `paneText(screen, label)` slice instead.
- **Long substring assertions**: column wrap will split words across rows. Shorten or normalize before checking.
- **bubbletea startup ~1 s blank**: first `s.text()` after launch can be empty. Always use `waitReady` before reading.
- **tuistory cannot reliably emit CSI Z**: shift-tab tests are skipped (C2). Document inline and verify by inspection.
- **Do not re-introduce `lipgloss.Border()`**: we render boxes manually via `renderPaneBox` in `app.go` because lipgloss cannot produce the title-bar divider. Touch only `renderPaneBox` for box visual changes.
- **Tabs in Diff content**: split mode requires `expandTabs(line, 4)` before wrap/pad. Without it, terminal-side tab expansion shifts `│`.
- **CJK / wide chars**: padding is now ANSI-aware (uses `lipgloss.Width`) but the comment-pane wrap math still uses `utf8.RuneCountInString`. Wide-char content will mis-align in Comments. Add `go-runewidth` consistently if a real test fixture needs it.
- **Diff wrap is always on**: there is no toggle. A buffer line that exceeds the cell width is split into multiple display rows with cursor / `◆` rendered only on the first row, and continuation rows indented past the diff marker. `DiffViewport.Top` is a buffer-line index; `diffViewportHeight()` is in display rows; `displayRowsBetween` is the bridge.
- **Color SGR doesn't reach tuistory's `text()`**: ghostty parses ANSI into cell state, so substring assertions stay color-agnostic. The A9 smoke test guards against raw `\x1b` bytes leaking into the rendered text — keep it in place when adding new renderers.
- **Chroma case quirk**: registry key `rpgle` resolves to a Style whose `Name` is `RPGLE`. `theme.Resolve` canonicalizes on the registry key; do not rely on `Style.Name` matching the user-supplied name.
- **Bubbletea v1 has no color profile option**: `lipgloss.SetColorProfile(termenv.Ascii)` and `SetHasDarkBackground(true)` must be called BEFORE `tea.NewProgram`. `cmd/root.go` does this; new entry points must replicate.
- **Chroma init is eager**: importing `github.com/alecthomas/chroma/v2/styles` parses all 74 embedded XMLs at package init; `chroma/v2/lexers` registers ~250 lexers. Combined cold-start cost is ~500ms. Don't import these from hot-path packages — the theme module is the gateway.
- **Diff syntax highlighting needs the cache**: `Model.syntaxCache` is the only thing keeping diff rendering snappy. Don't accidentally drop the pointer when restructuring `Model` (e.g. via `NewModel` rewrites) — without the cache, e2e starts intermittently failing on `waitReady`.
- **`Model` has 3 reference-typed caches that must propagate across Updates**: `syntaxCache`, `patchLinesC.cache`, `rowCache`. They are pointer / map types so Bubbletea's value-copied `Model` shares the same backing storage. If you ever switch `Model` to a struct embedding pattern or change `NewModel` to copy these, regression is silent — caches just miss on every render, lag returns.
- **`s.press` / `s.type` are auto-settled in tests**: `launchGhRv` wraps the tuistory session so a 120ms wait fires after every keystroke. Don't add manual `await sleep(N)` after presses; if a test still races, the right fix is `await s.waitForText(<expected post-state>)` rather than upping the global settle.
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
- Never push to main / master; never force-push.
- Subject ≤ 70 chars; body explains the why if non-obvious.
- Trailer: `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.
- Stage by name when feasible. `git add -A` is allowed for the initial commit and when `.gitignore` is known to be correct, but not for arbitrary staging.
