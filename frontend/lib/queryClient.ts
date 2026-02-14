'use client'

/**
 * TanStack Query Configuration
 *
 * This module configures TanStack Query with environment-aware settings
 * and provides query client utilities for the application.
 */

import { QueryClient, DefaultOptions, QueryCache, MutationCache } from '@tanstack/react-query'
import { AuthError, AuthErrorCode } from './errors'

// Default query options for all queries
const defaultQueryOptions: DefaultOptions = {
  queries: {
    // Stale time: how long data is considered fresh
    staleTime: 5 * 60 * 1000, // 5 minutes
    // Cache time: how long data stays in cache after last use
    gcTime: 10 * 60 * 1000, // 10 minutes (formerly cacheTime)
    // Retry configuration
    retry: (failureCount, error: Error & { status?: number }) => {
      // Don't retry on 4xx errors (client errors)
      if (error?.status && error.status >= 400 && error.status < 500) {
        return false
      }
      // Retry up to 3 times for other errors
      return failureCount < 3
    },
    // Refetch on window focus (useful for development)
    refetchOnWindowFocus: process.env.NODE_ENV === 'development',
    // Refetch on reconnect
    refetchOnReconnect: true,
  },
  mutations: {
    // Don't retry mutations by default — auth mutations (login, register,
    // password change) and other non-idempotent operations shouldn't silently
    // retry. Individual mutations can opt into retry if needed.
    retry: 0,
  },
}

// Helper to check if an error is a session expiry error
function isSessionExpiredError(error: unknown): boolean {
  if (error instanceof AuthError) {
    return error.shouldRedirectToLogin
  }
  // Check for raw error objects with error_code
  const apiError = error as { code?: string; error_code?: string }
  return (
    apiError?.code === AuthErrorCode.TOKEN_EXPIRED ||
    apiError?.code === AuthErrorCode.TOKEN_INVALID ||
    apiError?.code === AuthErrorCode.TOKEN_MISSING ||
    apiError?.error_code === AuthErrorCode.TOKEN_EXPIRED ||
    apiError?.error_code === AuthErrorCode.TOKEN_INVALID ||
    apiError?.error_code === AuthErrorCode.TOKEN_MISSING
  )
}

// Function to create query client (for use in provider)
function makeQueryClient() {
  // Create caches with global error handlers
  const queryCache = new QueryCache({
    onError: (error, query) => {
      // When a session expires, invalidate the profile query to update auth state.
      // We intentionally DON'T call queryClient.clear() here — clearing causes all
      // active queries to refetch, each getting 401, each triggering this handler
      // again, creating an infinite cascade of clears and refetches.
      if (isSessionExpiredError(error)) {
        if (query.queryKey[0] !== 'auth' || query.queryKey[1] !== 'profile') {
          const profileState = browserQueryClient?.getQueryState(queryKeys.auth.profile)
          if (profileState?.status !== 'error') {
            browserQueryClient?.invalidateQueries({ queryKey: queryKeys.auth.profile })
          }
        }
      }
    },
  })

  const mutationCache = new MutationCache({
    onError: error => {
      // When a session expires during a mutation, invalidate profile to update
      // auth state. Same rationale as above — don't clear the entire cache.
      if (isSessionExpiredError(error)) {
        const profileState = browserQueryClient?.getQueryState(queryKeys.auth.profile)
        if (profileState?.status !== 'error') {
          browserQueryClient?.invalidateQueries({ queryKey: queryKeys.auth.profile })
        }
      }
    },
  })

  return new QueryClient({
    queryCache,
    mutationCache,
    defaultOptions: defaultQueryOptions,
  })
}

let browserQueryClient: QueryClient | undefined = undefined

export function getQueryClient() {
  if (typeof window === 'undefined') {
    // Server: always make a new query client
    return makeQueryClient()
  } else {
    // Browser: make a new query client if we don't already have one
    if (!browserQueryClient) browserQueryClient = makeQueryClient()
    return browserQueryClient
  }
}

// Query key factory for consistent key generation
export const queryKeys = {
  // Authentication queries
  auth: {
    profile: ['auth', 'profile'] as const,
    user: (id: string) => ['auth', 'user', id] as const,
  },

  // Show queries
  shows: {
    all: ['shows'] as const,
    list: (filters?: Record<string, unknown>) =>
      ['shows', 'list', filters] as const,
    cities: (timezone?: string) => ['shows', 'cities', timezone] as const,
    detail: (id: string) => ['shows', 'detail', id] as const,
    userShows: (userId: string) => ['shows', 'user', userId] as const,
  },

  // Venue queries
  venues: {
    all: ['venues'] as const,
    list: (filters?: Record<string, unknown>) =>
      ['venues', 'list', filters] as const,
    cities: ['venues', 'cities'] as const,
    detail: (idOrSlug: string | number) => ['venues', 'detail', String(idOrSlug)] as const,
    search: (query: string) =>
      ['venues', 'search', query.toLowerCase()] as const,
    shows: (venueIdOrSlug: string | number) => ['venues', 'shows', String(venueIdOrSlug)] as const,
    myPendingEdit: (venueIdOrSlug: string | number) =>
      ['venues', 'myPendingEdit', String(venueIdOrSlug)] as const,
  },

  // Admin queries
  admin: {
    stats: ['admin', 'stats'] as const,
    pendingVenueEdits: (limit: number, offset: number) =>
      ['admin', 'venues', 'pendingEdits', { limit, offset }] as const,
    unverifiedVenues: (limit: number, offset: number) =>
      ['admin', 'venues', 'unverified', { limit, offset }] as const,
    auditLogs: (limit: number, offset: number) =>
      ['admin', 'auditLogs', { limit, offset }] as const,
    users: (limit: number, offset: number, search: string) =>
      ['admin', 'users', { limit, offset, search }] as const,
  },

  // Artist queries
  artists: {
    all: ['artists'] as const,
    search: (query: string) =>
      ['artists', 'search', query.toLowerCase()] as const,
    detail: (idOrSlug: string | number) => ['artists', 'detail', String(idOrSlug)] as const,
    shows: (artistIdOrSlug: string | number) => ['artists', 'shows', String(artistIdOrSlug)] as const,
  },

  // Saved shows queries (user's "My List")
  savedShows: {
    all: ['savedShows'] as const,
    list: (userId?: string) => ['savedShows', 'list', userId] as const,
    check: (showId: string | number) =>
      ['savedShows', 'check', String(showId)] as const,
    batch: (showIds: number[]) =>
      ['savedShows', 'batch', showIds] as const,
  },

  // Favorite venues queries
  favoriteVenues: {
    all: ['favoriteVenues'] as const,
    list: (userId?: string) => ['favoriteVenues', 'list', userId] as const,
    check: (venueId: string | number) =>
      ['favoriteVenues', 'check', String(venueId)] as const,
    shows: (params?: Record<string, unknown>) =>
      ['favoriteVenues', 'shows', params] as const,
  },

  // User's submitted shows
  mySubmissions: {
    all: ['mySubmissions'] as const,
    list: () => ['mySubmissions', 'list'] as const,
  },

  // Show reports queries
  showReports: {
    all: ['showReports'] as const,
    myReport: (showId: string | number) =>
      ['showReports', 'myReport', String(showId)] as const,
    pending: (limit: number, offset: number) =>
      ['showReports', 'pending', { limit, offset }] as const,
  },

  // Artist reports queries
  artistReports: {
    all: ['artistReports'] as const,
    myReport: (artistId: string | number) =>
      ['artistReports', 'myReport', String(artistId)] as const,
    pending: (limit: number, offset: number) =>
      ['artistReports', 'pending', { limit, offset }] as const,
  },

  // System queries
  system: {
    health: ['system', 'health'] as const,
  },
} as const

// Utility function to invalidate related queries
export const createInvalidateQueries = (queryClient: QueryClient) => ({
  // Invalidate all auth-related queries
  auth: () => queryClient.invalidateQueries({ queryKey: ['auth'] }),

  // Invalidate all show-related queries
  shows: () => queryClient.invalidateQueries({ queryKey: ['shows'] }),

  // Invalidate specific show queries
  show: (id: string) =>
    queryClient.invalidateQueries({ queryKey: ['shows', 'detail', id] }),

  // Invalidate artist queries
  artists: () => queryClient.invalidateQueries({ queryKey: ['artists'] }),

  // Invalidate all venue-related queries
  venues: () => queryClient.invalidateQueries({ queryKey: ['venues'] }),

  // Invalidate saved shows queries
  savedShows: () =>
    queryClient.invalidateQueries({ queryKey: ['savedShows'] }),

  // Invalidate favorite venues queries
  favoriteVenues: () =>
    queryClient.invalidateQueries({ queryKey: ['favoriteVenues'] }),

  // Invalidate user's submissions queries
  mySubmissions: () =>
    queryClient.invalidateQueries({ queryKey: ['mySubmissions'] }),

  // Invalidate show reports queries
  showReports: () =>
    queryClient.invalidateQueries({ queryKey: ['showReports'] }),

  // Invalidate artist reports queries
  artistReports: () =>
    queryClient.invalidateQueries({ queryKey: ['artistReports'] }),

  // Invalidate audit logs queries
  auditLogs: () =>
    queryClient.invalidateQueries({ queryKey: ['admin', 'auditLogs'] }),

  // Invalidate admin users queries
  adminUsers: () =>
    queryClient.invalidateQueries({ queryKey: ['admin', 'users'] }),
})
