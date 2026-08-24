/**
 * Client-side retry policy for HTTP 429 (rate limited).
 *
 * WHY THIS EXISTS
 *
 * The shared query client used to treat every 4xx as non-retryable. That is
 * right for 400/403/404 (asking again cannot change the answer) and wrong for
 * 429, which means "ask again later". Because the policy lumped 429 in with the
 * rest, one rate-limited response turned a page block into a permanent failure
 * ("Failed to load discography") that only a manual reload could clear, even
 * though the budget refilled seconds later. An anonymous entity page fans out
 * into roughly fifteen parallel reads against a shared 100/min per-IP budget,
 * so clipping that budget mid-page is an ordinary event, not an exceptional
 * one.
 *
 * This module is deliberately PURE: no timers, no Sentry, no module state. It
 * hands React Query two functions and lets React Query own the waiting, so
 * there is no hand-rolled timer to leak on unmount or in tests. Telemetry
 * lives in `./rate-limit-telemetry`.
 *
 * WHAT THE BACKEND ACTUALLY TELLS US (measured, not assumed)
 *
 * The delay schedule below is shaped by four facts about the limiter, all
 * verified in `backend/internal/api/middleware/ratelimit.go` and in
 * `go-chi/httprate@v0.15.0`:
 *
 *  1. `Retry-After` is a CONSTANT, not a computed reset time. The 429 handler
 *     hardcodes `"60"` (ratelimit.go `RateLimitExceededHandler`, and the twin
 *     in `internal/api/routes/shared.go`), and httprate independently sets it
 *     to the window length. So the header says "one whole window" no matter
 *     how much of the window has already elapsed. Honoring it literally would
 *     park a page block on a spinner for a full minute for a bucket that has
 *     usually already drained.
 *
 *  2. The limiter is a SLIDING window, not a fixed one. httprate scores
 *     `prevCount * (window - elapsed) / window + currCount`, so the budget
 *     drains continuously rather than resetting in a step at the boundary.
 *     There is therefore no single instant to wait for: capacity reappears
 *     gradually, which makes an earlier probe genuinely useful.
 *
 *  3. A rejected request does NOT consume budget. httprate increments the
 *     counter only on the allow path, after the reject has already returned.
 *     So a retry that gets 429'd again does not push recovery further out; it
 *     costs a round trip and nothing else. That is what makes probing before
 *     the advertised 60s safe rather than self-defeating.
 *
 *  4. In production the browser CANNOT READ `Retry-After` at all. The frontend
 *     calls `https://api.psychichomily.com` cross-origin, the backend's CORS
 *     config sets no `Access-Control-Expose-Headers`, and `Retry-After` is not
 *     a CORS-safelisted response header, so `response.headers.get()` returns
 *     null and `ApiError.retryAfter` stays undefined. It populates only in
 *     development, where the same-origin `/api` proxy re-emits it explicitly,
 *     and on the server, which is not subject to CORS. The no-header branch
 *     below is therefore the PRODUCTION path, not the edge case.
 *
 * Taken together: treat `Retry-After` as an upper bound to clamp rather than a
 * deadline to obey, and make the header-less backoff the well-tuned one.
 */

import type { ApiError } from './api'

/**
 * Retries allowed for a 429, on top of the original request.
 *
 * Three, matching the existing 5xx budget, so there is one retry count to
 * reason about rather than two. Combined with the per-attempt ceiling below
 * this bounds the worst-case added latency at 60s, one full limiter window,
 * with no cumulative-delay bookkeeping. A caller still being refused after a
 * whole window has rolled by is being limited at a sustained rate rather than
 * briefly clipped; further retries only add load, so that case surfaces the
 * error state, which is the honest answer.
 */
export const RATE_LIMIT_MAX_RETRIES = 3

/**
 * Ceiling on the BASE delay for one 429 retry, before jitter.
 *
 * Chosen so that `RATE_LIMIT_MAX_RETRIES` attempts at the ceiling plus maximum
 * jitter still total no more than one 60s limiter window:
 * `3 * 16s * 1.25 = 60s` exactly. Raising either constant breaks that
 * arithmetic, which is the whole total-delay guarantee.
 */
export const RATE_LIMIT_MAX_BASE_DELAY_MS = 16_000

/**
 * Base for exponential backoff when the response carried no usable
 * `Retry-After`, which per fact (4) above is the production case. Yields 2s,
 * 4s, 8s, all comfortably under the ceiling, for a header-less worst case of
 * about 14s plus jitter.
 *
 * Deliberately eager: the window is sliding, so capacity is returning the
 * whole time, and a refused probe costs no budget.
 */
export const RATE_LIMIT_FALLBACK_BASE_MS = 2_000

/**
 * Extra wait added as a fraction of the base delay, to de-synchronize the
 * retry burst.
 *
 * Without it every blocked request on the page waits the same interval and
 * then retries in the same instant, recreating the spike that exhausted the
 * budget in the first place. Jitter is ADDITIVE ONLY, never negative, so it
 * can only ever push a retry later.
 *
 * It is applied AFTER the clamp, not before. Clamping the jittered value
 * instead would squeeze the jitter back out precisely when it matters most:
 * with `Retry-After` pinned at a constant 60s, every base delay saturates the
 * ceiling, every clamped result lands on the same number, and the whole page
 * retries in lockstep.
 */
export const RATE_LIMIT_JITTER_RATIO = 0.25

/** Status code that means "rate limited, try again later". */
const HTTP_TOO_MANY_REQUESTS = 429

/** React Query's own default curve, kept for every non-429 retryable error. */
const DEFAULT_RETRY_DELAY_BASE_MS = 1_000
const DEFAULT_RETRY_DELAY_CAP_MS = 30_000

/** Retries allowed for server errors and network failures. Unchanged. */
const SERVER_ERROR_MAX_RETRIES = 3

type MaybeApiError = Error & Partial<Pick<ApiError, 'status' | 'retryAfter'>>

/** True when the error is a rate-limit rejection rather than another 4xx. */
export function isRateLimitError(error: unknown): boolean {
  return (error as MaybeApiError | null)?.status === HTTP_TOO_MANY_REQUESTS
}

/**
 * `Retry-After` in milliseconds, or `undefined` when the header was absent or
 * unusable. `lib/api.ts` already parses the header into integer delta-seconds;
 * this re-checks the value only because `ApiError.retryAfter` is public
 * interface and a caller could hand us anything.
 */
function retryAfterMs(error: MaybeApiError): number | undefined {
  const seconds = error?.retryAfter
  if (typeof seconds !== 'number') return undefined
  if (!Number.isFinite(seconds) || seconds <= 0) return undefined
  return seconds * 1000
}

/**
 * How long to wait before retrying a rate-limited request.
 *
 * `random` is injected so the jitter is assertable; React Query only ever
 * calls this with `(failureCount, error)`, so the default applies in
 * production.
 */
export function rateLimitRetryDelay(
  failureCount: number,
  error: MaybeApiError,
  random: () => number = Math.random
): number {
  const requested =
    retryAfterMs(error) ?? RATE_LIMIT_FALLBACK_BASE_MS * 2 ** failureCount
  const base = Math.min(requested, RATE_LIMIT_MAX_BASE_DELAY_MS)
  return Math.round(base + base * RATE_LIMIT_JITTER_RATIO * random())
}

/**
 * The query-level retry predicate for the shared client.
 *
 * Order matters: 429 is checked BEFORE the generic 4xx short-circuit, because
 * 429 is the one 4xx where asking again is the correct move. Every other
 * status keeps its previous behaviour exactly: 4xx never retries, 5xx and
 * network errors keep their three attempts.
 *
 * `isBrowser` is a parameter with a runtime-derived default for the same
 * reason `random` is above: it makes the server branch testable. React Query
 * passes two arguments, so the default is what runs in production.
 *
 * WHY THE SERVER DOES NOT RETRY A 429. The same query client backs SSR
 * prefetch (`getQueryClient` mints a fresh client per request on the server),
 * and React Query's retry sleep is a real wait. A retried 429 there would
 * stall a server render for up to a full limiter window, turning a degraded
 * block into a dead page. Leaving it terminal on the server also loses
 * nothing: TanStack dehydrates only SUCCESSFUL queries, so a server-side 429
 * is simply absent from the hydration payload and the browser refetches on
 * mount, where this policy does apply.
 */
export function shouldRetryQuery(
  failureCount: number,
  error: MaybeApiError,
  isBrowser: boolean = typeof window !== 'undefined'
): boolean {
  if (isRateLimitError(error)) {
    return isBrowser && failureCount < RATE_LIMIT_MAX_RETRIES
  }
  // Other client errors are terminal: the request itself is the problem, so a
  // retry returns the same answer.
  if (error?.status && error.status >= 400 && error.status < 500) {
    return false
  }
  return failureCount < SERVER_ERROR_MAX_RETRIES
}

/**
 * Delay for ANY retryable query error.
 *
 * React Query takes a single `retryDelay` per client, so this dispatches:
 * rate limits follow the schedule above, and everything else keeps React
 * Query's own curve (`2 ** attempt * 1000`, capped at 30s) so 5xx and network
 * retry timing is unchanged by this module.
 */
export function queryRetryDelay(
  failureCount: number,
  error: MaybeApiError,
  random: () => number = Math.random
): number {
  if (isRateLimitError(error)) {
    return rateLimitRetryDelay(failureCount, error, random)
  }
  return Math.min(
    DEFAULT_RETRY_DELAY_BASE_MS * 2 ** failureCount,
    DEFAULT_RETRY_DELAY_CAP_MS
  )
}
