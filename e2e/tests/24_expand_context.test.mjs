// Category Expand — Diff pane context expansion (BOF / inter-hunk / EOF).

import { test } from 'node:test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import assert from 'node:assert/strict'

import { launchReva, waitReady, quit, paneText } from '../helpers/launch.mjs'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const FIXTURE = path.resolve(__dirname, '..', '..', 'testdata', 'expand-pr.json')

// Helper: poll until the Diff pane contains the synthetic hint, allowing
// the prefetch Cmd to land. The prefetch fires from Update's tail after
// PRLoadedMsg, so synthetic rows surface a frame or two after waitReady.
async function waitForSynthetic (s, { timeout = 3000 } = {}) {
  const start = Date.now()
  while (Date.now() - start < timeout) {
    const diff = paneText(await s.text(), 'Diff')
    if (diff && diff.includes('···')) return diff
    await new Promise(r => setTimeout(r, 60))
  }
  throw new Error('timed out waiting for synthetic `···` row in Diff pane')
}

test('Expand-1: synthetic `···` row appears for BOF/Mid/EOF after prefetch', async () => {
  const s = await launchReva({ fixture: FIXTURE })
  await waitReady(s)
  const diff = await waitForSynthetic(s)
  // BOF gap (4 hidden) + Mid gap (12 hidden) + EOF gap (7 hidden).
  assert.match(diff, /4 lines hidden/, `BOF synthetic expected:\n${diff}`)
  assert.match(diff, /12 lines hidden/, `Mid synthetic expected:\n${diff}`)
  assert.match(diff, /7 lines hidden/, `EOF synthetic expected:\n${diff}`)
  await quit(s)
})

test('Expand-2: Enter on BOF synthetic reveals 4 lines (capped) and removes the synthetic', async () => {
  const s = await launchReva({ fixture: FIXTURE })
  await waitReady(s)
  await waitForSynthetic(s)
  await s.press('tab'); await s.press('tab') // focus Diff
  // gg lands cursor on idx 0 (`---` file header). Buffer layout:
  //   idx 0: --- a/file.txt
  //   idx 1: +++ b/file.txt
  //   idx 2: SyntheticLine (BOF, 4 hidden)
  // Two j presses move cursor onto the BOF synthetic.
  await s.press('g'); await s.press('g')
  await s.press('j'); await s.press('j')
  await s.press('enter')
  // After expand, BOF hidden = 0 → synthetic gone; the 4 file lines L1..L4
  // appear as context.
  const diff = paneText(await s.text(), 'Diff')
  assert.doesNotMatch(diff, /4 lines hidden/, `BOF synthetic should be gone:\n${diff}`)
  assert.match(diff, /\bL1\b/, `expanded BOF ctx L1 expected:\n${diff}`)
  assert.match(diff, /\bL4\b/, `expanded BOF ctx L4 expected:\n${diff}`)
  await quit(s)
})
