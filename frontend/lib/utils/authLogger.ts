/**
 * Auth Logger Utility
 *
 * Provides structured logging for authentication-related events.
 * Debug logs are only shown in development mode.
 */

const AUTH_DEBUG = process.env.NODE_ENV === 'development'

// Log levels
type LogLevel = 'debug' | 'info' | 'warn' | 'error'

// Log entry structure
interface LogEntry {
  timestamp: string
  level: LogLevel
  message: string
  data?: Record<string, unknown>
  requestId?: string
}

/**
 * Format a log entry for console output
 */
function formatLog(entry: LogEntry): string {
  const prefix = `[Auth:${entry.level.toUpperCase()}]`
  const requestIdStr = entry.requestId
    ? ` [${entry.requestId.slice(0, 8)}]`
    : ''
  return `${prefix}${requestIdStr} ${entry.message}`
}

/**
 * Create a log entry
 */
function createLogEntry(
  level: LogLevel,
  message: string,
  data?: Record<string, unknown>,
  requestId?: string
): LogEntry {
  return {
    timestamp: new Date().toISOString(),
    level,
    message,
    data,
    requestId,
  }
}

/**
 * Auth logger with structured logging methods
 */
export const authLogger = {
  /**
   * Log a debug message (only in development)
   */
  debug: (
    message: string,
    data?: Record<string, unknown>,
    requestId?: string
  ): void => {
    if (!AUTH_DEBUG) return

    const entry = createLogEntry('debug', message, data, requestId)
    console.debug(formatLog(entry), data ?? '')
  },

  /**
   * Log an info message
   */
  info: (
    message: string,
    data?: Record<string, unknown>,
    requestId?: string
  ): void => {
    const entry = createLogEntry('info', message, data, requestId)
    console.info(formatLog(entry), data ?? '')
  },

  /**
   * Log a warning message
   */
  warn: (
    message: string,
    data?: Record<string, unknown>,
    requestId?: string
  ): void => {
    const entry = createLogEntry('warn', message, data, requestId)
    console.warn(formatLog(entry), data ?? '')
  },

  /**
   * Log an error message
   */
  error: (
    message: string,
    error: unknown,
    context?: Record<string, unknown>,
    requestId?: string
  ): void => {
    const entry = createLogEntry(
      'error',
      message,
      { error: serializeError(error), ...context },
      requestId
    )
    console.error(formatLog(entry), { error, ...context })
  },

  /**
   * Log a login attempt
   */
  loginAttempt: (email: string): void => {
    authLogger.debug('Login attempt', { email: maskEmail(email) })
  },

  /**
   * Log a successful login
   */
  loginSuccess: (userId: string | number, requestId?: string): void => {
    authLogger.info('Login successful', { userId }, requestId)
  },

  /**
   * Log a failed login
   */
  loginFailed: (
    errorCode: string,
    message: string,
    requestId?: string
  ): void => {
    authLogger.warn('Login failed', { errorCode, message }, requestId)
  },

  /**
   * Log a logout
   */
  logout: (userId?: string | number): void => {
    authLogger.info('User logged out', userId ? { userId } : undefined)
  },

  /**
   * Log a token refresh attempt
   */
  tokenRefresh: (success: boolean, requestId?: string): void => {
    if (success) {
      authLogger.debug('Token refreshed successfully', undefined, requestId)
    } else {
      authLogger.warn('Token refresh failed', undefined, requestId)
    }
  },

  /**
   * Log an auth state change
   */
  authStateChange: (
    isAuthenticated: boolean,
    userId?: string | number
  ): void => {
    authLogger.debug('Auth state changed', { isAuthenticated, userId })
  },

  /**
   * Log a profile fetch
   */
  profileFetch: (
    success: boolean,
    userId?: string | number,
    requestId?: string
  ): void => {
    if (success) {
      authLogger.debug('Profile fetched', { userId }, requestId)
    } else {
      authLogger.debug('Profile fetch failed', undefined, requestId)
    }
  },
}

/**
 * Mask an email address for logging (show first 2 chars + domain)
 */
function maskEmail(email: string): string {
  if (!email || email.length < 3) return '***'
  const atIndex = email.indexOf('@')
  if (atIndex <= 2) return email.slice(0, 2) + '***'
  return email.slice(0, 2) + '***' + email.slice(atIndex)
}

/**
 * Serialize an error for logging
 */
function serializeError(error: unknown): Record<string, unknown> {
  if (error instanceof Error) {
    return {
      name: error.name,
      message: error.message,
      stack: AUTH_DEBUG ? error.stack : undefined,
    }
  }
  return { value: String(error) }
}

export default authLogger
