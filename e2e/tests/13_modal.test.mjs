// Category F-modal — `<space>` opens a centered modal for the active pane
// (Files / Commits / Comments). The modal is a "zoomed" view of that pane:
// content renders at a wider width for better visibility, and j/k navigation
// inside the modal updates the underlying main state so closing the modal
// leaves the main UI on the same row. Diff pane keeps `<space>` as the
// split⇄unified toggle. Visual mode (v / y) is allowed inside the modal.

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchReva, waitReady, quit, paneText } from '../helpers/launch.mjs'

// Modal detection signature: the modal title row is `│ <Pane>   │`, with
// a single leading space before the pane name. The regular pane title
// rows always carry an accent — `│▶ Files` (active) or `│  Files` (two
// spaces, inactive) — so a single-space form unambiguously identifies
// the centered modal regardless of where it lands on screen.
const MODAL_TITLE_RE = /│ (Files|Commits|Comments)\s+│/

function modalVisible (screen) {
  return MODAL_TITLE_RE.test(screen)
}

function modalTitle (screen) {
  const m = MODAL_TITLE_RE.exec(screen)
  return m ? m[1] : null
}

// diffTitle returns the "Diff: <path>" header text from the underlying
// Diff pane (still rendered behind the modal). Used in F-modal-2 to
// verify that j/k inside the Files modal propagated to SelectedFile.
function diffTitle (screen) {
  const t = paneText(screen, 'Diff')
  return t.split('\n')[0] || ''
}

test('F-modal-1: Space in Files opens a centered modal; Space again closes it', async () => {
  const s = await launchReva()
  await waitReady(s)
  let screen = await s.text()
  assert.ok(!modalVisible(screen), 'no modal before pressing space')

  await s.press('space')
  screen = await s.text()
  assert.ok(modalVisible(screen), `expected modal after Space; tail:\n${screen.split('\n').slice(-12).join('\n')}`)
  const title = modalTitle(screen)
  assert.ok(/Files/.test(title), `modal title should mention Files; got ${JSON.stringify(title)}`)

  await s.press('space')
  screen = await s.text()
  assert.ok(!modalVisible(screen), 'second Space should close the modal')
  await quit(s)
})

test('F-modal-2: j inside Files modal updates main SelectedFile (Diff title follows)', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Initial: cursor on src/greeting.go (first file). Diff title shows that file.
  let screen = await s.text()
  assert.ok(/Diff:\s*src\/greeting\.go(?!_)/.test(diffTitle(screen)), `initial Diff title should target src/greeting.go; got ${JSON.stringify(diffTitle(screen))}`)

  await s.press('space')          // open Files modal
  screen = await s.text()
  assert.ok(modalVisible(screen), 'modal should be open after Space')

  await s.press('j')              // move cursor inside modal
  screen = await s.text()
  assert.ok(modalVisible(screen), 'j must not close the modal')

  await s.press('space')          // close modal
  screen = await s.text()
  assert.ok(!modalVisible(screen), 'modal should close on second Space')
  // Underlying SelectedFile must have advanced; Diff title now reflects file index 1.
  assert.ok(
    /Diff:\s*src\/greeting_test\.go/.test(diffTitle(screen)),
    `Diff title should follow modal j; got ${JSON.stringify(diffTitle(screen))}`,
  )
  await quit(s)
})

test('F-modal-3: Tab inside modal closes it and moves focus to next pane', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('space')          // open Files modal
  let screen = await s.text()
  assert.ok(modalVisible(screen), 'precondition: modal open')

  await s.press('tab')
  screen = await s.text()
  assert.ok(!modalVisible(screen), 'Tab should close the modal')
  assert.ok(/▶ Commits/.test(screen), 'Tab should advance focus to Commits')
  assert.ok(!/▶ Files/.test(screen), 'Files must not still be active after Tab')
  await quit(s)
})

test('F-modal-4: Space in Diff still toggles split⇄unified (no modal opens)', async () => {
  const s = await launchReva({ cols: 160, rows: 50 })
  await waitReady(s)
  await s.press('tab')            // Files → Commits
  await s.press('tab')            // Commits → Diff
  let screen = await s.text()
  assert.ok(/▶ Diff/.test(screen), 'precondition: Diff active')
  assert.ok(/\[split\]/.test(screen), 'precondition: Diff in split mode')

  await s.press('space')
  screen = await s.text()
  assert.ok(!modalVisible(screen), 'Diff Space must not open a modal')
  assert.ok(/\[unified\]/.test(screen), 'Diff Space should switch to unified')

  await s.press('space')
  screen = await s.text()
  assert.ok(/\[split\]/.test(screen), 'second Space should switch back to split')
  assert.ok(!modalVisible(screen), 'still no modal after second Space')
  await quit(s)
})

test('F-modal-5: Space in Comments opens the Comments modal', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Move to a file with at least one comment + put the Diff cursor on the
  // anchored line so Comments has visible threads.
  // src/greeting.go has comments anchored at new-file line 3 and 13.
  // Initial SelectedFile is src/greeting.go (index 0). Move Diff cursor to
  // a comment line via Tab to Diff, j several times.
  await s.press('tab')            // Commits
  await s.press('tab')            // Diff
  // Walk the cursor until Comments shows a header (best-effort: 12 j's
  // covers the patch we're using).
  for (let i = 0; i < 12; i++) await s.press('j')
  await s.press('tab')            // Comments
  let screen = await s.text()
  assert.ok(/▶ Comments/.test(screen), 'precondition: Comments active')

  await s.press('space')
  screen = await s.text()
  assert.ok(modalVisible(screen), `expected Comments modal after Space; tail:\n${screen.split('\n').slice(-12).join('\n')}`)
  assert.ok(/Comments/.test(modalTitle(screen) || ''), `modal title should mention Comments; got ${JSON.stringify(modalTitle(screen))}`)

  await s.press('space')
  screen = await s.text()
  assert.ok(!modalVisible(screen), 'second Space should close the Comments modal')
  await quit(s)
})

test('F-modal-8: Ctrl+C while a modal is open closes the modal (does not quit)', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('space')          // open Files modal
  let screen = await s.text()
  assert.ok(modalVisible(screen), 'precondition: modal open')

  await s.press(['ctrl', 'c'])
  screen = await s.text()
  assert.ok(!modalVisible(screen), 'Ctrl+C should close the modal')
  assert.ok(/▶ Files/.test(screen), 'Ctrl+C while modal open must not quit the app')
  await quit(s)
})

test('F-modal-7: q while a modal is open closes the modal (does not quit)', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('space')          // open Files modal
  let screen = await s.text()
  assert.ok(modalVisible(screen), 'precondition: modal open')

  await s.type('q')
  screen = await s.text()
  assert.ok(!modalVisible(screen), 'q should close the modal')
  assert.ok(/▶ Files/.test(screen), 'q while modal open must not quit the app')
  await quit(s)
})

test('F-modal-6: visual mode is usable inside the modal', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('space')          // open Files modal
  let screen = await s.text()
  assert.ok(modalVisible(screen), 'precondition: modal open')
  assert.ok(!/-- VISUAL --/.test(screen), 'precondition: visual not active')

  await s.type('v')
  screen = await s.text()
  assert.ok(/-- VISUAL --/.test(screen), 'v inside the modal should enter visual mode')
  assert.ok(modalVisible(screen), 'modal should still be visible after v')

  await s.press('j')
  await s.type('y')               // yank + exit visual
  screen = await s.text()
  assert.ok(!/-- VISUAL --/.test(screen), 'y should exit visual mode')
  assert.ok(modalVisible(screen), 'y must not close the modal')
  await quit(s)
})
