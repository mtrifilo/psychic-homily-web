/**
 * Post-`next build` gate: refuse to ship a fetch whose response is too large to
 * enter Vercel's Data Cache.
 *
 *   bun run lib/data-cache-budget/cli.ts
 *
 * Chained after `next build` in the `build` npm script, which is also Vercel's
 * `buildCommand` (frontend/vercel.json) — so a failure here fails the deploy and
 * leaves the previous deployment serving. It runs BEFORE
 * `lib/sitemap-prerender/cli.ts` on purpose: a budget breach also costs a
 * sitemap shard its prerendered body, and if the sitemap gate ran first it would
 * fail the build blaming a backend outage. First gate to speak should be the one
 * that knows the real cause.
 *
 * IT DOES NOT RUN IN CI. `.github/workflows/ci.yml` runs typecheck, lint and
 * unit tests, not a build, so a PR can go green with an over-cap payload; the
 * Vercel preview build is the first thing that fails. That is a deliberate
 * scoping choice — the check needs a real build against a real backend, which CI
 * does not do — but it means "CI is green" is not evidence about this gate.
 *
 * Lives under lib/ rather than scripts/ for the reason lib/sitemap-prerender and
 * lib/sitemap-monitor already record: tsconfig.json excludes `scripts`, so a
 * checker written there would itself be exempt from `bun run typecheck` and
 * `bun run lint`.
 *
 * The policy, the measurements behind it and why the on-disk entry is the right
 * thing to measure all live in ./check.ts and ./budget.ts. This file is the I/O.
 */
import { existsSync, readdirSync, readFileSync, rmSync, statSync } from 'node:fs'
import { join } from 'node:path'
import {
  BREACH_LOG_PATH,
  BUILD_STAMP_PATH,
  DATA_CACHE_BUDGET_BYTES,
  DATA_CACHE_ITEM_LIMIT_BYTES,
  encodedSize,
  formatMiB,
} from './budget'
import {
  formatBudgetFailures,
  partitionOverBudget,
  type FetchCacheEntry,
} from './check'

const ROOT = join(import.meta.dirname, '..', '..')
const BUILD_DIR = join(ROOT, '.next')
const FETCH_CACHE_DIR = join(BUILD_DIR, 'cache', 'fetch-cache')
const STAMP_PATH = join(ROOT, BUILD_STAMP_PATH)
const BREACH_PATH = join(ROOT, BREACH_LOG_PATH)

function fail(message: string): never {
  console.error(`\n${message}\n`)
  process.exit(1)
}

// ---------------------------------------------------------------------------
// 1. Breaches recorded at the fetch site.
//
// These are payloads OVER the cap, which Next refuses to write to disk — so the
// scan below structurally cannot see them and this log is the only record. On a
// page route the build already failed; this covers the metadata routes, where a
// throw leaves `next build` exiting 0. See ./assert.ts for the measured matrix.
// ---------------------------------------------------------------------------
if (existsSync(BREACH_PATH)) {
  const recorded = readFileSync(BREACH_PATH, 'utf8')
    .split('\n')
    .filter(Boolean)
    .map(line => JSON.parse(line) as { url: string; rawBytes: number })

  // Read once: a stale log riding along in a restored `.next/cache` would fail
  // every later build unfixably. ./stamp.ts also clears it before each build.
  rmSync(BREACH_PATH, { force: true })

  if (recorded.length > 0) {
    const message = formatBudgetFailures(
      // Dedupe: a breached fetch is attempted once per route that makes it, so
      // the same URL can be recorded several times in one build.
      [...new Map(recorded.map(b => [b.url, b])).values()].map(breach => ({
        key: breach.url,
        url: breach.url,
        // The cap applies to the base64 envelope; these are raw bytes.
        bytes: encodedSize(breach.rawBytes),
        fraction: encodedSize(breach.rawBytes) / DATA_CACHE_ITEM_LIMIT_BYTES,
      }))
    )

    // The break-glass has to reach here too. ./assert.ts suppresses its throw
    // but still records the breach, so without this the override would only
    // move the failure from `next build` to this gate — which is not an
    // override at all.
    if (process.env.DATA_CACHE_BUDGET_ENFORCE === 'warn') {
      console.warn(`\n${message}\n\nEnforcement disabled for this build; shipping anyway.\n`)
    } else {
      fail(message)
    }
  }
}

// ---------------------------------------------------------------------------
// 2. The disk scan: entries that still FIT, but are approaching the cap.
// ---------------------------------------------------------------------------

// Written by ./stamp.ts before `next build`. Missing means the build script was
// bypassed, and a gate that cannot tell fresh entries from restored ones must
// not guess — see ./stamp.ts for what went wrong when this was inferred.
const stamp = statSync(STAMP_PATH, { throwIfNoEntry: false })
if (!stamp) {
  fail(
    `Data Cache budget check could not read ${STAMP_PATH}.\n` +
      'Run the full `bun run build` script, which stamps the build start first.'
  )
}
const buildStartedAt = new Date(readFileSync(STAMP_PATH, 'utf8')).getTime()

const filenames = (() => {
  try {
    return readdirSync(FETCH_CACHE_DIR)
  } catch (error) {
    if ((error as NodeJS.ErrnoException)?.code === 'ENOENT') return null
    throw error
  }
})()

if (filenames === null) {
  // Not a pass. A build that cached nothing is a build this half of the gate did
  // not check, and saying "OK" would be the same false-clean signal the gate
  // exists to remove.
  console.warn(
    `Data Cache budget check MEASURED NOTHING: no ${FETCH_CACHE_DIR} directory.\n` +
      'No cached fetch ran during this build, so nothing was weighed against the cap.'
  )
  process.exit(0)
}

/**
 * The envelope is `{kind, data: {url, body, ...}, revalidate, tags}` with `body`
 * base64-encoded. Only `url` is read, and only to identify the entry.
 */
function readUrl(path: string): string | undefined {
  try {
    const parsed = JSON.parse(readFileSync(path, 'utf8')) as { data?: { url?: unknown } }
    return typeof parsed.data?.url === 'string' ? parsed.data.url : undefined
  } catch {
    return undefined
  }
}

const entries: FetchCacheEntry[] = []
const freshKeys = new Set<string>()

for (const key of filenames) {
  const path = join(FETCH_CACHE_DIR, key)
  const stat = statSync(path, { throwIfNoEntry: false })
  if (!stat?.isFile()) continue

  const entry: FetchCacheEntry = {
    key,
    bytes: stat.size,
    // Only paid for entries that are going to be reported.
    url: stat.size >= DATA_CACHE_BUDGET_BYTES ? readUrl(path) : undefined,
  }
  entries.push(entry)
  if (stat.mtimeMs >= buildStartedAt) freshKeys.add(key)
}

const { failures, allowlisted } = partitionOverBudget(entries)

// Reported every build, deliberately: an entry that sits here indefinitely is a
// route drifting toward a silent cache failure, and the reminder is the only
// thing keeping it from becoming a permanent baseline.
for (const entry of allowlisted) {
  console.warn(
    `Data Cache budget WARNING (recorded exception): ${entry.url ?? entry.key} is at ` +
      `${(entry.fraction * 100).toFixed(0)}% of the ${formatMiB(DATA_CACHE_ITEM_LIMIT_BYTES)} cache-item cap. ` +
      'See WARN_BAND_ALLOWLIST in lib/data-cache-budget/budget.ts.'
  )
}

// An entry whose URL could not be read cannot be matched against the allowlist,
// so it would otherwise fail the build for a condition that may well be excused
// — unfixably, since the allowlist keys on a URL that is by definition
// unavailable. Report the real problem (the envelope shape changed) instead.
const unidentified = failures.filter(entry => entry.url === undefined)
const identified = failures.filter(entry => entry.url !== undefined)

for (const entry of unidentified) {
  console.warn(
    `Data Cache budget: entry ${entry.key} is at ${(entry.fraction * 100).toFixed(0)}% of the cap ` +
      'but its URL could not be read from the cache envelope, so WARN_BAND_ALLOWLIST ' +
      'cannot be applied to it. The envelope shape has probably changed under a Next ' +
      'upgrade — update readUrl in lib/data-cache-budget/cli.ts.'
  )
}

// Fail only on entries THIS build wrote. A restored `.next/cache` can carry an
// over-budget entry from before the fix that introduced this gate, and failing
// on it would be unfixable — including on the build that fixes it. Everything is
// still REPORTED above regardless of age, so nothing goes quiet.
const freshFailures = identified.filter(entry => freshKeys.has(entry.key))
const staleFailures = identified.filter(entry => !freshKeys.has(entry.key))

for (const entry of staleFailures) {
  console.warn(
    `Data Cache budget WARNING (from an earlier build): ${entry.url} is at ` +
      `${(entry.fraction * 100).toFixed(0)}% of the cap. Not failing this build, because this ` +
      'build did not fetch it — but it is over the warn line and needs shrinking.'
  )
}

if (freshFailures.length > 0) {
  fail(formatBudgetFailures(freshFailures))
}

const measured = entries.filter(entry => freshKeys.has(entry.key))
if (measured.length === 0) {
  // Distinguished from a pass on purpose. Every fetch was served from a warm
  // restored cache (routine on a redeploy inside the revalidate window), so this
  // half of the gate weighed nothing this time.
  console.warn(
    `Data Cache budget check MEASURED NOTHING: all ${entries.length} fetch cache ` +
      'entries predate this build, so every fetch was served from the restored cache. ' +
      'Nothing new was weighed; warnings above still apply.'
  )
  process.exit(0)
}

// "largest" must exclude the recorded exceptions, or the line contradicts
// itself — reporting a largest ABOVE the budget it claims nothing exceeded.
const excused = new Set(allowlisted.map(entry => entry.key))
const withinBudget = measured.filter(entry => !excused.has(entry.key))
const largest = withinBudget.reduce((max, e) => (e.bytes > max ? e.bytes : max), 0)

console.log(
  `Data Cache budget check OK: ${withinBudget.length} of ${measured.length} fetch cache ` +
    `${measured.length === 1 ? 'entry' : 'entries'} written by this build are under ` +
    `${formatMiB(DATA_CACHE_BUDGET_BYTES)} (largest ${formatMiB(largest)})` +
    (allowlisted.length > 0
      ? `; ${allowlisted.length} recorded exception(s) above are over it and were not failed on`
      : '') +
    `. ${entries.length - measured.length} entries from earlier builds were reported but not judged.`
)
