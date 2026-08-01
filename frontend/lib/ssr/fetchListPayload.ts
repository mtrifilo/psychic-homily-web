import * as Sentry from '@sentry/nextjs'
import {
  createBuildTimeApiSignal,
} from '@/lib/build-time-api'

/**
 * How long a first-screen payload stays warm in Next's Data Cache.
 *
 * Matches every other list/entity fetch in `app/` (see `app/artists/[slug]`,
 * `app/venues/[slug]`, `lib/seo/fetchSeoList.ts`). An hour of drift is not the
 * exposure it looks like: `lib/proxy-revalidation.ts` calls `revalidatePath`
 * on `/shows`, `/venues` and `/scenes` after every mutation that changes them
 * — `/scenes` was added there for exactly this reason — so the window bounds
 * changes made OUTSIDE the app, not the latency of a submission made through
 * it. Verify that list rather than trusting this sentence if you change it.
 */
export const FIRST_SCREEN_REVALIDATE_SECONDS = 3600

/**
 * How long a first-screen fetch may block before the page gives up on it.
 *
 * Deliberately NOT `BUILD_TIME_API_FETCH_TIMEOUT_MS` (10s). That budget was
 * argued for a build step, where the only cost of waiting is a slower deploy.
 * This runs at request time: the Data Cache makes an expiry a
 * stale-while-revalidate refresh nobody waits on, but a genuine MISS — a cold
 * instance, an evicted entry — blocks a real visitor's list. Giving up at 2.5s
 * costs that visitor the server-rendered rows and nothing else; the component
 * fetches for itself either way. Ten seconds of blank page would be the worse
 * trade.
 */
export const FIRST_SCREEN_FETCH_TIMEOUT_MS = 2_500

interface FetchListPayloadOptions {
  /** Absolute API URL, already carrying any query parameters. */
  url: string
  /**
   * The response key that must hold an array for the body to be usable.
   *
   * A 200 whose shape has drifted is a contract break, not a short list, and
   * without this check it would be seeded into the query cache as though it
   * were data. The consequences are not uniform: a venue or scene payload
   * missing its rows renders as the empty state — the "confident, wrong answer
   * about the catalogue" this helper's `null` exists to prevent — while a
   * shows payload missing `pagination` throws during render, because
   * `ShowList` reads `data?.pagination.has_more` and the optional chain only
   * guards `data`. Checking here keeps both out of the cache.
   */
  collection: 'shows' | 'venues' | 'scenes' | 'cities'
  /** Sentry `service` tag, and the prefix of the reported message. */
  service: string
  /** Override only with the reason written down at the call site. */
  timeoutMs?: number
  /** Injection seam for tests; production always uses the global `fetch`. */
  fetchImpl?: typeof fetch
}

/**
 * Fetch a list endpoint server-side and return its whole parsed body, or
 * `null` when the request did not produce one.
 *
 * The `null` is the point, and it is what separates this from
 * `lib/seo/fetchSeoList.ts`. That helper returns `[]` on failure because it
 * feeds a JSON-LD block: a dropped `ItemList` costs a crawler an enrichment
 * and costs the reader nothing. Here the payload seeds the list the reader
 * actually sees, and `[]` would render as the "No upcoming shows at this
 * time." empty state — a backend outage reported to the visitor as a
 * confident, wrong answer about the catalogue. `null` cannot be mistaken for
 * "the catalogue is empty" by a caller that has to destructure it.
 *
 * It does NOT throw. Throwing would hand `error.tsx` a page the browser could
 * have rendered on its own: the list components fetch client-side too, so a
 * server-side blip that the client request survives is invisible to the
 * reader, and one the client also hits surfaces as that component's own
 * "Failed to load … / Retry" state — an error state, from the layer that can
 * retry it. Callers pass `null` straight through by skipping the hydration
 * seed; the client then owns the outcome.
 *
 * Returns the FULL body rather than one collection, because the caller seeds a
 * TanStack Query cache entry and the client hook expects the whole response
 * shape (`pagination`, `total`, and the rows).
 */
export async function fetchListPayload<T>({
  url,
  collection,
  service,
  timeoutMs = FIRST_SCREEN_FETCH_TIMEOUT_MS,
  fetchImpl = fetch,
}: FetchListPayloadOptions): Promise<T | null> {
  try {
    const res = await fetchImpl(url, {
      next: { revalidate: FIRST_SCREEN_REVALIDATE_SECONDS },
      signal: createBuildTimeApiSignal(timeoutMs),
    })

    if (!res.ok) {
      // Every non-OK status is reported, not just 5xx: both ends are ours, so
      // a 4xx here is a defect in our own request rather than an outage. The
      // `/venues?limit=200` 422 that silently suppressed an `ItemList` in
      // production for months is the cautionary case (see `VENUE_LIST_LIMIT`).
      Sentry.captureMessage(`${service}: API returned ${res.status}`, {
        level: 'error',
        tags: { service },
        extra: { url, status: res.status },
      })
      return null
    }

    const body = (await res.json()) as unknown
    if (body === null || typeof body !== 'object' || Array.isArray(body)) {
      Sentry.captureMessage(`${service}: response body is not an object`, {
        level: 'error',
        tags: { service },
        extra: { url },
      })
      return null
    }

    if (!Array.isArray((body as Record<string, unknown>)[collection])) {
      Sentry.captureMessage(
        `${service}: response has no "${collection}" array`,
        { level: 'error', tags: { service }, extra: { url } },
      )
      return null
    }

    return body as T
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service },
    })
    return null
  }
}
