// Category J / K — Errors, load states, quit semantics.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import path from 'node:path'

import { launchReva, waitReady, quit, BIN, FIXTURE_DEFAULT, REPO_ROOT } from '../helpers/launch.mjs'

test('K1: q quits cleanly', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('q')
  s.close()
})

test('K2: Ctrl-C quits cleanly', async () => {
  const s = await launchReva()
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
  const s = await launchReva({ args: ['--slow-load', '800ms'] })
  await s.waitForText('Loading PR', { timeout: 6000 })
  // Then verify the load eventually completes.
  await s.waitForText('Files', { timeout: 10000 })
  await quit(s)
})

test('J1b: loading marker is centered in the window', async () => {
  const cols = 160
  const rows = 50
  // Pin to the dome-only splash so the dome-glyph anchors below are
  // present (other variants drop the dome entirely or shift it to the
  // right of an ASCII REVA block, both of which break the centering
  // assertions this test owns).
  const s = await launchReva({ args: ['--slow-load', '800ms'], cols, rows, env: { GH_REVA_SPLASH_LAYOUT: '1' } })
  await s.waitForText('Loading PR', { timeout: 6000 })
  const screen = await s.text()
  const lines = screen.split('\n')
  const spinnerRow = lines.findIndex(l => l.includes('Loading PR'))
  assert.ok(spinnerRow >= 0, `'Loading PR' not in screen:\n${screen}`)

  // The loading view stacks logo + blank + spinner and centers the whole
  // block. Check the block midline (topmost logo glyph ↔ spinner row) sits
  // near the screen center, rather than the spinner row alone.
  const logoTop = lines.findIndex(l => /[█▓░]/.test(l))
  assert.ok(logoTop >= 0 && logoTop < spinnerRow, `logo not above spinner:\n${screen}`)
  const blockMid = Math.floor((logoTop + spinnerRow) / 2)
  const centerRow = Math.floor(rows / 2)
  assert.ok(
    Math.abs(blockMid - centerRow) <= 3,
    `block midline ${blockMid} (logoTop=${logoTop}, spinnerRow=${spinnerRow}) not within ±3 of center ${centerRow}`,
  )

  const text = lines[spinnerRow]
  const lead = (text.match(/^\s*/) || [''])[0].length
  const visible = text.trim().length
  const expectedLead = Math.floor((cols - visible) / 2)
  assert.ok(
    Math.abs(lead - expectedLead) <= 3,
    `loading column ${lead} not within ±3 of center ${expectedLead} (visibleW=${visible}, cols=${cols})`,
  )

  await s.waitForText('Files', { timeout: 10000 })
  await quit(s)
})

test('J1c: splash logo appears above the spinner during load', async () => {
  const cols = 160
  const rows = 50
  // Pin to the dome-only splash so the dome glyph + diagonal-axis check
  // can fire — other variants either skip the dome (layout 2) or pair
  // it with ASCII REVA on the left (layout 3, which shifts row
  // midpoints).
  const s = await launchReva({ args: ['--slow-load', '800ms'], cols, rows, env: { GH_REVA_SPLASH_LAYOUT: '1' } })
  await s.waitForText('Loading PR', { timeout: 6000 })
  const screen = await s.text()
  const lines = screen.split('\n')
  const spinnerRow = lines.findIndex(l => l.includes('Loading PR'))
  assert.ok(spinnerRow >= 0, `'Loading PR' not in screen:\n${screen}`)

  // The logo block (10 rows) sits above the spinner. Glyph rows must
  // contain the unique characters defined by the splash art.
  const before = lines.slice(0, spinnerRow).join('\n')
  for (const glyph of ['▓', '░', '█']) {
    assert.ok(
      before.includes(glyph),
      `expected logo glyph ${JSON.stringify(glyph)} above spinner row, got:\n${screen}`,
    )
  }

  // The logo block must be centered as a unit. Source rows have varying
  // widths because the leading-space gradient encodes the dome curve,
  // so glyph leads naturally vary — but each row's geometric midpoint
  // (lead + visible/2) must align on the same column. A regression
  // where each row was centered by its own width skews the midpoints
  // by 5+ cols at the dome's apex (the diagonal-lean bug).
  const logoRows = lines
    .slice(0, spinnerRow)
    .filter(l => /[█▓░]/.test(l))
  assert.ok(logoRows.length >= 6, `expected at least 6 logo rows, got ${logoRows.length}`)
  const midpoints = logoRows.map(l => {
    const lead = (l.match(/^\s*/) || [''])[0].length
    const visible = l.trimEnd().length - lead
    return lead + visible / 2
  })
  const minMid = Math.min(...midpoints)
  const maxMid = Math.max(...midpoints)
  assert.ok(
    maxMid - minMid <= 1,
    `logo midpoints inconsistent (min=${minMid}, max=${maxMid}); dome axis skewed:\n${logoRows.join('\n')}`,
  )
  // And the shared midpoint should sit at the screen center.
  const center = cols / 2
  assert.ok(
    Math.abs(midpoints[0] - center) <= 2,
    `logo midpoint ${midpoints[0]} not within ±2 of screen center ${center}`,
  )

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
  const s = await launchReva({ fixture, cols: 200, rows: 60 })
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
