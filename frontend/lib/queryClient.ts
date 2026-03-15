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
    staleTime: 15 * 60 * 1000, // 15 minutes
    // Cache time: how long data stays in cache after last use
    gcTime: 30 * 60 * 1000, // 30 minutes (formerly cacheTime)
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
    dataQuality: {
      summary: ['admin', 'dataQuality', 'summary'] as const,
      category: (category: string, limit: number, offset: number) =>
        ['admin', 'dataQuality', 'category', category, { limit, offset }] as const,
    },
  },

  // Artist queries
  artists: {
    all: ['artists'] as const,
    list: (filters?: Record<string, unknown>) =>
      ['artists', 'list', filters] as const,
    cities: ['artists', 'cities'] as const,
    search: (query: string) =>
      ['artists', 'search', query.toLowerCase()] as const,
    detail: (idOrSlug: string | number) => ['artists', 'detail', String(idOrSlug)] as const,
    shows: (artistIdOrSlug: string | number) => ['artists', 'shows', String(artistIdOrSlug)] as const,
    labels: (artistIdOrSlug: string | number) => ['artists', 'labels', String(artistIdOrSlug)] as const,
    aliases: (artistId: number) => ['artists', 'aliases', artistId] as const,
  },

  // Release queries
  releases: {
    all: ['releases'] as const,
    list: (filters?: Record<string, unknown>) =>
      ['releases', 'list', filters] as const,
    detail: (idOrSlug: string | number) => ['releases', 'detail', String(idOrSlug)] as const,
    artistReleases: (artistIdOrSlug: string | number) =>
      ['releases', 'artist', String(artistIdOrSlug)] as const,
  },

  // Label queries
  labels: {
    all: ['labels'] as const,
    list: (filters?: Record<string, unknown>) =>
      ['labels', 'list', filters] as const,
    detail: (idOrSlug: string | number) => ['labels', 'detail', String(idOrSlug)] as const,
    roster: (idOrSlug: string | number) => ['labels', 'roster', String(idOrSlug)] as const,
    catalog: (idOrSlug: string | number) => ['labels', 'catalog', String(idOrSlug)] as const,
  },

  // Festival queries
  festivals: {
    all: ['festivals'] as const,
    list: (filters?: Record<string, unknown>) =>
      ['festivals', 'list', filters] as const,
    detail: (idOrSlug: string | number) => ['festivals', 'detail', String(idOrSlug)] as const,
    artists: (idOrSlug: string | number, dayDate?: string) =>
      ['festivals', 'artists', String(idOrSlug), dayDate] as const,
    venues: (idOrSlug: string | number) =>
      ['festivals', 'venues', String(idOrSlug)] as const,
    artistFestivals: (artistIdOrSlug: string | number) =>
      ['festivals', 'artist', String(artistIdOrSlug)] as const,
  },

  // Calendar feed queries
  calendar: {
    all: ['calendar'] as const,
    tokenStatus: ['calendar', 'tokenStatus'] as const,
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

  // Pipeline queries
  pipeline: {
    venues: ['pipeline', 'venues'] as const,
    venueStats: (venueId: string | number) =>
      ['pipeline', 'venueStats', String(venueId)] as const,
    venueRuns: (venueId: string | number) =>
      ['pipeline', 'venueRuns', String(venueId)] as const,
  },

  // Contributor profile queries
  contributor: {
    profile: (username: string) => ['contributor', 'profile', username] as const,
    ownProfile: ['contributor', 'ownProfile'] as const,
    contributions: (username: string) => ['contributor', 'contributions', username] as const,
    ownContributions: ['contributor', 'ownContributions'] as const,
    sections: (username: string) => ['contributor', 'sections', username] as const,
    ownSections: ['contributor', 'ownSections'] as const,
  },

  // Collection queries
  collections: {
    all: ['collections'] as const,
    detail: (slug: string) => ['collections', 'detail', slug] as const,
    stats: (slug: string) => ['collections', 'stats', slug] as const,
    my: ['collections', 'my'] as const,
  },

  // Request queries
  requests: {
    all: ['requests'] as const,
    list: (params?: Record<string, unknown>) =>
      ['requests', 'list', params] as const,
    detail: (requestId: number) =>
      ['requests', 'detail', requestId] as const,
  },

  // Tag queries
  tags: {
    all: ['tags'] as const,
    list: (params?: Record<string, unknown>) => ['tags', 'list', params] as const,
    search: (query: string) => ['tags', 'search', query.toLowerCase()] as const,
    detail: (idOrSlug: string | number) => ['tags', 'detail', String(idOrSlug)] as const,
    aliases: (tagId: number) => ['tags', 'aliases', tagId] as const,
    entityTags: (entityType: string, entityId: number) => ['tags', 'entityTags', entityType, entityId] as const,
  },

  // Attendance (going/interested) queries
  attendance: {
    all: ['attendance'] as const,
    show: (showId: number) => ['attendance', 'show', showId] as const,
    batch: (showIds: number[]) => ['attendance', 'batch', ...showIds] as const,
    myShows: (params?: Record<string, unknown>) => ['attendance', 'my-shows', params] as const,
  },

  // Scene queries
  scenes: {
    all: ['scenes'] as const,
    list: ['scenes', 'list'] as const,
    detail: (slug: string) => ['scenes', 'detail', slug] as const,
    artists: (slug: string, period?: number) => ['scenes', 'artists', slug, period] as const,
  },

  // Revision history queries
  revisions: {
    all: ['revisions'] as const,
    entity: (entityType: string, entityId: string | number) =>
      ['revisions', 'entity', entityType, String(entityId)] as const,
    detail: (revisionId: number) =>
      ['revisions', 'detail', revisionId] as const,
    user: (userId: string | number) =>
      ['revisions', 'user', String(userId)] as const,
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

  // Invalidate release queries
  releases: () => queryClient.invalidateQueries({ queryKey: ['releases'] }),

  // Invalidate label queries
  labels: () => queryClient.invalidateQueries({ queryKey: ['labels'] }),

  // Invalidate festival queries
  festivals: () => queryClient.invalidateQueries({ queryKey: ['festivals'] }),

  // Invalidate all venue-related queries
  venues: () => queryClient.invalidateQueries({ queryKey: ['venues'] }),

  // Invalidate calendar queries
  calendar: () =>
    queryClient.invalidateQueries({ queryKey: ['calendar'] }),

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

  // Invalidate contributor profile queries
  contributor: () =>
    queryClient.invalidateQueries({ queryKey: ['contributor'] }),

  // Invalidate own contributor profile queries
  ownContributor: () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: ['contributor', 'ownProfile'] }),
      queryClient.invalidateQueries({ queryKey: ['contributor', 'ownSections'] }),
      queryClient.invalidateQueries({ queryKey: ['contributor', 'ownContributions'] }),
    ]),

  // Invalidate collection queries
  collections: () =>
    queryClient.invalidateQueries({ queryKey: ['collections'] }),

  // Invalidate request queries
  requests: () =>
    queryClient.invalidateQueries({ queryKey: ['requests'] }),

  // Invalidate tag queries
  tags: () =>
    queryClient.invalidateQueries({ queryKey: ['tags'] }),

  // Invalidate entity tag queries
  entityTags: (entityType: string, entityId: number) =>
    queryClient.invalidateQueries({ queryKey: ['tags', 'entityTags', entityType, entityId] }),

  // Invalidate attendance queries
  attendance: () =>
    queryClient.invalidateQueries({ queryKey: ['attendance'] }),

  // Invalidate scene queries
  scenes: () =>
    queryClient.invalidateQueries({ queryKey: ['scenes'] }),

  // Invalidate revision queries
  revisions: () =>
    queryClient.invalidateQueries({ queryKey: ['revisions'] }),
})
