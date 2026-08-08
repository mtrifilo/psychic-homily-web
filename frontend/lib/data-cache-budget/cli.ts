/**
 * Post-`next build` gate: refuse to ship a fetch whose response is too large to
 * enter Vercel's Data Cache.
 *
 *   bun run lib/data-cache-budget/cli.ts
 *
 * Chained after `next build` in the `build` npm script, which is also Vercel's
 * `buildCommand` (frontend/vercel.json) — so a failure here fails the deploy and
 * leaves the previous deployment serving.
 *
 * Lives under lib/ rather than scripts/ for the reason lib/sitemap-prerender and
 * lib/sitemap-monitor already record: tsconfig.json excludes `scripts`, so a
 * checker written there would itself be exempt from `bun run typecheck` and
 * `bun run lint`.
 *
 * The policy, the measurements behind it and why the on-disk entry is the right
 * thing to measure all live in ./check.ts. This file is only the I/O.
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { DATA_CACHE_BUDGET_BYTES, DATA_CACHE_ITEM_LIMIT_BYTES } from './budget'
import {
  findAllowlisted,
  findOverBudget,
  formatBudgetFailures,
  type FetchCacheEntry,
} from './check'

const BUILD_DIR = join(import.meta.dirname, '..', '..', '.next')
const FETCH_CACHE_DIR = join(BUILD_DIR, 'cache', 'fetch-cache')
const BUILD_ID_PATH = join(BUILD_DIR, 'BUILD_ID')

function fail(message: string): never {
  console.error(`\n${message}\n`)
  process.exit(1)
}

// `next build` always writes BUILD_ID, so a missing one means this ran outside a
// build rather than on a build with nothing to check — an error, not a skip.
const buildStartedAt = statSync(BUILD_ID_PATH, { throwIfNoEntry: false })?.mtimeMs
if (buildStartedAt === undefined) {
  fail(
    `Data Cache budget check could not read ${BUILD_ID_PATH}.\n` +
      'Run it after `next build`, from the frontend directory.'
  )
}

// Entries are keyed by hash with no extension and no index, so the directory
// listing is the only enumeration available.
const filenames = (() => {
  try {
    return readdirSync(FETCH_CACHE_DIR)
  } catch (error) {
    if ((error as NodeJS.ErrnoException)?.code === 'ENOENT') {
      // No cached fetch ran during this build. Legitimate — a build can serve
      // every fetch from a warm restored cache, and a build that fetched
      // nothing when it should have is the sitemap prerender gate's job, not
      // this one's.
      return null
    }
    throw error
  }
})()

if (filenames === null) {
  console.log(
    `Data Cache budget check: no ${FETCH_CACHE_DIR} directory, nothing this build cached to measure.`
  )
  process.exit(0)
}

/**
 * The envelope is `{kind, data: {url, body, ...}, revalidate, tags}` with `body`
 * base64-encoded. Only `url` is read, and only to name the offender: the size
 * assertion is on the FILE, so a changed envelope shape degrades the message
 * rather than the check.
 */
function readUrl(path: string): string | undefined {
  try {
    const parsed = JSON.parse(readFileSync(path, 'utf8')) as {
      data?: { url?: unknown }
    }
    return typeof parsed.data?.url === 'string' ? parsed.data.url : undefined
  } catch {
    return undefined
  }
}

const entries: FetchCacheEntry[] = []
let skippedStale = 0

for (const key of filenames) {
  const path = join(FETCH_CACHE_DIR, key)
  const stat = statSync(path, { throwIfNoEntry: false })
  if (!stat?.isFile()) continue

  // Written before this build started: a previous build's entry, already judged
  // by that build's gate. See ./check.ts for why skipping is the safe direction.
  if (stat.mtimeMs < buildStartedAt) {
    skippedStale += 1
    continue
  }

  entries.push({
    key,
    bytes: stat.size,
    // Only paid for entries that are going to be reported.
    url: stat.size >= DATA_CACHE_BUDGET_BYTES ? readUrl(path) : undefined,
  })
}

// Reported every build, deliberately: an entry that sits here indefinitely is a
// route drifting toward a silent cache failure, and the reminder is the only
// thing keeping it from becoming a permanent baseline.
for (const entry of findAllowlisted(entries)) {
  console.warn(
    `Data Cache budget WARNING (recorded exception): ${entry.url ?? entry.key} is at ` +
      `${((entry.bytes / DATA_CACHE_ITEM_LIMIT_BYTES) * 100).toFixed(0)}% of the 2 MB cache-item cap. ` +
      'See WARN_BAND_ALLOWLIST in lib/data-cache-budget/budget.ts.'
  )
}

const failures = findOverBudget(entries)

if (failures.length > 0) {
  fail(formatBudgetFailures(failures))
}

const largest = entries.reduce((max, e) => (e.bytes > max ? e.bytes : max), 0)
console.log(
  `Data Cache budget check OK: ${entries.length} fetch cache ${
    entries.length === 1 ? 'entry' : 'entries'
  } from this build under ${(DATA_CACHE_BUDGET_BYTES / 1024 / 1024).toFixed(2)} MB ` +
    `(largest ${(largest / 1024 / 1024).toFixed(2)} MB)` +
    (skippedStale > 0 ? `; skipped ${skippedStale} from earlier builds.` : '.')
)
