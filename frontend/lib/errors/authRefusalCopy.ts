/**
 * The fixed sentences the auth surfaces show for a refusal code. A refusal
 * reaches the browser as a code, and the page that renders it owns the words,
 * so no sentence on these surfaces comes from a URL.
 */

/** /auth, for refusal text or a code with no line of its own. */
export const AUTH_REFUSAL_GENERIC_MESSAGE =
  'Something went wrong signing you in. Please try again.'

/**
 * /auth, AGE_CONFIRMATION_REQUIRED. Also the server's message for the code,
 * which a backend test checks against this file.
 */
export const AGE_CONFIRMATION_REQUIRED_MESSAGE =
  'Your account was not created. Psychic Homily accounts are for people 16 and over.'

/** /auth, OAUTH_CANCELLED, which only Google's decline carries. */
export const OAUTH_CANCELLED_MESSAGE =
  'Google sign-in was cancelled. Try again, or choose another way below.'

/** /auth, RATE_LIMITED: the OAuth limiter counts per IP over one minute. */
export const AUTH_RATE_LIMITED_MESSAGE =
  'Too many sign-in attempts. Wait a minute, then try again.'

/**
 * The server's own sentences for the refusals below, as fixed copy here. Each
 * must stay identical to its backend string (backend/internal/errors/auth.go),
 * which a backend test checks against this file.
 */
export const OAUTH_LINK_REFUSED_MESSAGE =
  'An account already uses this email address. Sign in to that account first, then connect this provider in Settings under Connected accounts.'
export const ACCOUNT_UNAVAILABLE_MESSAGE =
  'Account unavailable. Please contact support.'
export const VALIDATION_FAILED_MESSAGE = 'Validation failed'
export const USER_EXISTS_MESSAGE = 'An account with this email already exists'
export const TERMS_ACCEPTANCE_REQUIRED_MESSAGE =
  'Please accept the Terms of Service and Privacy Policy before creating an account.'
