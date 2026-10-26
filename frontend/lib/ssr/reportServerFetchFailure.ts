import * as Sentry from '@sentry/nextjs'

interface ServerFetchFailure {
  /** The non-OK response. Only its status and `Retry-After` are read. */
  response: Response
  /** Sentry `service` tag, and the prefix of the reported message. */
  service: string
  /**
   * The backend route the request addressed, with no query string: a template
   * such as `/artists/{slug}`, or for a list read its pathname
   * (`listPathFamily`). A query string never belongs here: it can carry values
   * nobody reviewed for a third-party error tracker.
   */
  pathFamily: string
  /**
   * Whether a 404 from this route is an ANSWER ("that does not exist", expected
   * whenever the URL a visitor asked for names nothing) rather than a sign that
   * our own request is wrong.
   */
  notFoundIsAnswer: boolean
  level?: 'error' | 'warning'
  /**
   * Which entity the read was about (its slug, a period key), so triage can
   * find it. Values from the path only; never a query string or a credential.
   */
  context?: Record<string, string | number | undefined>
}

/**
 * The path family of a LIST endpoint read: the request's pathname, with the
 * query string and origin dropped. A list path carries at most a public entity
 * slug or id, so it is reported as is rather than templated.
 */
export function listPathFamily(url: string): string {
  return new URL(url, 'http://relative.invalid').pathname
}

/**
 * The query parameter NAMES of a list read, comma-joined, so a report can say
 * which parameters the failing request carried without carrying their values.
 */
export function listQueryParamNames(url: string): string {
  return [...new URL(url, 'http://relative.invalid').searchParams.keys()].join(',')
}

/**
 * How often one route template may report, per server instance: at most one
 * event per window. The first failure always reports; later ones inside the
 * window are counted, and the count rides on the next event the route emits.
 * A storm of 429s or 5xx is therefore bounded per route the same way. A count
 * still held when a route's failures stop, or when the instance ends, is never
 * sent: the event that opened the window is the only record of that burst.
 *
 * The key is the route template alone, whatever the service, status or level,
 * so a 4xx that opens a route's window holds back a 5xx on the same route
 * until the window closes.
 */
export const REPORT_WINDOW_MS = 60_000

/**
 * Upper bound on the routes tracked at once. Keys are route templates, so the
 * real set is small; past the bound the entry that reported longest ago is
 * dropped, which costs that route one extra event and the count it was
 * holding.
 */
const MAX_TRACKED_ROUTES = 500

/** Per route template: when it last reported, and failures held back since. */
const routeReports = new Map<string, { reportedAt: number; suppressed: number }>()

/** Forget every route's window, so one test's reports cannot hold back another's. */
export function resetServerFetchReportThrottleForTests(): void {
  routeReports.clear()
}

/**
 * Report a server-side backend read that did not answer 2xx, or stay silent
 * when its status is an expected answer.
 *
 * Reported: every 5xx, and every 4xx except a 404 that `notFoundIsAnswer`
 * covers. A 429 is the case that matters most: the callers turn a failed read
 * into a fallback (an entity page into its not-found render at HTTP 200, a list
 * into its client fetch), so without this a refused read leaves no trace.
 *
 * Throttled per `pathFamily` (see REPORT_WINDOW_MS). An event carries
 * `suppressedSinceLastReport`: how many failures of the same route the
 * window held back before it.
 *
 * The fields this sets are the path family, the status, `Retry-After` when
 * the backend sent one, the suppressed count, and the caller's `context`; none
 * of them is the URL. The SDK's own fetch breadcrumbs on the same scope are
 * outside this helper.
 */
export function reportServerFetchFailure({
  response,
  service,
  pathFamily,
  notFoundIsAnswer,
  level = 'error',
  context = {},
}: ServerFetchFailure): void {
  const { status } = response
  if (status === 404 && notFoundIsAnswer) return

  const now = Date.now()
  const previous = routeReports.get(pathFamily)
  if (previous && now - previous.reportedAt < REPORT_WINDOW_MS) {
    previous.suppressed += 1
    return
  }
  const suppressed = previous?.suppressed ?? 0
  routeReports.delete(pathFamily)
  if (routeReports.size >= MAX_TRACKED_ROUTES) {
    const oldest = routeReports.keys().next()
    if (!oldest.done) routeReports.delete(oldest.value)
  }
  routeReports.set(pathFamily, { reportedAt: now, suppressed: 0 })

  const retryAfter = response.headers.get('retry-after')
  Sentry.captureMessage(`${service}: API returned ${status}`, {
    level,
    tags: { service, path_family: pathFamily, status: String(status) },
    extra: {
      ...context,
      pathFamily,
      status,
      ...(retryAfter === null ? {} : { retryAfter }),
      suppressedSinceLastReport: suppressed,
    },
  })
}
