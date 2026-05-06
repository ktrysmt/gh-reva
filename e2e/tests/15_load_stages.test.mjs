// Category J — loading sequence stage transitions.

import { test } from 'node:test'

import { launchReva, waitReady, quit } from '../helpers/launch.mjs'

test('J3a: spinner cycles through the documented stage labels under --slow-load', async () => {
  // The loader is a tea.Sequence: metadata → commits → files → comments →
  // diffs → ready. --slow-load injects a per-API-call sleep so each stage
  // label is observable. Without this delay each label would flash for a
  // single render and waitForText could miss it on slow CI.
  //
  // CLAUDE.md §4 #7 pins this stage order; this test fails the moment a
  // stage is renamed, dropped, or reordered.
  const s = await launchReva({ args: ['--slow-load', '300ms'] })
  for (const stage of ['metadata', 'commits', 'files', 'comments', 'diffs']) {
    await s.waitForText(`Loading PR (${stage})`, { timeout: 5000 })
  }
  await waitReady(s)
  await quit(s)
})
