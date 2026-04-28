// Category G — Comments pane.

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchGhRv, waitReady, quit } from '../helpers/launch.mjs'

test('G1: only selectedFile comments are shown', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  // selectedFile auto = src/greeting.go on startup.
  const screen = await s.text()
  assert.ok(screen.includes('Consider extracting this'), 'greeting.go thread root should be visible')
  assert.ok(screen.includes('Good point, will refactor'), 'greeting.go reply should be visible')
  // greeting_test.go's comment must NOT appear yet:
  assert.ok(!screen.includes('Add a test for the empty input case'), 'other-file comments must not leak')
  await quit(s)
})

test('G2: thread structure rendered with indentation (reply indented vs root)', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  const screen = await s.text()
  const lines = screen.split('\n')
  const rootIdx = lines.findIndex(l => l.includes('Consider extracting this'))
  const replyIdx = lines.findIndex(l => l.includes('Good point, will refactor'))
  assert.ok(rootIdx >= 0 && replyIdx >= 0)
  const indent = (l) => l.match(/^\s*/)[0].length
  assert.ok(indent(lines[replyIdx]) > indent(lines[rootIdx]), 'reply should be more indented than its root')
  await quit(s)
})

test('G3: ascending time order (oldest at top)', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  const screen = await s.text()
  // Root posted at 13:00, reply at 14:30 → root must appear before reply.
  const rootIdx = screen.indexOf('Consider extracting this')
  const replyIdx = screen.indexOf('Good point, will refactor')
  assert.ok(rootIdx >= 0 && replyIdx >= 0)
  assert.ok(rootIdx < replyIdx, 'root must precede reply (chronological)')
  await quit(s)
})

test('G4: HEAD view hides outdated comments', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  const screen = await s.text()
  // Fixture comment id=1003 (dave) is outdated against HEAD.
  assert.ok(!screen.includes('This was the old struct definition'), 'outdated comment must not show at HEAD')
  await quit(s)
})

test('G5: single-commit view exposes comments anchored to that commit', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab')          // focus Commits
  await s.press('enter')         // select aaa1111
  const screen = await s.text()
  // Comment 1003 is anchored to aaa1111 → should now appear.
  assert.ok(screen.includes('This was the old struct definition'), 'single-commit view should expose anchored comment')
  await quit(s)
})

test('G6: j/k is linear across the tree (root → reply → next root)', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab'); await s.press('tab')   // focus Comments
  let screen = await s.text()
  assert.match(screen, /^>[^\n]*Consider extracting this|^>[^\n]*carol/m, 'cursor should start on first thread root')
  await s.type('j')
  screen = await s.text()
  // After one j, cursor should be on the reply (alice's "Good point").
  assert.match(screen, /^>[^\n]*Good point|^>[^\n]*alice/m, 'after j → cursor should move to reply')
  await quit(s)
})

test('G7: Comments cursor movement auto-scrolls Diff to the comment line', { skip: 'TODO: needs Diff pane to expose a deterministic anchor for the comment line; revisit after diff renderer lands' }, async () => {})

test('G8: h/l folds / unfolds the thread under the cursor', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab'); await s.press('tab')   // focus Comments
  // Initial state: thread expanded → reply visible.
  let screen = await s.text()
  assert.ok(screen.includes('Good point, will refactor'), 'reply should be visible (expanded by default)')
  await s.type('h')             // collapse
  screen = await s.text()
  assert.ok(!screen.includes('Good point, will refactor'), 'reply should be hidden after h (collapse)')
  await s.type('l')             // expand
  screen = await s.text()
  assert.ok(screen.includes('Good point, will refactor'), 'reply should reappear after l (expand)')
  await quit(s)
})

test('G9: Enter is a no-op in Phase 1', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab'); await s.press('tab')   // focus Comments
  const before = await s.text()
  await s.press('enter')
  const after = await s.text()
  assert.equal(before, after, 'Enter on Comments must be a no-op in Phase 1')
  await quit(s)
})

test('G10: Backspace returns focus to Diff', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab'); await s.press('tab')   // focus Comments
  await s.press('backspace')
  const screen = await s.text()
  assert.match(screen, /▶ Diff/, 'Backspace from Comments should land on Diff')
  await quit(s)
})
