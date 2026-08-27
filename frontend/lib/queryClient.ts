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

import {
  QueryClient,
  DefaultOptions,
  QueryCache,
  MutationCache,
} from '@tanstack/react-query'
import { AuthError, AuthErrorCode } from './errors'
import type { ApiError } from './api'
import {
  isRateLimitError,
  queryRetryDelay,
  shouldRetryQuery,
} from './query-retry-policy'
import { reportRateLimitExhausted } from './rate-limit-telemetry'
import { artistQueryKeys } from '@/features/artists/api'
import { venueQueryKeys } from '@/features/venues/api'
import { showQueryKeys } from '@/features/shows/api'
import { releaseQueryKeys } from '@/features/releases/api'
import { labelQueryKeys } from '@/features/labels/api'
import { festivalQueryKeys } from '@/features/festivals/api'
import { radioQueryKeys } from '@/features/radio/api'
import { chartQueryKeys } from '@/features/charts/api'
import { commentQueryKeys } from '@/features/comments/api'

// Default query options for all queries
const defaultQueryOptions: DefaultOptions = {
  queries: {
    // Stale time: how long data is considered fresh
    staleTime: 15 * 60 * 1000, // 15 minutes
    // Cache time: how long data stays in cache after last use
    gcTime: 30 * 60 * 1000, // 30 minutes (formerly cacheTime)
    // Retry configuration, including the delay curve, lives in
    // ./query-retry-policy. 429 is what forced it out of here: it is the one
    // 4xx that means "ask again later", and treating it as terminal made a
    // transient rate limit a permanently dead page block.
    retry: shouldRetryQuery,
    retryDelay: queryRetryDelay,
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

/**
 * The leading, non-parameter part of a query key, as a stable family label for
 * telemetry ("artists/releases", "collections/detail").
 *
 * Only the first two segments, and only primitives. Deeper segments and the
 * params bag most key factories append carry search terms, filter values and
 * user ids, none of which belong in an error report. Dropping non-primitives
 * rather than serializing them is the point: a stringified params object is
 * exactly the leak this avoids.
 */
function queryFamilyLabel(queryKey: readonly unknown[]): string {
  return queryKey
    .slice(0, 2)
    .filter(
      (segment): segment is string | number =>
        typeof segment === 'string' || typeof segment === 'number'
    )
    .join('/')
}

/**
 * Report a query that died on a 429 after exhausting its retry budget.
 *
 * A query only reaches the error state once `retry` has given up, so a 429
 * arriving here is one that outlived the whole budget: the case the user sees
 * as a dead block. This is the only place that distinction is observable. The
 * fetch boundary in `lib/api.ts` sees every 429 but cannot know which ones
 * went on to recover.
 *
 * Mutations are deliberately not paired with this. They never retry (see
 * `defaultQueryOptions.mutations`), so "exhausted" would be a misnomer there,
 * and the fetch-boundary hit signal already covers them.
 *
 * Browser only, for the same reason. On the server `shouldRetryQuery` makes a
 * 429 terminal by design, so it would arrive here on its FIRST failure and
 * report "retries exhausted" at `error` level having attempted none. That is
 * the loudest signal in the system firing for the case the policy explicitly
 * classifies as harmless (the browser refetches on mount). It would also be
 * the common case if it fired at all: the public-read limiter keys on client
 * IP, and every SSR render from one serverless instance shares an egress IP.
 * The fetch-boundary warning still covers server 429s.
 */
function reportExhaustedRateLimit(
  error: unknown,
  query: { queryKey: readonly unknown[]; state: { fetchFailureCount: number } }
): void {
  if (typeof window === 'undefined') return
  if (!isRateLimitError(error)) return
  // "Retries exhausted" has to mean a retry actually happened. A query that
  // passes its own `retry` opts out of the shared policy entirely, and six do
  // (`useContributorProfile` x3, `useCommentDeepLink`, and the two graph
  // hooks), so without this they reach here on their FIRST 429 and report an
  // error-level exhaustion having attempted nothing. `fetchFailureCount` is
  // the total attempt count, so 1 means the original request and no more.
  if (query.state.fetchFailureCount <= 1) return

  const apiError = error as ApiError
  reportRateLimitExhausted({
    queryFamily: queryFamilyLabel(query.queryKey),
    // Already the TOTAL attempt count, not the retry count: the terminal
    // `error` action increments `fetchFailureCount` on top of the per-retry
    // `failed` actions, and it is dispatched before this handler runs.
    // Verified against query-core's reducer, because the name reads like it
    // should need a +1 and it does not.
    attempts: query.state.fetchFailureCount,
    retryAfter: apiError.retryAfter,
    requestId: apiError.requestId,
  })
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
      reportExhaustedRateLimit(error, query)

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
            browserQueryClient?.invalidateQueries({
              queryKey: queryKeys.auth.profile,
            })
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
          browserQueryClient?.invalidateQueries({
            queryKey: queryKeys.auth.profile,
          })
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

// Shared by queryKeys.savedShows.countBatch and its prefix, so the optimistic
// update in useSaveShowToggle patches exactly the keys the query writes.
const SAVED_SHOWS_COUNT_BATCH_PREFIX = ['savedShows', 'countBatch'] as const

// Prefixes shared between a key factory below and VIEWER_TIER_QUERY_KEYS, for
// the same reason SAVED_SHOWS_COUNT_BATCH_PREFIX exists: these families are
// invalidated by prefix on an auth change, and a rename on only one side would
// silently stop matching and quietly restore the stale-payload bug.
const ENTITY_TAGS_PREFIX = ['tags', 'entityTags'] as const
const CONTRIBUTOR_PREFIX = ['contributor'] as const
const LEADERBOARD_PREFIX = ['community', 'leaderboard'] as const
const CONTRIBUTE_OPPORTUNITIES_PREFIX = ['contribute', 'opportunities'] as const

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
        [
          'admin',
          'dataQuality',
          'category',
          category,
          { limit, offset },
        ] as const,
    },
    analytics: {
      growth: (months: number) =>
        ['admin', 'analytics', 'growth', months] as const,
      engagement: (months: number) =>
        ['admin', 'analytics', 'engagement', months] as const,
      community: ['admin', 'analytics', 'community'] as const,
      dataQualityTrends: (months: number) =>
        ['admin', 'analytics', 'data-quality', months] as const,
    },
    pendingEdits: (params?: Record<string, unknown>) =>
      ['admin', 'pendingEdits', params] as const,
    entityReports: (params?: Record<string, unknown>) =>
      ['admin', 'entityReports', params] as const,
    // PSY-871: queued entity-creation requests (the moderation queue's 4th type).
    entityRequests: (params?: Record<string, unknown>) =>
      ['admin', 'entityRequests', params] as const,
    // Streaming-discovery triage worklist. Status filter + limit /
    // offset are part of the key so the status filter and pagination
    // produce independent cache entries; status mutations invalidate
    // the whole streamingWorklist branch.
    streamingWorklist: (params?: Record<string, unknown>) =>
      ['admin', 'streamingWorklist', params] as const,
    // Bulk-backfill music-link suggestion review queue (PSY-1207). Limit /
    // offset are part of the key so pagination produces independent cache
    // entries; an accept/reject invalidates the whole linkSuggestions branch
    // so every cached page refetches and the reviewed row drops out.
    linkSuggestions: (params?: Record<string, unknown>) =>
      ['admin', 'linkSuggestions', params] as const,
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
    listPrefix: (userId?: string | number) =>
      ['savedShows', 'list', userId ?? null] as const,
    list: (
      userId?: string,
      limit: number = 50,
      offset: number = 0,
      timeFilter?: 'upcoming' | 'past'
    ) =>
      [
        'savedShows',
        'list',
        userId ?? null,
        { limit, offset, timeFilter },
      ] as const,
    infiniteList: (
      userId: number | undefined,
      timeFilter: 'upcoming' | 'past'
    ) => ['savedShows', 'infiniteList', userId ?? null, timeFilter] as const,
    infiniteListPrefix: (userId: number | undefined) =>
      ['savedShows', 'infiniteList', userId ?? null] as const,
    // Public save counts. `isAuthenticated` is part of BOTH keys because the
    // same endpoint returns is_saved only for authenticated callers — without it
    // an anonymous cache entry would survive login and report is_saved: false
    // for a show the user had already saved.
    count: (
      showId: number,
      isAuthenticated: boolean,
      userId?: string | number
    ) =>
      ['savedShows', 'count', isAuthenticated, userId ?? null, showId] as const,
    // Prefix, exported so the optimistic-update path can patch every cached
    // batch without re-typing the key segments (a rename here would otherwise
    // silently stop matching).
    countBatchPrefix: (userId?: string | number) =>
      [...SAVED_SHOWS_COUNT_BATCH_PREFIX, true, userId ?? null] as const,
    countBatch: (
      showIds: number[],
      isAuthenticated: boolean,
      userId?: string | number
    ) =>
      [
        ...SAVED_SHOWS_COUNT_BATCH_PREFIX,
        isAuthenticated,
        userId ?? null,
        showIds,
      ] as const,
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

  // Artist reports queries. PSY-1633: only the caller's own read-back
  // remains — the admin queue is /admin/entity-reports (adminEntityReports).
  artistReports: {
    all: ['artistReports'] as const,
    myReport: (artistId: string | number) =>
      ['artistReports', 'myReport', String(artistId)] as const,
  },

  // Contributor profile queries
  contributor: {
    all: CONTRIBUTOR_PREFIX,
    profile: (username: string) =>
      [...CONTRIBUTOR_PREFIX, 'profile', username] as const,
    ownProfile: ['contributor', 'ownProfile'] as const,
    // PSY-1087: next-tier advancement progress for the profile card.
    advancement: ['contributor', 'advancement'] as const,
    contributions: (username: string) =>
      ['contributor', 'contributions', username] as const,
    ownContributions: ['contributor', 'ownContributions'] as const,
    sections: (username: string) =>
      ['contributor', 'sections', username] as const,
    ownSections: ['contributor', 'ownSections'] as const,
    activityHeatmap: (username: string) =>
      ['contributor', 'activityHeatmap', username] as const,
    rankings: (username: string) =>
      ['contributor', 'rankings', username] as const,
    following: (username: string, type?: string) =>
      ['contributor', 'following', username, type ?? 'all'] as const,
    // NOTE: limit is deliberately NOT in these keys — the profile sections
    // fetch one fixed page (API max 100) and slice client-side. A second
    // consumer with a different limit would get the first-mounted page
    // size; key on limit if that ever happens (PSY-1062).
    fieldNotes: (username: string) =>
      ['contributor', 'fieldNotes', username] as const,
  },

  // Collection queries
  collections: {
    all: ['collections'] as const,
    list: (params?: Record<string, unknown>) =>
      ['collections', 'list', params] as const,
    detail: (slug: string) => ['collections', 'detail', slug] as const,
    stats: (slug: string) => ['collections', 'stats', slug] as const,
    // PSY-366: artist-relationship subgraph for the collection's artist items.
    graph: (slug: string, types?: string[]) =>
      ['collections', 'graph', slug, types ?? null] as const,
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
      params && Object.values(params).some(v => v != null && v !== '')
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
    // NOTE: limit is deliberately NOT in this key — the profile surfaces
    // fetch one fixed page (API max) and slice client-side. A second
    // consumer with a different limit would be served the first-mounted
    // page size; key on limit if that ever happens (PSY-1062).
    userPublic: (username: string) =>
      ['collections', 'userPublic', username] as const,
  },

  // Request queries
  requests: {
    all: ['requests'] as const,
    list: (params?: Record<string, unknown>) =>
      ['requests', 'list', params] as const,
    detail: (requestId: number) => ['requests', 'detail', requestId] as const,
  },

  // Tag queries
  tags: {
    all: ['tags'] as const,
    list: (params?: Record<string, unknown>) =>
      ['tags', 'list', params] as const,
    search: (query: string, category?: string) =>
      ['tags', 'search', query.toLowerCase(), category ?? ''] as const,
    detail: (idOrSlug: string | number) =>
      ['tags', 'detail', String(idOrSlug)] as const,
    enrichedDetail: (idOrSlug: string | number) =>
      ['tags', 'detail', 'enriched', String(idOrSlug)] as const,
    aliases: (tagId: number) => ['tags', 'aliases', tagId] as const,
    allAliases: (params?: Record<string, unknown>) =>
      ['tags', 'aliases', 'all', params] as const,
    lowQuality: (params?: Record<string, unknown>) =>
      ['tags', 'low-quality', params] as const,
    genreHierarchy: ['tags', 'hierarchy', 'genre'] as const,
    entityTagsAll: ENTITY_TAGS_PREFIX,
    entityTags: (entityType: string, entityId: number) =>
      [...ENTITY_TAGS_PREFIX, entityType, entityId] as const,
    tagEntities: (
      idOrSlug: string | number,
      params?: Record<string, unknown>
    ) => ['tags', 'tagEntities', String(idOrSlug), params] as const,
    // Cross-entity tag intersection (PSY-995 / PSY-993 detail sections). Keyed on
    // the normalized (sorted) slug set + match so shoegaze,ambient and
    // ambient,shoegaze share a cache entry (intersection is symmetric).
    intersection: (slugs: string[], match: string, previewLimit?: number) =>
      [
        'tags',
        'intersection',
        [...slugs].sort().join(','),
        match,
        previewLimit ?? null,
      ] as const,
  },

  // Follow queries
  follows: {
    all: ['follows'] as const,
    // entityId: number for id-keyed entities; a scene SLUG for "scenes"
    // (PSY-1339 — slug-addressed follow routes).
    entity: (
      entityType: string,
      entityId: number | string,
      userId?: string | number
    ) => ['follows', entityType, userId ?? null, entityId] as const,
    batchPrefix: (entityType: string, userId?: string | number) =>
      ['follows', 'batch', entityType, userId ?? null] as const,
    batch: (
      entityType: string,
      entityIds: number[],
      userId?: string | number
    ) =>
      ['follows', 'batch', entityType, userId ?? null, ...entityIds] as const,
    myFollowing: (params?: Record<string, unknown>) =>
      ['follows', 'my-following', params] as const,
    libraryCounts: (userId?: string | number) =>
      ['follows', 'library', 'counts', userId ?? null] as const,
    libraryFollowing: (entityType: string, userId?: string | number) =>
      ['follows', 'library', 'following', userId ?? null, entityType] as const,
    followers: (entityType: string, entityId: number) =>
      ['follows', 'followers', entityType, entityId] as const,
    // Username-addressed user→user follow status (GET /users/{username}/followers).
    user: (username: string, userId?: string | number) =>
      ['follows', 'user', userId ?? null, username] as const,
  },

  // Scene queries
  scenes: {
    all: ['scenes'] as const,
    list: ['scenes', 'list'] as const,
    detail: (slug: string) => ['scenes', 'detail', slug] as const,
    artists: (slug: string, period?: number, limit?: number) =>
      ['scenes', 'artists', slug, period, limit] as const,
    // limit is part of the key for the same reason it is on `artists` and
    // `shows`: the cap changes WHICH bands come back, so two callers asking
    // different questions must not share one entry. There is no `days` — the
    // endpoint stopped being a window in PSY-1844.
    newArtists: (slug: string, limit?: number) =>
      ['scenes', 'newArtists', slug, limit] as const,
    // days/limit are part of the key: the Atlas preview's 7-day/3-row peek and
    // the scene page's 4-week/20-row calendar are different results from the
    // same path, and sharing one entry would paint whichever arrived first
    // (the PSY-1109 key-drift class).
    shows: (slug: string, days?: number, limit?: number) =>
      ['scenes', 'shows', slug, days, limit] as const,
    // clusterBy is the literal union (not string) so a drifted value at an
    // invalidation/prefetch site is a compile error, not a silent key
    // mismatch (the PSY-1109 key-drift class).
    graph: (
      slug: string,
      types?: string[],
      clusterBy?: 'venue' | 'community'
    ) =>
      ['scenes', 'graph', slug, types ?? null, clusterBy ?? 'venue'] as const,
  },

  // Community queries. The leaderboard route is optional-auth: it appends the
  // caller's own `user_rank`, so it is viewer-tier dependent despite the rest
  // of this group being public.
  community: {
    leaderboardAll: LEADERBOARD_PREFIX,
    leaderboard: (dimension: string, period: string, limit?: number) =>
      [...LEADERBOARD_PREFIX, dimension, period, limit] as const,
  },

  // Contribute worklist (PSY-1857: the `followed_artists_missing_links`
  // category is served only to signed-in viewers, and item order is per-viewer)
  contribute: {
    opportunities: CONTRIBUTE_OPPORTUNITIES_PREFIX,
    category: (category: string) =>
      [...CONTRIBUTE_OPPORTUNITIES_PREFIX, category] as const,
  },

  // Charts queries (public)
  charts: chartQueryKeys,

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
    upcomingShows: (params?: {
      limit?: number
      offset?: number
      cities?: Array<{ city: string; state: string }>
    }) =>
      params && Object.values(params).some(v => v != null)
        ? (['explore', 'upcomingShows', params] as const)
        : (['explore', 'upcomingShows'] as const),
  },

  discovery: {
    randomArtistTarget: ['discovery', 'randomArtistTarget'] as const,
  },

  // Passkey credential list (settings → security). One list read; refetched
  // after register / delete via its own key (no parent-prefix family needed
  // while there's a single list).
  passkeys: {
    credentials: ['passkeys', 'credentials'] as const,
  },

  // Bandcamp embed resolution (PSY-1102). MusicEmbed renders one instance
  // per show/artist card, so list pages mount many copies pointing at the
  // same album URL. Keying on the album URL dedups the `/api/bandcamp/album-id`
  // resolve across every instance and caches the result across nav/remount.
  bandcamp: {
    embed: (albumUrl: string) => ['bandcamp', 'embed', albumUrl] as const,
  },

  // System queries
  system: {
    health: ['system', 'health'] as const,
  },
} as const

/**
 * Query families whose PAYLOAD varies with the caller's privilege tier while
 * their cache key carries no viewer dimension. For these, one key means
 * different data before and after an auth change, so a cached entry outlives
 * the viewer it was fetched for: an admin who opened a panel while signed out
 * keeps the anonymous answer for the whole 15-minute `staleTime` above.
 *
 * Every entry below was checked against the backend route it reads. The
 * families that are DELIBERATELY absent, and why, are recorded here too so
 * the next person does not have to redo the audit:
 *
 *   - `savedShows.count` / `savedShows.countBatch`, `follows.entity`,
 *     `follows.batch`, `follows.libraryCounts`, `follows.libraryFollowing`,
 *     `follows.user`: already carry a viewer segment (`isAuthenticated` plus
 *     user id), so an auth change moves them to a different key and the
 *     previous viewer's entry can never be served in the new viewer's place.
 *   - `follows.followers` is the one follow key with NO viewer segment, and it
 *     is served by an optional-auth route. It is absent for a different
 *     reason: it has no consumer anywhere in the frontend, so it is dead key
 *     surface rather than a live cache. Add it here the moment something
 *     mounts it.
 *   - `shows.detail`: the only viewer-dependent behaviour is a 404 on a
 *     non-approved show for viewers who are neither admin nor the submitter.
 *     On a show page the viewer can already see, the bytes are identical.
 *   - `artists.graph`: `/artists/{id}/graph` IS an optional-auth route and its
 *     payload carries a `user_votes` map, but nothing in the UI reads it. Add
 *     the key here if that changes. Its siblings `/artists/{id}/related` (no
 *     consumer today), `/bill-composition` and `/relationships/{id}/provenance`
 *     are also optional-auth but read no viewer.
 *   - `field-notes`: `GET /shows/{id}/field-notes` reads no viewer at all, so
 *     the `user_vote` its comment-shaped rows can carry is never populated.
 *   - `admin.*`, `contributor.own*`, `collections.my`, personal `charts`,
 *     `passkeys`, `mySubmissions`, `calendar`, `savedShows.list`: signed-in
 *     only, with no anonymous variant to be confused with. Logout drops them
 *     wholesale via `queryClient.clear()`.
 *   - `tags` list/detail/search/entities, public `charts`, and the venue /
 *     release / label / festival / scene / radio / graph / explore families:
 *     served by routes registered without auth middleware, so the backend
 *     cannot see a viewer and the payload cannot vary.
 */
const VIEWER_TIER_QUERY_KEYS: readonly (readonly unknown[])[] = [
  // Revision history. Non-privileged viewers get the stored `address` and
  // `zipcode` values replaced with "(hidden)" on unverified venues and lose
  // the summary entirely (PSY-1717), and a non-approved show 404s its history
  // outright (PSY-1715) — so a tier flip changes both the values shown and
  // which rows exist at all. The family prefix also covers the
  // `{ limit, offset }`-suffixed entity/user keys and the
  // `{ attribution: true }` key that useEntityAttribution appends.
  queryKeys.revisions.all,
  // Every comment node carries the caller's own `user_vote`, so an anonymous
  // payload reports "not voted" on threads the viewer has voted in.
  commentQueryKeys.all,
  // Entity tags carry `user_vote` the same way. Scoped to the `entityTags`
  // branch on purpose: the rest of the `tags` family is served by unauthed
  // routes and would only be refetched for nothing.
  queryKeys.tags.entityTagsAll,
  // Collections vary the most: a private collection 403s for anyone but its
  // creator, `user_likes_this` and `is_subscribed` are per-viewer, the
  // entity-collections list unions in the viewer's own private collections,
  // and nested tags carry `user_vote`.
  queryKeys.collections.all,
  // Contributor profiles gate per-field on owner-versus-not: stats, last
  // active, sections, contributions, rankings and following each collapse to
  // a count or vanish for a non-owner, and a private profile 404s.
  queryKeys.contributor.all,
  // Entity requests carry the caller's `user_vote` per row.
  queryKeys.requests.all,
  // The leaderboard appends the caller's own `user_rank`.
  queryKeys.community.leaderboardAll,
  // The contribute worklist hides the `followed_artists_missing_links`
  // category from anonymous viewers, which also changes `total_items`, and
  // orders items per-viewer.
  queryKeys.contribute.opportunities,
]

/**
 * Refresh every viewer-tier-dependent cache after the viewer GAINS privilege
 * (login, registration, magic-link sign-in, email verification, account
 * recovery).
 *
 * `invalidateQueries` rather than `resetQueries`: on the way UP the cached
 * payload is a subset of what the new viewer may see, so keeping it painted
 * during the refetch is a seamless upgrade rather than a loading flash.
 *
 * Safe to leave unawaited: `invalidateQueries` swallows per-query fetch
 * errors into query state and resolves rather than rejecting, so a floating
 * call here cannot produce an unhandled rejection.
 *
 * See `resetViewerTierQueries` for the other direction.
 */
export function refreshViewerTierQueries(queryClient: QueryClient) {
  return Promise.all(
    VIEWER_TIER_QUERY_KEYS.map(queryKey =>
      queryClient.invalidateQueries({ queryKey })
    )
  )
}

/**
 * Discard every viewer-tier-dependent cache after the viewer LOSES privilege
 * (logout).
 *
 * `resetQueries` rather than `invalidateQueries`: on the way DOWN the cached
 * payload is data the new viewer must not see, so it has to leave the screen
 * rather than keep being painted until an anonymous refetch lands.
 *
 * ORDER MATTERS, and it is the opposite of what it looks like. This must run
 * BEFORE `queryClient.clear()` in `useLogout`, not after. `clear()` empties
 * the cache but does not notify query OBSERVERS: it destroys each query and
 * notifies cache-level listeners, while a `useQuery` observer is subscribed to
 * the query itself. So a still-mounted observer goes on rendering the payload
 * it last saw, and a reset issued after the clear matches an empty cache and
 * does nothing at all. Running the reset first pushes the observer into a
 * pending state, and that notification is what re-renders the component and
 * rebuilds its query once `clear()` has destroyed the old one.
 *
 * The `clear()` cannot be relied on to do this by itself. It only self-heals
 * where some ANCESTOR of the panel consumes `useAuthContext` and re-renders on
 * the logout state change (VenueDetail, ShowDetail). Components that read auth
 * through `useIsAuthenticated` instead — ReleaseDetail and LabelDetail, both of
 * which render `<RevisionHistory>` — hold a `useProfile` query observer, which
 * `clear()` orphans in exactly the same way, so nothing re-renders them and the
 * privileged rows stay on screen for as long as the page is mounted.
 */
export function resetViewerTierQueries(queryClient: QueryClient) {
  return Promise.all(
    VIEWER_TIER_QUERY_KEYS.map(queryKey => queryClient.resetQueries({ queryKey }))
  )
}

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
  calendar: () => queryClient.invalidateQueries({ queryKey: ['calendar'] }),

  // Invalidate saved shows queries
  savedShows: () => queryClient.invalidateQueries({ queryKey: ['savedShows'] }),

  // Invalidate user's submissions queries
  mySubmissions: () =>
    queryClient.invalidateQueries({ queryKey: ['mySubmissions'] }),

  // Invalidate show reports queries
  showReports: () =>
    queryClient.invalidateQueries({ queryKey: ['showReports'] }),

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

  // PSY-871: invalidate admin entity-request (moderation) queries
  adminEntityRequests: () =>
    queryClient.invalidateQueries({ queryKey: ['admin', 'entityRequests'] }),

  // Invalidate contributor profile queries
  contributor: () =>
    queryClient.invalidateQueries({ queryKey: ['contributor'] }),

  // Invalidate own contributor profile queries
  ownContributor: () =>
    Promise.all([
      queryClient.invalidateQueries({
        queryKey: ['contributor', 'ownProfile'],
      }),
      queryClient.invalidateQueries({
        queryKey: ['contributor', 'ownSections'],
      }),
      queryClient.invalidateQueries({
        queryKey: ['contributor', 'ownContributions'],
      }),
    ]),

  // Invalidate collection queries
  collections: () =>
    queryClient.invalidateQueries({ queryKey: ['collections'] }),

  // Invalidate request queries
  requests: () => queryClient.invalidateQueries({ queryKey: ['requests'] }),

  // Invalidate tag queries
  tags: () => queryClient.invalidateQueries({ queryKey: ['tags'] }),

  // Invalidate entity tag queries
  entityTags: (entityType: string, entityId: number) =>
    queryClient.invalidateQueries({
      queryKey: ['tags', 'entityTags', entityType, entityId],
    }),

  // Invalidate follow queries
  follows: () => queryClient.invalidateQueries({ queryKey: ['follows'] }),

  // Invalidate the authenticated user's /charts summary after a contributing
  // save/follow mutation. Public chart modules use sibling keys and stay cached.
  personalCharts: async () => {
    // A plain invalidation cannot restart an active query whose initial fetch
    // is still pending. Cancel first so a mutation cannot be overwritten by a
    // pre-mutation /charts/me snapshot that resolves afterward.
    await queryClient.cancelQueries({ queryKey: chartQueryKeys.personalRoot })
    return queryClient.invalidateQueries({
      queryKey: chartQueryKeys.personalRoot,
    })
  },

  // Invalidate scene queries
  scenes: () => queryClient.invalidateQueries({ queryKey: ['scenes'] }),

  // Invalidate revision queries
  revisions: () => queryClient.invalidateQueries({ queryKey: ['revisions'] }),

  // Invalidate notification filter queries
  notificationFilters: () =>
    queryClient.invalidateQueries({ queryKey: ['notificationFilters'] }),
})
