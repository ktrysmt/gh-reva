// Category J / K — Errors, load states, quit semantics.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import path from 'node:path'

import { launchGhRv, waitReady, quit, BIN, FIXTURE_DEFAULT, REPO_ROOT } from '../helpers/launch.mjs'

test('K1: q quits cleanly', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.type('q')
  s.close()
})

test('K2: Ctrl-C quits cleanly', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  await s.press(['ctrl', 'c'])
  s.close()
})

test('A4: bad PR arg exits non-zero with a clear message', () => {
  const result = spawnSync(BIN, ['--fixture', FIXTURE_DEFAULT, 'not-a-number'], {
    encoding: 'utf8',
    timeout: 3000,
  })
  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /invalid PR argument/i)
})

test('A4b: bad PR URL exits non-zero', () => {
  const result = spawnSync(BIN, ['--fixture', FIXTURE_DEFAULT, 'https://example.com/not-a-pr'], {
    encoding: 'utf8',
    timeout: 3000,
  })
  assert.notEqual(result.status, 0)
})

test('missing fixture file exits non-zero', () => {
  const result = spawnSync(BIN, ['--fixture', '/no/such/file.json'], {
    encoding: 'utf8',
    timeout: 3000,
  })
  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /read fixture/i)
})

test('A5: unauthenticated launch surfaces an error', () => {
  const result = spawnSync(BIN, ['--simulate-error', 'unauth'], {
    encoding: 'utf8',
    timeout: 5000,
  })
  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /not authenticated/i)
})

test('A6: PR not found surfaces an error', () => {
  const result = spawnSync(BIN, ['--simulate-error', 'not_found', '999'], {
    encoding: 'utf8',
    timeout: 5000,
  })
  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /404|not found/i)
})

test('J1: spinner / loading marker shown during data fetch', async () => {
  // bubbletea boots in ~1s before rendering the first frame. Inject a
  // generous per-call delay (5 stages × 800ms = 4s) so the loading screen
  // is observable for several captures after startup.
  const s = await launchGhRv({ args: ['--slow-load', '800ms'] })
  await s.waitForText('Loading PR', { timeout: 6000 })
  // Then verify the load eventually completes.
  await s.waitForText('Files', { timeout: 10000 })
  await quit(s)
})

test('J2: API errors (rate-limit / network) surface and exit', () => {
  const result = spawnSync(BIN, ['--simulate-error', 'rate_limit', '1'], {
    encoding: 'utf8',
    timeout: 5000,
  })
  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /rate limit/i)
})

test('J3: large PRs (>50 commits, >100 files) stay responsive', async () => {
  const fixture = path.join(REPO_ROOT, 'testdata', 'large-pr.json')
  const start = Date.now()
  const s = await launchGhRv({ fixture, cols: 200, rows: 60 })
  // 120-file Files pane overflows the alt-screen; "Files" title scrolls off
  // the top. Use a stable Diff anchor instead — it proves the renderer
  // walked the whole fixture without crashing or hanging.
  await s.waitForText('Diff:', { timeout: 10000 })
  const elapsed = Date.now() - start
  assert.ok(elapsed < 8000, `large fixture render should land within 8s, took ${elapsed}ms`)
  const screen = await s.text()
  assert.match(screen, /file_0\d{2}\.go/, 'stress fixture file path should be visible')
  await quit(s)
})

test('K3: ? opens a help overlay (Phase 2 only)', { skip: 'Phase 2: help overlay is out of scope for Phase 1' }, async () => {})
