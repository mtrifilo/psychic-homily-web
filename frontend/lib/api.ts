/**
 * API Configuration Utility
 *
 * This module provides centralized API configuration that automatically
 * selects the correct backend URL based on the environment.
 *
 * In development, requests go through a Next.js API proxy (/api/*)
 * to handle cookie same-origin requirements.
 */

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
  },
  ARTISTS: {
    SEARCH: `${API_BASE_URL}/artists/search`,
  },
  VENUES: {
    SEARCH: `${API_BASE_URL}/venues/search`,
  },

  // System endpoints
  HEALTH: `${API_BASE_URL}/health`,
  OPENAPI: `${API_BASE_URL}/openapi.json`,
} as const

// Utility function to make API requests with proper configuration
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

  const response = await fetch(endpoint, config)

  if (!response.ok) {
    const error = await response.json().catch(() => ({
      message: `HTTP ${response.status}: ${response.statusText}`,
    }))

    // Create a custom error object that can be checked by retry logic
    const apiError: Error & {
      status?: number
      statusText?: string
      details?: unknown
    } = new Error(
      error.message || `HTTP ${response.status}: ${response.statusText}`
    )
    apiError.status = response.status
    apiError.statusText = response.statusText
    apiError.details = error.details || error.errors || error

    throw apiError
  }

  return response.json()
}

// Environment information for debugging
export const getEnvironmentInfo = () => ({
  apiBaseUrl: API_BASE_URL,
  environment: process.env.NODE_ENV,
  isDevelopment: process.env.NODE_ENV === 'development',
  isProduction: process.env.NODE_ENV === 'production',
})
