// Category D — Files pane.

import { test, describe, before } from 'node:test'
import assert from 'node:assert/strict'

import { launchGhRv, waitReady, quit, paneText, activePaneLabel } from '../helpers/launch.mjs'

// D1 + D2 share the same initial-render assertions.
describe('D1+D2: Files pane initial render (flat list)', () => {
  let screen
  before(async () => {
    const s = await launchGhRv()
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
  const s = await launchGhRv()
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
  const s = await launchGhRv()
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

test('D3b: j/k in Files auto-selects file (Diff/Commits sync, focus stays)', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  // Phase 1.5 auto-selects first file on load: greeting.go.
  let screen = await s.text()
  assert.match(screen, /▶ Files/, 'focus starts on Files')
  assert.match(screen, /Diff: src\/greeting\.go/, 'Diff initially shows greeting.go')
  // j → cursor moves to greeting_test.go AND Diff/Comments must follow.
  await s.type('j')
  screen = await s.text()
  assert.match(paneText(screen, 'Files'), /^>[^\n]*src\/greeting_test\.go/m, 'Files cursor on greeting_test.go')
  assert.match(screen, /Diff: src\/greeting_test\.go/, 'Diff should switch to greeting_test.go')
  assert.ok(screen.includes('Add a test for the empty'), 'Comments should switch to greeting_test.go thread')
  assert.match(screen, /▶ Files/, 'focus must remain on Files')
  // k → back to greeting.go.
  await s.type('k')
  screen = await s.text()
  assert.match(screen, /Diff: src\/greeting\.go/, 'Diff should switch back to greeting.go')
  await quit(s)
})

test('D3c: visual mode j/k must NOT change SelectedFile (yank-only mutation)', async () => {
  const s = await launchGhRv()
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
  const s = await launchGhRv()
  await waitReady(s)
  const before = await s.text()
  await s.type('h')
  await s.type('l')
  const after = await s.text()
  assert.equal(before, after, 'h and l should be no-ops in Files')
  await quit(s)
})

test('D5: Enter on a directory toggles expand/collapse', async () => {
  const s = await launchGhRv()
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

test('D6: Enter on a file selects it, refreshes Diff & Comments, focus → Commits', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  // Move cursor to second file (greeting_test.go) and Enter.
  await s.type('j')
  await s.press('enter')
  const screen = await s.text()
  assert.match(screen, /▶ Commits/, 'focus should move to Commits')
  assert.match(screen, /Diff: src\/greeting_test\.go/, 'Diff header should show the selected file')
  assert.ok(screen.includes('TestHello'), 'Diff body should show greeting_test.go content')
  assert.ok(screen.includes('Add a test for the empty'), 'Comments pane should show greeting_test.go thread')
  await quit(s)
})

// D7/D8: removed — manual `space` filter toggle was replaced by an
// auto-filter keyed off SelectedFile. The Commits pane always shows only
// the commits that touch the current file (see E9). No explicit toggle key
// remains, so there is no title suffix or `*` marker to assert.
