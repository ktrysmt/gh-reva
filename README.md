<h1 align="center">
  <img src="assets/logo2.png" alt="gh-reva">
</h1>

<p align="center">PR review TUI distributed as a <code>gh</code> CLI extension.</p>

`gh-reva` is a four-pane terminal viewer for GitHub Pull Requests with a focus
on per-file review flow: pin a file, then walk only the commits that touch it
without losing your place in the diff or comments.

<p align="center">
  <img src="assets/screenshot.png" alt="gh-reva screenshot" width="900">
</p>

## Install

```sh
gh extension install ktrysmt/gh-reva

# upgrade
gh extension upgrade reva
```

The first run downloads the precompiled binary that matches your OS and
architecture from the latest GitHub release.

## Usage

```sh
gh reva <PR-number>            # explicit PR in the current repo
gh reva <PR-URL>               # any PR by URL
gh reva                        # auto-detect the PR for the current branch
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
| `j` / `k` | Move cursor up / down | All |
| `Shift-J` / `Shift-K` | Advance to next / previous file (focus preserved) | All |
| `H` / `M` / `L` | Top / middle / bottom of viewport | Diff |
| `gg` / `G` | Buffer start / end | Diff |
| `Ctrl-d` / `Ctrl-u` | Half-page down / up | Diff |
| `Ctrl-f` / `Ctrl-b` | Full-page down / up | Diff |
| `<space>` | Files / Commits / Comments: open zoom modal • Diff: split⇄unified | All |
| `t` | Files: toggle flat ⇄ tree rendering | Files |
| `Enter` | Diff (uncommented row): start a new pending review comment • Diff (commented row): jump into the Comments zoom modal • Comments: edit your own comment in place • Files (tree-mode dir): fold / unfold | Diff, Comments, Files |
| `r` | Reply to the thread under the cursor | Comments |
| `v` / `y` / `Esc` | Visual mode + yank to clipboard / cancel | All |
| `?` | Toggle the help modal (dismiss with `?` / `Esc` / `q` / `Ctrl-C`) | All |
| `q` / `Ctrl-C` | Quit (closes any open modal first) | All |

`tab` / `shift-tab` are the only keys that move focus between panes. `j` / `k`
in Files and Commits auto-selects the cursor row, so the Diff and Comments
panes follow the cursor live without an explicit drill-in step.

### Visual yank shapes

- Files: file path(s), newline-separated
- Commits: `<sha> <subject>` per row
- Comments: `<user> @ <date>\n<body>` per row
- Diff: raw line(s) of the patch buffer

## Per-file commit history

The Commits pane is auto-filtered by the cursor file in Files: only
commits that touched it are listed, with an `[A] / [M] / [D] / [R]`
annotation showing how each one changed the file. Move the Files cursor
to switch the filter; there is no separate pin / unpin step.

## Comments view rules

- Coupled to the Diff cursor: the pane shows only threads anchored at the
  current Diff buffer line (the rows decorated with `◆`). When the cursor is
  not on a `◆` row, the column reads `(no comment at cursor)`.
- Whole-PR view filters to active threads (non-outdated, anchored to HEAD);
  single-commit view shows comments anchored to that commit (including ones
  that became outdated against HEAD), tagged with `[outdated]` when relevant.
- Threads always render fully expanded with replies indented under the root.
  Moving the Comments cursor (`j` / `k`) auto-scrolls the Diff pane to the
  buffer line of the cursored comment.

## Configuration

`gh-reva` reads an optional `reva.toml` so you can teach it about file
extensions chroma's built-in matcher doesn't know. The first existing
path wins:

1. `--config <path>` (explicit; missing path is a hard error)
2. `$XDG_CONFIG_HOME/reva.toml`
3. `$HOME/.config/reva.toml`

If none exist, `gh-reva` runs with defaults — no config file required.

### `[syntax.extensions]`

Map a filename suffix (with leading dot) to a chroma lexer name or alias:

```toml
[syntax.extensions]
".j2" = "jinja"
".html.j2" = "html"
".tfvars" = "hcl"
```

- Longest-suffix match wins, so `".html.j2"` shadows `".j2"` for
  `templates/page.html.j2`.
- Lexer names are anything chroma resolves — run `chroma --list` (or
  see the [chroma lexers
  registry](https://github.com/alecthomas/chroma/tree/master/lexers))
  for the catalog. Common ones: `yaml`, `jinja`, `html`, `hcl`,
  `dockerfile`, `bash`, `terraform`.
- An unknown lexer name silently falls back to chroma's default
  extension matcher, so a typo in `reva.toml` doesn't strip syntax
  from every other file in the PR.

## Color theming

`gh-reva` ships with `gruvbox` as the default palette. Pass `--theme
<name>` to swap in any chroma styles registry entry (`dracula`, `nord`,
`tokyonight-night`, `monokai`, `builtin-dark`, and 70+ others). Run
`gh reva --list-themes` to see every accepted name.

```sh
gh reva --theme dracula
gh reva --no-color           # also honors NO_COLOR / CLICOLOR
GH_REVA_THEME=nord gh reva     # env var fallback when --theme is not set
```

The chosen theme drives per-token syntax foreground inside diff content,
pane chrome (border / title / status badges), the cursor accent, and the
spinner. Diff add / delete signals are deliberately theme-independent: the
row-wide bg is a uniform dark green (`#0d3b13`) / dark red (`#3b0d0d`) and
the leading `+` / `-` marker is bold bright green (`#3fb950`) / bright red
(`#f85149`) regardless of theme — so the change extent and direction read
at a glance even when a palette ships unusual diff hues. Light backgrounds
are not yet auto-detected — picking a light-theme name on a dark terminal
is allowed but may render with poor contrast.

| Flag | Purpose |
| --- | --- |
| `--theme <name>` | Pick a color palette (default: `gruvbox`) |
| `--no-color` | Disable color output. Also reads `NO_COLOR` / `CLICOLOR` |
| `--list-themes` | Print every accepted theme name on stdout and exit 0 |

## Zoom modal

In Files, Commits, and Comments, press `<space>` to open a centered
zoom modal that re-renders the active pane at a wider width. Files and
Commits get extra horizontal room for long paths and subjects;
Comments wraps at up to 80 columns so long bodies stay readable.
Inside the modal, `j` / `k` propagate to the underlying main state, so
closing the modal leaves the cursor on the same row you landed on.
Press `<space>` (or `Esc` / `q` / `Ctrl-C`) to close. The Comments
modal additionally honors `Enter` (edit your own comment) and `r`
(reply) so you don't have to close it first to act on a thread. Diff
keeps `<space>` reserved for the split ⇄ unified toggle.

## Pending review comments

Press `Enter` on a Diff line to start a new pending review comment,
or `Enter` / `r` on a Comments thread to edit your own comment / reply.
gh-reva opens the body in `$VISUAL` (or `$EDITOR`) by default and falls
back to an in-app textarea (`Ctrl-S` save, `Esc` cancel) when neither is
set. The submitted comment appends to the user's pending review on
GitHub and renders with a `[pending]` tag in the Comments column until
finalized. gh-reva does not expose a "submit review" gesture — finalize
via the GitHub web UI or `gh api graphql` once the draft is ready.

Visual mode in Diff captures a multi-line range: enter `v`, move to the
other endpoint, then `Enter` to compose against the range.

## Development

```sh
go mod tidy
go build ./...
go vet ./...
go run . --fixture testdata/sample-pr.json
```

### End-to-end tests

```sh
cd ./e2e/
pnpm install        # first run only
pnpm test           # runs `go build` then node --test against tuistory
pnpm run test:smoke # smoke subset
```

The runner relies on `tuistory` plus `--test-force-exit` to handle the
bubbletea PTY. If a hung child slows you down, `pkill -f 'gh-reva --fixture'`
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

This produces `dist/` with per-OS/arch binaries named `gh-reva_<os>_<arch>`,
`checksums.txt`, and a snapshot manifest. The same name template is what
`gh extension install` consumes from a real release.

## License

MIT
