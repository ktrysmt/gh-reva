// Category D — Files pane.

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchGhRv, waitReady, quit } from '../helpers/launch.mjs'

test('D1: changed files are listed (flat list)', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  const screen = await s.text()
  for (const path of ['src/greeting.go', 'src/greeting_test.go', 'src/main.go', 'docs/api.md', 'go.mod']) {
    assert.ok(screen.includes(path), `file "${path}" missing in Files pane`)
  }
  await quit(s)
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

test('D2: each file shows status (A/M/D/R) and comment count when > 0', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  const screen = await s.text()
  assert.match(screen, /M\s+src\/greeting\.go\s+\(2\)/, 'expected M + (2) for greeting.go')
  assert.match(screen, /A\s+src\/greeting_test\.go\s+\(1\)/, 'expected A + (1) for greeting_test.go')
  assert.match(screen, /M\s+src\/main\.go(?!\s*\()/, 'main.go has no comments → no count')
  assert.match(screen, /A\s+docs\/api\.md(?!\s*\()/, 'api.md has no comments → no count')
  assert.match(screen, /M\s+go\.mod(?!\s*\()/, 'go.mod has no comments → no count')
  await quit(s)
})

test('D3: j/k moves the Files cursor', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  let screen = await s.text()
  assert.match(screen, /^>[^\n]*src\/greeting\.go/m, 'cursor should start on first file')
  await s.type('j')
  screen = await s.text()
  assert.match(screen, /^>[^\n]*src\/greeting_test\.go/m, 'after j → cursor on second file')
  await s.type('k')
  screen = await s.text()
  assert.match(screen, /^>[^\n]*src\/greeting\.go/m, 'after k → cursor back on first file')
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
  let screen = await s.text()
  assert.match(screen, /^>\s*v\s+src\//m, 'cursor should be on the expanded src/ header')
  // Tree-row anchor for the file: "<spaces>M greeting.go (2)" (parent indent
  // + status + basename). Use this to assert presence/absence in the Files
  // pane only — the Diff pane also contains "greeting.go" via patch headers.
  const fileRowRE = /^\s+M\s+greeting\.go\s+\(\d+\)/m
  assert.match(screen, fileRowRE, 'expanded src/ should expose greeting.go row')
  await s.press('enter')   // collapse
  screen = await s.text()
  assert.match(screen, /^>\s*>\s+src\//m, 'src/ should now show as folded')
  assert.ok(!fileRowRE.test(screen), 'greeting.go file row should be hidden under folded src/')
  await s.press('enter')   // re-expand
  screen = await s.text()
  assert.match(screen, /^>\s*v\s+src\//m, 'src/ should re-expand')
  assert.match(screen, fileRowRE, 'greeting.go row reappears after expand')
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
  assert.ok(screen.includes('Add a test for the empty input case'), 'Comments pane should show greeting_test.go thread')
  await quit(s)
})

test('D7: <space> toggles commitFilterFile on the cursor file', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('space')
  let screen = await s.text()
  assert.match(screen, /Commits \(filter: src\/greeting\.go\)/, 'expected filter mode title')
  await s.press('space')
  screen = await s.text()
  assert.doesNotMatch(screen, /\(filter:/, 'filter should clear after second space')
  await quit(s)
})

test('D8: filter target file shows a `*` marker', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press('space')
  const screen = await s.text()
  assert.match(screen, /^>\s*\*M\s+src\/greeting\.go/m, 'expected `*` marker on filter target row')
  // Other files must NOT carry `*`.
  assert.doesNotMatch(screen, /\*A\s+src\/greeting_test\.go/)
  await quit(s)
})
