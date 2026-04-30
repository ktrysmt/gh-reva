// Category B — Layout & initial render.

import { test, describe, before } from 'node:test'
import assert from 'node:assert/strict'

import { launchGhRv, waitReady, quit, activePaneLabel, paneText } from '../helpers/launch.mjs'

// B1/B2/B6 all observe the same initial-render state. Capture the screen once
// and run all three assertion sets against it to avoid redundant launches.
describe('B1+B2+B6: initial-render observations (default 160 cols)', () => {
  let screen
  before(async () => {
    const s = await launchGhRv()
    await waitReady(s)
    screen = await s.text()
    await quit(s)
  })

  test('B1: four panes (Files / Commits / Diff / Comments) visible', () => {
    for (const label of ['Files', 'Commits', 'Diff', 'Comments']) {
      assert.ok(screen.includes(label), `missing pane label: ${label}`)
    }
  })

  test('B2: initial focus is Files (▶ marker on Files only)', () => {
    const matches = []
    for (const label of ['Files', 'Commits', 'Diff', 'Comments']) {
      if (screen.includes(`▶ ${label}`)) matches.push(label)
    }
    assert.equal(matches.length, 1, `exactly one active pane expected, got ${matches.join(', ')}`)
    assert.equal(matches[0], 'Files', 'initial focus should be Files')
  })

  test('B6: panes are arranged in 3 columns (left=Files+Commits, middle=Diff, right=Comments)', () => {
    const lines = screen.split('\n')
    const find = (label) => {
      for (let i = 0; i < lines.length; i++) {
        const idx = lines[i].search(new RegExp('(?:▶ |  )' + label + '\\b'))
        if (idx >= 0) return { row: i, col: idx }
      }
      return null
    }
    const f = find('Files'), c = find('Commits'), d = find('Diff'), cm = find('Comments')
    assert.ok(f && c && d && cm, 'all 4 pane headers should be visible')
    assert.equal(d.row, f.row, 'Files and Diff should share the top row')
    assert.equal(cm.row, f.row, 'Files and Comments should share the top row')
    assert.equal(c.col, f.col, 'Commits should share the left column with Files')
    assert.ok(c.row > f.row, 'Commits should sit below Files in the left column')
    assert.ok(d.col > f.col, 'Diff column should be to the right of Files')
    assert.ok(cm.col > d.col, 'Comments column should be to the right of Diff')
  })
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

test('B8: each pane shows a horizontal separator under the title (├──┤)', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  const screen = await s.text()
  // Each of the 4 panes should carry a title-bar divider rendered as
  // `├` … `┤`. Count both edge characters separately to detect any pane
  // that lost the divider.
  const lefts = (screen.match(/├/g) || []).length
  const rights = (screen.match(/┤/g) || []).length
  assert.ok(lefts >= 4, `expected ≥4 ├ separators (one per pane), got ${lefts}`)
  assert.ok(rights >= 4, `expected ≥4 ┤ separators, got ${rights}`)
  await quit(s)
})

test('B7: each pane is rendered with a visible border', async () => {
  const s = await launchGhRv()
  await waitReady(s)
  const screen = await s.text()
  // lipgloss.NormalBorder() uses ─│┌┐└┘. Four panes (Files / Commits / Diff /
  // Comments) → at least 4 top-left corners.
  assert.ok(screen.includes('─'), 'horizontal border char must appear')
  const tl = (screen.match(/┌/g) || []).length
  assert.ok(tl >= 4, `expected ≥4 top-left corners (one per pane), got ${tl}`)
  await quit(s)
})

test('B5: narrow terminal (<100 cols) auto-falls back to unified Diff', async () => {
  const s = await launchGhRv({ cols: 80, rows: 30 })
  await waitReady(s)
  const screen = await s.text()
  // At narrow widths the Diff column is too tight to keep the full title on
  // one row, so the "[unified]" tag may live on a wrapped continuation. Walk
  // the column slice instead of asserting against the raw screen.
  const diff = paneText(screen, 'Diff')
  assert.ok(diff.includes('[unified]'), `narrow view should use unified Diff; Diff slice:\n${diff}`)
  assert.ok(!diff.includes('[split]'), `narrow view should NOT use split Diff; Diff slice:\n${diff}`)
  await quit(s)
})
