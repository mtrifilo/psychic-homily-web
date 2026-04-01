'use client'

/**
 * TanStack Query Configuration
 *
 * This module configures TanStack Query with environment-aware settings
 * and provides query client utilities for the application.
 */

import { QueryClient, DefaultOptions, QueryCache, MutationCache } from '@tanstack/react-query'
import { AuthError, AuthErrorCode } from './errors'
import { artistQueryKeys } from '@/features/artists/api'
import { venueQueryKeys } from '@/features/venues/api'
import { showQueryKeys } from '@/features/shows/api'
import { releaseQueryKeys } from '@/features/releases/api'
import { labelQueryKeys } from '@/features/labels/api'
import { festivalQueryKeys } from '@/features/festivals/api'

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

  // Show queries (defined in features/shows/api.ts)
  shows: showQueryKeys,

  // Venue queries (defined in features/venues/api.ts)
  venues: venueQueryKeys,

  // Admin queries
  admin: {
    stats: ['admin', 'stats'] as const,
    activity: ['admin', 'activity'] as const,
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
    analytics: {
      growth: (months: number) => ['admin', 'analytics', 'growth', months] as const,
      engagement: (months: number) => ['admin', 'analytics', 'engagement', months] as const,
      community: ['admin', 'analytics', 'community'] as const,
      dataQualityTrends: (months: number) => ['admin', 'analytics', 'data-quality', months] as const,
    },
    pendingEdits: (params?: Record<string, unknown>) =>
      ['admin', 'pendingEdits', params] as const,
    entityReports: (params?: Record<string, unknown>) =>
      ['admin', 'entityReports', params] as const,
  },

  // Artist queries (defined in features/artists/api.ts)
  artists: artistQueryKeys,

  // Release queries (defined in features/releases/api.ts)
  releases: releaseQueryKeys,

  // Label queries (defined in features/labels/api.ts)
  labels: labelQueryKeys,

  // Festival queries (defined in features/festivals/api.ts)
  festivals: festivalQueryKeys,

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
    imports: (limit: number, offset: number) =>
      ['pipeline', 'imports', String(limit), String(offset)] as const,
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
    activityHeatmap: (username: string) => ['contributor', 'activityHeatmap', username] as const,
    rankings: (username: string) => ['contributor', 'rankings', username] as const,
  },

  // Crate queries
  crates: {
    all: ['crates'] as const,
    detail: (slug: string) => ['crates', 'detail', slug] as const,
    stats: (slug: string) => ['crates', 'stats', slug] as const,
    my: ['crates', 'my'] as const,
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
    tagEntities: (idOrSlug: string | number, params?: Record<string, unknown>) => ['tags', 'tagEntities', String(idOrSlug), params] as const,
  },

  // Attendance (going/interested) queries
  attendance: {
    all: ['attendance'] as const,
    show: (showId: number) => ['attendance', 'show', showId] as const,
    batch: (showIds: number[]) => ['attendance', 'batch', ...showIds] as const,
    myShows: (params?: Record<string, unknown>) => ['attendance', 'my-shows', params] as const,
  },

  // Follow queries
  follows: {
    all: ['follows'] as const,
    entity: (entityType: string, entityId: number) =>
      ['follows', entityType, entityId] as const,
    batch: (entityType: string, entityIds: number[]) =>
      ['follows', 'batch', entityType, ...entityIds] as const,
    myFollowing: (params?: Record<string, unknown>) =>
      ['follows', 'my-following', params] as const,
    followers: (entityType: string, entityId: number) =>
      ['follows', 'followers', entityType, entityId] as const,
  },

  // Scene queries
  scenes: {
    all: ['scenes'] as const,
    list: ['scenes', 'list'] as const,
    detail: (slug: string) => ['scenes', 'detail', slug] as const,
    artists: (slug: string, period?: number) => ['scenes', 'artists', slug, period] as const,
    genres: (slug: string) => ['scenes', 'genres', slug] as const,
  },

  // Charts queries (public)
  charts: {
    all: ['charts'] as const,
    overview: ['charts', 'overview'] as const,
    trendingShows: (limit?: number) => ['charts', 'trending-shows', limit] as const,
    popularArtists: (limit?: number) => ['charts', 'popular-artists', limit] as const,
    activeVenues: (limit?: number) => ['charts', 'active-venues', limit] as const,
    hotReleases: (limit?: number) => ['charts', 'hot-releases', limit] as const,
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

  // Notification filter queries
  notificationFilters: {
    all: ['notificationFilters'] as const,
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

  // Invalidate admin pending edits queries
  adminPendingEdits: () =>
    queryClient.invalidateQueries({ queryKey: ['admin', 'pendingEdits'] }),

  // Invalidate admin entity reports queries
  adminEntityReports: () =>
    queryClient.invalidateQueries({ queryKey: ['admin', 'entityReports'] }),

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

  // Invalidate crate queries
  crates: () =>
    queryClient.invalidateQueries({ queryKey: ['crates'] }),

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

  // Invalidate follow queries
  follows: () =>
    queryClient.invalidateQueries({ queryKey: ['follows'] }),

  // Invalidate scene queries
  scenes: () =>
    queryClient.invalidateQueries({ queryKey: ['scenes'] }),

  // Invalidate revision queries
  revisions: () =>
    queryClient.invalidateQueries({ queryKey: ['revisions'] }),

  // Invalidate notification filter queries
  notificationFilters: () =>
    queryClient.invalidateQueries({ queryKey: ['notificationFilters'] }),
})
