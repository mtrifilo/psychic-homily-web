import * as Sentry from '@sentry/nextjs'
import {
  BUILD_TIME_API_FETCH_TIMEOUT_MS,
  createBuildTimeApiSignal,
} from '@/lib/build-time-api'

/**
 * How long a fetched SEO list stays warm in Next's Data Cache. These lists feed
 * a JSON-LD block that crawlers read, not anything a human waits on, so an hour
 * of staleness costs nothing.
 *
 * This hint is the ONLY source of the `1h`/`1y` window on `/artists`, `/shows`
 * and `/venues`. It binds unconditionally: builds with the backend reachable
 * AND blackholed both emit `initialRevalidateSeconds: 3600` for all three
 * routes, and both exit 0. Unlike `app/sitemap.ts`, whose route mode flips to
 * `ƒ` when the build-time fetch fails, these pages keep their window either way
 * — `fetchSeoList` fails open, so the render always succeeds and the fetch
 * always registers its hint.
 *
 * A LARGE `Age` ON THESE ROUTES IS NOT A STUCK REVALIDATION. Vercel ISR
 * regenerates on request, not on a timer: nothing runs during a quiet period,
 * so `Age` grows without bound and the first request after the quiet period
 * necessarily sees `x-vercel-cache: STALE` at whatever that gap was. It serves
 * the stale body and triggers the re-render, and the NEXT request is a fresh
 * `HIT`. Measured on PRODUCTION `/venues`, 2026-08-01, 45 s polling:
 *
 *   age 3576  HIT    entry Date 04:15:18   ← no proactive regen at the window
 *   age 3622  STALE  entry Date 04:15:18   ← first sample past 3600
 *   age   46  HIT    entry Date 05:15:42   ← rendered 1 s after the STALE hit
 *
 * `/artists` did the same thing in the same second. Read the entry `Date`
 * (`now - Age` on a HIT), never `Age` alone: `Date` advances only on a
 * SUCCESSFUL render, so a genuinely failing revalidation shows a FROZEN `Date`
 * with a climbing `Age` (PSY-1644's held sitemap). PSY-1641 recorded `/venues`
 * as "stuck STALE" from ONE sample at Age ~16 h and no follow-up; a second
 * probe 45 s later would have refuted it. One cache-header sample cannot
 * distinguish a quiet route from a broken one — take two.
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
 * On `/shows`, `/venues` and `/artists` the list a human reads is client-
 * rendered inside `<Suspense>`; the server fetch here feeds a JSON-LD
 * `ItemList` and nothing else. Throwing would turn a backend blip into a 500 on
 * a page that works fine for humans, in order to protect a crawler enrichment.
 * Reporting to Sentry and rendering without the block keeps the failure visible
 * without making it the reader's problem.
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
      const body = (await res.json()) as Record<string, unknown> | null
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
    Sentry.captureException(error, {
      level: 'error',
      tags: { service },
    })
  }
  return []
}
