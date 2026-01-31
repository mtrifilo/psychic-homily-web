'use client'

/**
 * TanStack Query Configuration
 *
 * This module configures TanStack Query with environment-aware settings
 * and provides query client utilities for the application.
 */

import { QueryClient, DefaultOptions } from '@tanstack/react-query'

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
    // Retry mutations once on failure
    retry: 1,
  },
}

// Function to create query client (for use in provider)
function makeQueryClient() {
  return new QueryClient({
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
    pendingVenueEdits: (limit: number, offset: number) =>
      ['admin', 'venues', 'pendingEdits', { limit, offset }] as const,
    unverifiedVenues: (limit: number, offset: number) =>
      ['admin', 'venues', 'unverified', { limit, offset }] as const,
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
})
