import { describe, it, expect } from 'vitest'
import { readdirSync, readFileSync } from 'node:fs'
import { extname, join, relative, sep } from 'node:path'

/**
 * Venue show alerts deliver, so no surface may tell a venue follower they are
 * "still being switched on".
 *
 * A source scan rather than a render test, because the claim has been written
 * both through a shared constant and by hand inside JSX, and a render test
 * only covers the surfaces someone remembered to render. Whitespace is
 * collapsed before matching so a sentence wrapped across JSX lines is still
 * one sentence.
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

const normalized = (text: string): string =>
  text.replace(/\s+/g, ' ').toLowerCase()

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

  // Reads every source file in the frontend (about 17 MB), which outlasts the
  // 5 s default when the suite runs alongside other test files.
  it('appears nowhere under frontend/', { timeout: 30_000 }, () => {
    const offenders = files.flatMap(file => {
      const text = normalized(readFileSync(file, 'utf8'))
      return RETIRED_PHRASES.filter(phrase => text.includes(phrase)).map(
        phrase => `${relative(FRONTEND_ROOT, file)}: "${phrase}"`
      )
    })
    expect(offenders).toEqual([])
  })
})
