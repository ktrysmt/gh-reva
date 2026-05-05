import { launchTerminal } from 'tuistory'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
export const REPO_ROOT = path.resolve(__dirname, '..', '..')
export const FIXTURE_DEFAULT = path.join(REPO_ROOT, 'testdata', 'sample-pr.json')
export const BIN = path.join(REPO_ROOT, 'gh-reva')

/**
 * Launch gh-reva with a fixture file. Returns a tuistory session.
 *
 * @param {object} opts
 * @param {string[]} [opts.args]      extra CLI args
 * @param {string}   [opts.fixture]   path to fixture JSON
 * @param {number}   [opts.cols]
 * @param {number}   [opts.rows]
 * @param {object}   [opts.env]
 */
export async function launchReva ({
  args = [],
  fixture = FIXTURE_DEFAULT,
  cols = 160,
  rows = 50,
  env = {},
} = {}) {
  // bubbletea v1.3.0's tea_init.go calls lipgloss.HasDarkBackground() at
  // import time, which makes termenv send OSC 11 + DSR queries on stdout
  // and block up to termenv.OSCTimeout (5s) for a response. The PTY has
  // no terminal to answer, so every launch ate the full 5s — ~96 launches
  // × 5s ≈ 8 minutes of pure idle wait across the suite.
  // termenv.termStatusReport short-circuits when TERM starts with "screen"
  // or "tmux", so forcing TERM=tmux-256color skips the wait. Pass it via
  // `sh -c` because tuistory hard-codes TERM=xterm-truecolor in its child
  // env (session.js spreads options.env BEFORE setting TERM), and `exec`
  // re-applies our value just before the binary starts.
  const escaped = [BIN, '--fixture', fixture, ...args]
    .map(a => "'" + String(a).replace(/'/g, "'\\''") + "'").join(' ')
  const session = await launchTerminal({
    command: '/bin/sh',
    args: ['-c', `TERM=tmux-256color COLORTERM=truecolor exec ${escaped}`],
    cols,
    rows,
    env: { ...process.env, ...env },
  })
  // Wrap press / type so callers don't need to manually sync after each
  // keystroke. bubbletea's Update → View pipeline is async and ghostty's
  // parser needs a beat to drain the SGR-laden output before subsequent
  // text() reads see the post-keystroke screen. ~120ms covers the worst
  // case observed under the colored renderer; a stable test that passes
  // here also passes interactively.
  const SETTLE_MS = 120
  const sleep = (ms) => new Promise(r => setTimeout(r, ms))
  for (const fn of ['press', 'type']) {
    const orig = session[fn].bind(session)
    session[fn] = async (...a) => {
      const r = await orig(...a)
      await sleep(SETTLE_MS)
      return r
    }
  }
  return session
}

/**
 * Wait until the fixture-loaded UI is rendered (i.e. the loading screen has
 * gone away). Phase 1 marker is the literal "Files" heading shown in the
 * top-left pane after PR data arrives.
 *
 * The 10s default accommodates chroma's styles + lexers package init
 * (~74 styles parsed at startup, several hundred lexers registered) which
 * can add ~500ms to first-frame latency on cold caches.
 */
export async function waitReady (session, { timeout = 10000 } = {}) {
  await session.waitForText('Files', { timeout })
}

/**
 * Quit cleanly using `q`. Falls back to Ctrl-C if needed.
 */
export async function quit (session) {
  try {
    await session.type('q')
  } catch {
    await session.press(['ctrl', 'c'])
  }
  try {
    session.close()
  } catch {
    /* ignore */
  }
}

/**
 * Identify which pane currently carries the `▶ <Pane>` active marker.
 * Throws if zero or more than one pane reports active.
 */
export async function activePaneLabel (session) {
  const screen = await session.text()
  const matches = []
  for (const label of ['Files', 'Commits', 'Diff', 'Comments']) {
    if (screen.includes(`▶ ${label}`)) matches.push(label)
  }
  if (matches.length === 0) {
    throw new Error(`no active pane in screen:\n${screen}`)
  }
  if (matches.length > 1) {
    throw new Error(`multiple active panes (${matches.join(', ')}) in screen:\n${screen}`)
  }
  return matches[0]
}

const PANE_LABELS = ['Files', 'Commits', 'Diff', 'Comments']

function locatePanes (screen) {
  const lines = screen.split('\n')
  const slots = {}
  for (const label of PANE_LABELS) {
    const re = new RegExp('(?:▶ |  )' + label + '\\b')
    for (let i = 0; i < lines.length; i++) {
      const m = re.exec(lines[i])
      if (m) {
        slots[label] = { row: i, col: m.index }
        break
      }
    }
  }
  return { lines, slots }
}

/**
 * Extract one pane's vertical column slice from a screen capture. Returns the
 * pane's rows joined by newlines, with right-side padding trimmed.
 *
 * Useful in tests that anchor on cursor markers (`^>`) in a column other than
 * the leftmost — anchoring the regex to a column slice avoids matching the
 * cursor of an unrelated pane on the same row.
 */
export function paneText (screen, label) {
  const { lines, slots } = locatePanes(screen)
  const me = slots[label]
  if (!me) return ''
  const otherCols = Object.values(slots)
    .filter(s => s.col > me.col)
    .map(s => s.col)
    .sort((a, b) => a - b)
  const endCol = otherCols.length > 0 ? otherCols[0] : (lines[me.row] || '').length + 100
  // Where does this pane stop vertically? Look for another pane in the same
  // column starting on a later row (e.g. Commits below Files).
  const inSameColRows = Object.entries(slots)
    .filter(([k, s]) => k !== label && s.col === me.col && s.row > me.row)
    .map(([, s]) => s.row)
  const endRow = inSameColRows.length > 0 ? Math.min(...inSameColRows) : lines.length
  const out = []
  for (let i = me.row; i < endRow; i++) {
    // Strip trailing border chars (`│`) introduced by the column's right edge
    // and the adjacent pane's left edge — they would otherwise leak into
    // assertions that count split separators or apply `^>` anchors.
    out.push((lines[i] || '').slice(me.col, endCol).replace(/(?:\s*│)+\s*$/, '').trimEnd())
  }
  return out.join('\n')
}

/**
 * Count rows in the given pane that render with a `> ` cursor prefix at the
 * pane's left edge. Used to verify visual-selection extents.
 */
export function countSelectedRows (screen, label) {
  const sliced = paneText(screen, label)
  if (!sliced) return 0
  let n = 0
  for (const line of sliced.split('\n').slice(1)) {  // skip header
    if (line.startsWith('> ')) n++
  }
  return n
}
