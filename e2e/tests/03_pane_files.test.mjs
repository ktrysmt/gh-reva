// Category D — Files pane.

import { test, describe, before } from 'node:test'
import assert from 'node:assert/strict'

import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { launchReva, waitReady, quit, paneText, activePaneLabel } from '../helpers/launch.mjs'

const REPO_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..')

// D1 + D2 share the same initial-render assertions. The Files pane is
// tree-only: directory rows render `v <name>/` and files render their
// basename + bracketed status.
describe('D1+D2: Files pane initial render (tree)', () => {
  let screen
  before(async () => {
    const s = await launchReva()
    await waitReady(s)
    screen = await s.text()
    await quit(s)
  })

  test('D1: directories and file basenames are listed', () => {
    const files = paneText(screen, 'Files')
    // Expanded directory headers.
    assert.match(files, /v\s+docs\//, 'tree shows expanded docs/ header')
    assert.match(files, /v\s+src\//, 'tree shows expanded src/ header')
    // File basenames (tree shows the leaf, not the full path).
    for (const name of ['greeting.go', 'greeting_test.go', 'main.go', 'api.md', 'go.mod']) {
      assert.match(files, new RegExp(`\\s${name.replace('.', '\\.')}(?:\\s|$)`), `basename "${name}" missing in Files pane:\n${files}`)
    }
  })

  test('D2: each file shows status ([A]/[M]/[D]/[R]) and comment count when > 0', () => {
    const files = paneText(screen, 'Files')
    assert.match(files, /\[M\]\s+greeting\.go\s+\(3\)/, 'expected [M] + (3) for greeting.go (carol root + alice reply + bob resolved)')
    assert.match(files, /\[A\]\s+greeting_test\.go\s+\(2\)/, 'expected [A] + (2) for greeting_test.go')
    assert.match(files, /\[M\]\s+main\.go(?!\s*\()/, 'main.go has no comments → no count')
    assert.match(files, /\[A\]\s+api\.md(?!\s*\()/, 'api.md has no comments → no count')
    assert.match(files, /\[M\]\s+go\.mod(?!\s*\()/, 'go.mod has no comments → no count')
  })

  test('D2b: All row carries [*] marker (symmetric to Commits)', () => {
    const files = paneText(screen, 'Files')
    assert.match(files, /\[\*\]\s+All \(\d+ files\)/, 'All row should annotate [*] in front of the label')
  })
})

test('D1b: the Files pane renders as a tree by default (no flat mode)', async () => {
  const s = await launchReva()
  await waitReady(s)
  const files = paneText(await s.text(), 'Files')
  // Directory headers + basenames, never full paths.
  assert.match(files, /v\s+src\//, 'tree mode shows expanded src/ header')
  assert.match(files, /v\s+docs\//, 'tree mode shows expanded docs/ header')
  assert.match(files, /\sgreeting\.go(?:\s|$)/, 'tree mode shows basename greeting.go')
  assert.ok(!files.includes('src/greeting.go'), 'Files pane must not show full paths (tree-only)')
  await quit(s)
})

test('D3: j/k moves the Files cursor', async () => {
  const s = await launchReva()
  await waitReady(s) // cursor lands on greeting.go
  let files = paneText(await s.text(), 'Files')
  assert.match(files, /^>[^\n]*greeting\.go/m, 'cursor should start on greeting.go')
  await s.type('j')
  files = paneText(await s.text(), 'Files')
  assert.match(files, /^>[^\n]*greeting_test\.go/m, 'after j → cursor on greeting_test.go')
  await s.type('k')
  files = paneText(await s.text(), 'Files')
  assert.match(files, /^>[^\n]*greeting\.go/m, 'after k → cursor back on greeting.go')
  await quit(s)
})

test('D3b: j/k in Files moves the cursor but does NOT change SelectedFile', async () => {
  // Per-keystroke Diff re-render felt sluggish during j/k navigation, so
  // auto-select was retired from j/k/gg/G. Cursor moves only; SelectedFile
  // changes via Enter (commit) or Shift+J/K (advanceFile from any pane).
  const s = await launchReva()
  await waitReady(s)
  let screen = await s.text()
  assert.match(screen, /▶ Files/, 'focus starts on Files')
  assert.match(screen, /Diff: src\/greeting\.go/, 'Diff initially shows greeting.go')

  await s.type('j')
  screen = await s.text()
  assert.match(paneText(screen, 'Files'), /^>[^\n]*greeting_test\.go/m, 'Files cursor advances to greeting_test.go')
  assert.match(screen, /Diff: src\/greeting\.go/, 'Diff must stay on greeting.go (no auto-select)')
  assert.match(screen, /▶ Files/, 'focus stays on Files after j')

  await s.type('k')
  screen = await s.text()
  assert.match(paneText(screen, 'Files'), /^>[^\n]*greeting\.go/m, 'k moves cursor back to greeting.go')
  assert.match(screen, /Diff: src\/greeting\.go/, 'Diff still on greeting.go after k')
  await quit(s)
})

test('D3c: visual mode j/k must NOT change SelectedFile (yank-only mutation)', async () => {
  const s = await launchReva()
  await waitReady(s)
  // Enter visual; extend selection with j. SelectedFile should remain greeting.go
  // (auto-select is gated outside visual mode).
  await s.type('v')
  await s.type('j'); await s.type('j')
  const screen = await s.text()
  assert.match(screen, /-- VISUAL --/, 'visual mode indicator visible')
  assert.match(screen, /Diff: src\/greeting\.go/, 'SelectedFile must remain unchanged during visual selection')
  await s.press('esc')
  await quit(s)
})

test('D4: h/l is not bound in Files', async () => {
  const s = await launchReva()
  await waitReady(s)
  const before = await s.text()
  await s.type('h')
  await s.type('l')
  const after = await s.text()
  assert.equal(before, after, 'h and l should be no-ops in Files')
  await quit(s)
})

test('D5: Enter on a directory toggles expand/collapse', async () => {
  const s = await launchReva()
  await waitReady(s) // cursor lands on src/greeting.go
  // Move up to the src/ directory header (one row above greeting.go).
  await s.type('k')
  let files = paneText(await s.text(), 'Files')
  assert.match(files, /^>\s*v\s+src\//m, 'cursor should be on the expanded src/ header')
  // Tree-row anchor for the file: "<spaces>[M] greeting.go (2)" (parent indent
  // + bracketed status + basename).
  const fileRowRE = /^\s+\[M\]\s+greeting\.go\s+\(\d+\)/m
  assert.match(files, fileRowRE, 'expanded src/ should expose greeting.go row')
  await s.press('enter')   // collapse
  files = paneText(await s.text(), 'Files')
  assert.match(files, /^>\s*>\s+src\//m, 'src/ should now show as folded')
  assert.ok(!fileRowRE.test(files), 'greeting.go file row should be hidden under folded src/')
  await s.press('enter')   // re-expand
  files = paneText(await s.text(), 'Files')
  assert.match(files, /^>\s*v\s+src\//m, 'src/ should re-expand')
  assert.match(files, fileRowRE, 'greeting.go row reappears after expand')
  await quit(s)
})

test('D6: Enter on a file row commits the selection and shifts focus to Diff', async () => {
  // Now that j/k stop auto-selecting, Enter is the deliberate commit
  // gesture: it sets SelectedFile to the cursor's file and moves focus
  // to the Diff pane so the user can start reading.
  const s = await launchReva()
  await waitReady(s)
  await s.type('j') // cursor on greeting_test.go but Diff still on greeting.go
  let screen = await s.text()
  assert.match(screen, /Diff: src\/greeting\.go/, 'precondition: j alone keeps Diff on greeting.go')

  await s.press('enter')
  screen = await s.text()
  assert.match(screen, /▶ Diff/, 'Enter on a file row must shift focus to Diff')
  assert.match(screen, /Diff: src\/greeting_test\.go/, 'Enter must commit the cursor file (Diff header follows)')
  await quit(s)
})

test('D9: Files pane scrolls so the cursor stays visible (120-file fixture)', async () => {
  // large-pr has 120 files nested under api/sub*/ … ui/sub*/; the tree
  // overflows the pane height and must scroll as the cursor walks to the
  // bottom. Tree order is alphabetical by path, so the last row is not a
  // numeric "last file" — assert on the cursor/All-row visibility instead.
  const fixture = path.join(REPO_ROOT, 'testdata', 'large-pr.json')
  const s = await launchReva({ fixture })
  await waitReady(s, { allView: true }) // focus stays on Files, cursor on [*] All
  let files = paneText(await s.text(), 'Files')
  assert.match(files, /\[\*\] All/, 'All row visible at the top before scrolling')
  await s.type('G')
  files = paneText(await s.text(), 'Files')
  assert.match(files, /^>[^\n]*\.go/m, 'after G the cursor row (last file) must be visible')
  assert.ok(!files.includes('[*] All'), 'All row must scroll off when the cursor reaches the bottom')
  await s.type('g')
  await s.type('g')
  files = paneText(await s.text(), 'Files')
  assert.match(files, /^>[^\n]*\[\*\] All/m, 'gg must scroll back to the [*] All row')
  await quit(s)
})

// D7/D8: removed — manual `space` filter toggle was replaced by an
// auto-filter keyed off SelectedFile. The Commits pane always shows only
// the commits that touch the current file (see E9). No explicit toggle key
// remains, so there is no title suffix or `*` marker to assert.
