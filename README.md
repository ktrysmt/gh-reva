# gh-rv

PR review TUI distributed as a `gh` CLI extension.

`gh-rv` is a four-pane terminal viewer for GitHub Pull Requests with a focus
on per-file review flow: pin a file, then walk only the commits that touch it
without losing your place in the diff or comments.

## Status

Phase 1 (read-only viewer) lands the four-pane layout, keymap, and file-scoped
commit filter. Phase 1.5 wires up the real GitHub REST API. Editor-driven
comment posting and resolved-thread toggling stay scoped to Phase 2.

Detailed design notes live in
`results/00020_gh拡張CLI-PR-Review-TUI設計/` (機能要求整理 + 設計詳細).

## Install

```sh
gh extension install ktrysmt/gh-rv
```

The first run downloads the precompiled binary that matches your OS and
architecture from the latest GitHub release.

## Usage

```sh
gh rv <PR-number>            # explicit PR in the current repo
gh rv <PR-URL>               # any PR by URL
gh rv                        # auto-detect the PR for the current branch
```

The TUI requires a real terminal (TTY); piping output is not supported.

## Layout

```
+-------------+----------------+----------------+
| [1] Files   |                |                |
+-------------+   [3] Diff     |  [4] Comments  |
| [2] Commits |                |                |
+-------------+----------------+----------------+
```

The active pane is marked with `▶ <Pane>`. The cursor row in every pane is
prefixed with `> `. Visual selection extends `> ` across the selected range.

## Keymap

| Key | Action | Active panes |
| --- | --- | --- |
| `tab` / `shift-tab` | Cycle panes Files → Commits → Diff → Comments | All |
| `Enter` | Drill in (Files → Commits → Diff → Comments) | All |
| `Backspace` | Drill out | All |
| `j` / `k` | Move cursor up / down | All |
| `h` / `l` | Diff: horizontal scroll • Comments: fold/unfold | Diff, Comments |
| `w` / `b` / `e` | Word-wise horizontal move | Diff |
| `H` / `M` / `L` | Top / middle / bottom of viewport | Diff |
| `gg` / `G` | Buffer start / end | Diff |
| `Ctrl-d` / `Ctrl-u` | Half-page down / up | Diff |
| `Ctrl-f` / `Ctrl-b` | Full-page down / up | Diff |
| `<space>` | Files: toggle commit-history filter • Diff: split⇄unified | Files, Diff |
| `t` | Files: toggle flat ⇄ tree rendering | Files |
| `v` / `y` / `Esc` | Visual mode + yank to clipboard / cancel | All |
| `q` / `Ctrl-C` | Quit | All |

In tree mode, `Enter` on a directory row folds / unfolds that subtree and the
cursor stays on the directory.

### Visual yank shapes

- Files: file path(s), newline-separated
- Commits: `<sha> <subject>` per row
- Comments: `<user> @ <date>\n<body>` per row
- Diff: raw line(s) of the patch buffer

## Per-file commit filter

`<space>` on a Files row pins it, marks the row with `*`, and rewrites the
Commits pane to show only the commits that touched that path. The Commits
title becomes `Commits (filter: <path>)`. Press `<space>` again on the same
row to clear the filter. Each commit row also gains an `[A] / [M] / [D] / [R]`
annotation describing how it changed the selected file.

## Comments view rules

- Whole-PR view: shows only active threads (non-outdated, anchored to HEAD)
- Single-commit view: shows comments anchored to that commit (including ones
  that became outdated against HEAD), tagged with `[outdated]` when relevant
- Threads render with replies indented under the root; `h` / `l` fold and
  unfold the thread under the cursor

## Development

```sh
go mod tidy
go build ./...
go vet ./...
go run . --fixture testdata/sample-pr.json
```

### End-to-end tests

```sh
cd e2e
pnpm install        # first run only
pnpm test           # runs `go build` then node --test against tuistory
pnpm run test:smoke # smoke subset
```

The runner relies on `tuistory` plus `--test-force-exit` to handle the
bubbletea PTY. If a hung child slows you down, `pkill -f 'gh-rv --fixture'`
clears it.

### Test-only flags

| Flag | Purpose |
| --- | --- |
| `--fixture <path>` | Load PR data from a JSON fixture (skips the gh API) |
| `--simulate-error <kind>` | Inject `unauth` / `not_found` / `rate_limit` errors |
| `--slow-load <duration>` | Inject a per-call delay so the loading spinner is observable |
| `--diff-height <int>` | Pin the Diff viewport height (used by F6 viewport assertions) |

The simulate-error, slow-load, and diff-height flags are hidden from `--help`
and only intended for the E2E suite.

### Stress fixture

A larger fixture (60 commits, 120 files) lives at `testdata/large-pr.json`.
Regenerate it with:

```sh
go run testdata/gen_large_fixture.go testdata/large-pr.json
```

The TUI shows a Braille spinner with a stage label (`metadata`, `commits`,
`files`, `comments`, `diffs`) while loading.

## Release process

Releases are produced by `goreleaser` from a `v*` tag pushed to the default
branch (see `.github/workflows/release.yml`).

Sanity-check locally:

```sh
goreleaser release --snapshot --clean
```

This produces `dist/` with per-OS/arch binaries named `gh-rv_<os>_<arch>`,
`checksums.txt`, and a snapshot manifest. The same name template is what
`gh extension install` consumes from a real release.

## License

MIT
