// Category I — Inter-pane synchronisation.
//
// Comments are now coupled to the Diff cursor row (◆ rows only). Tests in
// this file that assert on Comments content first walk the Diff cursor onto
// the ◆ row for the comment they want to surface.

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchReva, waitReady, quit, paneText } from '../helpers/launch.mjs'

async function pressN (s, key, n) {
  for (let i = 0; i < n; i++) await s.press(key)
}

test('I1+I2: file selection re-renders Diff and re-filters Comments', async () => {
  const s = await launchReva()
  await waitReady(s)
  // j alone auto-selects greeting_test.go and refreshes Diff/Comments.
  await s.press('j')
  // carol's greeting_test.go comment is anchored at newLine=11 → buffer
  // index 13 in the visible patch (header×2 + hunk + 11 add lines).
  await s.press('tab'); await s.press('tab')   // Files → Diff
  await pressN(s, 'j', 13)
  const screen = await s.text()
  const cms = paneText(screen, 'Comments')
  assert.match(screen, /Diff: src\/greeting_test\.go/, 'Diff should switch to greeting_test.go')
  assert.ok(cms.includes('Add a test for the empty'), `Comments should expose greeting_test.go thread when cursor is on ◆ row; slice:\n${cms}`)
  assert.ok(!screen.includes('Consider extracting this'), 'greeting.go thread should not leak after switching files')
  await quit(s)
})

test('I3+I4: commit selection re-renders Diff and re-computes active Comments', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Drill into aaa1111 via j+j+k (Tab → All commits row → aaa1111 → bbb2222
  // → aaa1111). Tab is the only focus mover.
  await s.press('tab')          // focus Commits, cursor on All commits row
  await s.press('j')            // aaa1111 auto-selected
  await s.press('j')            // bbb2222 auto-selected
  await s.press('k')            // aaa1111 auto-selected
  await s.press('tab')          // Commits → Diff
  // dave's comment is anchored at original_line=5 → buffer index 7
  // in the aaa1111 patch (header×2 + hunk + 5 add lines).
  await pressN(s, 'j', 7)
  const screen = await s.text()
  const diff = paneText(screen, 'Diff')
  const cms = paneText(screen, 'Comments')
  assert.match(diff, /Diff:[^\n]*aaa1111/, `Diff should reference aaa1111; slice:\n${diff}`)
  assert.ok(cms.includes('dave:'), `aaa1111-anchored comment (dave) should show with cursor on ◆ row; Comments slice:\n${cms}`)
  await quit(s)
})

// I5: removed — manual `space` filter toggle and the `(filter: ...)` title
// suffix were replaced by an auto-filter keyed off SelectedFile (see E1/E9).

test('I6: Comments cursor movement auto-scrolls Diff', { skip: 'TODO: Diff scroll observation requires renderer-defined anchors; revisit after Diff renderer lands' }, async () => {})

test('I7: split/unified toggle re-renders Diff only', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  const before = await s.text()
  await s.press('space')                       // toggle
  const after = await s.text()
  assert.notEqual(before, after, 'Diff should re-render after split/unified toggle')
  // Other panes' titles must remain identical (we only check Files & Commits & Comments labels persist).
  for (const label of ['Files', 'Commits', 'Comments']) {
    assert.ok(after.includes(label), `${label} pane should still be visible`)
  }
  await quit(s)
})
