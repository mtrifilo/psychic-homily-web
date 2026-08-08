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
 * Base64 grows 4 bytes for every 3. Used to convert the cap into a budget that
 * can be compared against a RAW response body, which is the only size a fetch
 * caller can cheaply measure.
 */
export const BASE64_INFLATION = 4 / 3

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
export const encodedSize = (bytes: number): number => Math.ceil(bytes / 3) * 4

export const formatMib = (bytes: number): string => `${(bytes / 1024 / 1024).toFixed(2)} MB`

/**
 * Fetches known to be in the warn band already, which the gate reports but does
 * not fail on.
 *
 * A strict gate landing on a codebase that already has a violation has two
 * honest options: fix the violation in the same change, or record it here. This
 * one is recorded because it belongs to a different design — the sitemap shards
 * by FAMILY (PSY-1622), and the fix for a single family outgrowing a shard is to
 * sub-shard that family, which is the sitemap's own decision to make and would
 * more than double this diff.
 *
 * THE ENTRY IS THE TICKET. Anything listed here is a route heading for a silent
 * cache failure on the current growth curve, so an entry that has been here for
 * a while is a bug, not a baseline. Nothing may be added without a measurement
 * and a reason, and nothing here is exempt from the HARD cap — a genuine breach
 * still fails the build for every URL, allowlisted or not.
 */
export const WARN_BAND_ALLOWLIST: ReadonlyArray<{
  /** Matched as a substring of the fetch URL. */
  match: string
  reason: string
}> = [
  {
    match: 'sitemap/entries?family=releases',
    reason:
      'Measured 2026-08-08 at 1.93 MB encoded, 97% of the cap — the largest sitemap ' +
      'family, already sharded per family by PSY-1622. Sub-sharding releases is the ' +
      'real fix and is its own change. It is close: expect a breach, and a failing ' +
      'build, on the next sizeable release import.',
  },
]

/** Whether `url` is a recorded, still-cacheable warn-band exception. */
export function isWarnBandAllowlisted(url: string | undefined): boolean {
  if (!url) return false
  return WARN_BAND_ALLOWLIST.some(entry => url.includes(entry.match))
}
