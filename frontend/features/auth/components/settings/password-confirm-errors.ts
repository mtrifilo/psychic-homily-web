import type { ApiError } from '@/lib/api'

/**
 * Copy for one password-confirming surface.
 *
 * `throttledSentence` is a complete sentence, terminal punctuation included and
 * no trailing space: the formatter appends a second sentence after it. It must
 * not name the surface it is shown on, because one budget meters every surface
 * that uses this formatter and a person can arrive at one with the budget
 * already spent on another. Every caller passing the same sentence is asserted
 * in password-confirm-errors.test.ts.
 *
 * `fallback` is the line shown when a failure carries no message of its own.
 */
interface PasswordConfirmErrorCopy {
  fallback: string
  throttledSentence: string
}

/**
 * Turn a failed password-confirming request into one line a person can act on.
 *
 * The 429 branch is the one this exists for: the raw limiter body reads as a
 * server complaint rather than as "slow down, your password is fine".
 *
 * The headerless branch is the deployed one. `ApiError.retryAfter` is populated
 * only when the browser can read `Retry-After`, and no CORS config exposes that
 * header cross-origin; `lib/query-retry-policy.ts` carries the measurement. The
 * seconds branch is what a same-origin caller sees.
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
