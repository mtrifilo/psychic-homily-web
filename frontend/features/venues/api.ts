/**
 * Venues API Configuration
 *
 * Co-located endpoint definitions and query keys for the venues feature.
 * Imported by venue hooks and re-exported from lib/api.ts and lib/queryClient.ts
 * for backward compatibility.
 */

import { API_BASE_URL } from '@/lib/api-base'

// ============================================================================
// Endpoints
// ============================================================================

export const venueEndpoints = {
  LIST: `${API_BASE_URL}/venues`,
  CITIES: `${API_BASE_URL}/venues/cities`,
  SEARCH: `${API_BASE_URL}/venues/search`,
  GET: (venueIdOrSlug: string | number) => `${API_BASE_URL}/venues/${venueIdOrSlug}`,
  SHOWS: (venueIdOrSlug: string | number) => `${API_BASE_URL}/venues/${venueIdOrSlug}/shows`,
  GENRES: (venueIdOrSlug: string | number) => `${API_BASE_URL}/venues/${venueIdOrSlug}/genres`,
  UPDATE: (venueIdOrSlug: string | number) => `${API_BASE_URL}/venues/${venueIdOrSlug}`,
  DELETE: (venueIdOrSlug: string | number) => `${API_BASE_URL}/venues/${venueIdOrSlug}`,
} as const

// ============================================================================
// Query Keys
// ============================================================================

export const venueQueryKeys = {
  all: ['venues'] as const,
  list: (filters?: Record<string, unknown>) =>
    ['venues', 'list', filters] as const,
  cities: ['venues', 'cities'] as const,
  detail: (idOrSlug: string | number) => ['venues', 'detail', String(idOrSlug)] as const,
  search: (query: string) =>
    ['venues', 'search', query.toLowerCase()] as const,
  shows: (venueIdOrSlug: string | number) => ['venues', 'shows', String(venueIdOrSlug)] as const,
  genres: (venueIdOrSlug: string | number) => ['venues', 'genres', String(venueIdOrSlug)] as const,
} as const
