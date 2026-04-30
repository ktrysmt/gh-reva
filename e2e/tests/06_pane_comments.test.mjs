// Category G — Comments pane.

import { test, describe, before } from 'node:test'
import assert from 'node:assert/strict'
import path from 'node:path'

import { launchGhRv, waitReady, quit, REPO_ROOT, paneText } from '../helpers/launch.mjs'

// G1+G2+G3+G4 share the initial Comments state for greeting.go.
describe('G1+G2+G3+G4: Comments pane initial render (greeting.go selected)', () => {
  let screen, cms
  before(async () => {
    const s = await launchGhRv()
    await waitReady(s)
    screen = await s.text()
    cms = paneText(screen, 'Comments')
    await quit(s)
  })

  test('G1: only selectedFile comments are shown', () => {
    assert.ok(cms.includes('Consider extracting'), `greeting.go thread root should be visible:\n${cms}`)
    assert.ok(cms.includes('Good point'), `greeting.go reply should be visible:\n${cms}`)
    assert.ok(!cms.includes('Add a test for the empty'), 'other-file comments must not leak')
  })

  test('G2: thread structure rendered with indentation (reply indented vs root)', () => {
    const cmsLines = cms.split('\n')
    const rootIdx = cmsLines.findIndex(l => l.includes('Consider extracting'))
    const replyIdx = cmsLines.findIndex(l => l.includes('Good point'))
    assert.ok(rootIdx >= 0 && replyIdx >= 0, `both root and reply should appear:\n${cms}`)
    const indentOf = (l) => l.match(/^\s*/)[0].length
    assert.ok(indentOf(cmsLines[replyIdx]) > indentOf(cmsLines[rootIdx]), 'reply should be more indented than its root')
  })

  test('G3: ascending time order (oldest at top)', () => {
    // Root posted at 13:00, reply at 14:30 → root must appear before reply.
    const rootIdx = cms.indexOf('Consider extracting')
    const replyIdx = cms.indexOf('Good point')
    assert.ok(rootIdx >= 0 && replyIdx >= 0)
    assert.ok(rootIdx < replyIdx, 'root must precede reply (chronological)')
  })

  test('G4: HEAD view hides outdated comments', () => {
    // Fixture comment id=1003 (dave) is outdated against HEAD.
    assert.ok(!screen.includes('This was the old struct'), 'outdated comment must not show at HEAD')
  })
})

test('G5: single-commit view exposes comments anchored to that commit', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  // Drill into aaa1111 explicitly: tab + j + k primes auto-select on aaa1111
  // (Enter alone preserves the PR-wide view; j/k drives commit-pick).
  await s.press('tab')          // focus Commits
  await s.type('j')             // cursor → bbb2222 (auto-select SingleCommit bbb2222)
  await s.type('k')             // cursor → aaa1111 (auto-select SingleCommit aaa1111)
  await s.press('enter')         // → Diff focus, SelectedRange = aaa1111
  const cms = paneText(await s.text(), 'Comments')
  // Comment 1003 (dave) is anchored to aaa1111 — its header should now show
  // in the Comments column. The body wraps tightly (the "[outdated]" tag
  // pushes the header past 40 cols), so we anchor on the user identifier.
  assert.ok(cms.includes('dave:'), `single-commit view should expose dave's anchored comment; Comments slice:\n${cms}`)
  assert.ok(cms.includes('[outdated]'), `comment 1003 carries the [outdated] tag in single-commit view; slice:\n${cms}`)
  await quit(s)
})

test('G6: j/k is linear across the tree (root → reply → next root)', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab'); await s.press('tab')   // focus Comments
  let cms = paneText(await s.text(), 'Comments')
  assert.match(cms, /^>[^\n]*carol\b/m, 'cursor should start on the carol root')
  await s.type('j')
  cms = paneText(await s.text(), 'Comments')
  // After one j, cursor should be on the reply (alice).
  assert.match(cms, /^>[^\n]*alice\b/m, 'after j → cursor should move to alice reply')
  await quit(s)
})

test('G7: Comments cursor movement auto-scrolls Diff to the comment line', { skip: 'TODO: needs Diff pane to expose a deterministic anchor for the comment line; revisit after diff renderer lands' }, async () => {})

test('G8: h/l folds / unfolds the thread under the cursor', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab'); await s.press('tab')   // focus Comments
  // Initial state: thread expanded → reply visible.
  let screen = await s.text()
  assert.ok(screen.includes('Good point'), 'reply should be visible (expanded by default)')
  await s.type('h')             // collapse
  screen = await s.text()
  assert.ok(!screen.includes('Good point'), 'reply should be hidden after h (collapse)')
  await s.type('l')             // expand
  screen = await s.text()
  assert.ok(screen.includes('Good point'), 'reply should reappear after l (expand)')
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

test('G11: long comment body wraps within the Comments column', async () => {
  // Default cols=160 — the Comments column is narrow enough that the long
  // body wraps onto multiple rows. We assert structural wrap (head and tail
  // on different rows within the column) without pinning the exact indent,
  // which depends on column width.
  const s = await launchGhRv({
    fixture: path.join(REPO_ROOT, 'testdata', 'wrap-pr.json'),
  })
  await waitReady(s)
  const cms = paneText(await s.text(), 'Comments')
  const cmsLines = cms.split('\n')
  const headIdx = cmsLines.findIndex(l => l.includes('Consider extracting'))
  assert.ok(headIdx >= 0, `comment header line should appear in Comments column:\n${cms}`)
  // "evolve" is the unique tail-marker word in the wrap-pr fixture body.
  const tailIdx = cmsLines.findIndex(l => l.includes('evolve'))
  assert.ok(tailIdx > headIdx, `tail must wrap onto a later row (head=${headIdx}, tail=${tailIdx})`)
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
