// Category I — Inter-pane synchronisation.

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchGhRv, waitReady, quit, paneText } from '../helpers/launch.mjs'

test('I1+I2: file selection re-renders Diff and re-filters Comments', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  // Move to greeting_test.go and Enter.
  await s.type('j')
  await s.press('enter')
  const screen = await s.text()
  assert.match(screen, /Diff: src\/greeting_test\.go/, 'Diff should switch to greeting_test.go')
  assert.ok(screen.includes('Add a test for the empty'), 'Comments should now show greeting_test.go thread')
  assert.ok(!screen.includes('Consider extracting this'), 'greeting.go thread should be gone')
  await quit(s)
})

test('I3+I4: commit selection re-renders Diff and re-computes active Comments', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  // Drill into aaa1111 via j+k (Enter alone keeps PR-wide view under the new contract).
  await s.press('tab')          // focus Commits
  await s.type('j')             // bbb2222 auto-selected
  await s.type('k')             // aaa1111 auto-selected
  await s.press('enter')         // → Diff focus, SingleCommit = aaa1111
  const screen = await s.text()
  const diff = paneText(screen, 'Diff')
  const cms = paneText(screen, 'Comments')
  assert.match(diff, /Diff:[^\n]*aaa1111/, `Diff should reference aaa1111; slice:\n${diff}`)
  // Comment 1003 (dave, anchored to aaa1111) becomes active in single-commit view.
  assert.ok(cms.includes('dave:'), `aaa1111-anchored comment (dave) should show; Comments slice:\n${cms}`)
  await quit(s)
})

// I5: removed — manual `space` filter toggle and the `(filter: ...)` title
// suffix were replaced by an auto-filter keyed off SelectedFile (see E1/E9).

test('I6: Comments cursor movement auto-scrolls Diff', { skip: 'TODO: Diff scroll observation requires renderer-defined anchors; revisit after Diff renderer lands' }, async () => {})

test('I7: split/unified toggle re-renders Diff only', async () => {
  const s = await launchGhRv()
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
