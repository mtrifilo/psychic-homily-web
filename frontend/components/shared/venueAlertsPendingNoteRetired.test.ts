import { describe, it, expect } from 'vitest'
import { readdirSync, readFileSync } from 'node:fs'
import { extname, join, relative, sep } from 'node:path'

/**
 * Venue show alerts deliver, so no surface may tell a venue follower they are
 * "still being switched on".
 *
 * A source scan rather than a render test, because the claim has been written
 * both through a shared constant and by hand inside JSX, and a render test
 * only covers the surfaces someone remembered to render. Each phrase matches
 * across any whitespace, so a sentence wrapped across JSX lines is still one
 * sentence.
 *
 * Release alerts are still pending and keep their own "still being switched
 * on" note, which is why the phrases below name venues rather than the bare
 * clause.
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

const phrasePattern = (phrase: string): RegExp =>
  new RegExp(phrase.split(' ').join('\\s+'), 'i')
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
  it('scans the surfaces that carried the note', () => {
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

  // Reads every source file in the frontend (about 17 MB). Alongside other
  // test files that read alone has come within reach of the 5 s default.
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
