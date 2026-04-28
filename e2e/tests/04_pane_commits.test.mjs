// Category E — Commits pane.

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchGhRv, waitReady, quit } from '../helpers/launch.mjs'

test('E1: PR commits are listed in chronological order (oldest first)', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  const screen = await s.text()
  const idxA = screen.indexOf('aaa1111')
  const idxB = screen.indexOf('bbb2222')
  const idxC = screen.indexOf('ccc3333')
  assert.notEqual(idxA, -1)
  assert.notEqual(idxB, -1)
  assert.notEqual(idxC, -1)
  assert.ok(idxA < idxB && idxB < idxC, 'expected aaa1111 < bbb2222 < ccc3333 in screen order')
  await quit(s)
})

test('E1b: each commit shows short_sha + subject', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  const screen = await s.text()
  assert.ok(screen.includes('aaa1111 Add greeting.go skeleton'))
  assert.ok(screen.includes('bbb2222 Implement Hello function'))
  assert.ok(screen.includes('ccc3333 Add tests and docs'))
  await quit(s)
})

test('E2: each commit annotates [A]/[M]/[D]/[R] for selectedFile', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  // selectedFile auto = src/greeting.go on startup.
  const screen = await s.text()
  // greeting.go was added in aaa1111, modified in bbb2222, untouched in ccc3333.
  assert.match(screen, /\[A\]\s+aaa1111/, 'aaa1111 should annotate [A]')
  assert.match(screen, /\[M\]\s+bbb2222/, 'bbb2222 should annotate [M]')
  assert.doesNotMatch(screen, /\[\w\]\s+ccc3333/, 'ccc3333 should NOT annotate (greeting.go untouched)')
  await quit(s)
})

test('E3: j/k moves the Commits cursor', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab')   // focus Commits
  let screen = await s.text()
  assert.match(screen, /^>[^\n]*aaa1111/m, 'cursor should start on first commit')
  await s.type('j')
  screen = await s.text()
  assert.match(screen, /^>[^\n]*bbb2222/m, 'after j → cursor on bbb2222')
  await s.type('k')
  screen = await s.text()
  assert.match(screen, /^>[^\n]*aaa1111/m, 'after k → cursor back on aaa1111')
  await quit(s)
})

test('E4: filter mode shows only commits that touch commitFilterFile', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  // Cursor is on src/greeting.go; press space to enable filter.
  await s.press('space')
  const screen = await s.text()
  // Use commit subjects (unique to the Commits pane) rather than short SHAs,
  // because comment headers also embed the SHA "ccc3333" via commit_id.
  assert.ok(screen.includes('Add greeting.go skeleton'), 'aaa1111 should remain visible under filter')
  assert.ok(screen.includes('Implement Hello function'), 'bbb2222 should remain visible under filter')
  assert.ok(!screen.includes('Add tests and docs'), 'ccc3333 (does not touch greeting.go) should be hidden')
  await quit(s)
})

test('E5: filter state shows in pane title', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('space')
  const screen = await s.text()
  assert.match(screen, /Commits \(filter: src\/greeting\.go\)/)
  await quit(s)
})

test('E6: Enter on a commit drills into single-commit Diff and focuses Diff', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab')          // focus Commits
  await s.press('enter')         // select aaa1111 → focus Diff
  const screen = await s.text()
  assert.match(screen, /▶ Diff/, 'focus should move to Diff')
  assert.match(screen, /Diff:[^\n]*aaa1111/, 'Diff header should reference selected commit')
  await quit(s)
})
