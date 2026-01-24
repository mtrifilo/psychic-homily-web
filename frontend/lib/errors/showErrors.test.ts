import { describe, it, expect } from 'vitest'
import {
  ShowError,
  ShowErrorCode,
  isShowError,
  getShowErrorMessage,
} from './showErrors'

describe('ShowError', () => {
  describe('constructor', () => {
    it('creates an error with message and code', () => {
      const error = new ShowError('Test error', ShowErrorCode.SHOW_NOT_FOUND)

      expect(error.message).toBe('Test error')
      expect(error.code).toBe(ShowErrorCode.SHOW_NOT_FOUND)
      expect(error.name).toBe('ShowError')
    })

    it('includes optional requestId', () => {
      const error = new ShowError('Test error', ShowErrorCode.SHOW_NOT_FOUND, {
        requestId: 'req-123',
      })

      expect(error.requestId).toBe('req-123')
    })

    it('includes optional status', () => {
      const error = new ShowError('Test error', ShowErrorCode.SHOW_NOT_FOUND, {
        status: 404,
      })

      expect(error.status).toBe(404)
    })

    it('includes optional showId', () => {
      const error = new ShowError('Test error', ShowErrorCode.SHOW_NOT_FOUND, {
        showId: 42,
      })

      expect(error.showId).toBe(42)
    })

    it('accepts string showId', () => {
      const error = new ShowError('Test error', ShowErrorCode.SHOW_NOT_FOUND, {
        showId: 'show-123',
      })

      expect(error.showId).toBe('show-123')
    })

    it('includes optional cause', () => {
      const cause = new Error('Original error')
      const error = new ShowError('Wrapped error', ShowErrorCode.UNKNOWN, {
        cause,
      })

      expect(error.cause).toBe(cause)
    })

    it('is an instance of Error', () => {
      const error = new ShowError('Test', ShowErrorCode.UNKNOWN)

      expect(error).toBeInstanceOf(Error)
      expect(error).toBeInstanceOf(ShowError)
    })
  })

  describe('getter methods', () => {
    it('isNotFound returns true for SHOW_NOT_FOUND', () => {
      const error = new ShowError('Not found', ShowErrorCode.SHOW_NOT_FOUND)
      expect(error.isNotFound).toBe(true)
    })

    it('isNotFound returns false for other codes', () => {
      const error = new ShowError('Failed', ShowErrorCode.SHOW_CREATE_FAILED)
      expect(error.isNotFound).toBe(false)
    })

    it('isServiceUnavailable returns true for SERVICE_UNAVAILABLE', () => {
      const error = new ShowError('Down', ShowErrorCode.SERVICE_UNAVAILABLE)
      expect(error.isServiceUnavailable).toBe(true)
    })

    it('isRetryable returns true for SERVICE_UNAVAILABLE', () => {
      const error = new ShowError('Down', ShowErrorCode.SERVICE_UNAVAILABLE)
      expect(error.isRetryable).toBe(true)
    })

    it('isRetryable returns false for validation errors', () => {
      const error = new ShowError('Invalid', ShowErrorCode.SHOW_VALIDATION_FAILED)
      expect(error.isRetryable).toBe(false)
    })
  })

  describe('isValidationError', () => {
    it('returns true for SHOW_VALIDATION_FAILED', () => {
      const error = new ShowError('Invalid', ShowErrorCode.SHOW_VALIDATION_FAILED)
      expect(error.isValidationError).toBe(true)
    })

    it('returns true for VENUE_REQUIRED', () => {
      const error = new ShowError('Missing venue', ShowErrorCode.VENUE_REQUIRED)
      expect(error.isValidationError).toBe(true)
    })

    it('returns true for ARTIST_REQUIRED', () => {
      const error = new ShowError('Missing artist', ShowErrorCode.ARTIST_REQUIRED)
      expect(error.isValidationError).toBe(true)
    })

    it('returns true for INVALID_EVENT_DATE', () => {
      const error = new ShowError('Bad date', ShowErrorCode.INVALID_EVENT_DATE)
      expect(error.isValidationError).toBe(true)
    })

    it('returns false for SHOW_NOT_FOUND', () => {
      const error = new ShowError('Not found', ShowErrorCode.SHOW_NOT_FOUND)
      expect(error.isValidationError).toBe(false)
    })

    it('returns false for SHOW_CREATE_FAILED', () => {
      const error = new ShowError('Failed', ShowErrorCode.SHOW_CREATE_FAILED)
      expect(error.isValidationError).toBe(false)
    })
  })

  describe('fromResponse', () => {
    it('creates ShowError from response with all fields', () => {
      const response = {
        message: 'Show not found',
        error_code: 'SHOW_NOT_FOUND',
        request_id: 'req-456',
        status: 404,
      }

      const error = ShowError.fromResponse(response)

      expect(error.message).toBe('Show not found')
      expect(error.code).toBe(ShowErrorCode.SHOW_NOT_FOUND)
      expect(error.requestId).toBe('req-456')
      expect(error.status).toBe(404)
    })

    it('extracts error code from message format [CODE]', () => {
      const response = {
        message: 'Failed to create show [SHOW_CREATE_FAILED]',
      }

      const error = ShowError.fromResponse(response)

      expect(error.code).toBe(ShowErrorCode.SHOW_CREATE_FAILED)
    })

    it('extracts request_id from message format (request_id: uuid)', () => {
      const response = {
        message: 'Error occurred (request_id: abc-123-def)',
      }

      const error = ShowError.fromResponse(response)

      expect(error.requestId).toBe('abc-123-def')
    })

    it('prefers error_code over message extraction', () => {
      const response = {
        message: 'Some error [SHOW_NOT_FOUND]',
        error_code: 'SHOW_CREATE_FAILED',
      }

      const error = ShowError.fromResponse(response)

      expect(error.code).toBe(ShowErrorCode.SHOW_CREATE_FAILED)
    })

    it('prefers request_id field over message extraction', () => {
      const response = {
        message: 'Error (request_id: extracted-id)',
        request_id: 'field-id',
      }

      const error = ShowError.fromResponse(response)

      expect(error.requestId).toBe('field-id')
    })

    it('defaults code to UNKNOWN when not found', () => {
      const response = { message: 'Something went wrong' }

      const error = ShowError.fromResponse(response)

      expect(error.code).toBe(ShowErrorCode.UNKNOWN)
    })

    it('defaults message when not provided', () => {
      const response = { error_code: 'SHOW_NOT_FOUND' }

      const error = ShowError.fromResponse(response)

      expect(error.message).toBe('An error occurred')
    })
  })

  describe('fromUnknown', () => {
    it('returns same error if already ShowError', () => {
      const original = new ShowError('Test', ShowErrorCode.SHOW_NOT_FOUND)
      const result = ShowError.fromUnknown(original)

      expect(result).toBe(original)
    })

    it('wraps regular Error', () => {
      const original = new Error('Something failed')
      const result = ShowError.fromUnknown(original)

      expect(result).toBeInstanceOf(ShowError)
      expect(result.message).toBe('Something failed')
      expect(result.code).toBe(ShowErrorCode.UNKNOWN)
      expect(result.cause).toBe(original)
    })

    it('extracts error code from API error details', () => {
      const apiError = Object.assign(new Error('Show failed'), {
        status: 400,
        details: {
          error_code: 'VENUE_REQUIRED',
          request_id: 'req-789',
        },
      })

      const result = ShowError.fromUnknown(apiError)

      expect(result.code).toBe(ShowErrorCode.VENUE_REQUIRED)
      expect(result.requestId).toBe('req-789')
      expect(result.status).toBe(400)
    })

    it('includes showId when provided', () => {
      const error = new Error('Failed')
      const result = ShowError.fromUnknown(error, 123)

      expect(result.showId).toBe(123)
    })

    it('uses provided requestId as fallback', () => {
      const error = new Error('Failed')
      const result = ShowError.fromUnknown(error, undefined, 'fallback-req-id')

      expect(result.requestId).toBe('fallback-req-id')
    })

    it('converts non-Error values to string', () => {
      const result = ShowError.fromUnknown('string error')

      expect(result.message).toBe('string error')
      expect(result.code).toBe(ShowErrorCode.UNKNOWN)
    })

    it('handles null/undefined', () => {
      expect(ShowError.fromUnknown(null).message).toBe('null')
      expect(ShowError.fromUnknown(undefined).message).toBe('undefined')
    })
  })
})

describe('isShowError', () => {
  it('returns true for ShowError instances', () => {
    const error = new ShowError('Test', ShowErrorCode.UNKNOWN)
    expect(isShowError(error)).toBe(true)
  })

  it('returns false for regular Error', () => {
    const error = new Error('Test')
    expect(isShowError(error)).toBe(false)
  })

  it('returns false for null', () => {
    expect(isShowError(null)).toBe(false)
  })

  it('returns false for undefined', () => {
    expect(isShowError(undefined)).toBe(false)
  })

  it('returns false for plain objects', () => {
    expect(isShowError({ code: 'UNKNOWN', message: 'test' })).toBe(false)
  })
})

describe('getShowErrorMessage', () => {
  it('returns correct message for SHOW_NOT_FOUND', () => {
    expect(getShowErrorMessage(ShowErrorCode.SHOW_NOT_FOUND)).toBe(
      'Show not found'
    )
  })

  it('returns correct message for SHOW_CREATE_FAILED', () => {
    expect(getShowErrorMessage(ShowErrorCode.SHOW_CREATE_FAILED)).toBe(
      'Failed to create show. Please try again.'
    )
  })

  it('returns correct message for SHOW_UPDATE_FAILED', () => {
    expect(getShowErrorMessage(ShowErrorCode.SHOW_UPDATE_FAILED)).toBe(
      'Failed to update show. Please try again.'
    )
  })

  it('returns correct message for SHOW_DELETE_FAILED', () => {
    expect(getShowErrorMessage(ShowErrorCode.SHOW_DELETE_FAILED)).toBe(
      'Failed to delete show. Please try again.'
    )
  })

  it('returns correct message for SHOW_DELETE_UNAUTHORIZED', () => {
    expect(getShowErrorMessage(ShowErrorCode.SHOW_DELETE_UNAUTHORIZED)).toBe(
      'You are not authorized to delete this show.'
    )
  })

  it('returns correct message for SHOW_INVALID_ID', () => {
    expect(getShowErrorMessage(ShowErrorCode.SHOW_INVALID_ID)).toBe(
      'Invalid show ID'
    )
  })

  it('returns correct message for SHOW_VALIDATION_FAILED', () => {
    expect(getShowErrorMessage(ShowErrorCode.SHOW_VALIDATION_FAILED)).toBe(
      'Validation failed'
    )
  })

  it('returns correct message for VENUE_REQUIRED', () => {
    expect(getShowErrorMessage(ShowErrorCode.VENUE_REQUIRED)).toBe(
      'At least one venue is required'
    )
  })

  it('returns correct message for ARTIST_REQUIRED', () => {
    expect(getShowErrorMessage(ShowErrorCode.ARTIST_REQUIRED)).toBe(
      'At least one artist is required'
    )
  })

  it('returns correct message for INVALID_EVENT_DATE', () => {
    expect(getShowErrorMessage(ShowErrorCode.INVALID_EVENT_DATE)).toBe(
      'Invalid event date'
    )
  })

  it('returns correct message for SERVICE_UNAVAILABLE', () => {
    expect(getShowErrorMessage(ShowErrorCode.SERVICE_UNAVAILABLE)).toBe(
      'Service temporarily unavailable. Please try again.'
    )
  })

  it('returns default message for UNKNOWN', () => {
    expect(getShowErrorMessage(ShowErrorCode.UNKNOWN)).toBe('An error occurred')
  })
})
