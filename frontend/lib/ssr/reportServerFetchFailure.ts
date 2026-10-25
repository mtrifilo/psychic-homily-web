import * as Sentry from '@sentry/nextjs'

interface ServerFetchFailure {
  /** The non-OK response. Only its status and `Retry-After` are read. */
  response: Response
  /** Sentry `service` tag, and the prefix of the reported message. */
  service: string
  /**
   * The backend route the request addressed, as a template with no query
   * string, e.g. `/artists/{slug}`. A query string never belongs here: it can
   * carry values nobody reviewed for a third-party error tracker.
   */
  pathFamily: string
  /**
   * Whether a 404 is an ANSWER rather than a defect. True for a read of one
   * entity, where a 404 says the slug does not exist and is expected; false for
   * a list endpoint, where a 404 means our own request is wrong.
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
 * Report a server-side backend read that did not answer 2xx, or stay silent
 * when its status is an expected answer.
 *
 * Reported: every 5xx, and every 4xx except a 404 that `notFoundIsAnswer`
 * covers. A 429 is the case that matters most: the callers turn a failed read
 * into a fallback (an entity page into its not-found render at HTTP 200, a list
 * into its client fetch), so without this a refused read leaves no trace.
 *
 * The event carries the path family, the status, `Retry-After` when the
 * backend sent one, and the caller's `context`. It never carries the URL.
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

  const retryAfter = response.headers.get('retry-after')
  Sentry.captureMessage(`${service}: API returned ${status}`, {
    level,
    tags: { service, path_family: pathFamily, status: String(status) },
    extra: {
      ...context,
      pathFamily,
      status,
      ...(retryAfter === null ? {} : { retryAfter }),
    },
  })
}
