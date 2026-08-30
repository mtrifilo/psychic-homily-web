/**
 * Auth Error Types
 *
 * Typed error classes for authentication-related errors.
 * These match the error codes returned by the backend.
 */

/**
 * Auth error codes (must match backend/internal/errors/auth.go)
 */
export const AuthErrorCode = {
  INVALID_CREDENTIALS: 'INVALID_CREDENTIALS',
  USER_NOT_FOUND: 'USER_NOT_FOUND',
  TOKEN_EXPIRED: 'TOKEN_EXPIRED',
  TOKEN_INVALID: 'TOKEN_INVALID',
  TOKEN_MISSING: 'TOKEN_MISSING',
  SERVICE_UNAVAILABLE: 'SERVICE_UNAVAILABLE',
  USER_EXISTS: 'USER_EXISTS',
  VALIDATION_FAILED: 'VALIDATION_FAILED',
  UNAUTHORIZED: 'UNAUTHORIZED',
  UNKNOWN: 'UNKNOWN',
} as const

export type AuthErrorCodeType =
  (typeof AuthErrorCode)[keyof typeof AuthErrorCode]

/**
 * Options for creating an AuthError
 */
interface AuthErrorOptions {
  requestId?: string
  status?: number
  cause?: Error
}

/**
 * AuthError class for authentication-related errors
 *
 * Provides typed error handling with error codes, request IDs,
 * and helper methods for checking error types.
 */
export class AuthError extends Error {
  /** Error code for programmatic handling */
  readonly code: AuthErrorCodeType

  /** Request ID for debugging and correlation with backend logs */
  readonly requestId?: string

  /** HTTP status code */
  readonly status?: number

  /** Original error that caused this error */
  readonly cause?: Error

  constructor(
    message: string,
    code: AuthErrorCodeType,
    options?: AuthErrorOptions
  ) {
    super(message)
    this.name = 'AuthError'
    this.code = code
    this.requestId = options?.requestId
    this.status = options?.status
    this.cause = options?.cause

    // Maintain proper stack trace in V8 engines
    if (Error.captureStackTrace) {
      Error.captureStackTrace(this, AuthError)
    }
  }

  /**
   * Check if this is a token expiration error
   */
  get isExpired(): boolean {
    return this.code === AuthErrorCode.TOKEN_EXPIRED
  }

  /**
   * Check if this is an invalid credentials error
   */
  get isInvalidCredentials(): boolean {
    return this.code === AuthErrorCode.INVALID_CREDENTIALS
  }

  /**
   * Check if this is a token missing error
   */
  get isTokenMissing(): boolean {
    return this.code === AuthErrorCode.TOKEN_MISSING
  }

  /**
   * Check if this is a service unavailable error
   */
  get isServiceUnavailable(): boolean {
    return this.code === AuthErrorCode.SERVICE_UNAVAILABLE
  }

  /**
   * Check if this is a user exists error
   */
  get isUserExists(): boolean {
    return this.code === AuthErrorCode.USER_EXISTS
  }

  /**
   * Check if the user should be redirected to login
   */
  get shouldRedirectToLogin(): boolean {
    return (
      this.isExpired ||
      this.isTokenMissing ||
      this.code === AuthErrorCode.TOKEN_INVALID
    )
  }

  /**
   * Create an AuthError from an API response
   */
  static fromResponse(response: {
    message?: string
    error_code?: string
    request_id?: string
    status?: number
  }): AuthError {
    const code = (response.error_code as AuthErrorCodeType) || 'UNKNOWN'
    const message = response.message || 'An authentication error occurred'

    return new AuthError(message, code, {
      requestId: response.request_id,
      status: response.status,
    })
  }

  /**
   * Create an AuthError from an unknown error
   */
  static fromUnknown(error: unknown, requestId?: string): AuthError {
    if (error instanceof AuthError) {
      return error
    }

    if (error instanceof Error) {
      // Check if it's an API error with additional properties
      const apiError = error as Error & {
        status?: number
        details?: { error_code?: string; request_id?: string }
      }

      const code =
        (apiError.details?.error_code as AuthErrorCodeType) ||
        AuthErrorCode.UNKNOWN
      const errorRequestId = apiError.details?.request_id || requestId

      return new AuthError(error.message, code, {
        requestId: errorRequestId,
        status: apiError.status,
        cause: error,
      })
    }

    return new AuthError(String(error), AuthErrorCode.UNKNOWN, {
      requestId,
    })
  }
}

/**
 * Type guard to check if an error is an AuthError
 */
export function isAuthError(error: unknown): error is AuthError {
  return error instanceof AuthError
}

/**
 * Get a user-friendly message for an error code
 */
export function getAuthErrorMessage(code: AuthErrorCodeType): string {
  switch (code) {
    case AuthErrorCode.INVALID_CREDENTIALS:
      return 'Invalid email or password'
    case AuthErrorCode.TOKEN_EXPIRED:
      return 'Your session has expired. Please log in again.'
    case AuthErrorCode.TOKEN_INVALID:
      return 'Invalid authentication token'
    case AuthErrorCode.TOKEN_MISSING:
      return 'Authentication required'
    case AuthErrorCode.SERVICE_UNAVAILABLE:
      return 'Service temporarily unavailable. Please try again.'
    case AuthErrorCode.USER_EXISTS:
      return 'An account with this email already exists'
    case AuthErrorCode.VALIDATION_FAILED:
      return 'Validation failed'
    case AuthErrorCode.UNAUTHORIZED:
      return 'You are not authorized to perform this action'
    default:
      return 'An error occurred'
  }
}

/**
 * Is this failure the backend ANSWERING "there is no valid session", as opposed
 * to failing to answer at all?
 *
 * One definition, deliberately, because three had already grown and they
 * disagreed. The SSR profile prefetch classified on raw HTTP status, the auth
 * context classified on `AuthError` code, and `useProfile`'s retry policy
 * classified on code plus 403, so the same bodyless 401 could be read as
 * "definitely logged out" in one place and "keep retrying" in another. That
 * matters more than it sounds: a failure read as an ANSWER settles a viewer's
 * auth state, and settling it wrongly is what lets a gate act on a viewer it
 * has misidentified.
 *
 * Status is the primary signal and the code is the fallback, not the reverse.
 * `apiRequest` builds `code = errorBody.error_code || UNAUTHORIZED`, and
 * `shouldRedirectToLogin` recognizes only the three token codes, so a 401 from
 * an edge proxy, an HTML error page, or a future handler would otherwise read
 * as indefinite and strand a genuinely anonymous viewer as "unknown" forever.
 *
 * Takes the raw parts rather than an `AuthError` so the server-side prefetch,
 * which has a `Response` and a parsed body but no `AuthError`, can call the
 * same function instead of keeping its own copy.
 */
export function isDefinitiveUnauthenticated(
  status: number | undefined,
  code: string | undefined
): boolean {
  // 401 only. 403 is deliberately NOT here: it means authenticated but
  // forbidden, which is a different answer, and this backend never emits it on
  // /auth/profile (that route's JWT middleware writes 401 exclusively). A 403
  // reaching this function therefore comes from infrastructure (an edge rule,
  // a WAF, a platform protection page), and reading it as "no session" would
  // settle a signed-in viewer to anonymous on a proxy misconfiguration. Where
  // a 403 should stop a RETRY, the caller tests that separately: whether to
  // retry and who the viewer is are different questions.
  if (status === 401) return true

  // Any other status is not an answer about identity, whatever the body says.
  // `apiRequest` copies an unrecognized error body wholesale into `details`, so
  // a 403 / 404 / 429 forwarded by a proxy or WAF can arrive carrying a token
  // code; promoting those would settle a signed-in viewer as anonymous on an
  // infrastructure fault.
  if (status !== undefined) return false

  // No status to go on (a raw error object, or a thrown value from a layer that
  // does not attach one): the token codes are the only remaining evidence, and
  // this backend emits them only alongside a 401.
  return (
    code === AuthErrorCode.TOKEN_EXPIRED ||
    code === AuthErrorCode.TOKEN_MISSING ||
    code === AuthErrorCode.TOKEN_INVALID
  )
}

export default AuthError
