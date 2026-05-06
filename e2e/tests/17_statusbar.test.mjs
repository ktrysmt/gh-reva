// Category S — Bottom status bar (`internal/tui/statusbar.go`).
//
// Contract (CLAUDE.md §4 #6):
//   - 3 rows at the bottom are always reserved once the PR is loaded.
//     The bar is rendered as a bordered box (`┌─┐ │ <bar> │ └─┘`); the
//     middle row carries the keymap + URL.
//   - Left side: per-pane keymap context + common suffix
//     (`tab/shift+tab:pane J/K:file ?:help q:quit`) joined by 2 spaces.
//   - Right side: PR URL, picked from a longest-fitting ladder
//     (full https URL → owner/repo/pulls/N → owner/repo/N → repo/N).
//   - Visual / modal / help / compose / submit replace the context AND
//     drop the suffix; the URL still right-flushes.
//   - The Comments-modal hint additionally exposes `enter:edit r:reply`
//     before the close gesture, since edit/reply remain available inside
//     the zoomed Comments view.
//   - When even the shortest URL form does not fit alongside the
//     keymap, the suffix is dropped first (URL stays); after that the
//     URL is dropped entirely. Context never gets half-truncated
//     mid-token (uses ansi.Truncate + `…` only as last resort).

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { launchReva, waitReady, quit } from '../helpers/launch.mjs'

// statusBarRow returns the keymap content row of the borderless status
// bar. The bar is now 2 rows: content + blank. The blank row trims to
// "" and is skipped; the content row is the last non-empty row in the
// screen (and has no `│` / `└┘` border chars). Trailing whitespace is
// stripped so substring asserts don't have to worry about right
// padding.
function statusBarRow (screen) {
  const lines = screen.split('\n')
  for (let i = lines.length - 1; i >= 0; i--) {
    const trimmed = lines[i].replace(/\s+$/, '')
    if (trimmed === '') continue
    return trimmed
  }
  return ''
}

test('S1: Files (flat) status bar shows context + common suffix + PR URL', async () => {
  const s = await launchReva()
  await waitReady(s)
  const row = statusBarRow(await s.text())
  assert.match(row, /j\/k:move/)
  assert.match(row, /space:zoom/)
  assert.match(row, /t:tree/)
  // Common suffix
  assert.match(row, /tab\/shift\+tab:pane/)
  assert.match(row, /\?:help/)
  assert.match(row, /q:quit/)
  // R:submit was retired with the submit-review feature; ensure the
  // hint string is gone too.
  assert.ok(!/R:submit/.test(row), `R:submit should be retired; got: ${row}`)
  // PR URL is right-flushed at default 160-col width — full https form fits.
  assert.match(row, /https:\/\/github\.com\/octocat\/hello-world\/pull\/42/)
  await quit(s)
})

test('S2: Files (tree) adds enter:fold to the context hint', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('t')
  const row = statusBarRow(await s.text())
  assert.match(row, /enter:fold/)
  assert.match(row, /t:tree/)
  await quit(s)
})

test('S3: Commits pane drops t:tree, keeps j/k + space', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab')
  const row = statusBarRow(await s.text())
  assert.match(row, /j\/k:move/)
  assert.match(row, /space:zoom/)
  assert.ok(!/t:tree/.test(row), `t:tree must not appear in Commits status bar; got: ${row}`)
  assert.ok(!/enter:fold/.test(row), `enter:fold is Files-tree only; got: ${row}`)
  await quit(s)
})

test('S4: Diff pane shows j/k, H/M/L, gg/G, space:split, enter:comment', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab')
  await s.press('tab')
  const row = statusBarRow(await s.text())
  assert.match(row, /j\/k:move/)
  assert.match(row, /H\/M\/L:viewport/)
  assert.match(row, /gg\/G:top\/bottom/)
  assert.match(row, /space:split/)
  assert.match(row, /enter:comment/)
  await quit(s)
})

test('S5: Comments pane shows enter:edit and r:reply (split keymap)', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab')
  await s.press('tab')
  await s.press('tab')
  const row = statusBarRow(await s.text())
  assert.match(row, /j\/k:move/)
  assert.match(row, /space:zoom/)
  // Enter is now in-place edit (own comments only); r is the new reply
  // gesture. Both hints must appear in normal Comments-pane mode.
  assert.match(row, /enter:edit/)
  assert.match(row, /r:reply/)
  assert.ok(!/H\/M\/L/.test(row), `Diff-only hints must not leak into Comments status bar; got: ${row}`)
  await quit(s)
})

test('S6: visual mode replaces bar with -- VISUAL -- y/esc hint', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('v')
  const row = statusBarRow(await s.text())
  assert.match(row, /-- VISUAL --/)
  assert.match(row, /y:yank/)
  assert.match(row, /esc\/ctrl\+c:cancel/)
  // Common suffix is dropped while visual is active.
  assert.ok(!/tab\/shift\+tab:pane/.test(row), `common suffix must not coexist with visual hint; got: ${row}`)
  await s.press('esc')
  await quit(s)
})

test('S7: zoom modal replaces bar with close hint (ctrl+c also closes)', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('space')
  const row = statusBarRow(await s.text())
  assert.match(row, /space\/esc\/q\/ctrl\+c:close/)
  assert.ok(!/ctrl\+c:quit/.test(row), `ctrl+c is now a close gesture inside the modal; got: ${row}`)
  assert.ok(!/tab\/shift\+tab:pane/.test(row), `common suffix must not coexist with modal hint; got: ${row}`)
  await quit(s)
})

test('S11: Comments modal status bar adds enter:edit r:reply before close', async () => {
  const s = await launchReva()
  await waitReady(s)
  // src/greeting.go has comments anchored at new-file line 3 / 13.
  // Walk the cursor to a commented row so the Comments modal has visible
  // threads and edit/reply targets.
  await s.press('tab')            // Files → Commits
  await s.press('tab')            // Commits → Diff
  for (let i = 0; i < 12; i++) await s.press('j')
  await s.press('tab')            // Diff → Comments
  await s.press('space')          // open Comments modal
  const row = statusBarRow(await s.text())
  assert.match(row, /enter:edit/, `Comments modal must keep enter:edit; got: ${row}`)
  assert.match(row, /r:reply/, `Comments modal must keep r:reply; got: ${row}`)
  assert.match(row, /space\/esc\/q\/ctrl\+c:close/, `close gesture set must still appear; got: ${row}`)
  assert.ok(!/tab\/shift\+tab:pane/.test(row), `common suffix must not coexist with modal hint; got: ${row}`)
  await quit(s)
})

test('S12: status bar renders as a 2-row borderless block (content + blank)', async () => {
  const s = await launchReva()
  await waitReady(s)
  const lines = (await s.text()).split('\n')
  // Find the last non-empty row — that is the keymap content row.
  let contentIdx = -1
  for (let i = lines.length - 1; i >= 0; i--) {
    if (lines[i].replace(/\s+$/, '') !== '') { contentIdx = i; break }
  }
  assert.ok(contentIdx >= 1,
    `status bar content row not found in tail:\n${lines.slice(-6).join('\n')}`)
  const content = lines[contentIdx].replace(/\s+$/, '')
  // No border glyphs may sit on the content row — the bar is borderless now.
  assert.ok(!/^[┌└]/.test(content), `content row must not start with a border glyph; got: ${content}`)
  assert.ok(!/^│.*│$/.test(content), `content row must not be wrapped in │…│; got: ${content}`)
  // The keymap content (or visual / modal / compose / help replacement)
  // must be present so the row is meaningful — the canonical hints
  // include `q:quit` in normal mode, but the screen could show `--
  // VISUAL --` etc.; pick a token that is always part of the bar.
  assert.match(content, /([a-z]:|--)/,
    `content row must include a hint token (e.g. \`q:\` or \`--\`); got: ${content}`)
  // Below the content row there must be a blank row (or nothing — the
  // terminal renders trailing blanks the same as omitted lines).
  if (contentIdx + 1 < lines.length) {
    const next = lines[contentIdx + 1].replace(/\s+$/, '')
    assert.equal(next, '', `row directly below the bar must be blank; got: ${JSON.stringify(next)}`)
  }
  await quit(s)
})

test('S8: help modal replaces bar with close hint', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.type('?')
  const row = statusBarRow(await s.text())
  assert.match(row, /\?\/esc\/q:close/)
  await quit(s)
})

test('S9: narrow terminal drops the suffix; URL shrinks through the ladder', async () => {
  const s = await launchReva({ cols: 60 })
  await waitReady(s)
  const row = statusBarRow(await s.text())
  assert.match(row, /j\/k:move/)
  // Suffix items must vanish — no half-truncated hint, no q:quit visible.
  assert.ok(!/q:quit/.test(row), `common suffix should be dropped on narrow terminal; got: ${row}`)
  assert.ok(!/tab\/shift\+tab:pane/.test(row), `common suffix should be dropped on narrow terminal; got: ${row}`)
  // A shortened URL form must still appear on the right — at 60 cols the
  // full https URL (~46 chars) does not fit alongside even the bare
  // context, but `octocat/hello-world/pulls/42` (28 chars) does.
  // Accept any form from the ladder >= the shortest one.
  assert.ok(/(octocat\/hello-world\/(pulls\/)?42|hello-world\/42)/.test(row),
    `URL must appear on narrow terminal in some short form; got: ${row}`)
  // The full https form should NOT be present — proves the ladder shrank.
  assert.ok(!/https:\/\//.test(row), `full URL must not fit at 60 cols; got: ${row}`)
  await quit(s)
})

test('S10: status bar is absent during the loading splash', async () => {
  const s = await launchReva({ args: ['--slow-load', '500ms'] })
  // Sample mid-load — splash should be on screen, status bar suppressed.
  // Wait briefly so the spinner has time to render at least one frame
  // without waiting for ready.
  await new Promise(r => setTimeout(r, 200))
  const screen = await s.text()
  assert.ok(/Loading PR/.test(screen), `expected loading splash; got tail:\n${screen.split('\n').slice(-6).join('\n')}`)
  // The status bar shape must not appear above the splash:
  // pick a token that only the post-load bar carries.
  assert.ok(!/tab\/shift\+tab:pane/.test(screen), `status bar must be suppressed during loading; got tail:\n${screen.split('\n').slice(-6).join('\n')}`)
  // Drain to ready before quitting so the binary exits cleanly.
  await waitReady(s)
  await quit(s)
})
