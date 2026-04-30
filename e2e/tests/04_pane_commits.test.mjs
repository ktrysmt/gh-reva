// Category E — Commits pane.

import { test, describe, before } from 'node:test'
import assert from 'node:assert/strict'

import { launchGhRv, waitReady, quit, paneText } from '../helpers/launch.mjs'

// E1 + E1b + E2 share the initial-render state for greeting.go.
describe('E1+E1b+E2: Commits pane initial render (greeting.go selected)', () => {
  let screen
  before(async () => {
    const s = await launchGhRv()
    await waitReady(s)
    screen = await s.text()
    await quit(s)
  })

  test('E1: Commits chronological order, filtered to SelectedFile', () => {
    const commits = paneText(screen, 'Commits')
    const idxA = commits.indexOf('aaa1111 Add greeting.go skeleton')
    const idxB = commits.indexOf('bbb2222 Implement Hello function')
    assert.ok(idxA >= 0 && idxB >= 0, `aaa1111/bbb2222 should be visible; slice:\n${commits}`)
    assert.ok(idxA < idxB, 'aaa1111 must precede bbb2222 (chronological)')
    // ccc3333 does not touch greeting.go → must be hidden under the auto-filter.
    assert.ok(!commits.includes('ccc3333 Add tests and docs'), 'ccc3333 must be hidden when greeting.go is selected')
  })

  test('E1b: each visible commit shows short_sha + subject', () => {
    const commits = paneText(screen, 'Commits')
    assert.ok(commits.includes('aaa1111 Add greeting.go skeleton'))
    assert.ok(commits.includes('bbb2222 Implement Hello function'))
  })

  test('E2: each commit annotates [A]/[M]/[D]/[R] for selectedFile', () => {
    const commits = paneText(screen, 'Commits')
    // greeting.go was added in aaa1111, modified in bbb2222, untouched in ccc3333.
    assert.match(commits, /\[A\]\s+aaa1111/, 'aaa1111 should annotate [A]')
    assert.match(commits, /\[M\]\s+bbb2222/, 'bbb2222 should annotate [M]')
    assert.doesNotMatch(commits, /\[\w\]\s+ccc3333/, 'ccc3333 should NOT annotate (greeting.go untouched)')
  })
})

test('E3: j/k moves the Commits cursor', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab')   // focus Commits
  let commits = paneText(await s.text(), 'Commits')
  assert.match(commits, /^>[^\n]*aaa1111/m, 'cursor should start on first commit')
  await s.type('j')
  commits = paneText(await s.text(), 'Commits')
  assert.match(commits, /^>[^\n]*bbb2222/m, 'after j → cursor on bbb2222')
  await s.type('k')
  commits = paneText(await s.text(), 'Commits')
  assert.match(commits, /^>[^\n]*aaa1111/m, 'after k → cursor back on aaa1111')
  await quit(s)
})

// E4/E5: removed — manual `space` filter toggle (and "(filter: …)" title)
// were superseded by the SelectedFile-driven auto-filter. See E1.

test('E6: Enter on Commits focuses Diff WITHOUT auto-picking a commit (PR-wide diff)', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab')          // focus Commits, no j/k yet
  await s.press('enter')         // → Diff focus, SelectedRange unchanged
  const screen = await s.text()
  assert.match(screen, /▶ Diff/, 'focus should move to Diff')
  // No "@ <sha>" suffix because no commit was selected. The Diff title is
  // just "Diff: <path>" + view-mode tag — single-commit drill renders an
  // additional "@ aaa1111" segment.
  assert.doesNotMatch(screen, /Diff:[^\n]*@\s*[a-f0-9]+/, 'Enter without j/k must not trigger single-commit drill')
  await quit(s)
})

test('E7: j/k in Commits auto-selects the commit (Diff updates without Enter)', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab')          // focus Commits
  let screen = await s.text()
  // Initially: cursor on aaa1111, no auto-select fired (j/k untouched).
  assert.match(screen, /▶ Commits/)
  assert.doesNotMatch(screen, /Diff:[^\n]*@\s*[a-f0-9]+/, 'no SingleCommit before j/k')
  // j → cursor on bbb2222 → Diff must reflect the bbb2222 slice immediately.
  await s.type('j')
  screen = await s.text()
  assert.match(paneText(screen, 'Commits'), /^>[^\n]*bbb2222/m, 'cursor moved to bbb2222')
  assert.match(screen, /Diff:[^\n]*@\s*bbb2222/, 'Diff title should reflect bbb2222 SingleCommit slice')
  assert.match(screen, /▶ Commits/, 'focus stays on Commits')
  // k → back to aaa1111.
  await s.type('k')
  screen = await s.text()
  assert.match(screen, /Diff:[^\n]*@\s*aaa1111/, 'Diff title should reflect aaa1111 after k')
  await quit(s)
})

test('E8: explicit single-commit drill via j+k+Enter (j primes auto-select on aaa1111)', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab')          // focus Commits
  await s.type('j')             // cursor → bbb2222 (auto: SingleCommit bbb2222)
  await s.type('k')             // cursor → aaa1111 (auto: SingleCommit aaa1111)
  await s.press('enter')         // → Diff, SelectedRange unchanged = SingleCommit aaa1111
  const screen = await s.text()
  assert.match(screen, /▶ Diff/, 'focus on Diff')
  assert.match(screen, /Diff:[^\n]*@\s*aaa1111/, 'aaa1111 drill preserved through Enter')
  await quit(s)
})

test('E9: switching SelectedFile re-filters the Commits pane', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  // Initial: greeting.go selected → Commits = [aaa1111, bbb2222]
  let screen = await s.text()
  assert.ok(screen.includes('aaa1111 Add greeting.go skeleton'))
  assert.ok(!screen.includes('ccc3333 Add tests and docs'))
  // j in Files → greeting_test.go (touched only by ccc3333)
  await s.type('j')
  screen = await s.text()
  assert.ok(screen.includes('ccc3333 Add tests and docs'), 'ccc3333 should appear when greeting_test.go is selected')
  assert.ok(!screen.includes('aaa1111 Add greeting.go skeleton'), 'aaa1111 should disappear (does not touch greeting_test.go)')
  await quit(s)
})
