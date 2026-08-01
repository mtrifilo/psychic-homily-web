import * as Sentry from '@sentry/nextjs'
import {
  BUILD_TIME_API_FETCH_TIMEOUT_MS,
  createBuildTimeApiSignal,
} from '@/lib/build-time-api'

/**
 * How long a first-screen payload stays warm in Next's Data Cache.
 *
 * Matches every other list/entity fetch in `app/` (see `app/artists/[slug]`,
 * `app/venues/[slug]`, `lib/seo/fetchSeoList.ts`). An hour of drift is not the
 * exposure it looks like: `lib/proxy-revalidation.ts` calls `revalidatePath`
 * on `/shows`, `/venues` and the scene pages after every mutation that changes
 * them, so the window is a ceiling for changes made OUTSIDE the app, not the
 * latency of a submission made through it.
 */
export const FIRST_SCREEN_REVALIDATE_SECONDS = 3600

interface FetchListPayloadOptions {
  /** Absolute API URL, already carrying any query parameters. */
  url: string
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
  service,
  timeoutMs = BUILD_TIME_API_FETCH_TIMEOUT_MS,
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

    return body as T
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service },
    })
    return null
  }
}
