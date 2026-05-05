# gh-reva e2e tests

End-to-end tests for the `gh-reva` TUI using
[tuistory](https://github.com/remorses/tuistory) — Playwright for TUIs.

## Run

```sh
# 1. install Node deps (one-time)
cd gh-reva/e2e
pnpm install

# 2. build the gh-reva binary and run all tests
pnpm test

# or only the smoke test
pnpm run test:smoke
```

`pnpm test` rebuilds the binary via the `pretest` hook (`go build -o gh-reva ..`).

The tests run with `node --test --test-concurrency=1` so panes don't
race for the same fixture / terminal.

> The repo standardises on **pnpm** for the e2e workspace. `npm` will also
> work, but pnpm is the recommended default.

## How it works

- A small `--fixture <path>` flag in `cmd/root.go` swaps the real `gh` API
  client for a JSON-fed `FixtureClient`.
- `testdata/sample-pr.json` ships a representative PR with 3 commits, 5 files,
  4 review comments (including one outdated thread and one root+reply pair).
- Each test launches `gh-reva --fixture testdata/sample-pr.json`, waits for the
  UI to render, drives keys with `tuistory`, and asserts on `session.text()`.

## UI text conventions (test contract)

The Phase 1 UI is **plain-text observable**. The implementation MUST emit
these stable markers so e2e tests stay deterministic:

| Marker / format | Meaning |
| --- | --- |
| `▶ <Pane>` at the start of a pane title line | the pane is the active focus (exactly one per screen) |
| `> ` prefix on a row inside a pane | that row is the cursor row of the pane |
| `*` after the cursor column in Files (e.g. `> *M src/foo.go`) | this file is the current `commitFilterFile` |
| `Commits (filter: <path>)` as the Commits pane title | filter mode is on |
| `[A]` / `[M]` / `[D]` / `[R]` before a commit short_sha | the selectedFile was added / modified / deleted / renamed in that commit |
| `Diff: <path> [split]` or `[unified]` in the Diff pane title | current file + view mode |
| `[outdated]` tag next to a comment header | the comment is outdated against HEAD |
| `-- VISUAL --` somewhere on screen | visual mode is active |

The Files pane row format (Phase 1, flat list):

```
> *M src/greeting.go (2)        <-- cursor + filter + modified + 2 comments
   A src/greeting_test.go (1)
   M src/main.go
   A docs/api.md
   M go.mod
```

The Commits pane row format with a selected file:

```
> [A] aaa1111 Add greeting.go skeleton
  [M] bbb2222 Implement Hello function
      ccc3333 Add tests and docs
```

These conventions are **enforced by tests** under `e2e/tests/`. Changing them
requires updating the assertions.

## Test point coverage map

The matrix below is the **comprehensive Phase 1 test point list** used as the
source of truth for what each test file exercises. Items currently blocked on
implementation are wired as `t.skip` with a TODO note inside the relevant file
so they surface in the test report.

| File | Category | Points covered |
| --- | --- | --- |
| `00_smoke.test.mjs`        | Smoke / sanity                   | A1, A2, A3, A7, A8, B1, B2 |
| `01_layout.test.mjs`       | Layout & initial render (B)      | B1–B5 |
| `02_navigation.test.mjs`   | Pane traversal (C)               | C1–C7 |
| `03_pane_files.test.mjs`   | Files pane (D)                   | D1–D8 |
| `04_pane_commits.test.mjs` | Commits pane (E)                 | E1–E6 |
| `05_pane_diff.test.mjs`    | Diff pane (F)                    | F1–F9 |
| `06_pane_comments.test.mjs`| Comments pane (G)                | G1–G10 |
| `07_visual_yank.test.mjs`  | Visual mode + yank (H)           | H1–H8 |
| `08_sync.test.mjs`         | Inter-pane synchronisation (I)   | I1–I7 |
| `09_errors.test.mjs`       | Errors / load states / quit (J,K)| J1–J3, K1–K3 |

### Categories (from the Phase 1 spec)

#### A. Startup / args / quit
- A1 launch with no args (resolves current branch PR)
- A2 launch with `<PR-number>`
- A3 launch with `<PR-URL>`
- A4 invalid arg → exit non-zero
- A5 unauthenticated → exit non-zero
- A6 PR not found / no permission → exit non-zero
- A7 `q` quits
- A8 `Ctrl-C` quits

#### B. Layout & initial render
- B1 four panes (Files / Commits / Diff / Comments) visible
- B2 initial focus is Files
- B3 only the focused pane has the highlighted border
- B4 terminal resize re-flows the layout
- B5 narrow terminal (<100 cols) auto-falls back to unified Diff

#### C. Pane traversal
- C1 `tab` cycles `Files → Commits → Diff → Comments → Files`
- C2 `shift-tab` cycles in reverse
- C3 `Enter` drills 1 → 2 → 3 → 4
- C4 `Backspace` returns 4 → 3 → 2 → 1; no-op at Files
- C5 Backspace mash from Comments lands at Files
- C6 each pane preserves its selection across Backspace
- C7 numeric keys (1–4) **do not** jump panes (intentionally rejected)

#### D. Files pane
- D1 changed files are rendered as a directory tree
- D2 each file shows status (A/M/D/R) + comment count
- D3 `j/k` moves the cursor
- D4 `h/l` is **not** bound (Files only uses j/k)
- D5 `Enter` on a directory toggles expand/collapse, focus stays on Files
- D6 `Enter` on a file sets `selectedFile`, refreshes Diff & Comments, focus → Commits
- D7 `<space>` toggles `commitFilterFile`
- D8 the filter target file shows a marker (`*`)

#### E. Commits pane
- E1 PR commits are listed chronologically
- E2 each commit shows whether `selectedFile` was changed (A/M/D/R/none)
- E3 `j/k` moves the cursor
- E4 in filter mode, only commits that touch `commitFilterFile` are listed
- E5 the filter state shows in the pane title (e.g. `Commits (filter: src/greeting.go)`)
- E6 `Enter` drills to single-commit Diff, focus → Diff, Comments updates

#### F. Diff pane
- F1 split view by default
- F2 `<space>` toggles split ⇄ unified
- F3 narrow terminals fall back to unified
- F4 vertical: `j/k`, `Ctrl-d/u`, `Ctrl-f/b`, `gg`, `G`
- F5 horizontal: `h/l` (1 char), `w/b/e` (word)
- F6 viewport jumps: `H/M/L`
- F7 `Enter` on a line with a comment focuses Comments and selects that thread
- F8 `Enter` on a line without a comment is a no-op (Phase 1 read-only)
- F9 split/unified choice persists across file changes

#### G. Comments pane
- G1 only `selectedFile`'s comments are shown
- G2 thread structure rendered with indentation
- G3 ascending time order (oldest at top)
- G4 HEAD / whole-PR view: only `position != null` (active) comments
- G5 single-commit view: comments anchored to that commit
- G6 `j/k` is linear across the tree (root → reply → next root …)
- G7 cursor movement auto-scrolls the Diff pane to the comment's line; focus stays on Comments
- G8 `h/l` folds / unfolds the thread under the cursor
- G9 `Enter` is a no-op in Phase 1 (Phase 2 wires `$EDITOR` reply)
- G10 `Backspace` returns focus to Diff

#### H. Visual mode + yank
- H1 `v` enters visual mode scoped to the focused pane
- H2 Files / Commits / Comments are linewise
- H3 Diff is charwise
- H4 `j/k` extends linewise selection
- H5 `j/k/h/l/w/b/e` extends Diff selection
- H6 `y` copies to the system clipboard and exits visual mode
- H7 `Esc` exits visual mode without copying
- H8 yanked string is pane-shaped (path / `<sha> <subject>` / body+meta / raw diff)

#### I. Inter-pane synchronisation
- I1 file selection re-renders Diff
- I2 file selection re-filters Comments
- I3 commit selection re-renders Diff
- I4 commit selection re-computes active Comments
- I5 filter toggle rebuilds the Commits pane
- I6 Comments cursor movement auto-scrolls the Diff pane
- I7 split/unified toggle re-renders Diff only

#### J. Errors / load states
- J1 spinner (or "loading PR..." stand-in) is shown while data loads
- J2 API errors (rate-limit, network) surface and exit
- J3 large PRs (>50 commits, >100 files) stay responsive

#### K. Help / quit
- K1 `q` quits cleanly
- K2 `Ctrl-C` quits cleanly
- K3 `?` opens a help overlay (Phase 2)
