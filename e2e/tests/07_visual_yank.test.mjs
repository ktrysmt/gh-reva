// Category H — Visual mode + yank.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'

import { launchGhRv, waitReady, quit, countSelectedRows } from '../helpers/launch.mjs'

function pbpaste () {
  const r = spawnSync('pbpaste', { encoding: 'utf8' })
  if (r.status !== 0) throw new Error('pbpaste failed')
  return r.stdout
}

function clipboardAvailable () {
  return process.platform === 'darwin'
}

test('H1: v shows the -- VISUAL -- indicator', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.type('v')
  const screen = await s.text()
  assert.match(screen, /-- VISUAL --/, 'visual mode indicator should be visible')
  await s.press('esc')
  await quit(s)
})

test('H2: visual mode in Files is linewise (j extends selection by line)', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.type('v')
  await s.type('j')
  const screen = await s.text()
  assert.match(screen, /-- VISUAL --/)
  assert.equal(countSelectedRows(screen, 'Files'), 2, 'anchor + cursor rows should both render with "> "')
  await s.press('esc')
  await quit(s)
})

test('H3: visual mode in Diff renders linewise selection', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  await s.type('v')
  await s.type('j'); await s.type('j')
  const screen = await s.text()
  assert.match(screen, /-- VISUAL --/)
  assert.equal(countSelectedRows(screen, 'Diff'), 3, 'anchor + 2 cursor moves should mark 3 rows')
  await s.press('esc')
  await quit(s)
})

test('H4: j/k extends linewise selection', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.type('v')
  await s.type('j'); await s.type('j')
  let screen = await s.text()
  assert.equal(countSelectedRows(screen, 'Files'), 3, 'after v+j+j → 3 rows selected')
  await s.type('k')
  screen = await s.text()
  assert.equal(countSelectedRows(screen, 'Files'), 2, 'after k → 2 rows selected')
  await s.press('esc')
  await quit(s)
})

test('H5: visual mode in Diff extends with j/k', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')
  await s.type('v')
  await s.type('j'); await s.type('j'); await s.type('j')
  let screen = await s.text()
  assert.equal(countSelectedRows(screen, 'Diff'), 4, 'v+j×3 → 4 rows in Diff')
  await s.type('k')
  screen = await s.text()
  assert.equal(countSelectedRows(screen, 'Diff'), 3, 'k → 3 rows in Diff')
  await s.press('esc')
  await quit(s)
})

test('H6: y copies the Files cursor row to the system clipboard', { skip: clipboardAvailable() ? false : 'pbpaste only available on macOS in this test' }, async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.type('v')
  await s.type('y')
  const screen = await s.text()
  assert.doesNotMatch(screen, /-- VISUAL --/, 'visual mode should exit after y')
  // Phase 1 yank shape for Files: just the file path.
  const clip = pbpaste()
  assert.ok(clip.includes('src/greeting.go'), `expected path in clipboard, got: ${JSON.stringify(clip)}`)
  await quit(s)
})

test('H7: Esc cancels visual mode without copying', { skip: clipboardAvailable() ? false : 'pbpaste only available on macOS' }, async () => {
  const s = await launchGhRv()
  await waitReady(s)
  // Pre-set clipboard to a sentinel so we can detect a stray copy.
  spawnSync('pbcopy', { input: 'SENTINEL', encoding: 'utf8' })
  await s.type('v')
  await s.press('esc')
  const screen = await s.text()
  assert.doesNotMatch(screen, /-- VISUAL --/, 'visual mode should exit after Esc')
  const clip = pbpaste()
  assert.equal(clip, 'SENTINEL', 'Esc must not have written to the clipboard')
  await quit(s)
})

test('H8: yanked Commits row is shaped as `<sha> <subject>`', { skip: clipboardAvailable() ? false : 'pbpaste only available on macOS' }, async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab')          // focus Commits
  await s.type('v')
  await s.type('y')
  const clip = pbpaste()
  assert.match(clip, /aaa1111\s+Add greeting\.go skeleton/, `expected "<sha> <subject>", got ${JSON.stringify(clip)}`)
  await quit(s)
})
