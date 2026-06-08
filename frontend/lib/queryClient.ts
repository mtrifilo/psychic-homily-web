/**
 * TanStack Query Configuration
 *
 * This module configures TanStack Query with environment-aware settings
 * and provides query client utilities for the application.
 *
 * No `'use client'` directive: the module body is isomorphic so server
 * components (e.g. `app/artists/[slug]/page.tsx`) can import
 * `getQueryClient` for SSR prefetch + hydration. `getQueryClient` gates
 * on `typeof window` to mint a fresh client per request on the server
 * and reuse a singleton in the browser. The cache `onError` handlers
 * read `browserQueryClient?.…` which safely short-circuits on the server.
 */

import { QueryClient, DefaultOptions, QueryCache, MutationCache } from '@tanstack/react-query'
import { AuthError, AuthErrorCode } from './errors'
import { artistQueryKeys } from '@/features/artists/api'
import { venueQueryKeys } from '@/features/venues/api'
import { showQueryKeys } from '@/features/shows/api'
import { releaseQueryKeys } from '@/features/releases/api'
import { labelQueryKeys } from '@/features/labels/api'
import { festivalQueryKeys } from '@/features/festivals/api'
import { radioQueryKeys } from '@/features/radio/api'

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

// `useProfile` resolves its queryFn to a `UserProfile` shape — when the
// user is logged out the payload is `{ success: false, ... }` rather than
// an error. Used by the global error handlers below so a sibling query's
// 401 doesn't re-invalidate a profile that already knows it's anonymous
// (either via the SSR auth-profile seed in `prefetchAuthProfile` or a
// prior 401).
function profileAlreadyKnowsAnonymous(
  client: QueryClient | undefined
): boolean {
  if (!client) return false
  const state = client.getQueryState(queryKeys.auth.profile)
  if (!state) return false
  if (state.status === 'error') return true
  const data = state.data as { success?: boolean } | undefined
  return data?.success === false
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
          // Skip the invalidation if the profile cache already encodes
          // the "logged out" answer — either as an error from a prior
          // 401, or as the `{ success: false }` payload seeded by the
          // SSR pre-hydration. Invalidating in that case turns the
          // SSR-seeded cache into a wasted client refetch that races
          // with the very auth-gated buttons the seed was meant to
          // make safe.
          if (!profileAlreadyKnowsAnonymous(browserQueryClient)) {
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
        if (!profileAlreadyKnowsAnonymous(browserQueryClient)) {
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
    // Featured slots (admin-curated /explore picks). One list query
    // covers both slot types — list invalidations refresh both
    // panels after Set / Retire.
    featuredSlots: (params?: Record<string, unknown>) =>
      ['admin', 'featuredSlots', params] as const,
    // Streaming-discovery triage worklist. Status filter + limit /
    // offset are part of the key so the status filter and pagination
    // produce independent cache entries; status mutations invalidate
    // the whole streamingWorklist branch.
    streamingWorklist: (params?: Record<string, unknown>) =>
      ['admin', 'streamingWorklist', params] as const,
  },

  // Artist queries (defined in features/artists/api.ts)
  artists: artistQueryKeys,

  // Release queries (defined in features/releases/api.ts)
  releases: releaseQueryKeys,

  // Label queries (defined in features/labels/api.ts)
  labels: labelQueryKeys,

  // Festival queries (defined in features/festivals/api.ts)
  festivals: festivalQueryKeys,

  // Radio queries (defined in features/radio/api.ts)
  radio: radioQueryKeys,

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

  // Collection queries
  collections: {
    all: ['collections'] as const,
    list: (params?: Record<string, unknown>) => ['collections', 'list', params] as const,
    detail: (slug: string) => ['collections', 'detail', slug] as const,
    stats: (slug: string) => ['collections', 'stats', slug] as const,
    // PSY-366: artist-relationship subgraph for the collection's artist items.
    graph: (slug: string, types?: string[]) => ['collections', 'graph', slug, types ?? null] as const,
    // Bare prefix used by mutation invalidations — TanStack matches every
    // descendant query (myList(...) variants below) under this prefix.
    my: ['collections', 'my'] as const,
    /**
     * PSY-580: parameterized "Yours tab" key. Pass `{ search }` to scope a
     * query to a specific search term so loading + cached results don't
     * bleed across distinct searches. Bare invocation (no params) lands at
     * the same key as `my`, so existing callers that didn't search continue
     * to share that cache entry.
     */
    myList: (params?: { search?: string }) =>
      params && Object.values(params).some((v) => v != null && v !== '')
        ? (['collections', 'my', params] as const)
        : (['collections', 'my'] as const),
    // PSY-359: which of the user's own collections already contain a given
    // entity. Drives the pre-check state on the multi-select Add-to-Collection
    // popover. Cached per (entityType, entityId) so each entity page has its
    // own answer and the popover opens instantly on revisit.
    containing: (entityType: string, entityId: number) =>
      ['collections', 'containing', entityType, entityId] as const,
    entity: (entityType: string, entityId: number) =>
      ['collections', 'entity', entityType, entityId] as const,
    userPublic: (username: string) =>
      ['collections', 'userPublic', username] as const,
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
    search: (query: string, category?: string) => ['tags', 'search', query.toLowerCase(), category ?? ''] as const,
    detail: (idOrSlug: string | number) => ['tags', 'detail', String(idOrSlug)] as const,
    enrichedDetail: (idOrSlug: string | number) => ['tags', 'detail', 'enriched', String(idOrSlug)] as const,
    aliases: (tagId: number) => ['tags', 'aliases', tagId] as const,
    allAliases: (params?: Record<string, unknown>) => ['tags', 'aliases', 'all', params] as const,
    lowQuality: (params?: Record<string, unknown>) => ['tags', 'low-quality', params] as const,
    genreHierarchy: ['tags', 'hierarchy', 'genre'] as const,
    entityTags: (entityType: string, entityId: number) => ['tags', 'entityTags', entityType, entityId] as const,
    tagEntities: (idOrSlug: string | number, params?: Record<string, unknown>) => ['tags', 'tagEntities', String(idOrSlug), params] as const,
    // Cross-entity tag intersection (PSY-995 / PSY-993 detail sections). Keyed on
    // the normalized (sorted) slug set + match so shoegaze,ambient and
    // ambient,shoegaze share a cache entry (intersection is symmetric).
    intersection: (slugs: string[], match: string, previewLimit?: number) =>
      ['tags', 'intersection', [...slugs].sort().join(','), match, previewLimit ?? null] as const,
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
    graph: (slug: string, types?: string[]) => ['scenes', 'graph', slug, types ?? null] as const,
  },

  // Community queries (public)
  community: {
    leaderboard: (dimension: string, period: string, limit?: number) =>
      ['community', 'leaderboard', dimension, period, limit] as const,
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

  // /explore landing read endpoints (PSY-835/836/837)
  explore: {
    featured: ['explore', 'featured'] as const,
    upcomingShows: (params?: {
      limit?: number
      offset?: number
      cities?: Array<{ city: string; state: string }>
    }) =>
      params && Object.values(params).some(v => v != null)
        ? (['explore', 'upcomingShows', params] as const)
        : (['explore', 'upcomingShows'] as const),
    shuffleTarget: ['explore', 'shuffleTarget'] as const,
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
