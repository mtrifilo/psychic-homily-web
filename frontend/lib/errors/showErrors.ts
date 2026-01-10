/**
 * Show Error Types
 *
 * Typed error classes for show-related errors.
 * These match the error codes returned by the backend.
 */

/**
 * Show error codes (must match backend/internal/errors/show.go)
 */
export const ShowErrorCode = {
  SHOW_NOT_FOUND: 'SHOW_NOT_FOUND',
  SHOW_CREATE_FAILED: 'SHOW_CREATE_FAILED',
  SHOW_UPDATE_FAILED: 'SHOW_UPDATE_FAILED',
  SHOW_DELETE_FAILED: 'SHOW_DELETE_FAILED',
  SHOW_DELETE_UNAUTHORIZED: 'SHOW_DELETE_UNAUTHORIZED',
  SHOW_INVALID_ID: 'SHOW_INVALID_ID',
  SHOW_VALIDATION_FAILED: 'SHOW_VALIDATION_FAILED',
  VENUE_REQUIRED: 'VENUE_REQUIRED',
  ARTIST_REQUIRED: 'ARTIST_REQUIRED',
  INVALID_EVENT_DATE: 'INVALID_EVENT_DATE',
  SERVICE_UNAVAILABLE: 'SERVICE_UNAVAILABLE',
  UNKNOWN: 'UNKNOWN',
} as const

export type ShowErrorCodeType =
  (typeof ShowErrorCode)[keyof typeof ShowErrorCode]

/**
 * Options for creating a ShowError
 */
interface ShowErrorOptions {
  requestId?: string
  status?: number
  showId?: number | string
  cause?: Error
}

/**
 * ShowError class for show-related errors
 *
 * Provides typed error handling with error codes, request IDs,
 * and helper methods for checking error types.
 */
export class ShowError extends Error {
  /** Error code for programmatic handling */
  readonly code: ShowErrorCodeType

  /** Request ID for debugging and correlation with backend logs */
  readonly requestId?: string

  /** HTTP status code */
  readonly status?: number

  /** Show ID if applicable */
  readonly showId?: number | string

  /** Original error that caused this error */
  readonly cause?: Error

  constructor(
    message: string,
    code: ShowErrorCodeType,
    options?: ShowErrorOptions
  ) {
    super(message)
    this.name = 'ShowError'
    this.code = code
    this.requestId = options?.requestId
    this.status = options?.status
    this.showId = options?.showId
    this.cause = options?.cause

    // Maintain proper stack trace in V8 engines
    if (Error.captureStackTrace) {
      Error.captureStackTrace(this, ShowError)
    }
  }

  /**
   * Check if this is a not found error
   */
  get isNotFound(): boolean {
    return this.code === ShowErrorCode.SHOW_NOT_FOUND
  }

  /**
   * Check if this is a validation error
   */
  get isValidationError(): boolean {
    return (
      this.code === ShowErrorCode.SHOW_VALIDATION_FAILED ||
      this.code === ShowErrorCode.VENUE_REQUIRED ||
      this.code === ShowErrorCode.ARTIST_REQUIRED ||
      this.code === ShowErrorCode.INVALID_EVENT_DATE
    )
  }

  /**
   * Check if this is a service unavailable error
   */
  get isServiceUnavailable(): boolean {
    return this.code === ShowErrorCode.SERVICE_UNAVAILABLE
  }

  /**
   * Check if this error is retryable
   */
  get isRetryable(): boolean {
    return this.isServiceUnavailable
  }

  /**
   * Create a ShowError from an API response
   */
  static fromResponse(response: {
    message?: string
    error_code?: string
    request_id?: string
    status?: number
  }): ShowError {
    // Parse error code from message if not in error_code field
    let code: ShowErrorCodeType = ShowErrorCode.UNKNOWN
    const message = response.message || 'An error occurred'

    if (response.error_code) {
      code = response.error_code as ShowErrorCodeType
    } else {
      // Try to extract error code from message format: "message [CODE]"
      const codeMatch = message.match(/\[([A-Z_]+)\]/)
      if (codeMatch) {
        code = codeMatch[1] as ShowErrorCodeType
      }
    }

    // Try to extract request_id from message format: "(request_id: uuid)"
    let requestId = response.request_id
    if (!requestId) {
      const requestIdMatch = message.match(/\(request_id:\s*([a-f0-9-]+)\)/)
      if (requestIdMatch) {
        requestId = requestIdMatch[1]
      }
    }

    return new ShowError(message, code, {
      requestId,
      status: response.status,
    })
  }

  /**
   * Create a ShowError from an unknown error
   */
  static fromUnknown(
    error: unknown,
    showId?: number | string,
    requestId?: string
  ): ShowError {
    if (error instanceof ShowError) {
      return error
    }

    if (error instanceof Error) {
      // Check if it's an API error with additional properties
      const apiError = error as Error & {
        status?: number
        details?: { error_code?: string; request_id?: string }
      }

      const code =
        (apiError.details?.error_code as ShowErrorCodeType) ||
        ShowErrorCode.UNKNOWN
      const errorRequestId = apiError.details?.request_id || requestId

      return new ShowError(error.message, code, {
        requestId: errorRequestId,
        status: apiError.status,
        showId,
        cause: error,
      })
    }

    return new ShowError(String(error), ShowErrorCode.UNKNOWN, {
      requestId,
      showId,
    })
  }
}

/**
 * Type guard to check if an error is a ShowError
 */
export function isShowError(error: unknown): error is ShowError {
  return error instanceof ShowError
}

/**
 * Get a user-friendly message for an error code
 */
export function getShowErrorMessage(code: ShowErrorCodeType): string {
  switch (code) {
    case ShowErrorCode.SHOW_NOT_FOUND:
      return 'Show not found'
    case ShowErrorCode.SHOW_CREATE_FAILED:
      return 'Failed to create show. Please try again.'
    case ShowErrorCode.SHOW_UPDATE_FAILED:
      return 'Failed to update show. Please try again.'
    case ShowErrorCode.SHOW_DELETE_FAILED:
      return 'Failed to delete show. Please try again.'
    case ShowErrorCode.SHOW_DELETE_UNAUTHORIZED:
      return 'You are not authorized to delete this show.'
    case ShowErrorCode.SHOW_INVALID_ID:
      return 'Invalid show ID'
    case ShowErrorCode.SHOW_VALIDATION_FAILED:
      return 'Validation failed'
    case ShowErrorCode.VENUE_REQUIRED:
      return 'At least one venue is required'
    case ShowErrorCode.ARTIST_REQUIRED:
      return 'At least one artist is required'
    case ShowErrorCode.INVALID_EVENT_DATE:
      return 'Invalid event date'
    case ShowErrorCode.SERVICE_UNAVAILABLE:
      return 'Service temporarily unavailable. Please try again.'
    default:
      return 'An error occurred'
  }
}

export default ShowError
