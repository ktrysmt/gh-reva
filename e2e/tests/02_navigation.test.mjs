// Category C — Pane traversal (tab / shift-tab only).

import { test, describe, before, after } from 'node:test'
import assert from 'node:assert/strict'

import { launchReva, waitReady, quit, activePaneLabel, paneText } from '../helpers/launch.mjs'

// C1/C4/C5/C7 all begin and end at Files focus and do not mutate cursors or
// SelectedFile, so they can share a single launched session in sequence.
describe('C1+C4+C5+C7: Files-stable navigation sequences (shared launch)', () => {
  let session
  before(async () => {
    session = await launchReva()
    await waitReady(session)
  })
  after(async () => { await quit(session) })

  test('C1: tab cycles Files → Commits → Diff → Comments → Files', async () => {
    assert.equal(await activePaneLabel(session), 'Files')
    await session.press('tab'); assert.equal(await activePaneLabel(session), 'Commits')
    await session.press('tab'); assert.equal(await activePaneLabel(session), 'Diff')
    await session.press('tab'); assert.equal(await activePaneLabel(session), 'Comments')
    await session.press('tab'); assert.equal(await activePaneLabel(session), 'Files')
  })

  test('C4: Backspace is a no-op for pane focus', async () => {
    // Move to Comments via tab, then press backspace repeatedly. Focus must
    // remain on Comments — backspace no longer steps panes backward.
    await session.press('tab'); await session.press('tab'); await session.press('tab')
    assert.equal(await activePaneLabel(session), 'Comments')
    await session.press('backspace'); assert.equal(await activePaneLabel(session), 'Comments')
    await session.press('backspace'); assert.equal(await activePaneLabel(session), 'Comments')
    // Restore Files focus for the shared session.
    await session.press('tab'); assert.equal(await activePaneLabel(session), 'Files')
  })

  test('C5: Backspace mash from Comments leaves focus on Comments', async () => {
    await session.press('tab'); await session.press('tab'); await session.press('tab')
    for (let i = 0; i < 10; i++) await session.press('backspace')
    assert.equal(await activePaneLabel(session), 'Comments')
    // Restore Files focus.
    await session.press('tab'); assert.equal(await activePaneLabel(session), 'Files')
  })

  test('C7: numeric keys 1/2/3/4 jump directly to Files/Commits/Diff/Comments', async () => {
    // Start state: Files focused (restored at the end of each preceding
    // case). 1/2/3/4 jump unconditionally regardless of current focus.
    await session.type('2'); assert.equal(await activePaneLabel(session), 'Commits')
    await session.type('4'); assert.equal(await activePaneLabel(session), 'Comments')
    await session.type('3'); assert.equal(await activePaneLabel(session), 'Diff')
    await session.type('1'); assert.equal(await activePaneLabel(session), 'Files')
  })
})

test('C9: 4 reveals the Comments pane when hidden and focuses it', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Hide Comments first.
  await s.press(['ctrl', 'e'])
  let screen = await s.text()
  assert.ok(!/ Comments\b/.test(screen), 'pre-condition: Comments pane is hidden')
  // 4 must reveal AND focus the Comments column.
  await s.type('4')
  screen = await s.text()
  assert.ok(/ Comments\b/.test(screen), '4 must reveal the Comments column when hidden')
  assert.equal(await activePaneLabel(s), 'Comments', '4 must focus Comments after reveal')
  await quit(s)
})

test('C10: 1/2/3/4 cancel Visual mode and jump', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Enter Visual on Files (FilesCursor is on a real row after Shift+J in waitReady).
  await s.type('v')
  let screen = await s.text()
  assert.match(screen, /-- VISUAL --/, 'pre-condition: visual mode active on Files')
  // 3 must cancel visual AND land on Diff.
  await s.type('3')
  screen = await s.text()
  assert.ok(!/-- VISUAL --/.test(screen), '1-4 must cancel visual mode')
  assert.equal(await activePaneLabel(s), 'Diff', '3 must focus Diff')
  await quit(s)
})

test('C2: shift-tab cycles Files → Comments → Diff → Commits → Files', { skip: 'tuistory cannot reliably emit CSI Z (back-tab): the ["shift","tab"] chord is a no-op and `s.type("\\x1b[Z")` arrives as 3 separate key events (ESC, [, Z) due to typing-simulation pacing. The bubbletea handler (`case "shift+tab"` in keys.go) is verified correct by inspection and works against a real terminal.' }, async () => {
  // Manual reproduction (until tuistory grows raw-write support):
  //   gh-reva --fixture testdata/sample-pr.json
  //   <press Shift+Tab> — focus must cycle Files → Comments → Diff → Commits → Files.
})

test('C3: Enter on Commits is a no-op for focus (only Files Enter and Tab move panes)', async () => {
  // Files Enter is the deliberate "commit selection + go to Diff" gesture
  // — verified in 03_pane_files D6. Other panes' Enter are no-ops for
  // focus: Commits Enter no-ops, Diff Enter opens compose / focus
  // shifts to Comments depending on the row, Comments Enter
  // edits / replies. Pin only the Commits no-op here so the contract
  // stays observable from the navigation suite.
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab')
  assert.equal(await activePaneLabel(s), 'Commits')
  await s.press('enter')
  assert.equal(await activePaneLabel(s), 'Commits', 'Enter on Commits must not focus Diff')
  await quit(s)
})

test('C6: cursors persist across Tab navigation', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Stay on greeting.go (cursor=0, default) so the Commits pane has 2 real
  // entries (aaa1111 + bbb2222) plus the leading All commits virtual row.
  // Move to Commits via Tab, advance cursor twice (All commits → aaa1111 →
  // bbb2222), hop away through the Tab cycle, return — cursor stays on bbb2222.
  await s.press('tab')         // → Commits (cursor on All commits row)
  await s.type('j')           // → aaa1111 (auto-selects aaa1111)
  await s.type('j')           // → bbb2222 (auto-selects bbb2222)
  // Cycle: Commits → Diff → Comments → Files → Commits.
  await s.press('tab'); await s.press('tab'); await s.press('tab'); await s.press('tab')
  assert.equal(await activePaneLabel(s), 'Commits')
  const commits = paneText(await s.text(), 'Commits')
  assert.match(commits, /^>[^\n]*bbb2222/m, 'Commits cursor should remain on bbb2222 after the Tab cycle')
  await quit(s)
})

test('C8: Shift+J/K navigates files from any pane (focus preserved)', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Files: J → greeting.go → greeting_test.go
  let screen = await s.text()
  assert.match(screen, /▶ Files/, 'starts on Files')
  assert.match(screen, /Diff: src\/greeting\.go/, 'starts at greeting.go')
  await s.type('J')
  screen = await s.text()
  assert.match(screen, /Diff: src\/greeting_test\.go/, 'J from Files advances')
  assert.match(screen, /▶ Files/, 'focus stays on Files')
  // Commits: J → main.go
  await s.press('tab')
  assert.equal(await activePaneLabel(s), 'Commits')
  await s.type('J')
  screen = await s.text()
  assert.match(screen, /Diff: src\/main\.go/, 'J from Commits advances')
  assert.match(screen, /▶ Commits/, 'focus stays on Commits')
  // Diff: J → go.mod (tree order: …→ main.go → go.mod is last). Also
  // covered by F10 — kept here for cross-pane sanity.
  await s.press('tab')
  assert.equal(await activePaneLabel(s), 'Diff')
  await s.type('J')
  screen = await s.text()
  assert.match(screen, /Diff: go\.mod/, 'J from Diff advances')
  assert.match(screen, /▶ Diff/, 'focus stays on Diff')
  // Comments: K → main.go (back)
  await s.press('tab')
  assert.equal(await activePaneLabel(s), 'Comments')
  await s.type('K')
  screen = await s.text()
  assert.match(screen, /Diff: src\/main\.go/, 'K from Comments goes back')
  assert.match(screen, /▶ Comments/, 'focus stays on Comments')
  await quit(s)
})
