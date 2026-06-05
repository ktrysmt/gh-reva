// Category G — Comments wrap with CJK boundary words.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import path from 'node:path'

import { launchReva, waitReady, quit, REPO_ROOT, paneText } from '../helpers/launch.mjs'

async function pressN (s, key, n) {
  for (let i = 0; i < n; i++) await s.press(key)
}

test('G12: CJK glued behind ASCII does not strand the ASCII fragment alone', async () => {
  // Regression for the splitWrapWords rule (CLAUDE.md §4 #23c). The fixture
  // body is `slack コマンドの後すぐに通知が消えてしまう不具合があります。修正をお願いします。`.
  //
  // Without splitWrapWords, strings.Fields treats the ASCII↔CJK whitespace
  // as a word boundary, producing ["slack", "コマンドの後すぐに…"]. The CJK
  // run is far wider than the Comments column (~51 cells inner), so the
  // wrapper flushes "slack" alone on its own row and pushes the CJK to the
  // next. With the rule, the whole ASCII↔CJK segment is a single word that
  // hardBreak chunks at the cell boundary, keeping "slack" glued to the head
  // of the wrap output.
  //
  // Assertion: the row containing "slack" must also contain at least one CJK
  // rune. If the rule regresses, "slack" lives alone on its row.
  const s = await launchReva({
    fixture: path.join(REPO_ROOT, 'testdata', 'wrap-pr-cjk.json'),
  })
  await waitReady(s)
  await s.press('tab'); await s.press('tab')   // Files → Diff
  await pressN(s, 'j', 4)                      // 4x j → ◆ row (greeting.go new line 3)
  const cms = paneText(await s.text(), 'Comments')
  const slackRow = cms.split('\n').find(l => l.includes('slack'))
  assert.ok(slackRow, `comment body containing "slack" should be visible in Comments:\n${cms}`)
  assert.match(
    slackRow,
    /\p{Script=Han}|\p{Script=Hiragana}|\p{Script=Katakana}/u,
    `"slack" must stay glued to the following CJK; got isolated row "${slackRow}" in Comments slice:\n${cms}`,
  )
  await quit(s)
})
