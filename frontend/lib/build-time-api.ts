import * as Sentry from '@sentry/nextjs'

export const BUILD_TIME_API_FETCH_TIMEOUT_MS = 10_000

export function createBuildTimeApiSignal(): AbortSignal {
  return AbortSignal.timeout(BUILD_TIME_API_FETCH_TIMEOUT_MS)
}

/**
 * How long a fetched SEO list stays warm in Next's Data Cache. These lists feed
 * a JSON-LD block that crawlers read, not anything a human waits on, so an hour
 * of staleness costs nothing.
 */
export const SEO_LIST_REVALIDATE_SECONDS = 3600

interface SeoListOptions {
  /** Absolute API URL, already carrying any query parameters. */
  url: string
  /** The response body key holding the array — `shows`, `venues`, `artists`. */
  collection: string
  /** Sentry `service` tag, and the prefix of the reported message. */
  service: string
  /** Override only with a reason written down at the call site. */
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
 * Every non-OK status is reported, not just 5xx. A 4xx here is a defect in our
 * own request rather than a backend outage — both ends are ours — and treating
 * it as unremarkable is how `/venues?limit=200` sat in production returning 422
 * (the endpoint caps `limit` at 100) with its `ItemList` silently absent from
 * every render.
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
      signal: AbortSignal.timeout(timeoutMs),
    })
    if (res.ok) {
      const body: unknown = await res.json()
      const items = (body as Record<string, unknown> | null)?.[collection]
      // A 200 whose collection key is missing or not an array is a contract
      // break, not an empty list — report it rather than rendering as though
      // the catalogue were genuinely empty.
      if (Array.isArray(items)) {
        return items as T[]
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
