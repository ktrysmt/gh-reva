// Category S — Bottom status bar (`internal/tui/statusbar.go`).
//
// Contract (CLAUDE.md §4 #6):
//   - 1 row at the bottom always reserved once the PR is loaded.
//   - Context portion shows pane-specific keymap hints; common suffix
//     (`tab:focus J/K:file ?:help q:quit`) is right-flushed.
//   - Visual / modal / help states replace context AND drop suffix.
//   - On narrow terminals the suffix is dropped entirely (no half-truncation).

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchReva, waitReady, quit } from '../helpers/launch.mjs'

// statusBarRow returns the last non-empty row of the screen, which is the
// status bar after PR load. Trailing whitespace is stripped so substring
// asserts don't have to worry about right padding.
function statusBarRow (screen) {
  const lines = screen.split('\n')
  for (let i = lines.length - 1; i >= 0; i--) {
    const trimmed = lines[i].replace(/\s+$/, '')
    if (trimmed !== '') return trimmed
  }
  return ''
}

test('S1: Files (flat) status bar shows context + common suffix', async () => {
  const s = await launchReva()
  await waitReady(s)
  const row = statusBarRow(await s.text())
  assert.match(row, /j\/k:move/)
  assert.match(row, /space:zoom/)
  assert.match(row, /t:tree/)
  // Common suffix
  assert.match(row, /tab:focus/)
  assert.match(row, /R:submit/)
  assert.match(row, /\?:help/)
  assert.match(row, /q:quit/)
  await quit(s)
})

test('S2: Files (tree) adds enter:fold to the context hint', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('t')
  const row = statusBarRow(await s.text())
  assert.match(row, /enter:fold/)
  assert.match(row, /t:tree/)
  await quit(s)
})

test('S3: Commits pane drops t:tree, keeps j/k + space', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab')
  const row = statusBarRow(await s.text())
  assert.match(row, /j\/k:move/)
  assert.match(row, /space:zoom/)
  assert.ok(!/t:tree/.test(row), `t:tree must not appear in Commits status bar; got: ${row}`)
  assert.ok(!/enter:fold/.test(row), `enter:fold is Files-tree only; got: ${row}`)
  await quit(s)
})

test('S4: Diff pane shows j/k, H/M/L, gg/G, space:split, enter:comment', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab')
  await s.press('tab')
  const row = statusBarRow(await s.text())
  assert.match(row, /j\/k:move/)
  assert.match(row, /H\/M\/L:viewport/)
  assert.match(row, /gg\/G:top\/bottom/)
  assert.match(row, /space:split/)
  assert.match(row, /enter:comment/)
  await quit(s)
})

test('S5: Comments pane is the same minimal shape as Commits, plus enter:reply', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab')
  await s.press('tab')
  await s.press('tab')
  const row = statusBarRow(await s.text())
  assert.match(row, /j\/k:move/)
  assert.match(row, /space:zoom/)
  assert.match(row, /enter:reply/)
  assert.ok(!/H\/M\/L/.test(row), `Diff-only hints must not leak into Comments status bar; got: ${row}`)
  await quit(s)
})

test('S6: visual mode replaces bar with -- VISUAL -- y/esc hint', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('v')
  const row = statusBarRow(await s.text())
  assert.match(row, /-- VISUAL --/)
  assert.match(row, /y:yank/)
  assert.match(row, /esc\/ctrl\+c:cancel/)
  // Common suffix is dropped while visual is active.
  assert.ok(!/tab:focus/.test(row), `common suffix must not coexist with visual hint; got: ${row}`)
  await s.press('esc')
  await quit(s)
})

test('S7: zoom modal replaces bar with close hint (ctrl+c also closes)', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('space')
  const row = statusBarRow(await s.text())
  assert.match(row, /space\/esc\/q\/ctrl\+c:close/)
  assert.ok(!/ctrl\+c:quit/.test(row), `ctrl+c is now a close gesture inside the modal; got: ${row}`)
  assert.ok(!/tab:focus/.test(row), `common suffix must not coexist with modal hint; got: ${row}`)
  await quit(s)
})

test('S8: help modal replaces bar with close hint', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('?')
  const row = statusBarRow(await s.text())
  assert.match(row, /\?\/esc\/q:close/)
  await quit(s)
})

test('S9: narrow terminal drops the common suffix entirely', async () => {
  const s = await launchReva({ cols: 60 })
  await waitReady(s)
  const row = statusBarRow(await s.text())
  assert.match(row, /j\/k:move/)
  // Suffix items must vanish — no half-truncated hint, no q:quit visible.
  assert.ok(!/q:quit/.test(row), `common suffix should be dropped on narrow terminal; got: ${row}`)
  assert.ok(!/tab:focus/.test(row), `common suffix should be dropped on narrow terminal; got: ${row}`)
  await quit(s)
})

test('S10: status bar is absent during the loading splash', async () => {
  const s = await launchReva({ args: ['--slow-load', '500ms'] })
  // Sample mid-load — splash should be on screen, status bar suppressed.
  // Wait briefly so the spinner has time to render at least one frame
  // without waiting for ready.
  await new Promise(r => setTimeout(r, 200))
  const screen = await s.text()
  assert.ok(/Loading PR/.test(screen), `expected loading splash; got tail:\n${screen.split('\n').slice(-6).join('\n')}`)
  // The status bar shape must not appear above the splash:
  // pick a token that only the post-load bar carries.
  assert.ok(!/tab:focus/.test(screen), `status bar must be suppressed during loading; got tail:\n${screen.split('\n').slice(-6).join('\n')}`)
  // Drain to ready before quitting so the binary exits cleanly.
  await waitReady(s)
  await quit(s)
})
