// Category C — PR comment compose flow (Diff Enter / Comments Enter).
//
// Contract (CLAUDE.md §4 Diff #14, Comments #24b, Compose 27a-f):
//   - Diff Enter opens the comment compose modal anchored at the cursor row.
//     With $EDITOR set, tea.ExecProcess hands the terminal to the editor;
//     on exit the body is saved as a pending ReviewComment (no upstream
//     POST — submission is a separate flow that does not exist yet).
//     With no $EDITOR, the in-app textarea fallback (Ctrl+S save, Esc
//     cancel) collects the body inline.
//   - Comments Enter replies to the thread under the Comments cursor;
//     the reply is also stored as pending and inherits the parent's anchor.
//   - Empty body (TrimSpace) cancels — no pending entry is created.
//   - The Comments pane header tags pending entries with `[pending]` so
//     the user can tell at a glance which comments are local-only drafts.

import { test, before, after } from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'

import { launchReva, waitReady, quit, paneText } from '../helpers/launch.mjs'

let stubDir

before(async () => {
  stubDir = await fs.mkdtemp(path.join(os.tmpdir(), 'gh-reva-compose-'))
})

after(async () => {
  if (stubDir) await fs.rm(stubDir, { recursive: true, force: true })
})

// makeStubEditor writes a one-shot sh script that, when invoked with a single
// path argument, writes `body` into that path. Used as $EDITOR so the
// compose flow's tea.ExecProcess produces a deterministic body without
// needing a real editor in CI.
async function makeStubEditor (name, body) {
  const stubPath = path.join(stubDir, name)
  const escaped = JSON.stringify(body)
  await fs.writeFile(stubPath, `#!/bin/sh\nprintf '%s' ${escaped} > "$1"\n`, { mode: 0o755 })
  return stubPath
}

// navigateToDiffLine assumes Files focus + greeting.go selected; tabs twice
// into Diff and presses j `n` times so the cursor lands on buffer index `n`.
async function navigateToDiffLine (s, n) {
  await s.press('tab')
  await s.press('tab')
  for (let i = 0; i < n; i++) {
    await s.press('j')
  }
}

test('C1: Diff Enter saves the editor body as a pending comment', async () => {
  // Buffer 6 = "+func Hello(...)" on greeting.go (newLine 4) — no existing
  // comment in the fixture, so the appended pending body is uniquely visible.
  const editor = await makeStubEditor('c1.sh', 'inline-pending-from-gh-reva')
  const s = await launchReva({ env: { EDITOR: editor, VISUAL: '' } })
  await waitReady(s)
  await navigateToDiffLine(s, 6)
  await s.press('enter')
  // Editor → exit → applyComposeBody appends pending → Comments pane
  // re-renders with both the body and the [pending] tag.
  await s.waitForText('inline-pending-from-gh-reva', { timeout: 8000 })
  const screen = await s.text()
  const comments = paneText(screen, 'Comments')
  assert.match(comments, /\[pending\]/, `pending tag must appear in Comments header:\n${comments}`)
  await quit(s)
})

test('C2: Comments Enter saves a pending reply under the cursor thread', async () => {
  // Buffer 5 = first existing comment anchor (carol on line 3); tab to
  // Comments, Enter reply.
  const editor = await makeStubEditor('c2.sh', 'pending-reply-from-gh-reva')
  const s = await launchReva({ env: { EDITOR: editor, VISUAL: '' } })
  await waitReady(s)
  await navigateToDiffLine(s, 5)
  await s.press('tab') // Diff → Comments
  await s.press('enter')
  await s.waitForText('pending-reply-from-gh-reva', { timeout: 8000 })
  const screen = await s.text()
  const comments = paneText(screen, 'Comments')
  // Original root + new pending reply must both be visible.
  assert.match(comments, /carol/, `original thread root should remain visible:\n${comments}`)
  assert.match(comments, /\[pending\]/, `pending tag must mark the reply:\n${comments}`)
  await quit(s)
})

test('C3: empty body from $EDITOR cancels — no pending comment is added', async () => {
  // Stub writes an empty file; applyComposeBody.TrimSpace == "" → cancel.
  const editor = await makeStubEditor('c3.sh', '')
  const s = await launchReva({ env: { EDITOR: editor, VISUAL: '' } })
  await waitReady(s)
  await navigateToDiffLine(s, 6)
  await s.press('enter')
  // Wait for editor to exit and bubbletea to redraw.
  await s.waitForText('enter:comment', { timeout: 5000 })
  const screen = await s.text()
  const comments = paneText(screen, 'Comments')
  assert.match(comments, /\(no comment at cursor\)/,
    `cancel must not add any pending comment to the cursor row:\n${comments}`)
  assert.ok(!/\[pending\]/.test(comments), `no pending tag should appear after cancel`)
  await quit(s)
})

test('C5: textarea fallback when $EDITOR is unset saves pending on Ctrl+S', async () => {
  // EDITOR + VISUAL both empty → UseTextarea = true. Overlay shows
  // "New comment"; typing builds Body, Ctrl+S saves as pending.
  const s = await launchReva({ env: { EDITOR: '', VISUAL: '' } })
  await waitReady(s)
  await navigateToDiffLine(s, 6)
  await s.press('enter')
  await s.waitForText('New comment', { timeout: 5000 })
  await s.type('inline-textarea-body')
  await s.waitForText('inline-textarea-body', { timeout: 3000 })
  await s.press(['ctrl', 's'])
  // After save, the textarea overlay closes and the body appears in
  // the Comments pane with [pending] tag.
  await s.waitForText('[pending]', { timeout: 5000 })
  const screen = await s.text()
  const comments = paneText(screen, 'Comments')
  assert.match(comments, /inline-textarea-body/, `saved body must render in Comments`)
  await s.waitForText('enter:comment', { timeout: 3000 })
  await quit(s)
})

test('C5b: textarea Esc cancels without saving', async () => {
  const s = await launchReva({ env: { EDITOR: '', VISUAL: '' } })
  await waitReady(s)
  await navigateToDiffLine(s, 6)
  await s.press('enter')
  await s.waitForText('New comment', { timeout: 5000 })
  await s.type('discard-me')
  await s.press('esc')
  await s.waitForText('enter:comment', { timeout: 3000 })
  const screen = await s.text()
  const comments = paneText(screen, 'Comments')
  assert.ok(!/discard-me/.test(comments),
    `discarded body must not appear in Comments pane:\n${comments}`)
  assert.ok(!/\[pending\]/.test(comments), `no pending tag should appear after Esc`)
  await quit(s)
})
