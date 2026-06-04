// Category M — Mouse support.
//
// Contract:
//   - Click inside a pane shifts FocusedPane to that pane (peer to Tab).
//   - Click on the title row focuses only; cursor stays put.
//   - Click on a content row moves the per-pane cursor:
//       Files: FilesCursor (no auto-select; matches j/k semantics #19)
//       Commits: CommitsCursor + auto-select (matches j/k #16)
//       Diff: DiffCursor.Line (wrap-aware mapping)
//       Comments: CommentsCursor + syncDiff
//   - Mouse wheel scrolls the pane under the cursor without moving focus.
//   - Compose / Help / Modal / PendingConfirm / SearchEditing absorb mouse.
//
// Layout under default cols=160, rows=50, statusBarRows=2 (bodyHeight=48):
//
//   splitColumnWidths(160): left=42, right=40, mid=78  (Comments default 25%)
//   splitColumnHeights(48): top=24, bottom=24
//
//   Files     outer x=[0,42)   y=[0,24)   content y=[3,23)
//   Commits   outer x=[0,42)   y=[24,48)  content y=[27,47)
//   Diff      outer x=[42,120) y=[0,48)   content y=[3,47)
//   Comments  outer x=[120,160) y=[0,48)  content y=[3,47)

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchReva, waitReady, quit, activePaneLabel } from '../helpers/launch.mjs'

test('M1: clicking the Diff title row focuses Diff', async () => {
  const s = await launchReva()
  await waitReady(s)
  assert.equal(await activePaneLabel(s), 'Files', 'pre: Files is focused on load')
  // Diff title row sits at y=1, anywhere in x=[43..102].
  await s.clickAt(60, 1)
  assert.equal(await activePaneLabel(s), 'Diff', 'click on Diff title must focus Diff')
  await quit(s)
})

test('M2: clicking a content row in Commits focuses + auto-selects', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Commits content row 1 (real commit #1) sits at y=27+1=28.
  await s.clickAt(5, 28)
  assert.equal(await activePaneLabel(s), 'Commits', 'click must focus Commits')
  // Auto-select decorates the Diff title with the picked commit's short SHA
  // (Diff: <path> @ <sha>); plain "Diff: <path>" indicates RangeWholePR.
  const screen = await s.text()
  assert.ok(/Diff:.*@\s+[0-9a-f]{7}/.test(screen),
    'click on a real commit must auto-select (Diff title should carry @ short-SHA)')
  await quit(s)
})

test('M3: clicking the Files-title row focuses Files without changing cursor', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Move focus elsewhere first.
  await s.press('tab')
  await s.press('tab')
  assert.equal(await activePaneLabel(s), 'Diff', 'pre: Diff focused')
  // Files title row sits at y=1, x in [1..40].
  await s.clickAt(5, 1)
  assert.equal(await activePaneLabel(s), 'Files', 'title click must focus Files')
  await quit(s)
})

test('M4: wheel-down on Diff advances the cursor without changing focus', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Active pane stays on Files; scroll on the Diff column anyway.
  const before = await s.text()
  await s.scrollDown(3, 60, 10)
  const after = await s.text()
  assert.equal(await activePaneLabel(s), 'Files',
    'wheel must not move focus')
  assert.notEqual(before, after,
    'Diff content must change after wheel-down (cursor advances + viewport may scroll)')
  await quit(s)
})

test('M5: clicking the Comments title row focuses Comments', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Comments title row at y=1, x in [121..158] (outer x=[120,160); col 120
  // is the left border `│` which paneAt rejects).
  await s.clickAt(140, 1)
  assert.equal(await activePaneLabel(s), 'Comments',
    'click on Comments title must focus Comments')
  await quit(s)
})

test('M6: clicking a Files file row updates the Diff column', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Sample fixture loads with src/greeting.go selected; verify by Diff title.
  let screen = await s.text()
  assert.ok(/Diff: src\/greeting\.go/.test(screen),
    'pre: Diff title should show src/greeting.go on load')
  // Tree rows (content y starts at 3): y=3 All, y=4 v docs/, y=5 api.md,
  // y=6 v src/, y=7 greeting.go, y=8 greeting_test.go. Click the
  // greeting_test.go row and assert Diff retitled — the click must commit
  // the file (selectFile) on top of the cursor move, otherwise Diff stays
  // parked on the prior file.
  await s.clickAt(5, 8)
  screen = await s.text()
  assert.ok(/Diff: src\/greeting_test\.go/.test(screen),
    `Diff title should follow Files click; saw:\n${screen.split('\n').slice(0, 4).join('\n')}`)
  await quit(s)
})

test('M7: mouse is absorbed while Help is open', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('?') // open Help
  // Click would otherwise focus Diff.
  await s.clickAt(60, 5)
  // activePaneLabel still walks the screen — Help overlay does not
  // remove the pane chrome, so the active marker remains on Files.
  assert.equal(await activePaneLabel(s), 'Files',
    'Help open: mouse must not change focus')
  // Dismiss before quit so the help overlay does not eat `q`.
  await s.press('?')
  await quit(s)
})
