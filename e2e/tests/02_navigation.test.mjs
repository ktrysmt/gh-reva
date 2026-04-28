// Category C — Pane traversal (tab / shift-tab / Enter / Backspace / numeric).

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchGhRv, waitReady, quit, activePaneLabel } from '../helpers/launch.mjs'

test('C1: tab cycles Files → Commits → Diff → Comments → Files', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  assert.equal(await activePaneLabel(s), 'Files')
  await s.press('tab'); assert.equal(await activePaneLabel(s), 'Commits')
  await s.press('tab'); assert.equal(await activePaneLabel(s), 'Diff')
  await s.press('tab'); assert.equal(await activePaneLabel(s), 'Comments')
  await s.press('tab'); assert.equal(await activePaneLabel(s), 'Files')
  await quit(s)
})

test('C2: shift-tab cycles Files → Comments → Diff → Commits → Files', { skip: 'tuistory cannot reliably emit CSI Z (back-tab): the ["shift","tab"] chord is a no-op and `s.type("\\x1b[Z")` arrives as 3 separate key events (ESC, [, Z) due to typing-simulation pacing. The bubbletea handler (`case "shift+tab"` in keys.go) is verified correct by inspection and works against a real terminal.' }, async () => {
  // Manual reproduction (until tuistory grows raw-write support):
  //   gh-rv --fixture testdata/sample-pr.json
  //   <press Shift+Tab> — focus must cycle Files → Comments → Diff → Commits → Files.
})

test('C3: Enter drills Files → Commits → Diff → Comments', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('enter')   // Files: select first file → focus Commits
  assert.equal(await activePaneLabel(s), 'Commits')
  await s.press('enter')   // Commits: select first commit → focus Diff
  assert.equal(await activePaneLabel(s), 'Diff')
  // Diff: cursor on first line is the file header which has no comment.
  // To land on a commented line, move down until we hit a comment marker.
  // Convention: a line carrying a comment renders an end-of-line tag like " ◆"
  // (defined in pane_diff.go). For this test we just press Enter twice — the
  // first time should be a no-op (no comment), the second time we move to a
  // commented line first.
  // Simpler: jump cursor to a line that the impl guarantees has a comment by
  // scanning down. We approximate by pressing 'j' enough times to reach a
  // greeting.go comment line (the comment thread anchored at line 12 maps to
  // "func Hello" in the diff hunk).
  for (let i = 0; i < 20; i++) await s.type('j')
  await s.press('enter')
  // Either we land in Comments (comment found) or stay in Diff (no comment).
  // Phase 1 contract: at least one comment on src/greeting.go is reachable.
  assert.equal(await activePaneLabel(s), 'Comments')
  await quit(s)
})

test('C4: Backspace returns Comments → Diff → Commits → Files', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab'); await s.press('tab')
  assert.equal(await activePaneLabel(s), 'Comments')
  await s.press('backspace'); assert.equal(await activePaneLabel(s), 'Diff')
  await s.press('backspace'); assert.equal(await activePaneLabel(s), 'Commits')
  await s.press('backspace'); assert.equal(await activePaneLabel(s), 'Files')
  await s.press('backspace'); assert.equal(await activePaneLabel(s), 'Files')
  await quit(s)
})

test('C5: Backspace mash from Comments lands at Files', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('tab'); await s.press('tab'); await s.press('tab')
  for (let i = 0; i < 10; i++) await s.press('backspace')
  assert.equal(await activePaneLabel(s), 'Files')
  await quit(s)
})

test('C6: each pane preserves its selection across Backspace', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.type('j')           // Files cursor → second file
  await s.press('enter')       // → Commits
  assert.equal(await activePaneLabel(s), 'Commits')
  await s.type('j')           // Commits cursor → second commit (bbb2222)
  await s.press('backspace')   // → Files
  assert.equal(await activePaneLabel(s), 'Files')
  await s.press('enter')       // → Commits again
  const screen = await s.text()
  assert.match(screen, /^>[^\n]*bbb2222/m, 'Commits cursor should remain on bbb2222 after re-entering')
  await quit(s)
})

test('C7: numeric keys (1-4) do not jump panes', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  for (const key of ['1', '2', '3', '4']) await s.type(key)
  assert.equal(await activePaneLabel(s), 'Files', 'focus should remain Files after typing 1-4')
  await quit(s)
})
