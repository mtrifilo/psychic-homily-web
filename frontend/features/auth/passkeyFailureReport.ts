import * as Sentry from '@sentry/nextjs'
import {
  AuthErrorCode,
  isAuthError,
  isNotAllowedError,
  type AuthErrorCodeType,
} from '@/lib/errors'

/** The ceremony a passkey failure came from, as its Sentry tag reads. */
export type PasskeyFailureType = 'login_failed' | 'registration_failed'

/**
 * The typed refusals that are a correct answer about the account or the
 * session, not a fault, on either ceremony: a deleted account inside its
 * recovery window, an account that may no longer sign in, and a session too
 * old to add a credential. The surfaces show each one's sentence and offer
 * the remedy where there is one (account recovery, signing in again).
 */
const EXPECTED_REFUSAL_CODES: ReadonlySet<AuthErrorCodeType> = new Set([
  AuthErrorCode.ACCOUNT_RECOVERABLE,
  AuthErrorCode.ACCOUNT_INACTIVE,
  AuthErrorCode.REAUTH_REQUIRED,
])

/**
 * The name an expected failure is recorded under, or null when the failure is
 * not expected. The browser refusing the ceremony (NotAllowedError: the
 * viewer dismissed the prompt, it timed out, or no credential matched) is
 * expected, and its breadcrumb cannot tell those apart.
 */
function expectedFailureCode(error: unknown): string | null {
  if (isNotAllowedError(error)) return 'NotAllowedError'
  if (isAuthError(error) && EXPECTED_REFUSAL_CODES.has(error.code)) {
    return error.code
  }
  return null
}

/**
 * Reports a failed passkey ceremony to Sentry. An expected failure is an info
 * breadcrumb carrying only its code, so it adds context to a later event
 * without becoming one; every other failure is an exception at error level.
 */
export function reportPasskeyFailure(
  error: unknown,
  errorType: PasskeyFailureType
): void {
  const code = expectedFailureCode(error)
  if (code !== null) {
    Sentry.addBreadcrumb({
      category: 'passkey',
      level: 'info',
      message: 'Expected passkey refusal',
      data: { code, error_type: errorType },
    })
    return
  }
  Sentry.captureException(error, {
    level: 'error',
    tags: { service: 'passkey-auth', error_type: errorType },
  })
}
