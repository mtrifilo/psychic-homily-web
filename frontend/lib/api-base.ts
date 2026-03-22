/**
 * API Base URL Configuration
 *
 * Extracted to its own module to break circular imports between
 * lib/api.ts and feature module api.ts files.
 *
 * Feature modules (features/artists/api.ts, etc.) import API_BASE_URL from here.
 * lib/api.ts re-exports it for backward compatibility.
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
