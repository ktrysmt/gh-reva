// Category M — cursor-row hover popup in Files / Commits.
//
// M1: with --hover-delay set, the popup appears at the body bottom and
//     mirrors the cursor row's full path / subject.
// M2: --hover-delay 0 disables the popup outright.
// M3: pressing a navigation key cancels the current popup; a fresh one
//     appears for the new cursor row after the delay.

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchGhRv, waitReady, quit } from '../helpers/launch.mjs'

const sleep = (ms) => new Promise(r => setTimeout(r, ms))

// tail returns the last n lines of a screen capture, joined again with
// newlines. Used to isolate the popup region; pane content lives above.
function tail (screen, n) {
  const lines = screen.split('\n')
  return lines.slice(Math.max(0, lines.length - n)).join('\n')
}

test('M1: hover popup mirrors the cursor file after the hover delay', async () => {
  const s = await launchGhRv({ args: ['--hover-delay', '80ms'] })
  await waitReady(s)
  // Initial schedule fires from PRLoadedMsg; give it more than 80ms.
  await sleep(250)
  const screen = await s.text()
  const region = tail(screen, 3)
  assert.match(region, /src\/greeting\.go/, `expected popup to show cursor file at body bottom, region:\n${region}`)
  await quit(s)
})

test('M2: --hover-delay 0 keeps the popup off', async () => {
  const s = await launchGhRv({ args: ['--hover-delay', '0'] })
  await waitReady(s)
  await sleep(700)
  const screen = await s.text()
  const region = tail(screen, 3)
  assert.doesNotMatch(region, /src\/greeting\.go/, `popup should be disabled, region:\n${region}`)
  await quit(s)
})

test('M3: j moves the popup to the new cursor row after the delay', async () => {
  const s = await launchGhRv({ args: ['--hover-delay', '80ms'] })
  await waitReady(s)
  await sleep(250)
  // Press j: cursor moves to greeting_test.go. The wrapped settle is 120ms,
  // longer than the 80ms hover delay, so the new popup is already up by the
  // time we read the screen.
  await s.press('j')
  await sleep(180)
  const screen = await s.text()
  const region = tail(screen, 3)
  assert.match(region, /src\/greeting_test\.go/, `popup should follow cursor to test file, region:\n${region}`)
  await quit(s)
})

test('M4: hover popup follows focus into Commits', async () => {
  const s = await launchGhRv({ args: ['--hover-delay', '80ms'] })
  await waitReady(s)
  await s.press('tab')        // focus Commits
  await sleep(180)
  const screen = await s.text()
  const region = tail(screen, 3)
  // Default fixture's first commit subject begins with "Add greeting.go".
  assert.match(region, /Add greeting\.go|aaa1111/, `popup should show cursor commit subject, region:\n${region}`)
  await quit(s)
})
