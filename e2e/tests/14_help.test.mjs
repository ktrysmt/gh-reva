// Category N — Help modal triggered by `?`.
//
// Phase 1.5 contract: the modal is hidden by default and opened with `?`.
// Dismiss set: `?` (toggle), `Esc`, `Ctrl+C`, `q`. While open, every other
// keystroke is absorbed (j/k, Tab, v, t, etc. are no-ops). Visual mode
// renders `?` inert — opening Help during a selection is forbidden.

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchReva, waitReady, quit, paneText } from '../helpers/launch.mjs'

// helpVisible looks for the modal's `Help` title row inside its bordered
// box. Pane chrome never emits that exact label, so its presence uniquely
// identifies the modal.
function helpVisible (screen) {
  return /│\s*Help\b/.test(screen)
}

test('N1: ? opens the Help modal', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('?')
  const screen = await s.text()
  assert.ok(
    helpVisible(screen),
    `expected Help modal after pressing ?; tail:\n${screen.split('\n').slice(-12).join('\n')}`,
  )
  await quit(s)
})

test('N2: Help modal is hidden until ? is pressed', async () => {
  const s = await launchReva()
  await waitReady(s)
  const screen = await s.text()
  assert.ok(!helpVisible(screen), `Help modal should not show before ?`)
  await quit(s)
})

test('N3: a second ? toggles the modal closed', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('?')
  let screen = await s.text()
  assert.ok(helpVisible(screen), `modal should be open after first ?`)
  await s.type('?')
  screen = await s.text()
  assert.ok(!helpVisible(screen), `modal should be closed after second ?`)
  await quit(s)
})

test('N4: Esc closes the modal', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('?')
  await s.press('esc')
  const screen = await s.text()
  assert.ok(!helpVisible(screen), `modal should close on Esc`)
  await quit(s)
})

test('N5: Ctrl+C closes the modal without quitting the app', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('?')
  await s.press(['ctrl', 'c'])
  const screen = await s.text()
  assert.ok(!helpVisible(screen), `modal should close on Ctrl+C`)
  // App still alive: pane chrome still rendered.
  assert.ok(/▶ Files/.test(screen), `process should still be running after Ctrl+C-while-modal`)
  await quit(s)
})

test('N6: q while modal is open just closes the modal (does not quit)', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('?')
  await s.type('q')
  const screen = await s.text()
  assert.ok(!helpVisible(screen), `modal should close on q`)
  assert.ok(/▶ Files/.test(screen), `q while modal open must not quit the app`)
  await quit(s)
})

test('N7: navigation keys are inert while modal is open', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('?')
  await s.press('j')
  await s.press('j')
  await s.type('?')                          // close modal
  const screen = await s.text()
  // Cursor must still be on greeting.go (the initial cursor row). j/k were
  // absorbed by the modal so the Files cursor never advanced.
  const files = paneText(screen, 'Files')
  const cursorRow = files.split('\n').find(l => l.startsWith('> ')) || ''
  assert.ok(
    /greeting\.go(?!_)/.test(cursorRow),
    `Files cursor must not move while modal is open; cursor row was: ${JSON.stringify(cursorRow)}`,
  )
  await quit(s)
})

test('N8: Tab is inert while modal is open', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('?')
  await s.press('tab')
  await s.type('?')                          // close modal
  const screen = await s.text()
  assert.ok(/▶ Files/.test(screen), `Tab must not move focus while modal is open`)
  assert.ok(!/▶ Commits/.test(screen), `Commits must not have become active`)
  await quit(s)
})

test('N9: v while modal is open does not enter visual mode', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('?')
  await s.type('v')
  await s.type('?')                          // close modal
  const screen = await s.text()
  assert.ok(!/-- VISUAL --/.test(screen), `v must not enter visual mode while modal is open`)
  await quit(s)
})

test('N10: ? is inert in visual mode', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('v')
  await s.type('?')
  const screen = await s.text()
  assert.ok(!helpVisible(screen), `visual mode must suppress the Help modal`)
  assert.ok(/-- VISUAL --/.test(screen), `visual mode should remain active after a stray ?`)
  await s.press('esc')
  await quit(s)
})

test('N11: modal lists every pane section + Global / Visual via unique binding descriptions', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('?')
  const screen = await s.text()
  // Each description below appears only inside the Help modal — picked so
  // pane titles ("Files", "Commits", ...) cannot satisfy the assertion.
  const expected = [
    'Toggle help',          // Global ?
    'Next pane',            // Global Tab
    'Jump to Files',        // Global 1-4 (description copy)
    'Toggle tree mode',     // Files t
    'Toggle zoom modal',    // Files / Commits / Comments Space
    'Half page down',       // Diff Ctrl+D
    'Toggle split',         // Diff Space
    'Yank and exit',        // Visual y
  ]
  for (const desc of expected) {
    assert.ok(
      screen.includes(desc),
      `modal should describe "${desc}"; tail:\n${screen.split('\n').slice(-30).join('\n')}`,
    )
  }
  await quit(s)
})

test('N12: modal is roughly horizontally centered', async () => {
  const s = await launchReva({ cols: 160, rows: 50 })
  await waitReady(s)
  await s.type('?')
  const screen = await s.text()
  const lines = screen.split('\n')
  // Find the modal's top-border row by scanning for `┌─...─┐` whose start
  // column is far from the leftmost pane column (col 0). The pane boxes'
  // top borders all start at col 0, 42, or similar — anything past col 30
  // is the centered modal.
  let topRow = -1
  let topCol = -1
  for (let i = 0; i < lines.length; i++) {
    const m = /┌─+┐/.exec(lines[i])
    if (m && m.index > 30) {
      topRow = i
      topCol = m.index
      break
    }
  }
  assert.ok(topRow >= 0, `expected centered modal top-border; head:\n${lines.slice(0, 18).join('\n')}`)
  const widthMatch = /┌(─+)┐/.exec(lines[topRow])
  const w = widthMatch[1].length + 2
  const expectedLeft = Math.floor((160 - w) / 2)
  assert.ok(
    Math.abs(topCol - expectedLeft) <= 2,
    `modal should be horizontally centered; got col=${topCol}, expected ≈ ${expectedLeft} (width=${w})`,
  )
  await quit(s)
})

test('N13: modal is roughly vertically centered', async () => {
  const s = await launchReva({ cols: 160, rows: 50 })
  await waitReady(s)
  await s.type('?')
  const screen = await s.text()
  const lines = screen.split('\n')
  // Walk every match per row (globalized regex) so a pane border at
  // col 0 sharing a row with the modal's border at col ~50 doesn't
  // shadow the modal — `.exec` without /g only ever returns the
  // leftmost match. The col threshold (>30) still rejects the col-0
  // pane borders.
  const findFirst = (line, re) => {
    const g = new RegExp(re.source, 'g')
    let m
    while ((m = g.exec(line)) !== null) {
      if (m.index > 30) return m
    }
    return null
  }
  let topRow = -1
  let bottomRow = -1
  for (let i = 0; i < lines.length; i++) {
    if (findFirst(lines[i], /┌─+┐/)) { topRow = i; break }
  }
  for (let i = lines.length - 1; i >= 0; i--) {
    if (findFirst(lines[i], /└─+┘/)) { bottomRow = i; break }
  }
  assert.ok(topRow >= 0 && bottomRow > topRow,
    `expected modal top + bottom borders; head:\n${lines.slice(0, 20).join('\n')}`)
  const height = bottomRow - topRow + 1
  const expectedTop = Math.floor((50 - height) / 2)
  assert.ok(
    Math.abs(topRow - expectedTop) <= 2,
    `modal should be vertically centered; got row=${topRow}, expected ≈ ${expectedTop} (height=${height})`,
  )
  await quit(s)
})
