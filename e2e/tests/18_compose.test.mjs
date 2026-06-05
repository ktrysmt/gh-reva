// Category C — PR comment compose flow (Diff Enter / Comments r/Enter).
//
// Contract (CLAUDE.md §4 Diff #14, Comments #24b, Compose 27a-f):
//   - Diff Enter on a row WITHOUT existing comments opens the inline
//     compose modal anchored at that row.
//   - Diff Enter on a row WITH existing comments hands off to the
//     Comments zoom modal (no compose). The user picks an action there.
//   - Comments r replies to the thread under the cursor (any author).
//   - Comments Enter edits the cursor comment IN PLACE — gated on
//     viewer ownership; on a foreign-user comment a status-bar notice
//     is surfaced instead of starting Compose.
//   - Empty body (TrimSpace) cancels — no pending entry is created.
//   - The Comments pane header tags pending entries with `[pending]`.

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
// into Diff and walks the cursor so it lands on buffer index `n`. The folded
// `+++` header row (buffer 1) is non-navigable (skipped by j/k auto-skip),
// so reaching buffer index n (n>=2) takes n-1 presses.
async function navigateToDiffLine (s, n) {
  await s.press('tab')
  await s.press('tab')
  const presses = n >= 2 ? n - 1 : n
  for (let i = 0; i < presses; i++) {
    await s.press('j')
  }
}

test('C1: Diff Enter then y saves the editor body as a pending comment', async () => {
  // Buffer 6 = "+func Hello(...)" on greeting.go (newLine 4) — no existing
  // comment in the fixture, so the appended pending body is uniquely visible.
  const editor = await makeStubEditor('c1.sh', 'inline-pending-from-gh-reva')
  // cols=200 widens Comments past the narrow-width degradation threshold so
  // the `[pending]` state tag survives in the rendered header (see CLAUDE.md
  // §4 #23b — at the 25% default on the 160-col baseline the header would
  // overflow and padTrunc would clip the trailing tag).
  const s = await launchReva({ cols: 200, env: { EDITOR: editor, VISUAL: '' } })
  await waitReady(s)
  await navigateToDiffLine(s, 6)
  await s.press('enter')
  // Confirm modal overlays a centered prompt with title + target +
  // [y]es / [n]o footer. Without y the editor never launches.
  await s.waitForText('Start new comment?', { timeout: 3000 })
  await s.type('y')
  // Editor → exit → applyComposeBody appends pending → Comments pane
  // re-renders with both the body and the [pending] tag.
  await s.waitForText('inline-pending-from-gh-reva', { timeout: 8000 })
  const screen = await s.text()
  const comments = paneText(screen, 'Comments')
  assert.match(comments, /\[pending\]/, `pending tag must appear in Comments header:\n${comments}`)
  await quit(s)
})

test('C2: Comments r then y saves a pending reply under the cursor thread', async () => {
  // Buffer 5 = first existing comment anchor (carol on line 3). The
  // reply gesture moved from Enter to `r` when Enter was repurposed
  // for in-place edit on the viewer's own comments.
  const editor = await makeStubEditor('c2.sh', 'pending-reply-from-gh-reva')
  // cols=200: same rationale as C1 — preserve the `[pending]` header tag
  // at the 25% default Comments width.
  const s = await launchReva({ cols: 200, env: { EDITOR: editor, VISUAL: '' } })
  await waitReady(s)
  await navigateToDiffLine(s, 5)
  await s.press('tab') // Diff → Comments
  await s.type('r')
  await s.waitForText('Post reply?', { timeout: 3000 })
  await s.type('y')
  await s.waitForText('pending-reply-from-gh-reva', { timeout: 8000 })
  const screen = await s.text()
  const comments = paneText(screen, 'Comments')
  // Original root + new pending reply must both be visible.
  assert.match(comments, /carol/, `original thread root should remain visible:\n${comments}`)
  assert.match(comments, /\[pending\]/, `pending tag must mark the reply:\n${comments}`)
  await quit(s)
})

test('C2b: Comments Enter on a foreign comment surfaces a notice (no compose)', async () => {
  // carol's comment at buffer 5 line 3 is authored by "carol", not "you".
  // Enter must set the status-bar notice and refuse to open Compose.
  const s = await launchReva({ env: { EDITOR: '', VISUAL: '' } })
  await waitReady(s)
  await navigateToDiffLine(s, 5)
  await s.press('tab') // Diff → Comments
  await s.press('enter')
  // Notice replaces the per-pane keymap on the status bar.
  await s.waitForText('cannot edit comments by other users', { timeout: 3000 })
  const screen = await s.text()
  const comments = paneText(screen, 'Comments')
  // No compose modal must have opened — `New comment` / `Reply` / `Edit
  // comment` titles are the proof of intrusion.
  assert.ok(!/New comment|Reply|Edit comment/.test(screen),
    `no compose modal must open on foreign Enter`)
  // The original carol comment is still visible (we did not navigate away).
  assert.match(comments, /carol/)
  await quit(s)
})

test('C3: empty body from $EDITOR cancels — no pending comment is added', async () => {
  // Stub writes an empty file; applyComposeBody.TrimSpace == "" → cancel.
  const editor = await makeStubEditor('c3.sh', '')
  const s = await launchReva({ env: { EDITOR: editor, VISUAL: '' } })
  await waitReady(s)
  await navigateToDiffLine(s, 6)
  await s.press('enter')
  await s.waitForText('Start new comment?', { timeout: 3000 })
  await s.type('y')
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
  // cols=200: same rationale as C1 — preserve the `[pending]` header tag
  // at the 25% default Comments width.
  const s = await launchReva({ cols: 200, env: { EDITOR: '', VISUAL: '' } })
  await waitReady(s)
  await navigateToDiffLine(s, 6)
  await s.press('enter')
  await s.waitForText('Start new comment?', { timeout: 3000 })
  await s.type('y')
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
  await s.waitForText('Start new comment?', { timeout: 3000 })
  await s.type('y')
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

test('C6: confirm n cancels — no editor opens, no pending comment is added', async () => {
  // Stub editor would write a body if invoked, so a successful cancel
  // is provable by the absence of that body in Comments.
  const editor = await makeStubEditor('c6.sh', 'should-not-appear')
  const s = await launchReva({ env: { EDITOR: editor, VISUAL: '' } })
  await waitReady(s)
  await navigateToDiffLine(s, 6)
  await s.press('enter')
  await s.waitForText('Start new comment?', { timeout: 3000 })
  await s.type('n')
  // Modal closes; the Diff keymap is back on the status bar and the
  // confirm title is no longer painted.
  await s.waitForText('enter:comment', { timeout: 3000 })
  const screen = await s.text()
  assert.ok(!/Start new comment\?/.test(screen), `confirm modal must clear after n`)
  const comments = paneText(screen, 'Comments')
  assert.ok(!/should-not-appear/.test(comments),
    `cancelled body must never reach the Comments pane:\n${comments}`)
  assert.ok(!/\[pending\]/.test(comments), `no pending tag must appear after n`)
  await quit(s)
})

test('C6b: confirm Esc cancels (alternative cancel key)', async () => {
  const editor = await makeStubEditor('c6b.sh', 'should-not-appear-esc')
  const s = await launchReva({ env: { EDITOR: editor, VISUAL: '' } })
  await waitReady(s)
  await navigateToDiffLine(s, 6)
  await s.press('enter')
  await s.waitForText('Start new comment?', { timeout: 3000 })
  await s.press('esc')
  await s.waitForText('enter:comment', { timeout: 3000 })
  const screen = await s.text()
  assert.ok(!/Start new comment\?/.test(screen), `confirm modal must clear after Esc`)
  const comments = paneText(screen, 'Comments')
  assert.ok(!/should-not-appear-esc/.test(comments),
    `Esc-cancelled body must not reach Comments`)
  await quit(s)
})
