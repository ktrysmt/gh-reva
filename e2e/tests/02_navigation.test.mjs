// Category C — Pane traversal (tab / shift-tab / Enter / Backspace / numeric).

import { test, describe, before, after } from 'node:test'
import assert from 'node:assert/strict'

import { launchGhRv, waitReady, quit, activePaneLabel, paneText } from '../helpers/launch.mjs'

// C1/C4/C5/C7 all begin and end at Files focus and do not mutate cursors or
// SelectedFile, so they can share a single launched session in sequence.
describe('C1+C4+C5+C7: Files-stable navigation sequences (shared launch)', () => {
  let session
  before(async () => {
    session = await launchGhRv()
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

  test('C4: Backspace returns Comments → Diff → Commits → Files', async () => {
    await session.press('tab'); await session.press('tab'); await session.press('tab')
    assert.equal(await activePaneLabel(session), 'Comments')
    await session.press('backspace'); assert.equal(await activePaneLabel(session), 'Diff')
    await session.press('backspace'); assert.equal(await activePaneLabel(session), 'Commits')
    await session.press('backspace'); assert.equal(await activePaneLabel(session), 'Files')
    await session.press('backspace'); assert.equal(await activePaneLabel(session), 'Files')
  })

  test('C5: Backspace mash from Comments lands at Files', async () => {
    await session.press('tab'); await session.press('tab'); await session.press('tab')
    for (let i = 0; i < 10; i++) await session.press('backspace')
    assert.equal(await activePaneLabel(session), 'Files')
  })

  test('C7: numeric keys (1-4) do not jump panes', async () => {
    for (const key of ['1', '2', '3', '4']) await session.type(key)
    assert.equal(await activePaneLabel(session), 'Files', 'focus should remain Files after typing 1-4')
  })
})

test('C2: shift-tab cycles Files → Comments → Diff → Commits → Files', { skip: 'tuistory cannot reliably emit CSI Z (back-tab): the ["shift","tab"] chord is a no-op and `s.type("\\x1b[Z")` arrives as 3 separate key events (ESC, [, Z) due to typing-simulation pacing. The bubbletea handler (`case "shift+tab"` in keys.go) is verified correct by inspection and works against a real terminal.' }, async () => {
  // Manual reproduction (until tuistory grows raw-write support):
  //   gh-rv --fixture testdata/sample-pr.json
  //   <press Shift+Tab> — focus must cycle Files → Comments → Diff → Commits → Files.
})

test('C3: Enter drills Files → Commits → Diff → Comments', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('enter')   // Files: select greeting.go → focus Commits
  assert.equal(await activePaneLabel(s), 'Commits')
  await s.press('enter')   // Commits: focus Diff (WholePR view, no commit pick)
  assert.equal(await activePaneLabel(s), 'Diff')
  // Land on buffer line 5 — the new line "+// Hello returns ..." carries
  // comment 1001 (carol). Enter on a non-anchored line is a no-op now that
  // the "any-comment" fallback is gone.
  for (let i = 0; i < 5; i++) await s.type('j')
  await s.press('enter')
  assert.equal(await activePaneLabel(s), 'Comments')
  await quit(s)
})

test('C6: each pane preserves its selection across Backspace', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  // Stay on greeting.go (cursor=0, default) so the Commits pane has 2 entries
  // (aaa1111 + bbb2222). greeting_test.go would only show ccc3333 — too narrow
  // to exercise the second-commit cursor preservation.
  await s.press('enter')       // Files → Commits (cursor=0, aaa1111)
  assert.equal(await activePaneLabel(s), 'Commits')
  await s.type('j')           // Commits cursor → bbb2222 (auto-selects bbb2222)
  await s.press('backspace')   // → Files (selectFile on same path = no reset)
  assert.equal(await activePaneLabel(s), 'Files')
  await s.press('enter')       // → Commits again, CommitsCursor preserved
  const commits = paneText(await s.text(), 'Commits')
  assert.match(commits, /^>[^\n]*bbb2222/m, 'Commits cursor should remain on bbb2222 after re-entering')
  await quit(s)
})

test('C8: Shift+J/K navigates files from any pane (focus preserved)', async () => {
  const s = await launchGhRv()
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
  // Diff: J → docs/api.md (also covered by F10 — kept here for cross-pane sanity)
  await s.press('tab')
  assert.equal(await activePaneLabel(s), 'Diff')
  await s.type('J')
  screen = await s.text()
  assert.match(screen, /Diff: docs\/api\.md/, 'J from Diff advances')
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

