import { join } from 'node:path'

/**
 * The numbers behind the fetch Data Cache budget, in one place.
 *
 * ALL OF THESE WERE READ OUT OF NEXT'S OWN ENFORCEMENT, not inferred from docs
 * — three confident readings of this route family were wrong before anyone
 * measured. `next/dist/server/lib/incremental-cache/index.js` does:
 *
 *   const itemSize = JSON.stringify(data).length
 *   if (ctx.fetchCache && itemSize > 2 * 1024 * 1024 && !hasCustomCacheHandler
 *       && !ctx.isImplicitBuildTimeCache) {
 *     const warningText = `Failed to set Next.js data cache for ${url}, ` +
 *       `items over 2MB can not be cached (${itemSize} bytes)`
 *     if (dev) throw new Error(warningText)   // E394
 *     console.warn(warningText)
 *     return                                  // <- the entry is NOT written
 *   }
 *
 * Three consequences that shape everything in this directory:
 *
 *  1. `data` is the envelope whose `body` holds the response BASE64-ENCODED, so
 *     the size that binds is ~1.334x the raw body. Verified by decoding a real
 *     `.next/cache/fetch-cache` entry: 1,149,272 raw bytes were held as a
 *     1,533,430-byte entry. Next reported 4,312,227 bytes for the 3,233,345-byte
 *     `GET /artists` response, which is the same ratio.
 *  2. There IS a signal, contrary to how this was first reported: a thrown
 *     Error in dev, a `console.warn` in a build. But a build emits hundreds of
 *     lines and nothing fails, so it scrolled past for at least ten days while
 *     `/artists` re-pulled 3 MB from origin on every render.
 *  3. An over-cap entry is never written to disk. So scanning
 *     `.next/cache/fetch-cache` sizes CANNOT see the failure itself — only the
 *     approach to it. Catching the failure needs the assertion in ./assert.ts.
 *     Both halves are load-bearing; neither is sufficient.
 *
 * Vercel documents the same 2 MB ceiling on both Data Cache and Runtime Cache
 * ("items larger won't be cached"), so the limit does not change on deploy.
 */

/** Next's per-item cap, and Vercel's. Applied to the base64 envelope. */
export const DATA_CACHE_ITEM_LIMIT_BYTES = 2 * 1024 * 1024

/**
 * Base64 grows 4 bytes for every 3. The single expression of that fact — both
 * the raw limit below and `encodedSize` are derived from it, so they cannot
 * disagree. Not exported: everything callers need is already derived here.
 */
const BASE64_INFLATION = 4 / 3

/**
 * The largest raw response body that still fits the cap once encoded: ~1.5 MB.
 * This is the number to reason with when looking at a `curl` byte count.
 */
export const DATA_CACHE_RAW_LIMIT_BYTES = Math.floor(
  DATA_CACHE_ITEM_LIMIT_BYTES / BASE64_INFLATION
)

/**
 * Fail below the cap, so a gate fires while there is still headroom to act.
 *
 * `/artists` went from 73% of the cap to 206% in under two weeks of ordinary
 * catalogue growth. A line drawn at 100% would first go red on a deploy that
 * had ALREADY shipped a silently-uncached route, which is too late to be worth
 * much; 80% leaves roughly a growth spurt's warning.
 */
export const DATA_CACHE_BUDGET_FRACTION = 0.8

/** Warn line for an encoded, on-disk cache entry. */
export const DATA_CACHE_BUDGET_BYTES = Math.floor(
  DATA_CACHE_ITEM_LIMIT_BYTES * DATA_CACHE_BUDGET_FRACTION
)

/** Warn line for a raw response body, the same fraction of the raw limit. */
export const DATA_CACHE_RAW_BUDGET_BYTES = Math.floor(
  DATA_CACHE_RAW_LIMIT_BYTES * DATA_CACHE_BUDGET_FRACTION
)

/** Encoded entry size a raw body of `bytes` would occupy, wrapper excluded. */
export const encodedSize = (bytes: number): number => Math.ceil(bytes * BASE64_INFLATION)

/**
 * Mebibytes, and labelled as such. The cap is 2 × 1024², so reporting it in
 * decimal MB would put the label, the arithmetic and Next's own constant into
 * three-way disagreement — on a change whose whole premise is that unmeasured
 * readings of this route family were repeatedly wrong.
 */
export const formatMiB = (bytes: number): string => `${(bytes / 1024 / 1024).toFixed(2)} MiB`

/**
 * Where the fetch-site assertion records a breach for ./cli.ts to fail on, and
 * where ./stamp.ts records when the build began.
 *
 * Under `.next/cache` because that survives `next build` (Next's distDir clean
 * excludes /^(cache|dev|lock)/), and the assertion runs inside the build's
 * render workers while the CLI runs afterwards in a different process — a file
 * is the only channel between them.
 *
 * FUNCTIONS, not constants, and resolved from `process.cwd()` rather than from
 * this module's location. Two constraints meet here:
 *
 *   - `import.meta.dirname` is NOT available: this module is imported by app
 *     code (./assert.ts, reached from app/sitemap.ts), so the bundler evaluates
 *     it in a context where that is undefined. Using it crashed the build at
 *     module-eval time with ERR_INVALID_ARG_TYPE.
 *   - A bare relative string is worse: the writer resolved it against the
 *     worker's cwd while the readers resolved it against their module
 *     directory, so the two halves agreed only by coincidence and a chdir would
 *     have disarmed the metadata-route half in silence.
 *
 * `process.cwd()` is correct for every invocation this repo supports, because
 * all three stages are npm scripts in the same `bun run build` chain (which is
 * also Vercel's buildCommand) and therefore share one cwd, which the render
 * workers inherit. Resolving lazily also keeps module-eval free of I/O
 * assumptions. If the cwd ever did differ, ./cli.ts fails loudly on the missing
 * stamp rather than passing quietly — the failure mode is a red build, not a
 * disarmed gate.
 */
export const breachLogPath = (): string =>
  join(process.cwd(), '.next', 'cache', 'data-cache-budget-breaches.jsonl')

export const buildStampPath = (): string =>
  join(process.cwd(), '.next', 'cache', 'data-cache-budget-build-start')

/**
 * Fetches known to be in the warn band already, which the gate reports but does
 * not fail on.
 *
 * CURRENTLY EMPTY, and that is the intended steady state. A strict gate landing
 * on a codebase that already has a violation has two honest options: fix the
 * violation in the same change, or record it here. The one entry this list has
 * ever held was the sitemap's `releases` family at 97% of the cap, recorded
 * because the real fix — sub-sharding the family — was the sitemap's own
 * decision to make and would have more than doubled that diff. PSY-1763 made
 * that decision, so the entry went away rather than aging in place.
 *
 * IT WAIVES THE WARN BAND ONLY. A payload over the HARD cap has already stopped
 * being cached, so no entry excuses it and both halves of the gate enforce that
 * independently.
 *
 * THE ENTRY IS THE TICKET — but not only the ticket. Anything listed here is a
 * route heading for a silent cache failure on the current growth curve, so an
 * entry that has sat here for a while is a bug, not a baseline. `measuredAt` and
 * `ticket` are required: the first makes the age visible without reading prose,
 * the second means the deferral shows up in triage rather than only in a build
 * log. Nothing may be added without a measurement, a reason, and a ticket.
 *
 * `match` is compared against the fetch URL's pathname + `family` query, not as
 * a bare substring, so an entry excuses exactly the fetch it was measured
 * against rather than anything that happens to contain the string. Both halves
 * of the gate feed it the same absolute URL — the scan reads `data.url` from the
 * cache envelope, and every call site passes the URL it fetched — so an entry
 * cannot match one half and silently miss the other.
 */
export const WARN_BAND_ALLOWLIST: ReadonlyArray<{
  /** `pathname`, optionally `?family=…`, of the fetch this excuses. */
  match: string
  /** ISO date of the measurement in `reason`. Required: age is the signal. */
  measuredAt: string
  /** PSY ticket that removes the need for this entry. Required. */
  ticket: string
  reason: string
}> = []

/**
 * Whether `url` is a recorded, still-cacheable warn-band exception.
 *
 * Matched on the parsed URL rather than by substring: `?family=releases` must
 * not also excuse `?family=releases_v2`, or any unrelated URL that happens to
 * carry the string in a query value. A URL that cannot be parsed is not
 * allowlisted — the caller decides what to do with an unidentifiable entry.
 */
export function isWarnBandAllowlisted(url: string | undefined): boolean {
  if (!url) return false

  let identity: string
  try {
    const parsed = new URL(url)
    const family = parsed.searchParams.get('family')
    identity = family ? `${parsed.pathname}?family=${family}` : parsed.pathname
  } catch {
    return false
  }

  return WARN_BAND_ALLOWLIST.some(entry => entry.match === identity)
}
