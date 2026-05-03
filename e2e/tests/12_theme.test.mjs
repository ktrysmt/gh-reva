// Category L — color theme switching via --theme / --no-color / --list-themes.
//
// L1+L2: theme name resolves and the UI renders without crashing.
// L3:    unknown theme name fails fast with a clear message.
// L4:    --list-themes prints the builtin + chroma names and exits 0.
// L5:    --no-color renders the same text content (color is opt-in).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'

import { launchGhRv, waitReady, quit, paneText, BIN, FIXTURE_DEFAULT } from '../helpers/launch.mjs'

// assertCursorSurvives verifies the Files-pane cursor row (`> `) is reachable
// from the captured screen even when the renderer wraps content in SGR
// sequences. tuistory parses SGR through ghostty so a passing assertion
// proves the cell-state path is intact under the given theme; a regression
// would surface here before the user sees garbled output.
function assertCursorSurvives (screen, themeLabel) {
  const filesText = paneText(screen, 'Files')
  assert.ok(filesText, `Files pane slice empty under ${themeLabel}`)
  assert.match(filesText, /^> /m, `Files cursor "> " missing under ${themeLabel}`)
}

test('L1: --theme=dracula renders all four panes', async () => {
  const s = await launchGhRv({ args: ['--theme', 'dracula'] })
  await waitReady(s)
  const screen = await s.text()
  for (const label of ['Files', 'Commits', 'Diff', 'Comments']) {
    assert.ok(screen.includes(label), `pane label "${label}" missing under --theme=dracula`)
  }
  assert.ok(screen.includes('src/greeting.go'), 'fixture file missing under --theme=dracula')
  assertCursorSurvives(screen, '--theme=dracula')
  await quit(s)
})

test('L2: --theme=tokyonight-night renders all four panes', async () => {
  const s = await launchGhRv({ args: ['--theme', 'tokyonight-night'] })
  await waitReady(s)
  const screen = await s.text()
  for (const label of ['Files', 'Commits', 'Diff', 'Comments']) {
    assert.ok(screen.includes(label), `pane label "${label}" missing under --theme=tokyonight-night`)
  }
  assertCursorSurvives(screen, '--theme=tokyonight-night')
  await quit(s)
})

test('L3: unknown theme exits non-zero with helpful message', () => {
  const result = spawnSync(BIN, ['--fixture', FIXTURE_DEFAULT, '--theme', 'does-not-exist-xyz'], {
    encoding: 'utf8',
    timeout: 3000,
  })
  assert.notEqual(result.status, 0, 'expected non-zero exit for unknown theme')
  assert.match(result.stderr, /unknown theme/i)
})

test('L4: --list-themes prints builtin + chroma names and exits 0', () => {
  const result = spawnSync(BIN, ['--list-themes'], {
    encoding: 'utf8',
    timeout: 3000,
  })
  assert.equal(result.status, 0, `--list-themes should exit 0, got ${result.status}; stderr=${result.stderr}`)
  for (const want of ['builtin-dark', 'dracula', 'monokai', 'github-dark', 'nord']) {
    assert.ok(result.stdout.includes(want), `--list-themes output should include ${want}; got:\n${result.stdout}`)
  }
})

test('L5: --no-color still renders all four panes', async () => {
  const s = await launchGhRv({ args: ['--no-color'] })
  await waitReady(s)
  const screen = await s.text()
  for (const label of ['Files', 'Commits', 'Diff', 'Comments']) {
    assert.ok(screen.includes(label), `pane label "${label}" missing under --no-color`)
  }
  assertCursorSurvives(screen, '--no-color')
  await quit(s)
})
