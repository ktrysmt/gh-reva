// Category I — Inter-pane synchronisation.

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchGhRv, waitReady, quit } from '../helpers/launch.mjs'

test('I1+I2: file selection re-renders Diff and re-filters Comments', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  // Move to greeting_test.go and Enter.
  await s.type('j')
  await s.press('enter')
  const screen = await s.text()
  assert.match(screen, /Diff: src\/greeting_test\.go/, 'Diff should switch to greeting_test.go')
  assert.ok(screen.includes('Add a test for the empty input case'), 'Comments should now show greeting_test.go thread')
  assert.ok(!screen.includes('Consider extracting this'), 'greeting.go thread should be gone')
  await quit(s)
})

test('I3+I4: commit selection re-renders Diff and re-computes active Comments', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  // selectedFile = greeting.go by default. Drill: Files → Commits → Diff.
  await s.press('tab')          // focus Commits
  await s.press('enter')         // select aaa1111 → Diff focus
  const screen = await s.text()
  assert.match(screen, /Diff:[^\n]*aaa1111/, 'Diff should reference aaa1111')
  // Comment 1003 (anchored to aaa1111) becomes active in single-commit view.
  assert.ok(screen.includes('This was the old struct definition'), 'aaa1111-anchored comment should show')
  await quit(s)
})

test('I5: filter toggle rebuilds the Commits pane', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  // Use the unique commit subject — the SHA "ccc3333" also appears in
  // comment headers (commit_id) and would cause false positives.
  const CCC_SUBJECT = 'Add tests and docs'
  let screen = await s.text()
  assert.ok(screen.includes(CCC_SUBJECT), 'ccc3333 visible without filter')
  await s.press('space')
  screen = await s.text()
  assert.ok(!screen.includes(CCC_SUBJECT), 'ccc3333 hidden under filter')
  await s.press('space')
  screen = await s.text()
  assert.ok(screen.includes(CCC_SUBJECT), 'ccc3333 visible again after toggle off')
  await quit(s)
})

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
