// Category M — cursor-row hover popup in Files / Commits.
//
// M1: with --hover-delay set, the popup appears next to the active pane
//     and mirrors the cursor row's full path / subject.
// M2: --hover-delay 0 disables the popup outright.
// M3: pressing a navigation key cancels the current popup; a fresh one
//     appears for the new cursor row after the delay.
// M4: the popup follows focus into Commits.

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchGhRv, waitReady, quit } from '../helpers/launch.mjs'

const sleep = (ms) => new Promise(r => setTimeout(r, ms))

// hasPopup looks for the popup signature: a `│` border glyph immediately
// followed by the focused content. Pane rows always carry `> ` or `[A] `
// prefixes between the border and the content text, so this adjacency
// only occurs inside the hover popup.
function hasPopup (screen, contentRegex) {
  // contentRegex must NOT include the leading │; we prepend it here.
  const re = new RegExp('│' + contentRegex.source, contentRegex.flags.includes('g') ? contentRegex.flags : contentRegex.flags + 'g')
  return re.test(screen)
}

test('M1: hover popup mirrors the cursor file after the hover delay', async () => {
  const s = await launchGhRv({ args: ['--hover-delay', '80ms'] })
  await waitReady(s)
  await sleep(250)
  const screen = await s.text()
  assert.ok(hasPopup(screen, /src\/greeting\.go(?!_)/), `expected popup with cursor file path; screen tail:\n${screen.split('\n').slice(-20).join('\n')}`)
  await quit(s)
})

test('M2: --hover-delay 0 keeps the popup off', async () => {
  const s = await launchGhRv({ args: ['--hover-delay', '0'] })
  await waitReady(s)
  await sleep(700)
  const screen = await s.text()
  assert.ok(!hasPopup(screen, /src\/greeting\.go(?!_)/), `popup should be disabled`)
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
  assert.ok(hasPopup(screen, /src\/greeting_test\.go/), `popup should follow cursor to test file`)
  await quit(s)
})

test('M4: hover popup follows focus into Commits', async () => {
  const s = await launchGhRv({ args: ['--hover-delay', '80ms'] })
  await waitReady(s)
  await s.press('tab')        // focus Commits
  await sleep(180)
  const screen = await s.text()
  // Commits popup body starts with "<short_sha> <subject>"; the fixture's
  // first commit is aaa1111 "Add greeting.go skeleton".
  assert.ok(hasPopup(screen, /aaa1111 Add greeting\.go/), `popup should mirror cursor commit`)
  await quit(s)
})

test('M5: popup sits above the cursor row, not at body bottom', async () => {
  const s = await launchGhRv({ args: ['--hover-delay', '80ms'] })
  await waitReady(s)
  // Move down a few rows so there is plenty of empty space both above
  // and below the cursor; popup placement preference is "above".
  await s.press('j')
  await s.press('j')
  await sleep(220)
  const screen = await s.text()
  const lines = screen.split('\n')
  // Popup is anchored above the active pane's cursor row (Files pane
  // top-left); the bottom 5 rows of the screen should NOT carry the
  // path content of the cursor row.
  const tail = lines.slice(-5).join('\n')
  assert.ok(!/│src\/main\.go/.test(tail), `popup should not be at body bottom; tail:\n${tail}`)
  // ... but the path still appears somewhere on screen (in the popup
  // and in the Files row).
  assert.ok(hasPopup(screen, /src\/main\.go/), `popup should still be visible somewhere`)
  await quit(s)
})
