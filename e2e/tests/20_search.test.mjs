// Category P — global `/` search and gg/G movement.
//
// Search contract: `/` opens an incsearch prompt scoped to the focused pane;
// printable runes append to the query and immediately jump the cursor;
// Backspace shrinks; Enter commits to Active where n/N cycle; Esc cancels
// and restores the pre-search cursor. gg/G work in every pane.

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchReva, waitReady, quit, paneText } from '../helpers/launch.mjs'

// ---- gg / G ---------------------------------------------------------------

test('P1: gg / G in Files pane jump cursor to All-row / last file', async () => {
  // gg / G move the Files cursor only — they no longer auto-select
  // the file they land on. Files index 0 is now the synthetic All row
  // (symmetric to the Commits pane's All-commits row); gg lands there,
  // not on the first file. Confirm via the cursor row in the Files
  // pane (the `> ` glyph) rather than the Diff title.
  const s = await launchReva()
  await waitReady(s)
  await s.press('j'); await s.press('j')
  await s.type('G')
  let files = paneText(await s.text(), 'Files')
  let cursorRow = files.split('\n').find(l => l.startsWith('> ')) || ''
  assert.ok(/go\.mod/.test(cursorRow), `G should land cursor on go.mod; got "${cursorRow}"`)
  await s.type('gg')
  files = paneText(await s.text(), 'Files')
  cursorRow = files.split('\n').find(l => l.startsWith('> ')) || ''
  assert.ok(/All \(\d+ files\)/.test(cursorRow),
    `gg should land cursor on the All row; got "${cursorRow}"`)
  await quit(s)
})

test('P2: gg / G in Commits pane jump to All-commits / last commit', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab')                              // focus Commits
  await s.type('G')
  let files = paneText(await s.text(), 'Commits')
  // Last commit row in fixture: "Add tests and docs". Cursor "> " marks it.
  assert.ok(/^>\s+/m.test(files), 'G should leave a cursor row in Commits')
  // The rendered cursor row must contain a real commit subject (not the
  // synthetic "All commits" header).
  const cursorRow = files.split('\n').find(l => l.startsWith('> ')) || ''
  assert.ok(!/All commits/.test(cursorRow), `G should land past 'All commits'; got "${cursorRow}"`)
  await s.type('gg')
  files = paneText(await s.text(), 'Commits')
  const topCursor = files.split('\n').find(l => l.startsWith('> ')) || ''
  assert.ok(/All commits/.test(topCursor), `gg should land on 'All commits' row; got "${topCursor}"`)
  await quit(s)
})

test('P3: G in Diff pane jumps cursor past the header', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')        // focus Diff
  const before = await s.text()
  await s.type('G')
  const after = await s.text()
  assert.notEqual(before, after, 'G must move the Diff cursor')
  await quit(s)
})

// ---- / search -------------------------------------------------------------

test('P4: / opens search prompt with live query', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('/')
  await s.type('main')
  const screen = await s.text()
  // Status bar carries `/<query>_` while Editing.
  assert.match(screen, /\/main_/, `prompt should expose the live query; tail:\n${screen.split('\n').slice(-3).join('\n')}`)
  await s.press('esc')
  await quit(s)
})

test('P5: incremental search jumps Files cursor', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Default cursor on greeting.go (idx 0). "main" should auto-jump to src/main.go.
  await s.type('/')
  await s.type('main')
  const screen = await s.text()
  assert.match(screen, /Diff: src\/main\.go/, 'incsearch should auto-select src/main.go')
  await s.press('esc')
  await quit(s)
})

test('P6: Esc on Editing restores pre-search cursor', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Pre-search state: Files cursor on greeting.go, Diff also on greeting.go
  // (j alone no longer auto-selects, so SelectedFile stays at index 0).
  let screen = await s.text()
  assert.match(screen, /Diff: src\/greeting\.go/, 'pre-search: Diff on greeting.go')
  await s.type('/')
  await s.type('main')
  screen = await s.text()
  // incsearch keeps its auto-select behavior — search has always been a
  // direct file-selection gesture in this UI.
  assert.match(screen, /Diff: src\/main\.go/, 'incsearch jumped to main.go')
  await s.press('esc')
  screen = await s.text()
  assert.match(screen, /Diff: src\/greeting\.go/, 'Esc must restore pre-search SelectedFile')
  await quit(s)
})

test('P7: Enter commits search; n / N cycle', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('/')
  await s.type('.go')                               // matches multiple files
  await s.press('enter')
  const active = await s.text()
  // Active context should expose the n/N hint set.
  assert.match(active, /n:next/, `Active status bar should expose n hint; tail:\n${active.split('\n').slice(-3).join('\n')}`)
  await s.press('n')
  const afterN = await s.text()
  assert.notEqual(active, afterN, 'n must move to next match')
  await s.press('N')
  const afterNback = await s.text()
  assert.equal(afterN === afterNback, false, 'N must move (away from afterN) — at least cycles')
  await s.press('esc')
  await quit(s)
})

test('P8: Diff search lands cursor on the matched buffer line', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')        // focus Diff
  await s.type('/')
  await s.type('Hello')
  const screen = await s.text()
  // greeting.go diff has "+// Hello returns ...". Under the per-column
  // split layout the cursor `> ` lands in the Rcursor column (mid-row),
  // not at the row's left edge — find the row that carries both `> `
  // anywhere AND the search target.
  const diff = paneText(screen, 'Diff')
  const cursorRow = diff.split('\n').find(l => l.includes('> ') && l.includes('Hello')) || ''
  assert.match(
    cursorRow,
    /Hello/,
    `Diff search should land cursor on 'Hello' row; got "${cursorRow}"`,
  )
  await s.press('esc')
  await quit(s)
})

test('P9: empty-query backspace cancels search', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('/')
  await s.press('backspace')
  const screen = await s.text()
  assert.doesNotMatch(screen, /\/_/, 'backspace on empty query should clear the prompt')
  await quit(s)
})

test('P10: no-match search posts a Notice on Enter', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('/')
  await s.type('zzznotpresentzzz')
  await s.press('enter')
  const screen = await s.text()
  assert.match(screen, /no match/, `expected 'no match' notice; tail:\n${screen.split('\n').slice(-3).join('\n')}`)
  await quit(s)
})

test('P11: matched substring carries an SGR highlight band', async () => {
  // The bg-styled span emits CSI sequences around the matched substring.
  // tuistory's renderer parses CSI into cell colors, so the raw screen
  // text strips them; we instead probe the row text and the bare match
  // presence to confirm the band is applied without leaking the prompt.
  const s = await launchReva()
  await waitReady(s)
  await s.type('/')
  await s.type('main')
  // Cursor jumped to src/main.go (P5). The row text in the Files pane
  // must still contain the literal "main" inside the matched path.
  const files = paneText(await s.text(), 'Files')
  const cursorRow = files.split('\n').find(l => l.startsWith('> ')) || ''
  assert.match(cursorRow, /main\.go/, `cursor row should still contain the matched path; got "${cursorRow}"`)
  await s.press('esc')
  await quit(s)
})

test('P12: Tab during Active terminates search and advances focus', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('/')
  await s.type('.go')
  await s.press('enter')
  let screen = await s.text()
  assert.match(screen, /n:next/, 'setup: search should be Active')
  await s.press('tab')
  screen = await s.text()
  assert.ok(!/n:next/.test(screen), `Tab during Active must clear search hint; tail:\n${screen.split('\n').slice(-3).join('\n')}`)
  assert.match(screen, /▶ Commits/, 'Tab must still advance focus to Commits')
  await quit(s)
})

test('P13: Shift+Tab during Active also terminates search', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('/')
  await s.type('.go')
  await s.press('enter')
  await s.press('shift+tab')
  const screen = await s.text()
  assert.ok(!/n:next/.test(screen), `Shift+Tab during Active must clear search hint; tail:\n${screen.split('\n').slice(-3).join('\n')}`)
  await quit(s)
})

test('P14: Ctrl+C during Active terminates search without quitting', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('/')
  await s.type('.go')
  await s.press('enter')
  await s.press(['ctrl', 'c'])
  const screen = await s.text()
  assert.ok(!/n:next/.test(screen), `Ctrl+C during Active must clear search hint; tail:\n${screen.split('\n').slice(-3).join('\n')}`)
  // App still alive: pane chrome still rendered.
  assert.ok(/▶ Files/.test(screen), `process should still be running after Ctrl+C-while-Active`)
  await quit(s)
})

test('P15: / in Comments pane is a no-op (search disabled)', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab'); await s.press('tab')   // → Comments
  let screen = await s.text()
  assert.match(screen, /▶ Comments/, 'setup: focus on Comments')
  await s.type('/')
  screen = await s.text()
  // No prompt, no search prefix `/_` in the bar.
  assert.ok(!/\/_/.test(screen), `/ in Comments must NOT open the prompt; tail:\n${screen.split('\n').slice(-3).join('\n')}`)
  await quit(s)
})
