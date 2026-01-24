import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// Store original NODE_ENV
const originalEnv = process.env.NODE_ENV

// Mock console methods
const mockConsoleDebug = vi.fn()
const mockConsoleInfo = vi.fn()
const mockConsoleWarn = vi.fn()
const mockConsoleError = vi.fn()

describe('authLogger', () => {
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
      const { authLogger } = await import('./authLogger')
      authLogger.debug('Test debug message', { key: 'value' })

      expect(mockConsoleDebug).toHaveBeenCalled()
      const [message] = mockConsoleDebug.mock.calls[0]
      expect(message).toContain('[Auth:DEBUG]')
      expect(message).toContain('Test debug message')
    })

    it('logs debug messages with request ID', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.debug('Test message', undefined, 'req-12345678-abcd')

      expect(mockConsoleDebug).toHaveBeenCalled()
      const [message] = mockConsoleDebug.mock.calls[0]
      expect(message).toContain('[req-1234]')
    })

    it('logs info messages', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.info('Test info message', { userId: '123' })

      expect(mockConsoleInfo).toHaveBeenCalled()
      const [message] = mockConsoleInfo.mock.calls[0]
      expect(message).toContain('[Auth:INFO]')
      expect(message).toContain('Test info message')
    })

    it('logs warning messages', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.warn('Test warning', { errorCode: 'ERR_001' })

      expect(mockConsoleWarn).toHaveBeenCalled()
      const [message] = mockConsoleWarn.mock.calls[0]
      expect(message).toContain('[Auth:WARN]')
    })

    it('logs error messages with error details', async () => {
      const { authLogger } = await import('./authLogger')
      const error = new Error('Test error')
      authLogger.error('Something failed', error, { context: 'login' })

      expect(mockConsoleError).toHaveBeenCalled()
      const [message] = mockConsoleError.mock.calls[0]
      expect(message).toContain('[Auth:ERROR]')
      expect(message).toContain('Something failed')
    })

    it('includes stack trace in error logs in development', async () => {
      const { authLogger } = await import('./authLogger')
      const error = new Error('Test error')
      error.stack = 'Error: Test error\n    at test.ts:1:1'
      authLogger.error('Something failed', error)

      expect(mockConsoleError).toHaveBeenCalled()
      const [, errorData] = mockConsoleError.mock.calls[0]
      expect(errorData.error.stack).toBeDefined()
    })

    it('loginAttempt logs masked email', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.loginAttempt('test@example.com')

      expect(mockConsoleDebug).toHaveBeenCalled()
      const [message, data] = mockConsoleDebug.mock.calls[0]
      expect(message).toContain('Login attempt')
      expect(data.email).toBe('te***@example.com')
    })

    it('loginSuccess logs user ID', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.loginSuccess('user-123', 'req-abc')

      expect(mockConsoleInfo).toHaveBeenCalled()
      const [message] = mockConsoleInfo.mock.calls[0]
      expect(message).toContain('Login successful')
    })

    it('loginFailed logs error details', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.loginFailed('INVALID_CREDENTIALS', 'Wrong password', 'req-abc')

      expect(mockConsoleWarn).toHaveBeenCalled()
      const [message] = mockConsoleWarn.mock.calls[0]
      expect(message).toContain('Login failed')
    })

    it('logout logs user ID when provided', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.logout('user-123')

      expect(mockConsoleInfo).toHaveBeenCalled()
      const [message, data] = mockConsoleInfo.mock.calls[0]
      expect(message).toContain('User logged out')
      expect(data.userId).toBe('user-123')
    })

    it('logout logs without user ID', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.logout()

      expect(mockConsoleInfo).toHaveBeenCalled()
      const [message] = mockConsoleInfo.mock.calls[0]
      expect(message).toContain('User logged out')
    })

    it('tokenRefresh logs success', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.tokenRefresh(true, 'req-abc')

      expect(mockConsoleDebug).toHaveBeenCalled()
      const [message] = mockConsoleDebug.mock.calls[0]
      expect(message).toContain('Token refreshed successfully')
    })

    it('tokenRefresh logs failure', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.tokenRefresh(false, 'req-abc')

      expect(mockConsoleWarn).toHaveBeenCalled()
      const [message] = mockConsoleWarn.mock.calls[0]
      expect(message).toContain('Token refresh failed')
    })

    it('authStateChange logs state change', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.authStateChange(true, 'user-123')

      expect(mockConsoleDebug).toHaveBeenCalled()
      const [message, data] = mockConsoleDebug.mock.calls[0]
      expect(message).toContain('Auth state changed')
      expect(data.isAuthenticated).toBe(true)
    })

    it('profileFetch logs success', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.profileFetch(true, 'user-123', 'req-abc')

      expect(mockConsoleDebug).toHaveBeenCalled()
      const [message] = mockConsoleDebug.mock.calls[0]
      expect(message).toContain('Profile fetched')
    })

    it('profileFetch logs failure', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.profileFetch(false, undefined, 'req-abc')

      expect(mockConsoleDebug).toHaveBeenCalled()
      const [message] = mockConsoleDebug.mock.calls[0]
      expect(message).toContain('Profile fetch failed')
    })
  })

  describe('in production mode', () => {
    beforeEach(() => {
      process.env.NODE_ENV = 'production'
    })

    it('does not log debug messages', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.debug('Test debug message')

      expect(mockConsoleDebug).not.toHaveBeenCalled()
    })

    it('still logs info messages', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.info('Test info message')

      expect(mockConsoleInfo).toHaveBeenCalled()
    })

    it('still logs warning messages', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.warn('Test warning')

      expect(mockConsoleWarn).toHaveBeenCalled()
    })

    it('still logs error messages', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.error('Test error', new Error('Test'))

      expect(mockConsoleError).toHaveBeenCalled()
    })

    it('logs error message with formatted prefix', async () => {
      const { authLogger } = await import('./authLogger')
      const error = new Error('Test error')
      authLogger.error('Something failed', error)

      const [message] = mockConsoleError.mock.calls[0]
      expect(message).toContain('[Auth:ERROR]')
      expect(message).toContain('Something failed')
    })
  })

  describe('email masking', () => {
    beforeEach(() => {
      process.env.NODE_ENV = 'development'
    })

    it('masks email with 2 chars before @ and shows domain', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.loginAttempt('test@example.com')

      const [, data] = mockConsoleDebug.mock.calls[0]
      expect(data.email).toBe('te***@example.com')
    })

    it('masks short emails (2 chars or less before @)', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.loginAttempt('ab@x.com')

      const [, data] = mockConsoleDebug.mock.calls[0]
      expect(data.email).toBe('ab***')
    })

    it('masks very short emails', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.loginAttempt('a@x.com')

      const [, data] = mockConsoleDebug.mock.calls[0]
      // Should show first 2 chars + ***
      expect(data.email).toBe('a@***')
    })

    it('handles empty email', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.loginAttempt('')

      const [, data] = mockConsoleDebug.mock.calls[0]
      expect(data.email).toBe('***')
    })

    it('handles invalid email format', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.loginAttempt('invalid')

      const [, data] = mockConsoleDebug.mock.calls[0]
      // No @ sign, so it shows first 2 chars + ***
      expect(data.email).toBe('in***')
    })
  })

  describe('error serialization', () => {
    beforeEach(() => {
      process.env.NODE_ENV = 'development'
    })

    it('serializes Error instances', async () => {
      const { authLogger } = await import('./authLogger')
      const error = new Error('Test error')
      authLogger.error('Failed', error)

      const [, errorData] = mockConsoleError.mock.calls[0]
      expect(errorData.error.name).toBe('Error')
      expect(errorData.error.message).toBe('Test error')
    })

    it('serializes non-Error values', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.error('Failed', 'string error')

      const [, errorData] = mockConsoleError.mock.calls[0]
      expect(errorData.error).toBe('string error')
    })

    it('serializes objects', async () => {
      const { authLogger } = await import('./authLogger')
      authLogger.error('Failed', { code: 500, message: 'Server error' })

      const [, errorData] = mockConsoleError.mock.calls[0]
      expect(errorData.error.code).toBe(500)
    })
  })
})
