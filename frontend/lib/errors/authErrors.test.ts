import { describe, it, expect } from 'vitest'
import {
  AuthError,
  AuthErrorCode,
  isAuthError,
  isDefinitiveUnauthenticated,
  getAuthErrorMessage,
} from './authErrors'

describe('AuthError', () => {
  describe('constructor', () => {
    it('creates an error with message and code', () => {
      const error = new AuthError('Test error', AuthErrorCode.INVALID_CREDENTIALS)

      expect(error.message).toBe('Test error')
      expect(error.code).toBe(AuthErrorCode.INVALID_CREDENTIALS)
      expect(error.name).toBe('AuthError')
    })

    it('includes optional requestId', () => {
      const error = new AuthError('Test error', AuthErrorCode.TOKEN_EXPIRED, {
        requestId: 'req-123',
      })

      expect(error.requestId).toBe('req-123')
    })

    it('includes optional status', () => {
      const error = new AuthError('Test error', AuthErrorCode.UNAUTHORIZED, {
        status: 403,
      })

      expect(error.status).toBe(403)
    })

    it('includes optional cause', () => {
      const cause = new Error('Original error')
      const error = new AuthError('Wrapped error', AuthErrorCode.UNKNOWN, {
        cause,
      })

      expect(error.cause).toBe(cause)
    })

    it('is an instance of Error', () => {
      const error = new AuthError('Test', AuthErrorCode.UNKNOWN)

      expect(error).toBeInstanceOf(Error)
      expect(error).toBeInstanceOf(AuthError)
    })
  })

  describe('getter methods', () => {
    it('isExpired returns true for TOKEN_EXPIRED', () => {
      const error = new AuthError('Expired', AuthErrorCode.TOKEN_EXPIRED)
      expect(error.isExpired).toBe(true)
    })

    it('isExpired returns false for other codes', () => {
      const error = new AuthError('Invalid', AuthErrorCode.INVALID_CREDENTIALS)
      expect(error.isExpired).toBe(false)
    })

    it('isInvalidCredentials returns true for INVALID_CREDENTIALS', () => {
      const error = new AuthError('Invalid', AuthErrorCode.INVALID_CREDENTIALS)
      expect(error.isInvalidCredentials).toBe(true)
    })

    it('isInvalidCredentials returns false for other codes', () => {
      const error = new AuthError('Expired', AuthErrorCode.TOKEN_EXPIRED)
      expect(error.isInvalidCredentials).toBe(false)
    })

    it('isTokenMissing returns true for TOKEN_MISSING', () => {
      const error = new AuthError('Missing', AuthErrorCode.TOKEN_MISSING)
      expect(error.isTokenMissing).toBe(true)
    })

    it('isServiceUnavailable returns true for SERVICE_UNAVAILABLE', () => {
      const error = new AuthError('Down', AuthErrorCode.SERVICE_UNAVAILABLE)
      expect(error.isServiceUnavailable).toBe(true)
    })

    it('isUserExists returns true for USER_EXISTS', () => {
      const error = new AuthError('Exists', AuthErrorCode.USER_EXISTS)
      expect(error.isUserExists).toBe(true)
    })
  })

  describe('shouldRedirectToLogin', () => {
    it('returns true for TOKEN_EXPIRED', () => {
      const error = new AuthError('Expired', AuthErrorCode.TOKEN_EXPIRED)
      expect(error.shouldRedirectToLogin).toBe(true)
    })

    it('returns true for TOKEN_MISSING', () => {
      const error = new AuthError('Missing', AuthErrorCode.TOKEN_MISSING)
      expect(error.shouldRedirectToLogin).toBe(true)
    })

    it('returns true for TOKEN_INVALID', () => {
      const error = new AuthError('Invalid', AuthErrorCode.TOKEN_INVALID)
      expect(error.shouldRedirectToLogin).toBe(true)
    })

    it('returns false for INVALID_CREDENTIALS', () => {
      const error = new AuthError('Invalid', AuthErrorCode.INVALID_CREDENTIALS)
      expect(error.shouldRedirectToLogin).toBe(false)
    })

    it('returns false for USER_EXISTS', () => {
      const error = new AuthError('Exists', AuthErrorCode.USER_EXISTS)
      expect(error.shouldRedirectToLogin).toBe(false)
    })

    it('returns false for UNAUTHORIZED', () => {
      const error = new AuthError('Unauthorized', AuthErrorCode.UNAUTHORIZED)
      expect(error.shouldRedirectToLogin).toBe(false)
    })
  })

  describe('fromResponse', () => {
    it('creates AuthError from response with all fields', () => {
      const response = {
        message: 'Invalid credentials',
        error_code: 'INVALID_CREDENTIALS',
        request_id: 'req-456',
        status: 401,
      }

      const error = AuthError.fromResponse(response)

      expect(error.message).toBe('Invalid credentials')
      expect(error.code).toBe(AuthErrorCode.INVALID_CREDENTIALS)
      expect(error.requestId).toBe('req-456')
      expect(error.status).toBe(401)
    })

    it('defaults code to UNKNOWN when not provided', () => {
      const response = { message: 'Something went wrong' }

      const error = AuthError.fromResponse(response)

      expect(error.code).toBe('UNKNOWN')
    })

    it('defaults message when not provided', () => {
      const response = { error_code: 'TOKEN_EXPIRED' }

      const error = AuthError.fromResponse(response)

      expect(error.message).toBe('An authentication error occurred')
    })
  })

  describe('fromUnknown', () => {
    it('returns same error if already AuthError', () => {
      const original = new AuthError('Test', AuthErrorCode.TOKEN_EXPIRED)
      const result = AuthError.fromUnknown(original)

      expect(result).toBe(original)
    })

    it('wraps regular Error', () => {
      const original = new Error('Something failed')
      const result = AuthError.fromUnknown(original)

      expect(result).toBeInstanceOf(AuthError)
      expect(result.message).toBe('Something failed')
      expect(result.code).toBe(AuthErrorCode.UNKNOWN)
      expect(result.cause).toBe(original)
    })

    it('extracts error code from API error details', () => {
      const apiError = Object.assign(new Error('Auth failed'), {
        status: 401,
        details: {
          error_code: 'TOKEN_EXPIRED',
          request_id: 'req-789',
        },
      })

      const result = AuthError.fromUnknown(apiError)

      expect(result.code).toBe(AuthErrorCode.TOKEN_EXPIRED)
      expect(result.requestId).toBe('req-789')
      expect(result.status).toBe(401)
    })

    it('uses provided requestId as fallback', () => {
      const error = new Error('Failed')
      const result = AuthError.fromUnknown(error, 'fallback-req-id')

      expect(result.requestId).toBe('fallback-req-id')
    })

    it('converts non-Error values to string', () => {
      const result = AuthError.fromUnknown('string error')

      expect(result.message).toBe('string error')
      expect(result.code).toBe(AuthErrorCode.UNKNOWN)
    })

    it('handles null/undefined', () => {
      expect(AuthError.fromUnknown(null).message).toBe('null')
      expect(AuthError.fromUnknown(undefined).message).toBe('undefined')
    })
  })
})

describe('isAuthError', () => {
  it('returns true for AuthError instances', () => {
    const error = new AuthError('Test', AuthErrorCode.UNKNOWN)
    expect(isAuthError(error)).toBe(true)
  })

  it('returns false for regular Error', () => {
    const error = new Error('Test')
    expect(isAuthError(error)).toBe(false)
  })

  it('returns false for null', () => {
    expect(isAuthError(null)).toBe(false)
  })

  it('returns false for undefined', () => {
    expect(isAuthError(undefined)).toBe(false)
  })

  it('returns false for plain objects', () => {
    expect(isAuthError({ code: 'UNKNOWN', message: 'test' })).toBe(false)
  })
})

describe('getAuthErrorMessage', () => {
  it('returns correct message for INVALID_CREDENTIALS', () => {
    expect(getAuthErrorMessage(AuthErrorCode.INVALID_CREDENTIALS)).toBe(
      'Invalid email or password'
    )
  })

  it('returns correct message for TOKEN_EXPIRED', () => {
    expect(getAuthErrorMessage(AuthErrorCode.TOKEN_EXPIRED)).toBe(
      'Your session has expired. Please log in again.'
    )
  })

  it('returns correct message for TOKEN_INVALID', () => {
    expect(getAuthErrorMessage(AuthErrorCode.TOKEN_INVALID)).toBe(
      'Invalid authentication token'
    )
  })

  it('returns correct message for TOKEN_MISSING', () => {
    expect(getAuthErrorMessage(AuthErrorCode.TOKEN_MISSING)).toBe(
      'Authentication required'
    )
  })

  it('returns correct message for SERVICE_UNAVAILABLE', () => {
    expect(getAuthErrorMessage(AuthErrorCode.SERVICE_UNAVAILABLE)).toBe(
      'Service temporarily unavailable. Please try again.'
    )
  })

  it('returns correct message for USER_EXISTS', () => {
    expect(getAuthErrorMessage(AuthErrorCode.USER_EXISTS)).toBe(
      'An account with this email already exists'
    )
  })

  it('returns correct message for VALIDATION_FAILED', () => {
    expect(getAuthErrorMessage(AuthErrorCode.VALIDATION_FAILED)).toBe(
      'Validation failed'
    )
  })

  it('returns correct message for UNAUTHORIZED', () => {
    expect(getAuthErrorMessage(AuthErrorCode.UNAUTHORIZED)).toBe(
      'You are not authorized to perform this action'
    )
  })

  it('returns default message for UNKNOWN', () => {
    expect(getAuthErrorMessage(AuthErrorCode.UNKNOWN)).toBe('An error occurred')
  })
})


// The single decision point four modules now share: the SSR profile prefetch,
// AuthContext's settled-auth signal, useProfile's retry policy, and the query
// cache's session-expiry handler. Before this existed each had its own copy and
// they disagreed about the same response, so the full truth table is pinned
// here rather than sampled indirectly through consumers.
describe('isDefinitiveUnauthenticated', () => {
  it('treats a 401 as definitive even with no recognizable error code', () => {
    // `apiRequest` falls back to a generic UNAUTHORIZED code when the body
    // carries none. A code-only test would call this indefinite and strand a
    // genuinely anonymous viewer as "unknown" forever.
    expect(isDefinitiveUnauthenticated(401, undefined)).toBe(true)
    expect(isDefinitiveUnauthenticated(401, AuthErrorCode.UNAUTHORIZED)).toBe(
      true
    )
  })

  it('treats a 401 with any token code as definitive', () => {
    expect(isDefinitiveUnauthenticated(401, AuthErrorCode.TOKEN_EXPIRED)).toBe(
      true
    )
    expect(isDefinitiveUnauthenticated(401, AuthErrorCode.TOKEN_MISSING)).toBe(
      true
    )
    expect(isDefinitiveUnauthenticated(401, AuthErrorCode.TOKEN_INVALID)).toBe(
      true
    )
  })

  // 403 means authenticated-but-forbidden, which is a different answer, and
  // /auth/profile never emits it (its JWT middleware writes 401 exclusively).
  // A 403 here is infrastructure, so reading it as "no session" would settle a
  // signed-in viewer to anonymous on a proxy misconfiguration.
  it('does NOT treat a 403 as an answer about identity', () => {
    expect(isDefinitiveUnauthenticated(403, undefined)).toBe(false)
    expect(isDefinitiveUnauthenticated(403, AuthErrorCode.UNAUTHORIZED)).toBe(
      false
    )
  })

  it('does not treat server failures as an answer', () => {
    expect(isDefinitiveUnauthenticated(500, undefined)).toBe(false)
    expect(isDefinitiveUnauthenticated(503, undefined)).toBe(false)
    expect(isDefinitiveUnauthenticated(429, undefined)).toBe(false)
  })

  // Status wins where the two disagree. `apiRequest` copies an unrecognized
  // error body wholesale into `details`, so a 5xx whose payload happens to
  // mention a token code must not be promoted into "the backend said no
  // session" by the code fallback.
  it('lets a 5xx status override a token code in the body', () => {
    expect(isDefinitiveUnauthenticated(503, AuthErrorCode.TOKEN_EXPIRED)).toBe(
      false
    )
  })

  // With no status to go on (a raw thrown value from a layer that attaches
  // none) the token codes are the only remaining evidence, and they are only
  // ever emitted alongside a real 401.
  it('falls back to token codes when there is no status', () => {
    expect(
      isDefinitiveUnauthenticated(undefined, AuthErrorCode.TOKEN_EXPIRED)
    ).toBe(true)
    expect(
      isDefinitiveUnauthenticated(undefined, AuthErrorCode.TOKEN_MISSING)
    ).toBe(true)
    expect(
      isDefinitiveUnauthenticated(undefined, AuthErrorCode.TOKEN_INVALID)
    ).toBe(true)
  })

  it('is not definitive when it knows nothing', () => {
    expect(isDefinitiveUnauthenticated(undefined, undefined)).toBe(false)
    expect(isDefinitiveUnauthenticated(undefined, AuthErrorCode.UNKNOWN)).toBe(
      false
    )
  })
})
