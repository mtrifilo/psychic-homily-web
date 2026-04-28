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
  billComposition: (idOrSlug: string | number, months: number) =>
    ['artists', 'billComposition', String(idOrSlug), months] as const,
} as const
