import * as Sentry from '@sentry/nextjs'
import {
  createBuildTimeApiSignal,
} from '@/lib/build-time-api'
import {
  DataCacheBudgetError,
  readJsonWithinDataCacheBudget,
} from '@/lib/data-cache-budget/assert'

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
 * Deliberately shorter than `BUILD_TIME_API_FETCH_TIMEOUT_MS` (10s), which was
 * argued for a build step where the only cost of waiting is a slower deploy.
 * The hydration seeds are read at request time instead: the Data Cache makes an
 * expiry a stale-while-revalidate refresh nobody waits on, but a genuine MISS —
 * a cold instance, an evicted entry — blocks a real visitor's list. Giving up
 * at 2.5s costs that visitor the server-rendered rows and nothing else; the
 * component fetches for itself either way.
 *
 * This is the DEFAULT, not a rule about where the helper runs. `/shows` calls
 * it from the page body, which is part of the static-shell prerender, and its
 * payload also feeds a JSON-LD `ItemList` — so that one call site overrides
 * back to the build budget, because giving up early there bakes a schema-less
 * page into the shell for a whole revalidate window.
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
   * were data — a venue or scene payload missing its rows would render as the
   * empty state, the "confident, wrong answer about the catalogue" this
   * helper's `null` exists to prevent.
   *
   * Only the collection array is checked. Sibling fields are NOT validated, so
   * a `shows` payload that kept its rows but lost `pagination` still gets
   * seeded. `ShowList` survives that because every read of `pagination` is
   * fully optional-chained AND the Load More control is gated on one of those
   * reads, so nothing dereferences it unguarded. That is a property of the
   * consumer, not a guarantee from here. Widening this into a per-collection
   * required-key list is a reasonable next step; do not assume from reading
   * this that it already happened.
   */
  collection: 'shows' | 'venues' | 'scenes' | 'cities' | 'artists'
  /** Sentry `service` tag, and the prefix of the reported message. */
  service: string
  /** Override only with the reason written down at the call site. */
  timeoutMs?: number
  /**
   * Data Cache lifetime, seconds. Override only with the reason written down
   * at the call site.
   *
   * The default suits a payload whose staleness degrades gradually. A payload
   * scoped to a CALENDAR PERIOD does not: at the period's boundary it stops
   * being slightly old and starts being about the wrong period. Such a caller
   * shortens this to bound that window.
   */
  revalidateSeconds?: number
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
  revalidateSeconds = FIRST_SCREEN_REVALIDATE_SECONDS,
  fetchImpl = fetch,
}: FetchListPayloadOptions): Promise<T | null> {
  try {
    const res = await fetchImpl(url, {
      next: { revalidate: revalidateSeconds },
      signal: createBuildTimeApiSignal(timeoutMs),
    })

    if (!res.ok) {
      // Every non-OK status is reported, not just 5xx: both ends are ours, so
      // a 4xx here is a defect in our own request rather than an outage. The
      // `/venues?limit=200` 422 that silently suppressed an `ItemList` in
      // production for months is the cautionary case (see
      // `app/venues/venuesMetadata.ts`).
      Sentry.captureMessage(`${service}: API returned ${res.status}`, {
        level: 'error',
        tags: { service },
        extra: { url, status: res.status },
      })
      return null
    }

    // Weighed on the way through: an over-cap response is never written to the
    // Data Cache (Next warns once and carries on), so the fetch site is the
    // only place that condition is observable. See lib/data-cache-budget.
    const body = await readJsonWithinDataCacheBudget<unknown>(url, res)
    if (body === null || typeof body !== 'object' || Array.isArray(body)) {
      Sentry.captureMessage(`${service}: response body is not an object`, {
        level: 'error',
        tags: { service },
        extra: { url },
      })
      return null
    }

    const record = body as Record<string, unknown>
    const items = record[collection]
    if (!Array.isArray(items)) {
      Sentry.captureMessage(
        `${service}: response has no "${collection}" array`,
        { level: 'error', tags: { service }, extra: { url } },
      )
      return null
    }

    // Drop null/undefined rows. `Array.isArray` admits `[null]`, and the Go
    // handlers marshal a nil element of a pointer slice (`[]*ShowResponse`) as
    // exactly that. One null row would otherwise reach `.filter(s => !!s.slug)`
    // in `app/shows/page.tsx` — OUTSIDE this try block, so it 500s the route
    // with no Sentry event from here — and reach `ShowCard` through the seeded
    // cache, crashing the client render too. `lib/seo/fetchSeoList.ts` guards
    // the same hazard; this helper replaced it on `/shows` and has to keep it.
    // `total` is deliberately left as the server reported it. It counts rows in
    // the CATALOGUE, not rows in this page, so a dropped null makes the count
    // label read "50 of 65" over 49 cards until the client revalidation lands.
    // Adjusting it would be the wrong correction to the wrong number.
    const rows = items.filter(item => item != null)
    if (rows.length !== items.length) {
      Sentry.captureMessage(
        `${service}: "${collection}" contained ${items.length - rows.length} null row(s)`,
        { level: 'error', tags: { service }, extra: { url } },
      )
    }

    return { ...record, [collection]: rows } as T
  } catch (error) {
    // Never degraded into a null seed: a build-time budget failure IS the gate,
    // and swallowing it here would restore the silence it exists to remove.
    if (error instanceof DataCacheBudgetError) throw error
    Sentry.captureException(error, {
      level: 'error',
      tags: { service },
    })
    return null
  }
}
