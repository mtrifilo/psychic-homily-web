/**
 * Show Logger Utility
 *
 * Provides structured logging for show-related events.
 * Debug logs are only shown in development mode.
 */

const SHOW_DEBUG = process.env.NODE_ENV === 'development'

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
  const prefix = `[Show:${entry.level.toUpperCase()}]`
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
 * Show logger with structured logging methods
 */
export const showLogger = {
  /**
   * Log a debug message (only in development)
   */
  debug: (
    message: string,
    data?: Record<string, unknown>,
    requestId?: string
  ): void => {
    if (!SHOW_DEBUG) return

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
   * Log a show submission attempt
   */
  submitAttempt: (data: {
    venueCount: number
    artistCount: number
    city: string
    state: string
  }): void => {
    showLogger.debug('Show submission attempt', data)
  },

  /**
   * Log a successful show submission
   */
  submitSuccess: (showId: number | string, requestId?: string): void => {
    showLogger.info('Show submitted successfully', { showId }, requestId)
  },

  /**
   * Log a failed show submission
   */
  submitFailed: (
    errorCode: string,
    message: string,
    requestId?: string
  ): void => {
    showLogger.warn('Show submission failed', { errorCode, message }, requestId)
  },

  /**
   * Log a show update attempt
   */
  updateAttempt: (showId: number | string, updateFields: string[]): void => {
    showLogger.debug('Show update attempt', { showId, updateFields })
  },

  /**
   * Log a successful show update
   */
  updateSuccess: (showId: number | string, requestId?: string): void => {
    showLogger.info('Show updated successfully', { showId }, requestId)
  },

  /**
   * Log a failed show update
   */
  updateFailed: (
    showId: number | string,
    errorCode: string,
    message: string,
    requestId?: string
  ): void => {
    showLogger.warn(
      'Show update failed',
      { showId, errorCode, message },
      requestId
    )
  },

  /**
   * Log a show delete attempt
   */
  deleteAttempt: (showId: number | string): void => {
    showLogger.debug('Show delete attempt', { showId })
  },

  /**
   * Log a successful show deletion
   */
  deleteSuccess: (showId: number | string, requestId?: string): void => {
    showLogger.info('Show deleted successfully', { showId }, requestId)
  },

  /**
   * Log a failed show deletion
   */
  deleteFailed: (
    showId: number | string,
    errorCode: string,
    message: string,
    requestId?: string
  ): void => {
    showLogger.warn(
      'Show deletion failed',
      { showId, errorCode, message },
      requestId
    )
  },
}

/**
 * Serialize an error for logging
 */
function serializeError(error: unknown): Record<string, unknown> {
  if (error instanceof Error) {
    return {
      name: error.name,
      message: error.message,
      stack: SHOW_DEBUG ? error.stack : undefined,
    }
  }
  return { value: String(error) }
}

export default showLogger
