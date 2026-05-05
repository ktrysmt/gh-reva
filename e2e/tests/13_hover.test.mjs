// Category M — cursor-row hover popup in Files / Commits.
//
// Phase 1.5 contract: the popup is hidden by default and toggled on / off
// with `<space>` while focused on Files or Commits. Pressing j / k while
// the popup is open updates its body to the new cursor row (Show stays
// true). Visual mode and other panes (Diff, Comments) suppress it.

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchReva, waitReady, quit } from '../helpers/launch.mjs'

// hasPopup looks for the popup signature: a `│` border glyph immediately
// followed by the focused content. Pane rows always carry `> ` or `[A] `
// prefixes between the border and the content text, so this adjacency
// only occurs inside the hover popup.
function hasPopup (screen, contentRegex) {
  const re = new RegExp('│' + contentRegex.source, contentRegex.flags.includes('g') ? contentRegex.flags : contentRegex.flags + 'g')
  return re.test(screen)
}

test('M1: <space> in Files opens a popup mirroring the cursor file', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type(' ')
  const screen = await s.text()
  assert.ok(
    hasPopup(screen, /src\/greeting\.go(?!_)/),
    `expected popup with cursor file path; screen tail:\n${screen.split('\n').slice(-20).join('\n')}`,
  )
  await quit(s)
})

test('M2: popup is hidden until space is pressed', async () => {
  const s = await launchReva()
  await waitReady(s)
  const screen = await s.text()
  assert.ok(
    !hasPopup(screen, /src\/greeting\.go(?!_)/),
    `popup should not show before space; screen head:\n${screen.split('\n').slice(0, 8).join('\n')}`,
  )
  await quit(s)
})

test('M2b: a second space hides the popup', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type(' ')               // open
  let screen = await s.text()
  assert.ok(hasPopup(screen, /src\/greeting\.go(?!_)/), `popup should be open after first space`)
  await s.type(' ')               // close
  screen = await s.text()
  assert.ok(
    !hasPopup(screen, /src\/greeting\.go(?!_)/),
    `popup should be closed after second space; screen head:\n${screen.split('\n').slice(0, 8).join('\n')}`,
  )
  await quit(s)
})

test('M3: j updates the popup body to the new cursor row', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type(' ')               // open popup at greeting.go
  await s.press('j')              // cursor → greeting_test.go; popup follows
  const screen = await s.text()
  assert.ok(
    hasPopup(screen, /src\/greeting_test\.go/),
    `popup should follow cursor to greeting_test.go`,
  )
  await quit(s)
})

test('M4: <space> in Commits opens the commit-subject popup', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab')            // focus Commits, cursor on All commits virtual row
  await s.type('j')               // step onto aaa1111 (popup is per-commit)
  await s.type(' ')               // open popup
  const screen = await s.text()
  assert.ok(
    hasPopup(screen, /aaa1111 Add greeting\.go/),
    `popup should mirror cursor commit`,
  )
  await quit(s)
})

test('M5: popup sits above the cursor row, not at body bottom', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('j')
  await s.press('j')
  await s.type(' ')               // open
  const screen = await s.text()
  const lines = screen.split('\n')
  const tail = lines.slice(-5).join('\n')
  assert.ok(!/│src\/main\.go/.test(tail), `popup should not be at body bottom; tail:\n${tail}`)
  assert.ok(hasPopup(screen, /src\/main\.go/), `popup should still be visible somewhere`)
  await quit(s)
})

test('M6: Files popup is anchored above the path column with content-fit width', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type(' ')
  const screen = await s.text()
  const lines = screen.split('\n')
  // Files row format inside the pane (after the left border at col 0):
  //   col 1..2 cursor (`> `), col 3 space, col 4 status (M), col 5 space,
  //   col 6.. path. The popup's `┌` should sit above col 6 so the body
  //   text aligns with the path column below.
  let popupRow = -1
  for (let i = 0; i < lines.length; i++) {
    if (lines[i][6] === '┌') { popupRow = i; break }
  }
  assert.ok(popupRow >= 0, `popup ┌ at col 6 not found; screen:\n${lines.slice(0, 8).join('\n')}`)
  // Width should fit "src/greeting.go (2 comments)" (28 cols) + 2 borders.
  const m = lines[popupRow].slice(6).match(/^┌(─+)┐/)
  assert.ok(m, `popup top border at col 6 should match ┌─+┐; got: ${JSON.stringify(lines[popupRow].slice(6, 40))}`)
  const totalW = m[1].length + 2
  assert.ok(
    Math.abs(totalW - 30) <= 2,
    `popup width should match content length (~30); got ${totalW}`,
  )
  // Pane chrome past the popup must survive (spliceMid preserves suffix).
  const past = lines[popupRow].slice(6 + totalW).trim()
  assert.ok(
    past.length > 30,
    `pane chrome past popup must survive; got: ${JSON.stringify(past.slice(0, 80))}`,
  )
  await quit(s)
})

test('M7: Commits popup is anchored above the SHA column', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab')
  await s.type('j')               // step off the All commits row onto aaa1111
  await s.type(' ')
  const screen = await s.text()
  const lines = screen.split('\n')
  // Commits row: col 7 = SHA. Popup hugs the SHA column so body's
  // `<sha> <subject>` lines up with the row underneath it.
  let popupRow = -1
  for (let i = 0; i < lines.length; i++) {
    if (lines[i][7] === '┌') { popupRow = i; break }
  }
  assert.ok(popupRow >= 0, `popup ┌ at col 7 not found`)
  const bodyRow = lines[popupRow + 1] || ''
  assert.ok(
    /│aaa1111 Add greeting\.go/.test(bodyRow),
    `popup body should start with "<sha> <subject>" right after the border; got: ${JSON.stringify(bodyRow.slice(0, 60))}`,
  )
  await quit(s)
})

test('M8: tree-mode popup shows the full path of the cursor file', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('t')               // toggle tree mode
  await s.type(' ')               // open popup
  const screen = await s.text()
  assert.ok(
    /│src\/greeting\.go(?!_)/.test(screen),
    `tree popup should show src/greeting.go; screen head:\n${screen.split('\n').slice(0, 14).join('\n')}`,
  )
  await quit(s)
})

test('M9: tree-mode popup on a directory shows the dir path', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('t')               // tree mode
  await s.press('k')               // step up to the `v src/` dir row
  await s.type(' ')                // open popup
  const screen = await s.text()
  assert.ok(
    /│src\/│/.test(screen),
    `tree popup on a directory should be exactly "src/"; screen head:\n${screen.split('\n').slice(0, 14).join('\n')}`,
  )
  await quit(s)
})

test('M10: Files popup includes the comment count when > 0', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type(' ')
  const screen = await s.text()
  assert.ok(
    /│src\/greeting\.go \(2 comments\)/.test(screen),
    `popup should include comment count for greeting.go; head:\n${screen.split('\n').slice(0, 8).join('\n')}`,
  )
  await quit(s)
})

test('M11: Files popup omits the count when CommentCount is 0; singular for 1', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('j')              // greeting_test.go (1 comment)
  await s.press('j')              // main.go (0 comments)
  await s.type(' ')               // open popup on main.go
  const screen = await s.text()
  assert.ok(
    /│src\/main\.go│/.test(screen),
    `popup with zero comments should show only the path; head:\n${screen.split('\n').slice(0, 8).join('\n')}`,
  )
  await s.press('k')              // back to greeting_test.go; popup follows
  const screen2 = await s.text()
  assert.ok(
    /│src\/greeting_test\.go \(1 comment\)/.test(screen2),
    `singular form for count=1; head:\n${screen2.split('\n').slice(0, 8).join('\n')}`,
  )
  await quit(s)
})

test('M12: <space> in Diff still toggles split⇄unified, not the hover popup', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab')           // Commits
  await s.press('tab')           // Diff
  await s.type(' ')              // toggles split → unified, no popup
  const screen = await s.text()
  assert.ok(/Diff: src\/greeting\.go \[unified\]/.test(screen), `Diff space should still toggle to unified`)
  // Popup must NOT appear (focused pane is Diff, not Files / Commits).
  assert.ok(!hasPopup(screen, /src\/greeting\.go(?!_)/), `Diff space must not open the hover popup`)
  await quit(s)
})
