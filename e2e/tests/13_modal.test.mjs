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

test('F-modal-2: Enter inside Files modal commits the cursor file (Diff title follows)', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Initial: cursor on src/greeting.go (first file). Diff title shows that file.
  let screen = await s.text()
  assert.ok(/Diff:\s*src\/greeting\.go(?!_)/.test(diffTitle(screen)), `initial Diff title should target src/greeting.go; got ${JSON.stringify(diffTitle(screen))}`)

  await s.press('space')          // open Files modal
  screen = await s.text()
  assert.ok(modalVisible(screen), 'modal should be open after Space')

  await s.press('j')              // move cursor inside modal — Diff still on greeting.go
  screen = await s.text()
  assert.ok(modalVisible(screen), 'j must not close the modal')
  assert.ok(/Diff:\s*src\/greeting\.go(?!_)/.test(diffTitle(screen)),
    `j inside modal must NOT change SelectedFile; got ${JSON.stringify(diffTitle(screen))}`)

  await s.press('enter')          // commit the cursor file and shift focus to Diff
  screen = await s.text()
  assert.ok(!modalVisible(screen), 'Enter should close the modal')
  assert.ok(/▶ Diff/.test(screen), 'Enter should shift focus to Diff')
  assert.ok(
    /Diff:\s*src\/greeting_test\.go/.test(diffTitle(screen)),
    `Diff title should follow Enter commit; got ${JSON.stringify(diffTitle(screen))}`,
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

test('F-modal-5: Space in Comments opens the Comments modal (only when threads visible)', async () => {
  const s = await launchReva()
  await waitReady(s)
  // src/greeting.go thread 1001 is anchored at new-file line 3 → buffer
  // index 5 in the visible patch (header×2 + hunk + 3 lines). Walking
  // there ensures Comments has visible threads, which the new spec
  // requires before Space opens the modal.
  await s.press('tab')            // Commits
  await s.press('tab')            // Diff
  for (let i = 0; i < 4; i++) await s.press('j')
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

test('F-modal-5b: Space in Comments is a no-op when the cursor row has no thread', async () => {
  // The placeholder "(no comment at cursor)" state should not zoom — a
  // modal that just wraps the placeholder text is noise. Verified
  // directly by the unit test TestComments_SpaceNoopWhenNoThread; this
  // e2e covers the user-facing screen so the contract is observable.
  // Selects src/main.go (a file with zero comments in sample-pr.json)
  // so the placeholder shows regardless of which Diff row the cursor
  // lands on — the file-overview short-circuit on meta rows returns
  // an empty thread list when the file has no comments.
  const s = await launchReva()
  await waitReady(s)
  // Files focus, cursor drilled to greeting.go (idx 1). j j → main.go
  // (idx 3); enter → commit selection + focus Diff. tab → Comments.
  await s.press('j'); await s.press('j')
  await s.press('enter')
  await s.press('tab')
  let screen = await s.text()
  assert.ok(/▶ Comments/.test(screen), 'precondition: Comments active')
  assert.ok(/\(no comment at cursor\)/.test(screen),
    'precondition: Comments shows the placeholder')

  await s.press('space')
  screen = await s.text()
  assert.ok(!modalVisible(screen), 'Space on (no comment) must NOT open the modal')
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

test('F-modal-9: Diff Enter on commented row shifts focus to Comments (no modal)', async () => {
  // The previous behavior — auto-opening the Comments zoom modal — was
  // retired once Ctrl+E gave the column a stable visibility gesture.
  // Diff Enter on a commented row now plain-shifts focus; the user can
  // press Space from Comments to open the zoom modal if they want.
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab') // Files → Commits
  await s.press('tab') // Commits → Diff
  for (let i = 0; i < 4; i++) await s.press('j')
  let screen = await s.text()
  assert.ok(/▶ Diff/.test(screen), 'precondition: Diff active')

  await s.press('enter')
  screen = await s.text()
  assert.ok(!modalVisible(screen),
    `Diff Enter must NOT open a modal; tail:\n${screen.split('\n').slice(-12).join('\n')}`)
  assert.ok(/▶ Comments/.test(screen),
    `Diff Enter must shift focus to Comments; tail:\n${screen.split('\n').slice(-12).join('\n')}`)
  await quit(s)
})

test('F-modal-10: Comments space → space stays on Comments', async () => {
  // Comments modal opened via space from Comments has Origin=Comments;
  // close gesture must keep focus there. Walk Diff cursor onto a ◆
  // row first (line 3 → buffer 5) so Space has a visible thread to
  // zoom (per the new no-thread no-op rule).
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab') // Commits
  await s.press('tab') // Diff
  for (let i = 0; i < 4; i++) await s.press('j')
  await s.press('tab') // Diff → Comments
  let screen = await s.text()
  assert.ok(/▶ Comments/.test(screen), 'precondition: Comments active')

  await s.press('space') // open from Comments
  screen = await s.text()
  assert.ok(modalVisible(screen), 'precondition: modal open')

  await s.press('space') // close
  screen = await s.text()
  assert.ok(!modalVisible(screen), 'space must close the modal')
  assert.ok(/▶ Comments/.test(screen),
    `space close must keep focus on Comments (the opener); tail:\n${screen.split('\n').slice(-12).join('\n')}`)
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
