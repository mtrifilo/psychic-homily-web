/**
 * Festivals API Configuration
 *
 * Co-located endpoint definitions and query keys for the festivals feature.
 * Imported by festival hooks and re-exported from lib/api.ts and lib/queryClient.ts
 * for backward compatibility.
 */

import { API_BASE_URL } from '@/lib/api-base'

// ============================================================================
// Endpoints
// ============================================================================

export const festivalEndpoints = {
  LIST: `${API_BASE_URL}/festivals`,
  SEARCH: `${API_BASE_URL}/festivals/search`,
  GET: (idOrSlug: string | number) => `${API_BASE_URL}/festivals/${idOrSlug}`,
  CREATE: `${API_BASE_URL}/festivals`,
  UPDATE: (festivalId: string | number) =>
    `${API_BASE_URL}/festivals/${festivalId}`,
  DELETE: (festivalId: string | number) =>
    `${API_BASE_URL}/festivals/${festivalId}`,
  ARTISTS: (festivalId: string | number) =>
    `${API_BASE_URL}/festivals/${festivalId}/artists`,
  ADD_ARTIST: (festivalId: string | number) =>
    `${API_BASE_URL}/festivals/${festivalId}/artists`,
  UPDATE_ARTIST: (festivalId: string | number, artistId: string | number) =>
    `${API_BASE_URL}/festivals/${festivalId}/artists/${artistId}`,
  REMOVE_ARTIST: (festivalId: string | number, artistId: string | number) =>
    `${API_BASE_URL}/festivals/${festivalId}/artists/${artistId}`,
  VENUES: (festivalId: string | number) =>
    `${API_BASE_URL}/festivals/${festivalId}/venues`,
  ADD_VENUE: (festivalId: string | number) =>
    `${API_BASE_URL}/festivals/${festivalId}/venues`,
  REMOVE_VENUE: (festivalId: string | number, venueId: string | number) =>
    `${API_BASE_URL}/festivals/${festivalId}/venues/${venueId}`,
  ARTIST_FESTIVALS: (artistIdOrSlug: string | number) =>
    `${API_BASE_URL}/artists/${artistIdOrSlug}/festivals`,
  // Festival intelligence endpoints
  SIMILAR: (festivalId: string | number) =>
    `${API_BASE_URL}/festivals/${festivalId}/similar`,
  OVERLAP: (festivalAId: string | number, festivalBId: string | number) =>
    `${API_BASE_URL}/festivals/${festivalAId}/overlap/${festivalBId}`,
  BREAKOUTS: (festivalId: string | number) =>
    `${API_BASE_URL}/festivals/${festivalId}/breakouts`,
  ARTIST_TRAJECTORY: (artistIdOrSlug: string | number) =>
    `${API_BASE_URL}/artists/${artistIdOrSlug}/festival-trajectory`,
  SERIES_COMPARE: (seriesSlug: string) =>
    `${API_BASE_URL}/festivals/series/${seriesSlug}/compare`,
} as const

// ============================================================================
// Query Keys
// ============================================================================

export const festivalQueryKeys = {
  all: ['festivals'] as const,
  list: (filters?: Record<string, unknown>) =>
    ['festivals', 'list', filters] as const,
  search: (query: string) =>
    ['festivals', 'search', query.toLowerCase()] as const,
  detail: (idOrSlug: string | number) => ['festivals', 'detail', String(idOrSlug)] as const,
  artists: (idOrSlug: string | number, dayDate?: string) =>
    ['festivals', 'artists', String(idOrSlug), dayDate] as const,
  venues: (idOrSlug: string | number) =>
    ['festivals', 'venues', String(idOrSlug)] as const,
  artistFestivals: (artistIdOrSlug: string | number) =>
    ['festivals', 'artist', String(artistIdOrSlug)] as const,
  similar: (idOrSlug: string | number) =>
    ['festivals', 'similar', String(idOrSlug)] as const,
  overlap: (aId: string | number, bId: string | number) =>
    ['festivals', 'overlap', String(aId), String(bId)] as const,
  breakouts: (idOrSlug: string | number) =>
    ['festivals', 'breakouts', String(idOrSlug)] as const,
  artistTrajectory: (artistIdOrSlug: string | number) =>
    ['festivals', 'trajectory', String(artistIdOrSlug)] as const,
  seriesCompare: (seriesSlug: string, years: number[]) =>
    ['festivals', 'series', seriesSlug, years.join(',')] as const,
} as const
