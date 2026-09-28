import { describe, it, expect } from 'vitest'
import { readdirSync, readFileSync } from 'node:fs'
import { extname, join, relative, sep } from 'node:path'

/**
 * Venue show alerts deliver, so no surface may tell a venue follower they are
 * "still being switched on".
 *
 * A source scan rather than a render test: it covers hand-written JSX as well
 * as shared constants, and every file rather than only the surfaces a test
 * renders. Words may be separated by any whitespace or a JSX `{' '}` token, so
 * a sentence wrapped or split across JSX lines is still one sentence.
 *
 * Release alerts are still pending and keep their own "still being switched
 * on" note, which is why the phrases below name venues rather than the bare
 * clause. This file is the one place the full phrases may appear; an absence
 * assertion elsewhere pins a fragment instead, or it would trip this scan.
 */

const FRONTEND_ROOT = join(import.meta.dirname, '..', '..')
const THIS_FILE = join('components', 'shared', 'venueAlertsPendingNoteRetired.test.ts')

const SKIPPED_DIR_NAMES = new Set([
  'node_modules',
  '.next',
  'coverage',
  'playwright-report',
  'test-results',
])
const SCANNED_EXTENSIONS = new Set([
  '.ts',
  '.tsx',
  '.js',
  '.jsx',
  '.mjs',
  '.cjs',
  '.md',
  '.mdx',
  '.json',
])

const RETIRED_PHRASES = [
  'shows a venue you follow adds are still being switched on',
  'this setting decides what they will cover once they are',
  'venue alerts are still being switched on',
]

// Between two words: whitespace, or a JSX space token such as {' '} or {" "}.
const WORD_GAP = String.raw`(?:\s|\{['"] ['"]\})+`

const phrasePattern = (phrase: string): RegExp =>
  new RegExp(phrase.split(' ').join(WORD_GAP), 'i')
const RETIRED_PATTERNS = RETIRED_PHRASES.map(phrase => ({
  phrase,
  pattern: phrasePattern(phrase),
}))

// Every retired phrase contains one of these words, and a word cannot be split
// across a line, so a file without either cannot hold a phrase.
const PREFILTER = /switched|decides/i

function scannedFiles(dir: string): string[] {
  const files: string[] = []
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (entry.isDirectory()) {
      if (!SKIPPED_DIR_NAMES.has(entry.name)) {
        files.push(...scannedFiles(join(dir, entry.name)))
      }
    } else if (entry.isFile() && SCANNED_EXTENSIONS.has(extname(entry.name))) {
      files.push(join(dir, entry.name))
    }
  }
  return files
}

describe('retired venue-alerts pending note', () => {
  const files = scannedFiles(FRONTEND_ROOT).filter(
    file => relative(FRONTEND_ROOT, file) !== THIS_FILE
  )

  // Guards the scan itself: a wrong root or an over-eager skip list would
  // otherwise pass by scanning nothing.
  it('scans the alert surfaces', () => {
    const scanned = new Set(files.map(file => relative(FRONTEND_ROOT, file)))
    for (const surface of [
      'components/shared/followAlertChoices.ts',
      'components/shared/FollowAlertsReveal.tsx',
      'components/shared/FollowAlertsMenu.tsx',
      'components/shared/LibraryAlertsBar.tsx',
      'features/auth/components/settings/alert-settings.tsx',
    ]) {
      expect(scanned).toContain(surface.split('/').join(sep))
    }
  })

  // Reads every source file in the frontend, about 17 MB. Under a parallel
  // run that read can approach the 5 s default timeout.
  it('appears nowhere under frontend/', { timeout: 15_000 }, () => {
    const offenders = files.flatMap(file => {
      const text = readFileSync(file, 'utf8')
      if (!PREFILTER.test(text)) return []
      return RETIRED_PATTERNS.filter(({ pattern }) => pattern.test(text)).map(
        ({ phrase }) => `${relative(FRONTEND_ROOT, file)}: "${phrase}"`
      )
    })
    expect(offenders).toEqual([])
  })
})
