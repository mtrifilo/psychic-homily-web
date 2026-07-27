/**
 * Artists API Configuration
 *
 * Co-located endpoint definitions and query keys for the artists feature.
 * Imported by artist hooks and re-exported from lib/api.ts and lib/queryClient.ts
 * for backward compatibility.
 */

import { API_BASE_URL } from '@/lib/api-base'

// ============================================================================
// Endpoints
// ============================================================================

export const artistEndpoints = {
  LIST: `${API_BASE_URL}/artists`,
  CITIES: `${API_BASE_URL}/artists/cities`,
  SEARCH: `${API_BASE_URL}/artists/search`,
  GET: (artistIdOrSlug: string | number) => `${API_BASE_URL}/artists/${artistIdOrSlug}`,
  SHOWS: (artistIdOrSlug: string | number) => `${API_BASE_URL}/artists/${artistIdOrSlug}/shows`,
  LABELS: (artistIdOrSlug: string | number) => `${API_BASE_URL}/artists/${artistIdOrSlug}/labels`,
  ALIASES: (artistId: string | number) => `${API_BASE_URL}/artists/${artistId}/aliases`,
  GRAPH: (artistId: string | number) => `${API_BASE_URL}/artists/${artistId}/graph`,
  GRAPH_CARD: (artistId: string | number) => `${API_BASE_URL}/artists/${artistId}/graph-card`,
  BILL_COMPOSITION: (artistId: string | number) => `${API_BASE_URL}/artists/${artistId}/bill-composition`,
  RELATED: (artistId: string | number) => `${API_BASE_URL}/artists/${artistId}/related`,
  RELATIONSHIPS: {
    CREATE: `${API_BASE_URL}/artists/relationships`,
    VOTE: (sourceId: number, targetId: number) =>
      `${API_BASE_URL}/artists/relationships/${sourceId}/${targetId}/vote`,
  },
  DELETE: (artistId: string | number) => `${API_BASE_URL}/artists/${artistId}`,
  REPORT: (artistId: string | number) =>
    `${API_BASE_URL}/artists/${artistId}/report`,
  MY_REPORT: (artistId: string | number) =>
    `${API_BASE_URL}/artists/${artistId}/my-report`,
} as const

// ============================================================================
// Shared artist-shows request shape
// ============================================================================

/**
 * The page size EVERY artist-shows caller must use.
 *
 * `artistQueryKeys.shows()` keys on artist id + time filter and deliberately
 * not on limit or timezone, so two surfaces requesting the same artist's
 * upcoming shows share one cache entry. That is only safe while they request
 * the same page: a caller that quietly asked for 2 would hand the ARTIST PAGE
 * a two-row list — silently hiding real upcoming shows for the 5-minute
 * staleTime — or be handed fifty rows itself, depending purely on which
 * request landed first. One constant makes the agreement structural instead of
 * a comment in two files that can drift apart.
 *
 * Exactly the rule `VENUE_SHOWS_PAGE_LIMIT` states for the venue side; a
 * surface that wants fewer rows fetches this page and slices for display (the
 * Atlas artist drill-in shows two of them).
 */
export const ARTIST_SHOWS_PAGE_LIMIT = 50

/**
 * The timezone every artist-shows caller must send, for the same reason.
 *
 * It only sets the backend's "today" boundary for the upcoming/past split —
 * rendering is always done in the VENUE's zone (PSY-985/986), never this one.
 *
 * Evaluated at import, and this module reaches the server graph (via
 * `lib/queryClient.ts`), so on the server this resolves to the SERVER's zone.
 * That is inert rather than a hydration hazard: it is not part of
 * `artistQueryKeys.shows()`, and no route server-prefetches artist shows.
 */
export const ARTIST_SHOWS_VIEWER_TIMEZONE =
  typeof Intl !== 'undefined'
    ? Intl.DateTimeFormat().resolvedOptions().timeZone
    : 'UTC'

// ============================================================================
// Query Keys
// ============================================================================

export const artistQueryKeys = {
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
  graph: (idOrSlug: string | number, types?: string[]) =>
    ['artists', 'graph', String(idOrSlug), types] as const,
  graphCard: (idOrSlug: string | number) =>
    ['artists', 'graphCard', String(idOrSlug)] as const,
  billComposition: (idOrSlug: string | number, months: number) =>
    ['artists', 'billComposition', String(idOrSlug), months] as const,
} as const
