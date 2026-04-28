import { launchTerminal } from 'tuistory'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
export const REPO_ROOT = path.resolve(__dirname, '..', '..')
export const FIXTURE_DEFAULT = path.join(REPO_ROOT, 'testdata', 'sample-pr.json')
export const BIN = path.join(REPO_ROOT, 'gh-rv')

/**
 * Launch gh-rv with a fixture file. Returns a tuistory session.
 *
 * @param {object} opts
 * @param {string[]} [opts.args]      extra CLI args
 * @param {string}   [opts.fixture]   path to fixture JSON
 * @param {number}   [opts.cols]
 * @param {number}   [opts.rows]
 * @param {object}   [opts.env]
 */
export async function launchGhRv ({
  args = [],
  fixture = FIXTURE_DEFAULT,
  cols = 160,
  rows = 50,
  env = {},
} = {}) {
  return launchTerminal({
    command: BIN,
    args: ['--fixture', fixture, ...args],
    cols,
    rows,
    env: { ...process.env, ...env },
  })
}

/**
 * Wait until the fixture-loaded UI is rendered (i.e. the loading screen has
 * gone away). Phase 1 marker is the literal "Files" heading shown in the
 * top-left pane after PR data arrives.
 */
export async function waitReady (session, { timeout = 5000 } = {}) {
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
