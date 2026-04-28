// Category B — Layout & initial render.

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchGhRv, waitReady, quit, activePaneLabel } from '../helpers/launch.mjs'

test('B1: four panes (Files / Commits / Diff / Comments) visible', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  const screen = await s.text()
  for (const label of ['Files', 'Commits', 'Diff', 'Comments']) {
    assert.ok(screen.includes(label), `missing pane label: ${label}`)
  }
  await quit(s)
})

test('B2: initial focus is Files (▶ marker on Files only)', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  assert.equal(await activePaneLabel(s), 'Files')
  await quit(s)
})

test('B3: only the focused pane carries the ▶ marker', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  assert.equal(await activePaneLabel(s), 'Files')
  await s.press('tab')
  assert.equal(await activePaneLabel(s), 'Commits')
  await s.press('tab')
  assert.equal(await activePaneLabel(s), 'Diff')
  await s.press('tab')
  assert.equal(await activePaneLabel(s), 'Comments')
  await quit(s)
})

test('B4: a wider terminal still renders all four panes', async () => {
  const s = await launchGhRv({ cols: 200, rows: 60 })
  await waitReady(s)
  const screen = await s.text()
  for (const label of ['Files', 'Commits', 'Diff', 'Comments']) {
    assert.ok(screen.includes(label), `pane "${label}" missing at 200 cols`)
  }
  await quit(s)
})

test('B5: narrow terminal (<100 cols) auto-falls back to unified Diff', async () => {
  const s = await launchGhRv({ cols: 80, rows: 30 })
  await waitReady(s)
  const screen = await s.text()
  assert.match(screen, /Diff:[^\n]*\[unified\]/, 'narrow view should use unified Diff')
  assert.doesNotMatch(screen, /Diff:[^\n]*\[split\]/, 'narrow view should NOT use split Diff')
  await quit(s)
})
