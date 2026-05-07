// Category E — Commits pane.

import { test, describe, before } from 'node:test'
import assert from 'node:assert/strict'

import { launchReva, waitReady, quit, paneText } from '../helpers/launch.mjs'

// E1 + E1b + E2 share the initial-render state for greeting.go.
describe('E1+E1b+E2: Commits pane initial render (greeting.go selected)', () => {
  let screen
  before(async () => {
    const s = await launchReva()
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

test('E3: j/k moves the Commits cursor (All commits row sits above real commits)', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab')   // focus Commits
  let commits = paneText(await s.text(), 'Commits')
  assert.match(commits, /^>[^\n]*All commits/m, 'cursor should start on the All commits virtual row')
  await s.type('j')
  commits = paneText(await s.text(), 'Commits')
  assert.match(commits, /^>[^\n]*aaa1111/m, 'after j → cursor on first real commit aaa1111')
  await s.type('j')
  commits = paneText(await s.text(), 'Commits')
  assert.match(commits, /^>[^\n]*bbb2222/m, 'after j → cursor on bbb2222')
  await s.type('k')
  commits = paneText(await s.text(), 'Commits')
  assert.match(commits, /^>[^\n]*aaa1111/m, 'after k → cursor back on aaa1111')
  await s.type('k')
  commits = paneText(await s.text(), 'Commits')
  assert.match(commits, /^>[^\n]*All commits/m, 'after k → cursor back on the All commits row')
  await quit(s)
})

// E4/E5: removed — manual `space` filter toggle (and "(filter: …)" title)
// were superseded by the SelectedFile-driven auto-filter. See E1.

test('E6: Enter on Commits is a no-op (Tab is the only focus mover)', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab')          // focus Commits
  const before = await s.text()
  await s.press('enter')
  const after = await s.text()
  assert.equal(before, after, 'Enter on Commits must not change focus or state')
  assert.match(after, /▶ Commits/, 'focus stays on Commits')
  await quit(s)
})

test('E7: j/k in Commits auto-selects the commit (Diff updates without Enter)', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab')          // focus Commits, cursor on the All commits row
  let screen = await s.text()
  assert.match(screen, /▶ Commits/)
  assert.doesNotMatch(screen, /Diff:[^\n]*@\s*[a-f0-9]+/, 'All commits row → RangeWholePR (no SHA suffix)')
  // j → cursor on aaa1111 → Diff must reflect SingleCommit aaa1111 immediately.
  await s.type('j')
  screen = await s.text()
  assert.match(paneText(screen, 'Commits'), /^>[^\n]*aaa1111/m, 'cursor moved to aaa1111')
  assert.match(screen, /Diff:[^\n]*@\s*aaa1111/, 'Diff title should reflect aaa1111 SingleCommit slice')
  assert.match(screen, /▶ Commits/, 'focus stays on Commits')
  // j → bbb2222.
  await s.type('j')
  screen = await s.text()
  assert.match(screen, /Diff:[^\n]*@\s*bbb2222/, 'Diff title should reflect bbb2222 after second j')
  // k → back to aaa1111.
  await s.type('k')
  screen = await s.text()
  assert.match(screen, /Diff:[^\n]*@\s*aaa1111/, 'Diff title should reflect aaa1111 after k')
  await quit(s)
})

test('E8: single-commit drill via j+Tab (auto-select drives SelectedRange, Tab moves focus)', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab')          // focus Commits, cursor on All commits
  await s.type('j')             // cursor → aaa1111 (auto: SingleCommit aaa1111)
  await s.type('j')             // cursor → bbb2222 (auto: SingleCommit bbb2222)
  await s.type('k')             // cursor → aaa1111 (auto: SingleCommit aaa1111)
  await s.press('tab')          // → Diff, SelectedRange unchanged = SingleCommit aaa1111
  const screen = await s.text()
  assert.match(screen, /▶ Diff/, 'focus on Diff')
  assert.match(screen, /Diff:[^\n]*@\s*aaa1111/, 'aaa1111 drill preserved through Tab')
  await quit(s)
})

test('E10: All commits virtual row renders with file-filtered count', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Initial fixture: SelectedFile = src/greeting.go (touched by 2 of 3 commits).
  const commits = paneText(await s.text(), 'Commits')
  assert.match(commits, /All commits \(2 of 3\)/, 'filtered form should read "(2 of 3)" when greeting.go is selected')
  // The virtual row sits above the real commits.
  const allIdx = commits.indexOf('All commits')
  const aaaIdx = commits.indexOf('aaa1111')
  assert.ok(allIdx >= 0 && aaaIdx >= 0 && allIdx < aaaIdx,
    `All commits row must precede aaa1111; slice:\n${commits}`)
  await quit(s)
})

test('E11: returning to All commits row reverts Diff to whole-PR slice', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab')                      // focus Commits, cursor on All commits row
  await s.type('j')                         // cursor → aaa1111 (SingleCommit)
  let screen = await s.text()
  assert.match(screen, /Diff:[^\n]*@\s*aaa1111/, 'Diff drilled into aaa1111')
  await s.type('k')                         // cursor → All commits row (RangeWholePR)
  screen = await s.text()
  assert.match(paneText(screen, 'Commits'), /^>[^\n]*All commits/m, 'cursor back on All commits row')
  assert.doesNotMatch(screen, /Diff:[^\n]*@\s*[a-f0-9]+/, 'Diff title must drop the SHA suffix on All commits')
  await quit(s)
})

test('E12: shift+J resets Commits cursor to the All commits row', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab')                      // focus Commits
  await s.type('j')                         // cursor → aaa1111 (SingleCommit aaa1111)
  let screen = await s.text()
  assert.match(screen, /Diff:[^\n]*@\s*aaa1111/, 'drilled into aaa1111')
  await s.press('J')                        // advance file → reset CommitsCursor=0
  screen = await s.text()
  const commits = paneText(screen, 'Commits')
  assert.match(commits, /^>[^\n]*All commits/m, 'shift+J must place cursor back on All commits')
  assert.doesNotMatch(screen, /Diff:[^\n]*@\s*[a-f0-9]+/, 'Diff must show whole-PR slice for the new file')
  await quit(s)
})

test('E9: switching SelectedFile re-filters the Commits pane', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Initial: greeting.go selected → Commits = [aaa1111, bbb2222]
  let screen = await s.text()
  assert.ok(screen.includes('aaa1111 Add greeting.go skeleton'))
  assert.ok(!screen.includes('ccc3333 Add tests and docs'))
  // Move Files cursor + Enter to commit greeting_test.go (touched only
  // by ccc3333). j alone moves the cursor; SelectedFile changes only
  // on Enter / Shift+J/K (commit gesture).
  await s.type('j')
  await s.press('enter')
  screen = await s.text()
  assert.ok(screen.includes('ccc3333 Add tests and docs'), 'ccc3333 should appear when greeting_test.go is selected')
  assert.ok(!screen.includes('aaa1111 Add greeting.go skeleton'), 'aaa1111 should disappear (does not touch greeting_test.go)')
  await quit(s)
})
