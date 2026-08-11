import * as Sentry from '@sentry/nextjs'
import {
  BUILD_TIME_API_FETCH_TIMEOUT_MS,
  createBuildTimeApiSignal,
} from '@/lib/build-time-api'
import {
  DataCacheBudgetError,
  readJsonWithinDataCacheBudget,
} from '@/lib/data-cache-budget/assert'

/**
 * How long a fetched SEO list stays warm in Next's Data Cache. These lists feed
 * a JSON-LD block that crawlers read, not anything a human waits on, so an hour
 * of staleness costs nothing.
 *
 * This hint is the only source of the `1h` half of the `1h`/`1y` window on
 * `/artists`, `/shows` and `/venues`. (The `1y` is not from here — Next fills
 * expire from config `expireTime`; see `app/sitemap.ts`.) Unlike that sitemap,
 * whose backend-dependent shards drop to `ƒ` when the build-time fetch fails,
 * these three keep their window in EITHER build condition — measured on
 * 50000af1, backend reachable and backend blackholed: both exit 0, both emit
 * `◐ … 1h 1y` and `initialRevalidateSeconds: 3600` for all three. `fetchSeoList`
 * fails open, so the render always succeeds and the fetch always registers its
 * hint.
 *
 * A LARGE `Age` ON THESE ROUTES IS NOT EVIDENCE OF A STUCK REVALIDATION.
 * Regeneration is driven by an incoming REQUEST, not by a timer, so a route
 * nobody asks for is never regenerated at all: `Age` grows past the window
 * unbounded, and the first request after a quiet stretch sees `STALE` at the
 * length of that stretch, serves the stale body, and triggers the re-render.
 * Measured on PRODUCTION `/venues`, 2026-08-01 — the second block reproduces
 * the reported symptom on demand, on a route the first block proves healthy:
 *
 *   45 s polling across the window:
 *     age 3576  HIT    Date 04:15:18   not regenerated as the window nears
 *     age 3622  STALE  Date 04:15:18   first sample past 3600
 *     age   46  HIT    Date 05:15:42   re-rendered 1 s after that request
 *   then 30 min of deliberate silence, 26 min of it past expiry:
 *     age 5141  STALE  Date 05:30:48   1.4x the window, Date FROZEN
 *     age   99  HIT    Date 06:56:31   stamped 2 s after the STALE request
 *
 * `/artists` crossed the window one second after `/venues` with the same shape.
 *
 * DIAGNOSING ONE OF THESE FOR REAL. `Age` alone tells you nothing, and neither
 * does a frozen `Date` under a climbing `Age` — the healthy quiet case above
 * shows exactly that (5141 s, `Date` stuck at 05:30:48). Two things separate a
 * quiet route from a broken one, and both need the entry `Date` (`now - Age` on
 * a HIT), which advances only on a SUCCESSFUL render:
 *
 *   1. Sample again AFTER the one that returned `STALE`. Healthy: the `Date`
 *      has moved, because that request triggered the re-render. Broken: it is
 *      still frozen across repeated requests — PSY-1652 measured that on a
 *      route rigged to fail, and PSY-1644 on the sitemap held by a slow feed.
 *   2. Compare the `Date` against the last deploy. A `Date` later than the
 *      build cannot be a build artifact, so the route has revalidated at least
 *      once since — this is what settled `/venues` (13 h 19 m after its build).
 *
 * PSY-1641 read `/venues` as "stuck STALE" from ONE sample at Age ~16 h. Either
 * check above would have refuted it. Take two samples.
 */
export const SEO_LIST_REVALIDATE_SECONDS = 3600

interface SeoListOptions {
  /** Absolute API URL, already carrying any query parameters. */
  url: string
  /**
   * The response body key holding the array. A union rather than `string`
   * because a typo here produces a permanently empty `ItemList` that only
   * surfaces at runtime — the same discovery path as the bug this helper fixes.
   */
  collection: 'shows' | 'venues' | 'artists'
  /** Sentry `service` tag, and the prefix of the reported message. */
  service: string
  /** Override only with the reason written down at the call site. */
  timeoutMs?: number
  /** Injection seam for tests; production always uses the global `fetch`. */
  fetchImpl?: typeof fetch
}

/**
 * Fetch a list that feeds only SEO enrichment, never user-visible content.
 *
 * THIS FAILS OPEN ON PURPOSE, and this is the one place that decision is made.
 * What it feeds is a JSON-LD `ItemList` and nothing else, so throwing would
 * turn a backend blip into a 500 on a page that works fine for humans, in order
 * to protect a crawler enrichment. Reporting to Sentry and rendering without
 * the block keeps the failure visible without making it the reader's problem.
 *
 * Read the "and nothing else" literally, because PSY-1624 narrowed it. The
 * human-visible list is now server-rendered on `/shows`, `/venues` and — since
 * PSY-1774 bounded `GET /artists` — `/artists`, but from
 * `lib/ssr/fetchListPayload.ts`, not from here: `/venues` and `/artists` still
 * call this for their `ItemList` alongside that, and `/shows` no longer calls
 * it at all. No browse list is client-only any more.
 *
 * The opposite call is right whenever the fetch IS the artifact — see
 * `app/sitemap.ts`, where an empty document is a false success, so the fetch
 * throws instead. Load-bearing fetches must not use this helper.
 *
 * Every non-OK status is reported, not just 5xx: a 4xx here is a defect in our
 * own request rather than a backend outage, both ends being ours. Treating one
 * as unremarkable is how `/venues?limit=200` rendered without its `ItemList` in
 * production for months — see the note on `VENUE_LIST_LIMIT`.
 */
export async function fetchSeoList<T>({
  url,
  collection,
  service,
  timeoutMs = BUILD_TIME_API_FETCH_TIMEOUT_MS,
  fetchImpl = fetch,
}: SeoListOptions): Promise<T[]> {
  try {
    const res = await fetchImpl(url, {
      next: { revalidate: SEO_LIST_REVALIDATE_SECONDS },
      signal: createBuildTimeApiSignal(timeoutMs),
    })
    if (res.ok) {
      // Weighs the body against the Data Cache item cap on the way through. An
      // over-cap response is never written to that cache — Next logs one
      // console.warn and carries on — so the fetch site is the only place the
      // condition is observable. See lib/data-cache-budget/budget.ts.
      const body = await readJsonWithinDataCacheBudget<Record<string, unknown> | null>(
        url,
        res
      )
      const items = body?.[collection]
      // A 200 whose collection key is missing or not an array is a contract
      // break, not an empty list — report it rather than rendering as though
      // the catalogue were genuinely empty.
      if (Array.isArray(items)) {
        // Drop null/undefined elements: `Array.isArray` admits `[null]`, and
        // every caller dereferences `item.slug` OUTSIDE this try block, so one
        // null element would 500 the page — with no Sentry event, since the
        // throw escapes this catch. `app/sitemap.ts` guards the same hazard.
        return items.filter(item => item != null) as T[]
      }
      Sentry.captureMessage(`${service}: response has no "${collection}" array`, {
        level: 'error',
        tags: { service },
        extra: { url },
      })
    } else {
      Sentry.captureMessage(`${service}: API returned ${res.status}`, {
        level: 'error',
        tags: { service },
        extra: { url, status: res.status },
      })
    }
  } catch (error) {
    // The one thing this helper must NOT fail open on. A build-time budget
    // failure IS the gate; absorbing it here would restore exactly the silence
    // it exists to remove.
    if (error instanceof DataCacheBudgetError) throw error
    Sentry.captureException(error, {
      level: 'error',
      tags: { service },
    })
  }
  return []
}
