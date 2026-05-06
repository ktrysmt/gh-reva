// Category G — Comments pane.
//
// Comments-pane contract (post diff-cursor coupling):
// * The column shows ONLY the threads anchored at the Diff cursor's
//   current buffer line (the rows the Diff pane marks with ◆).
// * When the Diff cursor is not on a ◆ row, the column reads
//   "(no comment at cursor)".
// * Tests below position the Diff cursor explicitly via Tab+j navigation
//   before asserting on Comments content.
//
// Anchor map (sample-pr.json, src/greeting.go HEAD patch):
//   buf idx  newLine  content
//      5        3     +// Hello returns a greeting for the given name.
//   ↑ carol root (id 1001) + alice reply (id 1002) thread anchors here.

import { test, describe, before } from 'node:test'
import assert from 'node:assert/strict'
import path from 'node:path'

import { launchReva, waitReady, quit, REPO_ROOT, paneText } from '../helpers/launch.mjs'

// pressN sends `key` `n` times, one keystroke at a time. Multi-char strings
// passed to s.type can drop input under load (tuistory + bubbletea batching),
// so anything beyond ~5 repeats should go through this helper.
async function pressN (s, key, n) {
  for (let i = 0; i < n; i++) await s.press(key)
}

// focusDiffOnCarolAnchor moves focus to the Diff pane and walks the cursor
// to buffer line 5 (the ◆ row for the carol/alice thread on src/greeting.go
// in the default fixture).
async function focusDiffOnCarolAnchor (s) {
  await s.press('tab')              // Files → Commits
  await s.press('tab')              // Commits → Diff
  await pressN(s, 'j', 5)           // 5x j → buffer line 5 (◆ row)
}

// G1+G2+G3+G4 share the Comments state with the Diff cursor parked on the
// carol/alice anchor row.
describe('G1+G2+G3+G4: Comments pane renders the cursor-anchor thread', () => {
  let screen, cms
  before(async () => {
    const s = await launchReva()
    await waitReady(s)
    await focusDiffOnCarolAnchor(s)
    screen = await s.text()
    cms = paneText(screen, 'Comments')
    await quit(s)
  })

  test('G1: only the cursor-anchor thread is shown', () => {
    assert.ok(cms.includes('Consider extracting'), `greeting.go thread root should be visible:\n${cms}`)
    assert.ok(cms.includes('Good point'), `greeting.go reply should be visible:\n${cms}`)
    assert.ok(!cms.includes('Add a test for the empty'), 'other-file comments must not leak')
    assert.ok(!cms.includes('(no comment at cursor)'), 'placeholder must not appear when cursor is on ◆')
  })

  test('G2: thread structure rendered with indentation (reply indented vs root)', () => {
    const cmsLines = cms.split('\n')
    const rootIdx = cmsLines.findIndex(l => l.includes('carol:'))
    const replyIdx = cmsLines.findIndex(l => l.includes('alice:'))
    assert.ok(rootIdx >= 0 && replyIdx >= 0, `both root and reply headers should appear:\n${cms}`)
    const indentOf = (l) => l.match(/^\s*/)[0].length
    assert.ok(indentOf(cmsLines[replyIdx]) > indentOf(cmsLines[rootIdx]), 'reply header should be more indented than its root')
  })

  test('G3: ascending time order (oldest at top)', () => {
    // Root posted at 13:00, reply at 14:30 → root must appear before reply.
    const rootIdx = cms.indexOf('carol:')
    const replyIdx = cms.indexOf('alice:')
    assert.ok(rootIdx >= 0 && replyIdx >= 0)
    assert.ok(rootIdx < replyIdx, 'root must precede reply (chronological)')
  })

  test('G4: HEAD view hides outdated comments', () => {
    // Fixture comment id=1003 (dave) is outdated against HEAD.
    assert.ok(!screen.includes('This was the old struct'), 'outdated comment must not show at HEAD')
  })
})

test('G0: with the Diff cursor off any ◆ row, Comments shows the placeholder', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Initial Diff cursor is at buffer line 0 (file header), which has no anchor.
  const cms = paneText(await s.text(), 'Comments')
  assert.ok(cms.includes('(no comment at cursor)'), `placeholder expected; got:\n${cms}`)
  assert.ok(!cms.includes('carol:'), 'no thread should leak when cursor is off-anchor')
  await quit(s)
})

test('G5: single-commit view exposes comments anchored to that commit', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Drill into aaa1111 explicitly: tab + j×2 + k primes auto-select on aaa1111
  // (Tab lands on the All commits virtual row; j/k drives commit-pick from
  // there; Tab moves focus to Diff for inspection).
  await s.press('tab')          // focus Commits, cursor on All commits row
  await s.press('j')            // cursor → aaa1111 (auto-select SingleCommit aaa1111)
  await s.press('j')            // cursor → bbb2222 (auto-select SingleCommit bbb2222)
  await s.press('k')            // cursor → aaa1111 (auto-select SingleCommit aaa1111)
  await s.press('tab')          // → Diff focus, SelectedRange = aaa1111
  // dave's outdated comment is anchored at original_line=5, which maps to
  // buffer index 7 in the aaa1111 patch (header×2 + hunk + 5 add lines).
  await pressN(s, 'j', 7)       // 7x j → buffer line 7 (◆ row for dave)
  const cms = paneText(await s.text(), 'Comments')
  assert.ok(cms.includes('dave:'), `single-commit view should expose dave's anchored comment; Comments slice:\n${cms}`)
  assert.ok(cms.includes('[outdated]'), `comment 1003 carries the [outdated] tag in single-commit view; slice:\n${cms}`)
  await quit(s)
})

test('G6: j/k inside a thread walks root → reply', async () => {
  const s = await launchReva()
  await waitReady(s)
  await focusDiffOnCarolAnchor(s)
  await s.press('tab')                                  // Diff → Comments
  let cms = paneText(await s.text(), 'Comments')
  assert.match(cms, /^>[^\n]*carol:/m, `cursor should start on the carol header; slice:\n${cms}`)
  await s.type('j')
  cms = paneText(await s.text(), 'Comments')
  assert.match(cms, /^>[^\n]*alice:/m, `after j → cursor should move to the alice reply header; slice:\n${cms}`)
  await quit(s)
})

test('G7: Comments cursor movement auto-scrolls Diff to the comment line', { skip: 'TODO: needs Diff pane to expose a deterministic anchor for the comment line; revisit after diff renderer lands' }, async () => {})

test('G8: h/l are no-ops in Comments — threads are always expanded', async () => {
  const s = await launchReva()
  await waitReady(s)
  await focusDiffOnCarolAnchor(s)
  await s.press('tab')                                   // Diff → Comments
  // Initial state: thread expanded → reply visible.
  let screen = await s.text()
  assert.ok(screen.includes('Good point'), 'reply should be visible (always expanded)')
  // h must not collapse the thread.
  await s.type('h')
  screen = await s.text()
  assert.ok(screen.includes('Good point'), 'h must NOT hide the reply (folding is gone)')
  // l must not change anything either.
  await s.type('l')
  screen = await s.text()
  assert.ok(screen.includes('Good point'), 'l must NOT change visibility either')
  await quit(s)
})

test('G9: r on Comments opens the reply compose modal', async () => {
  // The reply gesture moved from Enter to `r`; Enter is now in-place
  // edit (own-author only). carol's comment is foreign so Enter
  // surfaces a Notice — covered by G9b. r still works for replies on
  // any author.
  const s = await launchReva({ env: { EDITOR: '', VISUAL: '' } })
  await waitReady(s)
  await s.press('tab'); await s.press('tab')                          // focus Diff
  for (let i = 0; i < 5; i++) await s.type('j')                       // anchor row
  await s.press('tab')                                                // → Comments
  await s.type('r')
  await s.waitForText('Reply', { timeout: 5000 })
  await s.press('esc')                                                // close modal
  const after = await s.text()
  assert.match(after, /▶ Comments/, 'focus stays on Comments after compose closes')
  await quit(s)
})

test('G9b: Enter on Comments without a thread is a no-op (no notice either)', async () => {
  // Buffer 0 (file header) has no anchored thread → buildComposeEdit
  // refuses without setting Notice (the foreign-author message would
  // mislead when the cursor is off-thread entirely).
  const s = await launchReva({ env: { EDITOR: '', VISUAL: '' } })
  await waitReady(s)
  await s.press('tab'); await s.press('tab'); await s.press('tab')    // focus Comments
  await s.press('enter')
  const after = await s.text()
  assert.ok(!/New comment|Reply|Edit comment/.test(after),
    `Comments Enter without a thread must not open compose`)
  assert.ok(!/cannot edit comments by other users/.test(after),
    `the foreign-author notice must not surface when there is no thread`)
  await quit(s)
})

test('G11: long comment body wraps within the Comments column', async () => {
  // Default cols=160 — the Comments column is narrow enough that the long
  // body wraps onto multiple rows. wrap-pr.json carries one carol comment
  // anchored at newLine=3 (buffer index 5 in the wrap-pr greeting.go patch:
  // header×2 + hunk + 3 add lines).
  const s = await launchReva({
    fixture: path.join(REPO_ROOT, 'testdata', 'wrap-pr.json'),
  })
  await waitReady(s)
  await s.press('tab'); await s.press('tab')           // Files → Diff
  await pressN(s, 'j', 5)                              // 5x j → ◆ row
  const cms = paneText(await s.text(), 'Comments')
  const cmsLines = cms.split('\n')
  const headerRowIdx = cmsLines.findIndex(l => l.includes('carol:'))
  assert.ok(headerRowIdx >= 0, `comment header should appear in Comments column:\n${cms}`)
  // Body is on its own row(s) below the header. Body fixture contains
  // "Consider" near the head and "evolve" near the tail.
  const bodyHeadIdx = cmsLines.findIndex((l, i) => i > headerRowIdx && l.includes('Consider'))
  const tailIdx = cmsLines.findIndex(l => l.includes('evolve'))
  assert.ok(bodyHeadIdx > headerRowIdx, `body head should wrap onto a later row than the header (header=${headerRowIdx}, bodyHead=${bodyHeadIdx})`)
  assert.ok(tailIdx > bodyHeadIdx, `body tail must wrap below the body head (bodyHead=${bodyHeadIdx}, tail=${tailIdx})`)
  await quit(s)
})

test('G10: Backspace is a no-op in Comments (Tab is the only focus mover)', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab'); await s.press('tab')   // focus Comments
  const before = await s.text()
  await s.press('backspace')
  const after = await s.text()
  assert.equal(before, after, 'Backspace on Comments must be a no-op')
  assert.match(after, /▶ Comments/, 'focus must remain on Comments')
  await quit(s)
})
