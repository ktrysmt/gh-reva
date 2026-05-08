// Category J — loading sequence behaviour.

import assert from 'node:assert/strict'
import { test } from 'node:test'

import { launchReva, waitReady, quit } from '../helpers/launch.mjs'

test('J3a: loadPRCmd fans the independent reads out concurrently — total wall time tracks the slowest leg, not the sum', async () => {
  // The loader was a tea.Sequence chaining metadata → commits → files →
  // comments → diffs sequentially; CLAUDE.md §4 #7 documents the
  // post-refactor errgroup fan-out. --slow-load injects a per-API-call
  // sleep that the fixture client applies inside GetPR / ListCommits /
  // ListFiles / ListComments / ViewerLogin, so under sequential load
  // total time would be ~5 * delay before "Loading PR…" disappears.
  //
  // Threshold: with delay=400ms, sequential ≥ 2000ms; parallel ≈ 400ms +
  // overhead. We assert the splash is gone within 1500ms — comfortably
  // under the sequential floor while leaving slow-CI margin for the
  // diff-cache assembly and tuistory's screen-poll cadence.
  const s = await launchReva({ args: ['--slow-load', '400ms'] })
  const start = Date.now()
  // waitReady polls until the Files pane title appears, which only
  // happens after PRLoadedMsg fires — same signal as the splash going
  // away. Generous timeout so the test fails on the assertion below
  // (with a meaningful message), not on waitReady's own timeout.
  await waitReady(s, { timeout: 6000 })
  const elapsed = Date.now() - start
  await quit(s)
  assert.ok(
    elapsed < 1500,
    `loadPRCmd ran sequentially (${elapsed}ms ≥ 1500ms under --slow-load 400ms). Expected parallel fan-out — see CLAUDE.md §4 #7.`,
  )
})

test('J3b: spinner still renders the bare "Loading PR…" caption (no per-stage parenthetical)', async () => {
  // The pre-parallel loader rendered "Loading PR (metadata)…" cycling
  // through stage labels; the parallel loader emits a single bare
  // "Loading PR…" until PRLoadedMsg arrives. Guards against a
  // regression where someone resurrects stageLabel and the bracketed
  // form leaks back in.
  const s = await launchReva({ args: ['--slow-load', '500ms'] })
  await s.waitForText('Loading PR', { timeout: 5000 })
  const screen = await s.text()
  assert.ok(
    screen.includes('Loading PR...') || screen.includes('Loading PR…'),
    `expected bare 'Loading PR…' caption; got tail:\n${screen.split('\n').slice(-6).join('\n')}`,
  )
  assert.ok(
    !/Loading PR \([a-z]+\)/.test(screen),
    `stage parenthetical must not appear; got tail:\n${screen.split('\n').slice(-6).join('\n')}`,
  )
  await waitReady(s, { timeout: 6000 })
  await quit(s)
})
