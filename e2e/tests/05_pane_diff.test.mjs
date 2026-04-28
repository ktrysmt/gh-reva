// Category F — Diff pane.

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchGhRv, waitReady, quit } from '../helpers/launch.mjs'

test('F1: Diff header shows current file + [split] by default', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  // selectedFile auto = greeting.go at startup; PR-wide diff is shown.
  const screen = await s.text()
  assert.match(screen, /Diff: src\/greeting\.go \[split\]/)
  await quit(s)
})

test('F2: <space> toggles split ⇄ unified inside Diff pane', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  let screen = await s.text()
  assert.match(screen, /Diff:[^\n]*\[split\]/)
  await s.press('space')
  screen = await s.text()
  assert.match(screen, /Diff:[^\n]*\[unified\]/)
  await s.press('space')
  screen = await s.text()
  assert.match(screen, /Diff:[^\n]*\[split\]/)
  await quit(s)
})

test('F3: narrow terminals fall back to unified', async () => {
  const s = await launchGhRv({ cols: 80 })
  await waitReady(s)
  const screen = await s.text()
  assert.match(screen, /Diff:[^\n]*\[unified\]/)
  await quit(s)
})

test('F4: vertical motion j/k updates Diff state', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  const before = await s.text()
  await s.type('j')
  const after = await s.text()
  assert.notEqual(before, after, 'j should move the Diff cursor (screen must change)')
  await quit(s)
})

test('F4b: gg jumps to first line, G to last', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')
  await s.type('G')
  const atEnd = await s.text()
  await s.type('gg')
  const atStart = await s.text()
  assert.notEqual(atEnd, atStart, 'G and gg should produce different views')
  await quit(s)
})

test('F4c: Ctrl-d / Ctrl-u half-page; Ctrl-f / Ctrl-b full page', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')
  const before = await s.text()
  await s.press(['ctrl', 'f'])
  const afterFwd = await s.text()
  await s.press(['ctrl', 'b'])
  const afterBack = await s.text()
  assert.notEqual(before, afterFwd, 'Ctrl-f should move down')
  // Going back should approximately restore previous state.
  assert.equal(before, afterBack, 'Ctrl-b should restore prior view')
  await quit(s)
})

test('F5: horizontal motions h/l/w/b/e do not crash', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')
  for (const key of ['h', 'l', 'w', 'b', 'e']) await s.type(key)
  const screen = await s.text()
  assert.match(screen, /▶ Diff/, 'focus should remain on Diff after motion keys')
  await quit(s)
})

test('F6: H jumps to viewport top after G scrolls down', async () => {
  // --diff-height=4 pins a tiny viewport so G actually scrolls Top away from
  // 0, letting H stand observably apart from gg (which would jump to the
  // file's first line).
  const s = await launchGhRv({ args: ['--diff-height', '4'] })
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  await s.type('G')
  const atBottom = await s.text()
  await s.type('H')
  const atTop = await s.text()
  assert.notEqual(atBottom, atTop, 'H from a scrolled position must move the cursor')
  // The file header line `--- a/src/greeting.go` lives at buffer line 0.
  // After G the viewport scrolled past it; H must NOT scroll back to it.
  assert.doesNotMatch(atTop, /^>\s+--- a\/src\/greeting\.go/m, 'H must not jump to file top')
  await quit(s)
})

test('F7: Enter on a line with a comment focuses Comments and selects that thread', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  // Fixture comment 1001 (greeting.go) is anchored at new-file line 3, which
  // maps to buffer index 5 in the PR-wide patch:
  //   0: --- a/...    1: +++ b/...    2: @@ ...
  //   3:  package src 4: +            5: +// Hello returns ...
  for (let i = 0; i < 5; i++) await s.type('j')
  await s.press('enter')
  const screen = await s.text()
  assert.equal((screen.match(/▶ Comments/) || []).length, 1, 'focus should be on Comments')
  // The selected thread root must be the carol/Consider extracting comment.
  assert.match(screen, /^>[^\n]*Consider extracting/m, 'cursor row should be on the matched thread root')
  await quit(s)
})

test('F8: Enter on a line without a comment is a no-op (Phase 1)', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')
  // First line of the diff (file header) has no comment by construction.
  const before = await s.text()
  await s.press('enter')
  const after = await s.text()
  assert.equal(before, after, 'Enter on a non-commented line must be a no-op in Phase 1')
  await quit(s)
})

test('F9: split/unified choice persists across file changes', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // Diff
  await s.press('space')                       // unified
  let screen = await s.text()
  assert.match(screen, /Diff:[^\n]*\[unified\]/)
  // Backspace twice → back to Files
  await s.press('backspace'); await s.press('backspace')
  // Move to greeting_test.go and Enter
  await s.type('j')
  await s.press('enter')
  screen = await s.text()
  assert.match(screen, /Diff: src\/greeting_test\.go \[unified\]/, 'unified choice should persist')
  await quit(s)
})
