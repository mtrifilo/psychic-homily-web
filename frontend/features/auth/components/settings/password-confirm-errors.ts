import type { ApiError } from '@/lib/api'

/**
 * Copy for one password-confirming surface.
 *
 * `throttledSentence` is a complete sentence, terminal punctuation included and
 * no trailing space: the formatter appends a second sentence after it.
 * `fallback` is the line shown when a failure carries no message of its own.
 */
interface PasswordConfirmErrorCopy {
  fallback: string
  throttledSentence: string
}

/**
 * Turn a failed password-confirming request into one line a person can act on.
 *
 * The 429 branch is the one this exists for: `/auth/change-password` and
 * `/auth/account/delete` share one per-IP budget, and the raw limiter body
 * reads as a server complaint rather than as "slow down, your password is
 * fine".
 *
 * The headerless branch is the deployed one. `ApiError.retryAfter` is populated
 * only when the browser can read `Retry-After`, and the deployed frontend calls
 * the backend cross-origin, where no CORS config exposes that header; the
 * same-origin `/api` proxy re-emits it, so the seconds branch is what
 * development and the tests see. `lib/query-retry-policy.ts` carries the
 * measurement.
 *
 * Either way the number is the whole window the limiter names, not the time
 * left in the current one, and it does not tick down.
 */
export function formatPasswordConfirmError(
  error: unknown,
  copy: PasswordConfirmErrorCopy
): string {
  if (!error) return copy.fallback
  const apiErr = error as ApiError

  if (apiErr.status === 429) {
    const retryAfter = apiErr.retryAfter
    if (typeof retryAfter === 'number' && retryAfter > 0) {
      return `${copy.throttledSentence} Try again in ${retryAfter}s.`
    }
    return `${copy.throttledSentence} Try again in a minute.`
  }
  return apiErr.message || copy.fallback
}
