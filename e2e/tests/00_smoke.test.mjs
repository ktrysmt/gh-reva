// Smoke tests — runnable today against the Phase 1 skeleton + fixture mode.
// Coverage: A1, A2, A3, A7, A8, B1, B2 (initial render only).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'

import { launchReva, waitReady, quit, BIN, FIXTURE_DEFAULT } from '../helpers/launch.mjs'

test('A1+B1: launch with fixture renders all four pane labels and PR data', async () => {
  const s = await launchReva()
  await waitReady(s)
  const screen = await s.text()
  for (const label of ['Files', 'Commits', 'Diff', 'Comments']) {
    assert.ok(screen.includes(label), `pane label "${label}" missing in:\n${screen}`)
  }
  assert.ok(screen.includes('src/greeting.go'), 'fixture file path missing')
  assert.ok(screen.includes('aaa1111'), 'fixture short SHA missing')
  assert.ok(screen.includes('Implement Hello function'), 'fixture commit subject missing')
  await quit(s)
})

test('A2: launch accepts a PR number argument', async () => {
  const s = await launchReva({ args: ['42'] })
  await waitReady(s)
  await quit(s)
})

test('A3: launch accepts a PR URL argument', async () => {
  const s = await launchReva({ args: ['https://github.com/octocat/hello-world/pull/42'] })
  await waitReady(s)
  await quit(s)
})

test('A4: invalid arg exits non-zero with a helpful message', () => {
  const result = spawnSync(BIN, ['--fixture', FIXTURE_DEFAULT, 'not-a-number'], {
    encoding: 'utf8',
    timeout: 3000,
  })
  assert.notEqual(result.status, 0, `expected non-zero exit, got ${result.status}`)
  assert.match(result.stderr, /invalid PR argument/i)
})

test('A7: q quits cleanly', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('q')
  s.close()
})

test('A8: Ctrl-C quits cleanly', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press(['ctrl', 'c'])
  s.close()
})

test('A9: session.text() never carries raw ANSI escape bytes', async () => {
  // tuistory parses through ghostty so SGR sequences should be consumed by
  // the virtual terminal. A raw 0x1b in the captured text means a renderer
  // emitted bytes the parser could not interpret, which would break
  // substring-based assertions across the suite.
  const s = await launchReva()
  await waitReady(s)
  const screen = await s.text()
  const ESC = String.fromCharCode(0x1b)
  assert.ok(!screen.includes(ESC), 'raw ANSI escape byte (0x1b) leaked into rendered text')
  await quit(s)
})
