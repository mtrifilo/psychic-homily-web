import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// Store original NODE_ENV
const originalEnv = process.env.NODE_ENV

// Mock console methods
const mockConsoleDebug = vi.fn()
const mockConsoleInfo = vi.fn()
const mockConsoleWarn = vi.fn()
const mockConsoleError = vi.fn()

describe('showLogger', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Mock console methods
    vi.spyOn(console, 'debug').mockImplementation(mockConsoleDebug)
    vi.spyOn(console, 'info').mockImplementation(mockConsoleInfo)
    vi.spyOn(console, 'warn').mockImplementation(mockConsoleWarn)
    vi.spyOn(console, 'error').mockImplementation(mockConsoleError)
    // Reset module cache to allow re-importing with different env
    vi.resetModules()
  })

  afterEach(() => {
    // Restore original NODE_ENV
    process.env.NODE_ENV = originalEnv
    vi.restoreAllMocks()
  })

  describe('in development mode', () => {
    beforeEach(() => {
      process.env.NODE_ENV = 'development'
    })

    it('logs debug messages', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.debug('Test debug message', { key: 'value' })

      expect(mockConsoleDebug).toHaveBeenCalled()
      const [message] = mockConsoleDebug.mock.calls[0]
      expect(message).toContain('[Show:DEBUG]')
      expect(message).toContain('Test debug message')
    })

    it('logs debug messages with request ID', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.debug('Test message', undefined, 'req-12345678-abcd')

      expect(mockConsoleDebug).toHaveBeenCalled()
      const [message] = mockConsoleDebug.mock.calls[0]
      expect(message).toContain('[req-1234]')
    })

    it('logs info messages', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.info('Test info message', { showId: 123 })

      expect(mockConsoleInfo).toHaveBeenCalled()
      const [message] = mockConsoleInfo.mock.calls[0]
      expect(message).toContain('[Show:INFO]')
      expect(message).toContain('Test info message')
    })

    it('logs warning messages', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.warn('Test warning', { errorCode: 'ERR_001' })

      expect(mockConsoleWarn).toHaveBeenCalled()
      const [message] = mockConsoleWarn.mock.calls[0]
      expect(message).toContain('[Show:WARN]')
    })

    it('logs error messages with error details', async () => {
      const { showLogger } = await import('./showLogger')
      const error = new Error('Test error')
      showLogger.error('Something failed', error, { context: 'submit' })

      expect(mockConsoleError).toHaveBeenCalled()
      const [message] = mockConsoleError.mock.calls[0]
      expect(message).toContain('[Show:ERROR]')
      expect(message).toContain('Something failed')
    })

    it('submitAttempt logs show data', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.submitAttempt({
        venueCount: 1,
        artistCount: 3,
        city: 'Phoenix',
        state: 'AZ',
      })

      expect(mockConsoleDebug).toHaveBeenCalled()
      const [message, data] = mockConsoleDebug.mock.calls[0]
      expect(message).toContain('Show submission attempt')
      expect(data.venueCount).toBe(1)
      expect(data.artistCount).toBe(3)
      expect(data.city).toBe('Phoenix')
    })

    it('submitSuccess logs show ID', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.submitSuccess(123, 'req-abc')

      expect(mockConsoleInfo).toHaveBeenCalled()
      const [message, data] = mockConsoleInfo.mock.calls[0]
      expect(message).toContain('Show submitted successfully')
      expect(data.showId).toBe(123)
    })

    it('submitFailed logs error details', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.submitFailed('VALIDATION_ERROR', 'Invalid date', 'req-abc')

      expect(mockConsoleWarn).toHaveBeenCalled()
      const [message, data] = mockConsoleWarn.mock.calls[0]
      expect(message).toContain('Show submission failed')
      expect(data.errorCode).toBe('VALIDATION_ERROR')
      expect(data.message).toBe('Invalid date')
    })

    it('updateAttempt logs show ID and fields', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.updateAttempt(456, ['title', 'description'])

      expect(mockConsoleDebug).toHaveBeenCalled()
      const [message, data] = mockConsoleDebug.mock.calls[0]
      expect(message).toContain('Show update attempt')
      expect(data.showId).toBe(456)
      expect(data.updateFields).toEqual(['title', 'description'])
    })

    it('updateSuccess logs show ID', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.updateSuccess(789, 'req-xyz')

      expect(mockConsoleInfo).toHaveBeenCalled()
      const [message, data] = mockConsoleInfo.mock.calls[0]
      expect(message).toContain('Show updated successfully')
      expect(data.showId).toBe(789)
    })

    it('updateFailed logs error details', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.updateFailed(123, 'NOT_FOUND', 'Show not found', 'req-abc')

      expect(mockConsoleWarn).toHaveBeenCalled()
      const [message, data] = mockConsoleWarn.mock.calls[0]
      expect(message).toContain('Show update failed')
      expect(data.showId).toBe(123)
      expect(data.errorCode).toBe('NOT_FOUND')
    })

    it('deleteAttempt logs show ID', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.deleteAttempt(100)

      expect(mockConsoleDebug).toHaveBeenCalled()
      const [message, data] = mockConsoleDebug.mock.calls[0]
      expect(message).toContain('Show delete attempt')
      expect(data.showId).toBe(100)
    })

    it('deleteSuccess logs show ID', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.deleteSuccess(200, 'req-del')

      expect(mockConsoleInfo).toHaveBeenCalled()
      const [message, data] = mockConsoleInfo.mock.calls[0]
      expect(message).toContain('Show deleted successfully')
      expect(data.showId).toBe(200)
    })

    it('deleteFailed logs error details', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.deleteFailed(300, 'FORBIDDEN', 'Not authorized', 'req-abc')

      expect(mockConsoleWarn).toHaveBeenCalled()
      const [message, data] = mockConsoleWarn.mock.calls[0]
      expect(message).toContain('Show deletion failed')
      expect(data.showId).toBe(300)
      expect(data.errorCode).toBe('FORBIDDEN')
    })

    it('unpublishAttempt logs show ID', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.unpublishAttempt(400)

      expect(mockConsoleDebug).toHaveBeenCalled()
      const [message, data] = mockConsoleDebug.mock.calls[0]
      expect(message).toContain('Show unpublish attempt')
      expect(data.showId).toBe(400)
    })

    it('unpublishSuccess logs show ID', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.unpublishSuccess(500, 'req-unpub')

      expect(mockConsoleInfo).toHaveBeenCalled()
      const [message, data] = mockConsoleInfo.mock.calls[0]
      expect(message).toContain('Show unpublished successfully')
      expect(data.showId).toBe(500)
    })

    it('unpublishFailed logs error details', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.unpublishFailed(
        600,
        'INVALID_STATE',
        'Show already unpublished',
        'req-abc'
      )

      expect(mockConsoleWarn).toHaveBeenCalled()
      const [message, data] = mockConsoleWarn.mock.calls[0]
      expect(message).toContain('Show unpublish failed')
      expect(data.showId).toBe(600)
      expect(data.errorCode).toBe('INVALID_STATE')
    })

    it('handles string show IDs', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.submitSuccess('show-abc-123', 'req-xyz')

      expect(mockConsoleInfo).toHaveBeenCalled()
      const [, data] = mockConsoleInfo.mock.calls[0]
      expect(data.showId).toBe('show-abc-123')
    })
  })

  describe('in production mode', () => {
    beforeEach(() => {
      process.env.NODE_ENV = 'production'
    })

    it('does not log debug messages', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.debug('Test debug message')

      expect(mockConsoleDebug).not.toHaveBeenCalled()
    })

    it('does not log submitAttempt (uses debug)', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.submitAttempt({
        venueCount: 1,
        artistCount: 1,
        city: 'Phoenix',
        state: 'AZ',
      })

      expect(mockConsoleDebug).not.toHaveBeenCalled()
    })

    it('does not log updateAttempt (uses debug)', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.updateAttempt(123, ['title'])

      expect(mockConsoleDebug).not.toHaveBeenCalled()
    })

    it('does not log deleteAttempt (uses debug)', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.deleteAttempt(123)

      expect(mockConsoleDebug).not.toHaveBeenCalled()
    })

    it('does not log unpublishAttempt (uses debug)', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.unpublishAttempt(123)

      expect(mockConsoleDebug).not.toHaveBeenCalled()
    })

    it('still logs info messages', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.info('Test info message')

      expect(mockConsoleInfo).toHaveBeenCalled()
    })

    it('still logs warning messages', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.warn('Test warning')

      expect(mockConsoleWarn).toHaveBeenCalled()
    })

    it('still logs error messages', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.error('Test error', new Error('Test'))

      expect(mockConsoleError).toHaveBeenCalled()
    })

    it('still logs success messages', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.submitSuccess(123)
      showLogger.updateSuccess(456)
      showLogger.deleteSuccess(789)
      showLogger.unpublishSuccess(100)

      expect(mockConsoleInfo).toHaveBeenCalledTimes(4)
    })

    it('still logs failure warnings', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.submitFailed('ERR', 'msg')
      showLogger.updateFailed(1, 'ERR', 'msg')
      showLogger.deleteFailed(2, 'ERR', 'msg')
      showLogger.unpublishFailed(3, 'ERR', 'msg')

      expect(mockConsoleWarn).toHaveBeenCalledTimes(4)
    })
  })

  describe('error serialization', () => {
    beforeEach(() => {
      process.env.NODE_ENV = 'development'
    })

    it('serializes Error instances', async () => {
      const { showLogger } = await import('./showLogger')
      const error = new Error('Test error')
      showLogger.error('Failed', error)

      const [, errorData] = mockConsoleError.mock.calls[0]
      expect(errorData.error.name).toBe('Error')
      expect(errorData.error.message).toBe('Test error')
    })

    it('includes stack trace in development', async () => {
      const { showLogger } = await import('./showLogger')
      const error = new Error('Test error')
      error.stack = 'Error: Test error\n    at test.ts:1:1'
      showLogger.error('Failed', error)

      const [, errorData] = mockConsoleError.mock.calls[0]
      expect(errorData.error.stack).toBeDefined()
    })

    it('serializes non-Error values', async () => {
      const { showLogger } = await import('./showLogger')
      showLogger.error('Failed', 'string error')

      const [, errorData] = mockConsoleError.mock.calls[0]
      expect(errorData.error).toBe('string error')
    })
  })

  describe('in production mode - error logging', () => {
    beforeEach(() => {
      process.env.NODE_ENV = 'production'
    })

    it('logs error message with formatted prefix', async () => {
      const { showLogger } = await import('./showLogger')
      const error = new Error('Test error')
      showLogger.error('Failed', error)

      const [message] = mockConsoleError.mock.calls[0]
      expect(message).toContain('[Show:ERROR]')
      expect(message).toContain('Failed')
    })
  })
})
