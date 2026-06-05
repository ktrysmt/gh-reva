// Category — Resolved threads.
//
// A resolved thread (PullRequestReviewThread.isResolved on GitHub) is
// surfaced in two places:
//
//   1. Diff gutter: the anchor glyph swaps from ◆ to ✓ at the end row.
//      Multi-line ranges carry no in-gutter shape — the range span is
//      conveyed by the Comments header's `R<start>-<end>` tag instead.
//   2. Comments column: the header carries a leading `[resolved]` tag
//      (line-head position) so the reviewer reads "resolved" before the
//      author name and can skip stale threads at a glance.
//
// Fixture (sample-pr.json):
//   * bob's comment id=1006 on src/greeting.go new line 7 (buffer
//     index 9 in the HEAD patch) is resolved=true.
//   * carol/alice thread (1001 + 1002) on new line 3 (buffer 5)
//     remains unresolved (◆) — the carol/✓ split lets the same
//     screen capture exercise both glyphs.

import { test, describe, before } from 'node:test'
import assert from 'node:assert/strict'

import { launchReva, waitReady, quit, paneText } from '../helpers/launch.mjs'

// pressN sends `key` `n` times, one keystroke at a time.
async function pressN (s, key, n) {
  for (let i = 0; i < n; i++) await s.press(key)
}

describe('RES1: Diff gutter swaps ◆ → ✓ on resolved thread anchors', () => {
  let diff
  before(async () => {
    const s = await launchReva()
    await waitReady(s)
    await s.press('tab'); await s.press('tab')   // focus Diff
    await s.press('space')                       // unified mode
    diff = paneText(await s.text(), 'Diff')
    await quit(s)
  })

  test('RES1a: bob (resolved) anchor row carries ✓ next to the diff content', () => {
    // Bob's comment is on greeting.go new line 7 — the `+\t}` row
    // (closing brace of `if name == ""`). In unified mode the gutter
    // sits left of the diff marker, so ✓ must appear before the `+`.
    assert.match(
      diff,
      /✓\s+\+\s+\}/,
      `resolved anchor row should carry ✓ before its diff content; Diff slice:\n${diff}`,
    )
  })

  test('RES1b: carol (unresolved) anchor row still carries ◆ (not ✓)', () => {
    // carol/alice thread is unresolved → buffer 5 keeps ◆.
    assert.match(
      diff,
      /◆\s+\+\/\/\s*Hello returns a greeting/,
      `unresolved anchor row must remain ◆; Diff slice:\n${diff}`,
    )
    // And the resolved row must NOT carry ◆.
    const resolvedRow = diff.split('\n').find(l => /✓\s+\+\s+\}/.test(l)) || ''
    assert.ok(!resolvedRow.includes('◆'),
      `resolved anchor row must not carry both ◆ and ✓; got "${resolvedRow}"`)
  })
})

test('RES2: Comments header carries a leading [resolved] tag', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')     // focus Diff
  // Walk to buffer index 9 (bob's resolved anchor row). From the
  // initial cursor at 0, 9 j-presses lands on `+\t}` since header /
  // hunk / context / + rows all exist on the RIGHT side that j tracks.
  await pressN(s, 'j', 8)
  const cms = paneText(await s.text(), 'Comments')
  // The leading tag sits at the line head, before the author name.
  assert.match(
    cms,
    /\[resolved\]\s+bob:/,
    `Comments header should read "[resolved] bob:"; slice:\n${cms}`,
  )
  // Trailing tag slot stays empty for resolved-only entries.
  assert.doesNotMatch(cms, /bob:[^\n]*\[(pending|outdated)\]/,
    'no pending/outdated trailing tag for a plain resolved comment')
  await quit(s)
})

test('RES3: unresolved threads have no [resolved] tag in Comments', async () => {
  const s = await launchReva()
  await waitReady(s)
  await s.press('tab'); await s.press('tab')     // focus Diff
  await pressN(s, 'j', 4)                        // carol/alice anchor (buffer 5)
  const cms = paneText(await s.text(), 'Comments')
  assert.match(cms, /carol:/, `carol header should appear at the anchor row; slice:\n${cms}`)
  assert.ok(!cms.includes('[resolved]'),
    `unresolved thread must not carry [resolved]; slice:\n${cms}`)
  await quit(s)
})
