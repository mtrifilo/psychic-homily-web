import type { ApiError } from '@/lib/api'

/**
 * Copy for one password-confirming surface: the line shown when the request
 * failed for any other reason carries no message, and the sentence that opens
 * the throttle line.
 */
interface PasswordConfirmErrorCopy {
  fallback: string
  throttled: string
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
      return `${copy.throttled} Try again in ${retryAfter}s.`
    }
    return `${copy.throttled} Try again in a minute.`
  }
  return apiErr.message || copy.fallback
}
