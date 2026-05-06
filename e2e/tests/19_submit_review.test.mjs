// Category R — Submit-review modal (key `R`).
//
// Contract (CLAUDE.md §4 Compose 27a-h, Submit Review):
//   - `R` (uppercase) opens the submit-review modal from any pane.
//   - Modal title is "Submit review"; body shows pending count and 3
//     event choices (a:approve / c:comment / r:request changes).
//   - Pressing a/c/r kicks off submitPullRequestReview; on success the
//     modal closes and ListComments is refetched (Pending bits flip).
//   - Esc / Ctrl+C cancels the modal without submitting.
//
// Tests rely on the fixtureClient implementation: it does not actually
// POST anywhere, but it does flip Pending=false for every comment on
// submit. The flip becomes visible after the implicit refetch.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'

import { launchReva, waitReady, quit, paneText } from '../helpers/launch.mjs'

let stubDir

async function makeStubEditor (name, body) {
  if (!stubDir) {
    stubDir = await fs.mkdtemp(path.join(os.tmpdir(), 'gh-reva-submit-'))
  }
  const stubPath = path.join(stubDir, name)
  const escaped = JSON.stringify(body)
  await fs.writeFile(stubPath, `#!/bin/sh\nprintf '%s' ${escaped} > "$1"\n`, { mode: 0o755 })
  return stubPath
}

test('R1: R opens the submit-review modal showing pending count', async () => {
  // Save one pending comment first so the count is non-zero.
  const editor = await makeStubEditor('r1.sh', 'first-pending')
  const s = await launchReva({ env: { EDITOR: editor, VISUAL: '' } })
  await waitReady(s)
  await s.press('tab'); await s.press('tab')
  for (let i = 0; i < 6; i++) await s.press('j')
  await s.press('enter')
  await s.waitForText('first-pending', { timeout: 8000 })
  // Now invoke submit
  await s.press('R')
  await s.waitForText('Submit review', { timeout: 5000 })
  const screen = await s.text()
  assert.match(screen, /1 pending comment/, `pending count should be visible:\n${screen}`)
  assert.match(screen, /\[a\] approve/)
  assert.match(screen, /\[c\] comment/)
  assert.match(screen, /\[r\] request changes/)
  await s.press('esc')
  await quit(s)
})

test('R2: pressing c (comment) submits and clears pending tags', async () => {
  const editor = await makeStubEditor('r2.sh', 'will-be-submitted')
  const s = await launchReva({ env: { EDITOR: editor, VISUAL: '' } })
  await waitReady(s)
  await s.press('tab'); await s.press('tab')
  for (let i = 0; i < 6; i++) await s.press('j')
  await s.press('enter')
  await s.waitForText('will-be-submitted', { timeout: 8000 })
  // [pending] should be visible right after the save.
  let comments = paneText(await s.text(), 'Comments')
  assert.match(comments, /\[pending\]/, `pending tag must be visible before submit:\n${comments}`)
  // Submit with event=COMMENT.
  await s.press('R')
  await s.waitForText('Submit review', { timeout: 5000 })
  await s.press('c')
  // Modal closes; refetch flips Pending=false → tag should disappear
  // while the body stays visible in Comments.
  await s.waitForText('will-be-submitted', { timeout: 5000 })
  // Allow refetch to complete.
  await new Promise(r => setTimeout(r, 200))
  const after = await s.text()
  comments = paneText(after, 'Comments')
  assert.match(comments, /will-be-submitted/, `body must remain after submit:\n${comments}`)
  assert.ok(!/\[pending\]/.test(comments), `pending tag must be gone after submit:\n${comments}`)
  await quit(s)
})

test('R3: Esc on submit modal cancels without sending', async () => {
  const editor = await makeStubEditor('r3.sh', 'still-pending')
  const s = await launchReva({ env: { EDITOR: editor, VISUAL: '' } })
  await waitReady(s)
  await s.press('tab'); await s.press('tab')
  for (let i = 0; i < 6; i++) await s.press('j')
  await s.press('enter')
  await s.waitForText('still-pending', { timeout: 8000 })
  await s.press('R')
  await s.waitForText('Submit review', { timeout: 5000 })
  await s.press('esc')
  // Modal should be gone; pending tag should still be visible.
  await s.waitForText('enter:comment', { timeout: 3000 })
  const comments = paneText(await s.text(), 'Comments')
  assert.match(comments, /\[pending\]/, `pending tag must remain after Esc-cancel:\n${comments}`)
  await quit(s)
})

test('R4: R with zero pending shows 0-pending modal', async () => {
  const s = await launchReva({ env: { EDITOR: '', VISUAL: '' } })
  await waitReady(s)
  await s.press('R')
  await s.waitForText('Submit review', { timeout: 5000 })
  const screen = await s.text()
  assert.match(screen, /0 pending comments/, `should show "0 pending comments":\n${screen}`)
  await s.press('esc')
  await quit(s)
})

test.after(async () => {
  if (stubDir) await fs.rm(stubDir, { recursive: true, force: true })
})
