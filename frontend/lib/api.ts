/**
 * API Configuration Utility
 *
 * This module provides centralized API configuration that automatically
 * selects the correct backend URL based on the environment.
 *
 * In development, requests go through a Next.js API proxy (/api/*)
 * to handle cookie same-origin requirements.
 */

import { authLogger } from './utils/authLogger'
import { AuthError, AuthErrorCode } from './errors'

// Request ID header name (must match backend middleware)
const REQUEST_ID_HEADER = 'X-Request-ID'

// Get the API base URL
const getApiBaseUrl = (): string => {
  // Check for environment-specific API URL first
  if (process.env.NEXT_PUBLIC_API_URL) {
    return process.env.NEXT_PUBLIC_API_URL
  }

  // In browser during development, use Next.js API proxy
  // This handles same-origin cookie requirements
  if (typeof window !== 'undefined' && process.env.NODE_ENV === 'development') {
    return '/api'
  }

  // Server-side in development
  if (process.env.NODE_ENV === 'development') {
    return 'http://localhost:8080'
  }

  // Production fallback
  return 'https://api.psychichomily.com'
}

// Export the configured API base URL
export const API_BASE_URL = getApiBaseUrl()

// API endpoint configuration
export const API_ENDPOINTS = {
  // Authentication endpoints
  AUTH: {
    LOGIN: `${API_BASE_URL}/auth/login`,
    LOGOUT: `${API_BASE_URL}/auth/logout`,
    REGISTER: `${API_BASE_URL}/auth/register`,
    PROFILE: `${API_BASE_URL}/auth/profile`,
    REFRESH: `${API_BASE_URL}/auth/refresh`,
    // OAuth endpoints
    OAUTH_LOGIN: (provider: string) => `${API_BASE_URL}/auth/login/${provider}`,
    OAUTH_CALLBACK: (provider: string) =>
      `${API_BASE_URL}/auth/callback/${provider}`,
  },

  // Application endpoints
  SHOWS: {
    SUBMIT: `${API_BASE_URL}/shows`,
    UPCOMING: `${API_BASE_URL}/shows/upcoming`,
    GET: (showId: string | number) => `${API_BASE_URL}/shows/${showId}`,
    UPDATE: (showId: string | number) => `${API_BASE_URL}/shows/${showId}`,
    DELETE: (showId: string | number) => `${API_BASE_URL}/shows/${showId}`,
    UNPUBLISH: (showId: string | number) =>
      `${API_BASE_URL}/shows/${showId}/unpublish`,
  },
  ARTISTS: {
    SEARCH: `${API_BASE_URL}/artists/search`,
  },
  VENUES: {
    SEARCH: `${API_BASE_URL}/venues/search`,
  },

  // Saved shows (user's "My List") endpoints
  SAVED_SHOWS: {
    LIST: `${API_BASE_URL}/saved-shows`,
    SAVE: (showId: string | number) => `${API_BASE_URL}/saved-shows/${showId}`,
    UNSAVE: (showId: string | number) =>
      `${API_BASE_URL}/saved-shows/${showId}`,
    CHECK: (showId: string | number) =>
      `${API_BASE_URL}/saved-shows/${showId}/check`,
  },

  // Admin endpoints
  ADMIN: {
    SHOWS: {
      PENDING: `${API_BASE_URL}/admin/shows/pending`,
      APPROVE: (showId: string | number) =>
        `${API_BASE_URL}/admin/shows/${showId}/approve`,
      REJECT: (showId: string | number) =>
        `${API_BASE_URL}/admin/shows/${showId}/reject`,
    },
    VENUES: {
      VERIFY: (venueId: string | number) =>
        `${API_BASE_URL}/admin/venues/${venueId}/verify`,
    },
  },

  // System endpoints
  HEALTH: `${API_BASE_URL}/health`,
  OPENAPI: `${API_BASE_URL}/openapi.json`,
} as const

/**
 * Extended error type for API errors
 */
export interface ApiError extends Error {
  status?: number
  statusText?: string
  requestId?: string
  errorCode?: string
  details?: unknown
}

/**
 * Base response type that includes request ID (optional fields for compatibility)
 */
export interface ApiResponse {
  success?: boolean
  message?: string
  error_code?: string
  request_id?: string
  [key: string]: unknown // Allow additional properties
}

/**
 * Make API requests with proper configuration, error handling, and request ID extraction
 */
export const apiRequest = async <T = unknown>(
  endpoint: string,
  options: RequestInit = {}
): Promise<T> => {
  const defaultHeaders: Record<string, string> = {
    'Content-Type': 'application/json',
  }

  const config: RequestInit = {
    credentials: 'include', // Always include cookies for HTTP-only auth
    ...options,
    headers: {
      ...defaultHeaders,
      ...options.headers,
    },
  }

  authLogger.debug('API request', {
    endpoint: endpoint.replace(API_BASE_URL, ''),
    method: config.method || 'GET',
  })

  const response = await fetch(endpoint, config)

  // Extract request ID from response headers
  const requestId = response.headers.get(REQUEST_ID_HEADER) || undefined

  if (!response.ok) {
    const errorBody = await response.json().catch(() => ({
      message: `HTTP ${response.status}: ${response.statusText}`,
    }))

    // Log the API error with request ID
    authLogger.error(
      'API request failed',
      new Error(errorBody.message || response.statusText),
      {
        endpoint: endpoint.replace(API_BASE_URL, ''),
        status: response.status,
        errorCode: errorBody.error_code,
      },
      requestId || errorBody.request_id
    )

    // Check if this is an auth-related error
    if (response.status === 401 || response.status === 403) {
      throw new AuthError(
        errorBody.message || 'Authentication failed',
        errorBody.error_code || AuthErrorCode.UNAUTHORIZED,
        {
          requestId: requestId || errorBody.request_id,
          status: response.status,
        }
      )
    }

    // Create a standard API error
    const apiError: ApiError = new Error(
      errorBody.message || `HTTP ${response.status}: ${response.statusText}`
    )
    apiError.status = response.status
    apiError.statusText = response.statusText
    apiError.requestId = requestId || errorBody.request_id
    apiError.errorCode = errorBody.error_code
    apiError.details = errorBody.details || errorBody.errors || errorBody

    throw apiError
  }

  // Handle 204 No Content responses (e.g., DELETE operations)
  if (response.status === 204) {
    authLogger.debug(
      'API response',
      {
        endpoint: endpoint.replace(API_BASE_URL, ''),
        success: true,
      },
      requestId
    )
    return undefined as T
  }

  // Parse successful response
  const data = (await response.json()) as T

  // Inject request ID from header if not in response body (if data is an object)
  if (requestId && data && typeof data === 'object') {
    const dataObj = data as Record<string, unknown>
    if (!dataObj.request_id) {
      dataObj.request_id = requestId
    }
  }

  // Log response (safely access success property if it exists)
  const dataObj = data as Record<string, unknown> | null
  authLogger.debug(
    'API response',
    {
      endpoint: endpoint.replace(API_BASE_URL, ''),
      success: dataObj?.success,
    },
    requestId
  )

  return data
}

/**
 * Environment information for debugging
 */
export const getEnvironmentInfo = () => ({
  apiBaseUrl: API_BASE_URL,
  environment: process.env.NODE_ENV,
  isDevelopment: process.env.NODE_ENV === 'development',
  isProduction: process.env.NODE_ENV === 'production',
})

/**
 * Extract request ID from an error object
 */
export function getRequestIdFromError(error: unknown): string | undefined {
  if (error instanceof AuthError) {
    return error.requestId
  }
  if (error && typeof error === 'object' && 'requestId' in error) {
    return (error as ApiError).requestId
  }
  return undefined
}
