// Category — Files pane All-row (cross-file browse).
//
// Mirrors the Commits pane's "All commits" virtual row: a synthetic
// "All (N files)" row at Files cursor index 0 commits SelectedFile to
// the AllFilesPath sentinel. While the All row is active:
//   - Commits column shows the full PR history (no per-file filtering).
//   - Diff column shows the concatenated diff of every file in the
//     current commit scope (whole PR or single commit).
//   - Comments column displays a placeholder; compose / reply are
//     blocked with a Notice.

import { test, describe, before, after } from 'node:test'
import assert from 'node:assert/strict'

import { launchReva, waitReady, quit, paneText } from '../helpers/launch.mjs'

describe('AR1: All row renders at the top of the Files pane', () => {
  let screen
  before(async () => {
    const s = await launchReva()
    await waitReady(s, { allView: true })
    screen = await s.text()
    await quit(s)
  })

  test('AR1a: row label reads "[*] All (N files)" above every file row', () => {
    const files = paneText(screen, 'Files')
    assert.match(files, /\[\*\]\s+All \(\d+ files\)/, 'Files pane must include the All row with the [*] marker')
    const lines = files.split('\n')
    const allIdx = lines.findIndex(l => /All \(\d+ files\)/.test(l))
    const firstFileIdx = lines.findIndex(l => /src\/greeting\.go/.test(l))
    assert.ok(allIdx >= 0 && firstFileIdx > allIdx,
      `All row must precede the first file row; allIdx=${allIdx} firstFileIdx=${firstFileIdx}`)
  })

  test('AR1b: initial cursor lands on the [*] All row, not the first file', () => {
    const files = paneText(screen, 'Files')
    const cursorRow = files.split('\n').find(l => l.startsWith('> ')) || ''
    assert.ok(/\[\*\]\s+All \(\d+ files\)/.test(cursorRow),
      `initial cursor should be on the [*] All row; got "${cursorRow}"`)
  })

  test('AR1c: initial Diff column reflects the All view (concat)', () => {
    // SelectedFile=AllFilesPath is set synchronously with FilesCursor=0
    // at PRLoadedMsg, so the Diff title carries the "All files (N)"
    // label out of the gate — no Enter required.
    assert.match(screen, /Diff: All files \(\d+\)/, 'Diff title should reflect the All view at boot')
  })
})

describe('AR2: navigating to the All row', () => {
  let s
  before(async () => {
    s = await launchReva()
    await waitReady(s, { allView: true })
  })
  after(async () => {
    await quit(s)
  })

  test('AR2a: j from the [*] row lands on the first file; k back to [*]', async () => {
    await s.press('j')
    let files = paneText(await s.text(), 'Files')
    let cursorRow = files.split('\n').find(l => l.startsWith('> ')) || ''
    assert.ok(/src\/greeting\.go/.test(cursorRow),
      `j from cursor 0 should land on the first file; got "${cursorRow}"`)
    await s.press('k')
    files = paneText(await s.text(), 'Files')
    cursorRow = files.split('\n').find(l => l.startsWith('> ')) || ''
    assert.ok(/\[\*\]\s+All \(\d+ files\)/.test(cursorRow),
      `k from first file should return to the [*] All row; got "${cursorRow}"`)
  })

  test('AR2b: Enter on the All row keeps the All view and focuses Diff', async () => {
    // Cursor is back on the [*] row after AR2a. Enter commits the
    // already-active All view AND shifts FocusedPane to Diff. The
    // SelectedFile=AllFilesPath state is unchanged.
    await s.press('enter')
    const screen = await s.text()
    assert.match(screen, /Diff: All files \(\d+\)/, 'Diff title should reflect the All view')
    assert.match(screen, /▶ Diff/, 'focus should shift to Diff after Enter on [*]')
  })
})

describe('AR3: All view drops the Commits per-file filter', () => {
  let s
  before(async () => {
    s = await launchReva()
    await waitReady(s, { allView: true })
    // Loader lands on [*] with SelectedFile=AllFilesPath already. Reach
    // Commits via Tab (Files → Commits is one Tab).
    await s.press('tab')                    // → Commits
  })
  after(async () => {
    await quit(s)
  })

  test('AR3a: Commits pane shows every commit, no [A/M/D/R] annotation', async () => {
    const commits = paneText(await s.text(), 'Commits')
    // Fixture has 3 commits — every subject must appear.
    for (const subject of ['Add greeting.go skeleton', 'Implement Hello function', 'Add tests and docs']) {
      assert.ok(commits.includes(subject), `commit "${subject}" missing under All view:\n${commits}`)
    }
    // The All-commits label drops the "(M of N)" filtered form.
    assert.match(commits, /All commits \(3\)/, 'All commits label should read the unfiltered total')
    assert.doesNotMatch(commits, /All commits \(\d+ of \d+\)/, 'no M-of-N suffix in All view')
    // Per-row [A/M/D/R] annotation is suppressed. The [*] marker on
    // the All commits row is a synthetic glyph, not a file-status
    // mirror, so it must not match this assertion.
    assert.doesNotMatch(commits, /\[(A|M|D|R)\]/, 'no per-row status annotation in All view')
  })

  test('AR3b: picking a single commit keeps the cross-file concat in Diff', async () => {
    // Currently on the All-commits row in Commits pane; j moves to first commit.
    await s.press('j')
    const screen = await s.text()
    assert.match(screen, /Diff: All files \(\d+\) @ [a-f0-9]+/,
      'Diff title must show All view + short SHA on a single commit')
  })
})

test('AR4: Comments pane explains the disabled state under the All view', async () => {
  const s = await launchReva()
  await waitReady(s, { allView: true })
  // Loader already sits on the [*] row with SelectedFile=AllFilesPath
  // — Comments column shows the placeholder out of the gate.
  const screen = await s.text()
  const comments = paneText(screen, 'Comments')
  assert.match(comments, /Comments disabled in All view/,
    `Comments pane should explain the disabled state; got:\n${comments}`)
  await quit(s)
})
