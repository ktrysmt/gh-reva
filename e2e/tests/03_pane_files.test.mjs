// Category D — Files pane.

import { test, describe, before } from 'node:test'
import assert from 'node:assert/strict'

import { launchReva, waitReady, quit, paneText, activePaneLabel } from '../helpers/launch.mjs'

// D1 + D2 share the same initial-render assertions.
describe('D1+D2: Files pane initial render (flat list)', () => {
  let screen
  before(async () => {
    const s = await launchReva()
    await waitReady(s)
    screen = await s.text()
    await quit(s)
  })

  test('D1: changed files are listed', () => {
    const files = paneText(screen, 'Files')
    for (const path of ['src/greeting.go', 'src/greeting_test.go', 'src/main.go', 'docs/api.md', 'go.mod']) {
      assert.ok(files.includes(path), `file "${path}" missing in Files pane:\n${files}`)
    }
  })

  test('D2: each file shows status (A/M/D/R) and comment count when > 0', () => {
    const files = paneText(screen, 'Files')
    assert.match(files, /M\s+src\/greeting\.go\s+\(2\)/, 'expected M + (2) for greeting.go')
    assert.match(files, /A\s+src\/greeting_test\.go\s+\(1\)/, 'expected A + (1) for greeting_test.go')
    assert.match(files, /M\s+src\/main\.go(?!\s*\()/, 'main.go has no comments → no count')
    assert.match(files, /A\s+docs\/api\.md(?!\s*\()/, 'api.md has no comments → no count')
    assert.match(files, /M\s+go\.mod(?!\s*\()/, 'go.mod has no comments → no count')
  })
})

test('D1b: t toggles directory tree rendering', async () => {
  const s = await launchReva()
  await waitReady(s)
  let screen = await s.text()
  // Default flat: full paths visible.
  assert.ok(screen.includes('src/greeting.go'), 'flat mode shows full path')
  await s.type('t')
  screen = await s.text()
  // Tree: directory headers + basenames.
  assert.match(screen, /v\s+src\//, 'tree mode shows expanded src/ header')
  assert.match(screen, /v\s+docs\//, 'tree mode shows expanded docs/ header')
  assert.match(screen, /\sgreeting\.go(?:\s|$)/, 'tree mode shows basename greeting.go')
  // Toggle back to flat.
  await s.type('t')
  screen = await s.text()
  assert.ok(screen.includes('src/greeting.go'), 'flat mode restored')
  await quit(s)
})

test('D3: j/k moves the Files cursor', async () => {
  const s = await launchReva()
  await waitReady(s)
  let files = paneText(await s.text(), 'Files')
  assert.match(files, /^>[^\n]*src\/greeting\.go/m, 'cursor should start on first file')
  await s.type('j')
  files = paneText(await s.text(), 'Files')
  assert.match(files, /^>[^\n]*src\/greeting_test\.go/m, 'after j → cursor on second file')
  await s.type('k')
  files = paneText(await s.text(), 'Files')
  assert.match(files, /^>[^\n]*src\/greeting\.go/m, 'after k → cursor back on first file')
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
  assert.match(paneText(screen, 'Files'), /^>[^\n]*src\/greeting_test\.go/m, 'Files cursor advances to greeting_test.go')
  assert.match(screen, /Diff: src\/greeting\.go/, 'Diff must stay on greeting.go (no auto-select)')
  assert.match(screen, /▶ Files/, 'focus stays on Files after j')

  await s.type('k')
  screen = await s.text()
  assert.match(paneText(screen, 'Files'), /^>[^\n]*src\/greeting\.go/m, 'k moves cursor back to first file')
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
  await waitReady(s)
  await s.type('t')   // tree mode; cursor lands on src/greeting.go
  // Move up to the src/ directory header (one row above greeting.go).
  await s.type('k')
  let files = paneText(await s.text(), 'Files')
  assert.match(files, /^>\s*v\s+src\//m, 'cursor should be on the expanded src/ header')
  // Tree-row anchor for the file: "<spaces>M greeting.go (2)" (parent indent
  // + status + basename).
  const fileRowRE = /^\s+M\s+greeting\.go\s+\(\d+\)/m
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

// D7/D8: removed — manual `space` filter toggle was replaced by an
// auto-filter keyed off SelectedFile. The Commits pane always shows only
// the commits that touch the current file (see E9). No explicit toggle key
// remains, so there is no title suffix or `*` marker to assert.
