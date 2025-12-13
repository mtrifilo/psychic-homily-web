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

export default AuthError
