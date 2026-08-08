/**
 * Record when this build began, and clear the previous build's breach log.
 *
 *   bun run lib/data-cache-budget/stamp.ts
 *
 * Runs BEFORE `next build` in the `build` npm script. Two jobs, both of which
 * exist because ./cli.ts runs in a separate process after the build and has to
 * tell this build's state from a restored cache's.
 *
 * WHY NOT `.next/BUILD_ID`'s mtime, which an earlier draft used. Next writes
 * BUILD_ID partway through the build, after page-data collection — so fetches
 * made in the first half of the build look OLDER than the build itself and were
 * silently skipped. Worse, a Data Cache HIT does not rewrite the entry file at
 * all (`file-system-cache.js` re-`set`s only when the tag set drifts), so on any
 * Vercel build inside the 1 h revalidate window with `.next/cache` restored,
 * EVERY entry kept an older mtime, every entry was skipped, and the scan printed
 * "OK" having measured nothing. A stamp taken before the build starts fixes the
 * first problem; ./cli.ts's report-everything / fail-on-fresh split fixes the
 * second.
 *
 * Failure here is fatal rather than silent: without the stamp ./cli.ts cannot
 * tell fresh from restored, and a gate that cannot tell should not guess.
 */
import { mkdirSync, rmSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { BREACH_LOG_PATH, BUILD_STAMP_PATH } from './budget'

const root = join(import.meta.dirname, '..', '..')
const stampPath = join(root, BUILD_STAMP_PATH)
const breachLogPath = join(root, BREACH_LOG_PATH)

try {
  mkdirSync(dirname(stampPath), { recursive: true })
  writeFileSync(stampPath, new Date().toISOString())
  // A breach recorded by a PREVIOUS build must not fail this one — that would
  // be unfixable, since the file rides along in the restored cache.
  rmSync(breachLogPath, { force: true })
} catch (error) {
  console.error(`\nData Cache budget: could not stamp the build start.\n${String(error)}\n`)
  process.exit(1)
}
