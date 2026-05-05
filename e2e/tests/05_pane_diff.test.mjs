// Category F — Diff pane.

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchReva, waitReady, quit, paneText } from '../helpers/launch.mjs'

test('F1: Diff header shows current file + [split] by default', async () => {
  const s = await launchReva()
  await waitReady(s)
  // selectedFile auto = greeting.go at startup; PR-wide diff is shown.
  const screen = await s.text()
  assert.match(screen, /Diff: src\/greeting\.go \[split\]/)
  await quit(s)
})

test('F2: <space> toggles split ⇄ unified inside Diff pane', async () => {
  const s = await launchReva()
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

test('F2c: split mode shows old/new line numbers per side', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  const diff = paneText(await s.text(), 'Diff')
  // Context line " package src" maps to old=1 / new=1.  Both gutters render it.
  // Format per side: <right-padded ln 4 cols><space><content>.
  assert.match(
    diff,
    /1\s+package src.*│.*1\s+package src/,
    `context line should expose both old and new line numbers (=1); Diff:\n${diff}`,
  )
  // Added line "// Hello returns" — new line 3, no old line. Right gutter shows 3.
  assert.match(
    diff,
    /│\s*3\s+\+\/\/ Hello returns/,
    `added line should show new-line gutter on the right side`,
  )
  await quit(s)
})

test('F2d: split mode keeps │ vertically aligned across rows with tabs', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  const diff = paneText(await s.text(), 'Diff')
  const sepRows = diff.split('\n').filter(l => l.includes('│'))
  // All rows containing │ must place it at the same column. Tabs in source
  // code (Go uses tabs for indentation) must be expanded so rune count tracks
  // display width.
  const cols = new Set(sepRows.map(l => l.indexOf('│')))
  assert.equal(
    cols.size,
    1,
    `│ should align across all split rows; got column positions ${[...cols]} in:\n${diff}`,
  )
  await quit(s)
})

test('F2b: split mode renders the diff content in two columns separated by │', async () => {
  const s = await launchReva()  // default cols=160 → split mode wide enough
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  let diff = paneText(await s.text(), 'Diff')
  // Confirm we are in split mode initially.
  assert.match(diff, /\[split\]/, 'expect default split mode')
  // At least one diff row must carry the split separator.
  assert.ok(diff.includes('│'), `split mode rows should carry │; Diff slice:\n${diff}`)
  // Toggle to unified — separator must disappear.
  await s.press('space')
  diff = paneText(await s.text(), 'Diff')
  assert.match(diff, /\[unified\]/, 'expect unified after toggle')
  assert.ok(!diff.includes('│'), `unified mode must not contain split separator; Diff slice:\n${diff}`)
  await quit(s)
})

test('F3: narrow terminals fall back to unified', async () => {
  const s = await launchReva({ cols: 80 })
  await waitReady(s)
  const diff = paneText(await s.text(), 'Diff')
  assert.ok(diff.includes('[unified]'), `expected unified tag in Diff column; slice:\n${diff}`)
  await quit(s)
})

test('F4: vertical motion j/k updates Diff state', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  const before = await s.text()
  await s.type('j')
  const after = await s.text()
  assert.notEqual(before, after, 'j should move the Diff cursor (screen must change)')
  await quit(s)
})

test('F4b: gg jumps to first line, G to last', async () => {
  const s = await launchReva()
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
  const s = await launchReva()
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

test('F6: H jumps to viewport top after G scrolls down', async () => {
  // --diff-height=4 pins a tiny viewport so G actually scrolls Top away from
  // 0, letting H stand observably apart from gg (which would jump to the
  // file's first line).
  const s = await launchReva({ args: ['--diff-height', '4'] })
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

test('F7: Enter on a commented line is a no-op (Phase 2 will rebind to comment input)', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  // Fixture comment 1001 (greeting.go) is anchored at new-file line 3, which
  // maps to buffer index 5 in the PR-wide patch. Even on this anchored row,
  // Enter must not shift focus to Comments — the legacy "drill into thread"
  // gesture was retired with the column-movement cleanup.
  for (let i = 0; i < 5; i++) await s.type('j')
  const before = await s.text()
  await s.press('enter')
  const after = await s.text()
  assert.equal(before, after, 'Enter on a commented line must be a no-op')
  assert.match(after, /▶ Diff/, 'focus must remain on Diff')
  await quit(s)
})

test('F8: Enter on a non-commented line is a no-op', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')
  // First line of the diff (file header) has no comment by construction.
  const before = await s.text()
  await s.press('enter')
  const after = await s.text()
  assert.equal(before, after, 'Enter on a non-commented line must be a no-op')
  await quit(s)
})

test('F11: Diff lines with comments show a ◆ gutter marker (left of content)', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  // Force unified mode so the regex below asserts ◆ adjacent to the diff
  // content. (Split mode pads the left half with spaces, pushing `+//` to the
  // right of `│` — F2b covers that layout.)
  await s.press('space')
  const screen = await s.text()
  const diff = paneText(screen, 'Diff')
  // Comment 1001 anchored at greeting.go new line 3 → buffer line 5
  // ("+// Hello returns ..."). Gutter format: <cursor 2><marker 2><content>,
  // where marker is "◆ " on commented lines, "  " otherwise.
  assert.match(
    diff,
    /◆\s+\+\/\/\s*Hello returns a greeting/,
    `commented line should carry the ◆ marker before its diff content; Diff slice:\n${diff}`,
  )
  // The blank "+" addition at buffer line 4 has no comment — must NOT have ◆.
  const blankPlusLine = diff.split('\n').find(l => /^\s+\+\s*$/.test(l))
  assert.ok(blankPlusLine, `blank "+" addition line should be visible; Diff slice:\n${diff}`)
  assert.ok(
    !blankPlusLine.includes('◆'),
    `non-commented blank "+" must NOT carry ◆ (got "${blankPlusLine}")`,
  )
  await quit(s)
})

test('F10: Shift+J/K in Diff cycles files while keeping Diff focus', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  let screen = await s.text()
  assert.match(screen, /▶ Diff/, 'focus on Diff before navigation')
  assert.match(screen, /Diff: src\/greeting\.go/, 'starts on greeting.go')
  // Shift+J → next file (greeting_test.go). Focus must remain on Diff.
  await s.type('J')
  screen = await s.text()
  assert.match(screen, /Diff: src\/greeting_test\.go/, 'Shift+J advances to next file')
  assert.match(screen, /▶ Diff/, 'focus stays on Diff after Shift+J')
  // Shift+K → previous file.
  await s.type('K')
  screen = await s.text()
  assert.match(screen, /Diff: src\/greeting\.go/, 'Shift+K returns to previous file')
  assert.match(screen, /▶ Diff/, 'focus stays on Diff after Shift+K')
  await quit(s)
})

test('F10b: Shift+K at first file and Shift+J at last file are clamped', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  // At greeting.go (first file). Shift+K must be a no-op for SelectedFile.
  await s.type('K')
  let screen = await s.text()
  assert.match(screen, /Diff: src\/greeting\.go/, 'Shift+K at first file does not wrap')
  // Walk to the last file (5 files in fixture: 0..4 → 4 forward steps).
  await s.type('J'); await s.type('J'); await s.type('J'); await s.type('J')
  screen = await s.text()
  assert.match(screen, /Diff: go\.mod/, 'reached last file (go.mod)')
  // Shift+J at last file must clamp.
  await s.type('J')
  screen = await s.text()
  assert.match(screen, /Diff: go\.mod/, 'Shift+J at last file does not wrap')
  await quit(s)
})

test('F12: split mode wraps a long content line into multiple display rows', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  // greeting.go buffer line 5 = `+// Hello returns a greeting for the given name.`
  // (48 chars). At default cols=160, split halfW=21, so the line wraps to 3
  // display rows. The cell tail "name." lands in the second continuation row.
  const diff = paneText(await s.text(), 'Diff')
  assert.ok(
    /name\./.test(diff),
    `wrap continuation should expose text past the truncation; Diff slice:\n${diff}`,
  )
  await quit(s)
})

test('F13: cursor `>` appears only on the first display row of a wrapped line', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  // Move cursor to the long-wrapping buffer line 5.
  for (let i = 0; i < 5; i++) await s.type('j')
  const diff = paneText(await s.text(), 'Diff')
  const lines = diff.split('\n')
  const headRow = lines.findIndex(l => l.includes('Hello returns'))
  assert.ok(headRow >= 0, `expected first row of wrap; Diff:\n${diff}`)
  assert.ok(lines[headRow].startsWith('> '), `head row must carry "> "; got "${lines[headRow]}"`)
  // Continuation row carries the wrap of "...for the given..." (split halfW=22).
  const contRow = lines.findIndex(l => /for the give/.test(l))
  assert.ok(contRow >= 0, `expected wrap continuation; Diff:\n${diff}`)
  assert.ok(
    !lines[contRow].startsWith('> '),
    `continuation row must not carry "> " (cursor lives on the buffer line, not display row); got "${lines[contRow]}"`,
  )
  await quit(s)
})

test('F15: ◆ marker appears only on the first display row of a wrapped commented line', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  // Buffer line 5 carries comment 1001 and wraps in default split layout.
  const diff = paneText(await s.text(), 'Diff')
  const lines = diff.split('\n')
  const headRow = lines.findIndex(l => l.includes('Hello returns'))
  assert.ok(headRow >= 0, `expected wrap row; Diff:\n${diff}`)
  assert.ok(lines[headRow].includes('◆'), `first row of commented wrapped line must show ◆; got "${lines[headRow]}"`)
  const contRow = lines.findIndex(l => /for the give/.test(l))
  assert.ok(contRow >= 0, `expected wrap continuation; Diff:\n${diff}`)
  assert.ok(
    !lines[contRow].includes('◆'),
    `continuation row must not show ◆ (◆ is per-buffer-line, not per-display-row); got "${lines[contRow]}"`,
  )
  await quit(s)
})

test('F16: split `│` separator continues on every continuation display row', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')
  const diff = paneText(await s.text(), 'Diff')
  const lines = diff.split('\n')
  const headRow = lines.findIndex(l => l.includes('Hello returns'))
  assert.ok(headRow >= 0)
  const firstCol = lines[headRow].indexOf('│')
  assert.ok(firstCol >= 0, 'first row of wrapped buffer line must include │')
  const contRow = lines.findIndex(l => /for the give/.test(l))
  assert.ok(contRow >= 0, `expected wrap continuation; Diff:\n${diff}`)
  const contCol = lines[contRow].indexOf('│')
  assert.equal(contCol, firstCol, `│ on continuation row must be at the same column; first=${firstCol} cont=${contCol}; row="${lines[contRow]}"`)
  await quit(s)
})

test('F17: unified mode wraps long lines and indents continuation by 5 cols', async () => {
  const s = await launchReva({ cols: 80 })   // forces unified (F3)
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  // At cols=80 the unified content area is narrow enough to wrap line 5
  // (48-char `+// Hello returns ...`).
  const diff = paneText(await s.text(), 'Diff')
  const lines = diff.split('\n')
  const idx = lines.findIndex(l => l.includes('Hello returns'))
  assert.ok(idx >= 0, `expected wrap of long line in narrow unified mode; Diff:\n${diff}`)
  const cont = lines[idx + 1]
  assert.ok(cont !== undefined, `expected at least one continuation row; Diff:\n${diff}`)
  // Continuation must indent 5 cols (cursor 2 + ◆marker 2 + diff-marker 1).
  assert.match(cont, /^ {5}\S/, `unified continuation must indent 5 cols past the diff marker; got "${cont}"`)
  await quit(s)
})

test('F9: split/unified choice persists across file changes', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // Diff
  await s.press('space')                       // unified
  let screen = await s.text()
  assert.match(screen, /Diff:[^\n]*\[unified\]/)
  // Shift+J advances to the next file from Diff (focus stays on Diff).
  await s.type('J')
  screen = await s.text()
  assert.match(screen, /Diff: src\/greeting_test\.go \[unified\]/, 'unified choice should persist across file changes')
  await quit(s)
})
