// Category T — Comments pane toggle (Ctrl+E).
//
// Contract:
//   - Ctrl+E hides the right (Comments) pane and gives the saved width to
//     the middle (Diff) column. A second Ctrl+E reveals it again.
//   - Hiding while focus is on Comments shifts FocusedPane to Diff.
//   - Tab / Shift+Tab cycle skips Comments while hidden so the focus
//     ladder stays consistent (Files → Commits → Diff → Files).
//   - Diff Enter on a row carrying threads auto-reveals the pane before
//     opening the Comments zoom modal — the modal-close-restores-focus
//     contract should never strand focus on an invisible pane.

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchReva, waitReady, quit, activePaneLabel } from '../helpers/launch.mjs'

// commentsPaneVisible scans for the literal `Comments` pane title. The
// title carries an inactive (`  Comments`) or active (`▶ Comments`)
// prefix and is ringed by box-drawing chrome — its presence is the
// signature that the right column is rendered.
function commentsPaneVisible (screen) {
  return / Comments\b/.test(screen)
}

// diffRightEdge returns the column of the `│` that closes the Diff pane
// (the box character immediately after the Diff title text on the Diff
// header row). When Comments is visible the right edge sits in the
// middle of the screen; when Comments is hidden it lands on the
// terminal's far right because the Diff column absorbs the saved width.
function diffRightEdge (screen) {
  const lines = screen.split('\n')
  for (const line of lines) {
    const idx = line.indexOf('Diff:')
    if (idx < 0) continue
    const tail = line.indexOf('│', idx)
    return tail
  }
  return -1
}

test('T1: Ctrl+E hides the Comments pane', async () => {
  const s = await launchReva()
  await waitReady(s)
  let screen = await s.text()
  assert.ok(commentsPaneVisible(screen), 'Comments pane should be visible by default')

  await s.press(['ctrl', 'e'])
  screen = await s.text()
  assert.ok(!commentsPaneVisible(screen),
    `Comments pane should be hidden after Ctrl+E; tail:\n${screen.split('\n').slice(0, 6).join('\n')}`)
  await quit(s)
})

test('T2: Second Ctrl+E reveals the Comments pane', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press(['ctrl', 'e']) // hide
  await s.press(['ctrl', 'e']) // reveal
  const screen = await s.text()
  assert.ok(commentsPaneVisible(screen), 'Comments pane should reappear after a second Ctrl+E')
  await quit(s)
})

test('T3: Diff column expands when Comments is hidden', async () => {
  const s = await launchReva()
  await waitReady(s)
  const before = diffRightEdge(await s.text())
  assert.ok(before > 0, 'Diff column right edge must be detectable')
  await s.press(['ctrl', 'e'])
  const after = diffRightEdge(await s.text())
  assert.ok(after > before,
    `Diff column should extend right after hiding Comments; before=${before} after=${after}`)
  await quit(s)
})

test('T4: Tab from Diff with Comments hidden lands on Files', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Move focus to Diff (Files → Commits → Diff)
  await s.press('tab')
  await s.press('tab')
  await s.press(['ctrl', 'e'])
  await s.press('tab')
  const label = await activePaneLabel(s)
  assert.equal(label, 'Files',
    `Tab from Diff with Comments hidden must skip to Files; got active=${label}`)
  await quit(s)
})

test('T5: hiding from Comments shifts focus to Diff', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Move focus to Comments (Files → Commits → Diff → Comments)
  await s.press('tab')
  await s.press('tab')
  await s.press('tab')
  assert.equal(await activePaneLabel(s), 'Comments', 'pre-condition: focus on Comments')
  await s.press(['ctrl', 'e'])
  // After hide, Comments title is gone — focus must have moved.
  const screen = await s.text()
  assert.ok(!commentsPaneVisible(screen), 'Comments pane must be hidden')
  const label = await activePaneLabel(s)
  assert.equal(label, 'Diff',
    `hiding from Comments must shift focus to Diff; got active=${label}`)
  await quit(s)
})
