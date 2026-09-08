import type { ApiError } from '@/lib/api'

/**
 * The sentence every password-confirming surface opens its throttle line with.
 *
 * It is one exported value rather than a per-surface string because one budget
 * meters every surface that uses this formatter: a person can be refused here
 * by traffic they sent somewhere else, so a sentence naming the surface it
 * appears on would name the wrong cause. A complete sentence, terminal
 * punctuation included and no trailing space, since the formatter appends a
 * second sentence after it.
 */
export const PASSWORD_CONFIRM_THROTTLED_SENTENCE = 'Too many password attempts.'

/**
 * Copy for one password-confirming surface: the line shown when a failure
 * carries no message of its own.
 */
interface PasswordConfirmErrorCopy {
  fallback: string
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
      return `${PASSWORD_CONFIRM_THROTTLED_SENTENCE} Try again in ${retryAfter}s.`
    }
    return `${PASSWORD_CONFIRM_THROTTLED_SENTENCE} Try again in a minute.`
  }
  return apiErr.message || copy.fallback
}
