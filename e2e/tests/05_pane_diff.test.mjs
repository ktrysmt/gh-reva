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
  // Added line "// Hello returns" — new line 3, no old line. Right gutter
  // sits past `│` + Rmarker + Rcursor cols (the new per-column layout
  // adds 4 cols between `│` and newLn). The marker on this row is
  // ◆ (carried by comment 1001 on greeting.go new line 3); the regex
  // is intentionally loose about what fills the per-side gutter so a
  // future glyph swap doesn't fragment this assertion.
  assert.match(
    diff,
    /│[^0-9]*3\s+\+\/\/ Hello returns/,
    `added line should show new-line gutter on the right side; Diff:\n${diff}`,
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

test('F4d: single g is a no-op (it is the prefix of `gg`)', async () => {
  // Vim-correct semantics: a lone `g` waits for a follow-up. Only the second
  // `g` triggers gotoTop. This is also forward-compatible with `gd` / `gh` /
  // `gb` style mappings, which would all share the same pending-prefix slot.
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  for (let i = 0; i < 5; i++) await s.press('j')   // move cursor away from top
  const before = await s.text()
  await s.press('g')                                // sets pending; no view change
  const after = await s.text()
  assert.equal(before, after, 'single g must NOT move the Diff cursor; it is the prefix of `gg`')
  await s.press('g')                                // completes the sequence
  const atTop = await s.text()
  assert.notEqual(after, atTop, 'second g must complete the gg jump to top')
  await quit(s)
})

test('F4e: g + non-g cancels the pending prefix and dispatches the second key', async () => {
  // After `g` is pending, a non-`g` key must cancel pending AND act normally
  // (k moves cursor up by one). It must NOT cause a top jump.
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')
  for (let i = 0; i < 5; i++) await s.press('j')
  const atFive = await s.text()
  await s.press('g')         // pending
  await s.press('k')         // cancels pending; k moves cursor up
  const afterK = await s.text()
  assert.notEqual(atFive, afterK, 'k after pending g must move the cursor up')
  // Take an explicit "true top" snapshot for comparison.
  await s.type('gg')
  const atTop = await s.text()
  assert.notEqual(afterK, atTop, 'g + k must NOT have jumped to top; afterK should differ from atTop')
  await quit(s)
})

test('F4f: pending g is cleared on Tab focus change', async () => {
  // Leaking the pending prefix across panes would surprise the user — pressing
  // g, switching focus, and pressing g again would unexpectedly jump to top.
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  for (let i = 0; i < 5; i++) await s.press('j')
  await s.press('g')                            // pending
  await s.press('tab')                          // focus → Comments (must clear)
  await s.press('shift+tab')                    // back to Diff
  const before = await s.text()
  await s.press('g')                            // fresh single g — must be no-op
  const after = await s.text()
  assert.equal(before, after, 'pending g must be cleared on focus change; subsequent single g is a no-op')
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

test('F7: Enter on a commented diff line shifts focus to the Comments pane', async () => {
  // The Diff Enter handoff was simplified: rather than opening the
  // Comments zoom modal (whose existence pre-dated the Ctrl+E column
  // toggle), Enter now plain-shifts focus to the Comments pane. The
  // user can press Space from there to zoom; compose must NOT fire.
  const s = await launchReva({ env: { EDITOR: '', VISUAL: '' } })
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  // Fixture comment 1001 (greeting.go) is anchored at new-file line 3 →
  // buffer index 5.
  for (let i = 0; i < 5; i++) await s.type('j')
  await s.press('enter')
  await s.waitForText('▶ Comments', { timeout: 5000 })
  const screen = await s.text()
  assert.ok(!/│ Comments\s+│/.test(screen),
    `Diff Enter must NOT open the zoom modal anymore; tail:\n${screen.split('\n').slice(-12).join('\n')}`)
  assert.ok(!/New comment|Reply|Edit comment/.test(screen),
    `compose modal must not open on Diff Enter`)
  await quit(s)
})

test('F8: Enter on a header / hunk row is still a no-op', async () => {
  // Buffer 0 is `--- a/src/greeting.go` (file header). buildComposeInline
  // rejects header / hunk rows, so Enter cannot open compose there.
  const s = await launchReva({ env: { EDITOR: '', VISUAL: '' } })
  await waitReady(s)
  await s.press('tab'); await s.press('tab')
  const before = await s.text()
  await s.press('enter')
  const after = await s.text()
  assert.equal(before, after, 'Enter on a header line must be a no-op')
  // Modal must NOT have appeared.
  assert.ok(!/New comment/.test(after), 'header row Enter must not open compose')
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
  // Per-column layout: the `>` sits in the Rcursor column (between Rmarker
  // and newLn) for a RIGHT-side cursor. Just check that `> ` appears
  // somewhere on the head row — the leading-`> ` form was the pre-Side
  // layout where Lcursor sat at col 0.
  assert.ok(lines[headRow].includes('> '), `head row must carry "> " somewhere; got "${lines[headRow]}"`)
  const contRow = lines.findIndex(l => /given name/.test(l))
  assert.ok(contRow >= 0, `expected wrap continuation; Diff:\n${diff}`)
  assert.ok(
    !lines[contRow].includes('> '),
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
  const contRow = lines.findIndex(l => /given name/.test(l))
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
  const contRow = lines.findIndex(l => /given name/.test(l))
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

test('F18: multi-line range comment renders ┌/│/◆ gutter on greeting_test.go', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  await s.type('J')                            // advance to greeting_test.go
  await s.press('space')                       // unified mode for tight gutter
  const diff = paneText(await s.text(), 'Diff')
  const lines = diff.split('\n')
  // Range comment 1005 (start_line=5, line=10, RIGHT) on greeting_test.go.
  // file lines 5..10 inside the diff buffer:
  //   5: +func TestHello(t *testing.T) {  ← ┌
  //   6: +\tgot := Hello("world")          ← │
  //   7: +\twant := "Hello, world"         ← │
  //   8: +\tif got != want {               ← │
  //   9: +\t\tt.Errorf(...)                ← │
  //  10: +\t}                              ← ◆
  // Single-line comment 1004 still anchors at file line 11 (closing `}`).
  const rowFunc   = lines.find(l => l.includes('func TestHello'))
  const rowGot    = lines.find(l => l.includes('got := Hello'))
  const rowWant   = lines.find(l => l.includes('want := '))
  const rowIf     = lines.find(l => l.includes('if got != want'))
  const rowErrorf = lines.find(l => l.includes('t.Errorf'))
  assert.ok(rowFunc,   `expected diff row containing "func TestHello"; Diff:\n${diff}`)
  assert.ok(rowGot,    `expected diff row containing "got := Hello"`)
  assert.ok(rowWant,   `expected diff row containing "want :="`)
  assert.ok(rowIf,     `expected diff row containing "if got != want"`)
  assert.ok(rowErrorf, `expected diff row containing "t.Errorf"`)
  assert.ok(rowFunc.includes('┌'),  `range start row must show ┌; got "${rowFunc}"`)
  assert.ok(rowGot.includes('│'),    `range middle row must show │; got "${rowGot}"`)
  assert.ok(rowWant.includes('│'),   `range middle row must show │; got "${rowWant}"`)
  assert.ok(rowIf.includes('│'),     `range middle row must show │; got "${rowIf}"`)
  assert.ok(rowErrorf.includes('│'), `range middle row must show │; got "${rowErrorf}"`)
  // Range end (file line 10, "+\t}") collides with single-line cmt 1004 (line 11).
  // We assert that the closer of the two `}` lines (file line 10) carries ◆.
  // Both `+}` style rows exist (10 and 11); the count of ◆ rows must be ≥ 2.
  const diamondRows = lines.filter(l => l.includes('◆'))
  assert.ok(
    diamondRows.length >= 2,
    `expected at least 2 ◆ rows (range end + single-line); got ${diamondRows.length}; Diff:\n${diff}`,
  )
  // No ◆ on the range start row (must be ┌).
  assert.ok(!rowFunc.includes('◆'), `range start row must NOT carry ◆; got "${rowFunc}"`)
  await quit(s)
})

test('F18b: narrow pane keeps │ on continuation rows of wrapped range lines', async () => {
  // Reproduces the bug where a multi-line range comment's gutter line
  // visibly broke when a `┌` / `│` buffer line wrapped to multiple
  // display rows: the wrapped continuation rows had blank gutters,
  // fragmenting the range visual on narrow terminals.
  const s = await launchReva({ cols: 80 })   // forces unified (F3) + wraps
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  await s.type('J')                            // advance to greeting_test.go
  const diff = paneText(await s.text(), 'Diff')
  const lines = diff.split('\n')
  // Range comment 1005 (start_line=5, line=10, RIGHT). At cols=80 the
  // unified content area is narrow enough to wrap the `t.Errorf(...)`
  // row (file line 9, a markerMiddle row) to at least 2 display rows.
  const errIdx = lines.findIndex(l => l.includes('t.Errorf'))
  assert.ok(errIdx >= 0, `expected diff row containing "t.Errorf"; Diff:\n${diff}`)
  assert.ok(lines[errIdx].includes('│'), `range middle row must show │; got "${lines[errIdx]}"`)
  const cont = lines[errIdx + 1]
  assert.ok(cont !== undefined, `expected at least one continuation row; Diff:\n${diff}`)
  // Continuation row of a markerMiddle row must redraw │ in the gutter so
  // the multi-line range visual stays connected. Without the fix this row
  // was a 5-space indent — the gutter line broke here.
  assert.ok(
    cont.includes('│'),
    `continuation of wrapped range row must keep │; got "${cont}" (prev="${lines[errIdx]}")`,
  )
  await quit(s)
})

test('F19: status bar shows the Side tag ([A] default, [B] after h)', async () => {
  // The Diff hint composes a leading [A]/[B] tag (RIGHT/after vs
  // LEFT/before) so the user always sees which column the cursor is
  // parked on without opening Help. l switches back to RIGHT.
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  let screen = await s.text()
  assert.match(screen, /\[A\] h\/l:side/, `default Diff hint must show [A] tag with h/l binding; tail:\n${screen.split('\n').slice(-3).join('\n')}`)
  await s.type('h')
  screen = await s.text()
  assert.match(screen, /\[B\] h\/l:side/, `after h, hint must flip to [B]; tail:\n${screen.split('\n').slice(-3).join('\n')}`)
  await s.type('l')
  screen = await s.text()
  assert.match(screen, /\[A\] h\/l:side/, `after l, hint must restore [A]; tail:\n${screen.split('\n').slice(-3).join('\n')}`)
  await quit(s)
})

test('F20: j on RIGHT skips a `-` row (auto-skip per side)', async () => {
  // main.go is the only fixture file with a `-` row inside the diff
  // buffer (`-import "fmt"` at buffer index 4). Cursor in RIGHT mode
  // must hop over it.
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // focus Diff
  await s.type('J'); await s.type('J')         // greeting.go → greeting_test.go → main.go
  let screen = await s.text()
  assert.match(screen, /Diff: src\/main\.go/, 'expected main.go to be selected')
  // Walk down to the `package main` context row (buffer index 3 from top).
  await s.type('g'); await s.type('g')         // gg → top of side
  // After gg in RIGHT mode the cursor lands on the first RIGHT-existing
  // row (the file header `--- a/src/main.go` is still considered
  // header / both-sides per lineExistsOnSide). Step down to context.
  await s.type('j')                             // 0 → 1
  await s.type('j')                             // 1 → 2 (@@)
  await s.type('j')                             // 2 → 3 (` package main`)
  await s.type('j')                             // 3 → SKIP `-import "fmt"`, land on `+import (`
  const diff = paneText(await s.text(), 'Diff')
  // Cursor (`> `) must sit on the RIGHT-cursor column of the `+import (` row.
  // Look for the row containing `import (` — the cursor marker should be on
  // its physical row.
  const lines = diff.split('\n')
  const cursorRowIdx = lines.findIndex(l => l.includes('> ') && l.includes('import ('))
  assert.ok(
    cursorRowIdx >= 0,
    `expected cursor on the \`+import (\` row (RIGHT-side after auto-skip); Diff:\n${diff}`,
  )
  // The skipped `-import "fmt"` row exists in the buffer but must NOT
  // carry the cursor.
  const minusRow = lines.find(l => l.includes('-import "fmt"') || l.includes('import "fmt"'))
  if (minusRow) {
    assert.ok(!minusRow.startsWith('> '), `cursor must not land on the skipped \`-\` row; got "${minusRow}"`)
  }
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
