/**
 * Labels API Configuration
 *
 * Co-located endpoint definitions and query keys for the labels feature.
 * Imported by label hooks and re-exported from lib/api.ts and lib/queryClient.ts
 * for backward compatibility.
 */

import { API_BASE_URL } from '@/lib/api-base'

// ============================================================================
// Endpoints
// ============================================================================

export const labelEndpoints = {
  LIST: `${API_BASE_URL}/labels`,
  SEARCH: `${API_BASE_URL}/labels/search`,
  GET: (idOrSlug: string | number) => `${API_BASE_URL}/labels/${idOrSlug}`,
  CREATE: `${API_BASE_URL}/labels`,
  UPDATE: (labelId: string | number) => `${API_BASE_URL}/labels/${labelId}`,
  DELETE: (labelId: string | number) => `${API_BASE_URL}/labels/${labelId}`,
  ARTISTS: (idOrSlug: string | number) =>
    `${API_BASE_URL}/labels/${idOrSlug}/artists`,
  RELEASES: (idOrSlug: string | number) =>
    `${API_BASE_URL}/labels/${idOrSlug}/releases`,
} as const

// ============================================================================
// Query Keys
// ============================================================================

export const labelQueryKeys = {
  all: ['labels'] as const,
  list: (filters?: Record<string, unknown>) =>
    ['labels', 'list', filters] as const,
  search: (query: string) =>
    ['labels', 'search', query.toLowerCase()] as const,
  detail: (idOrSlug: string | number) => ['labels', 'detail', String(idOrSlug)] as const,
  roster: (idOrSlug: string | number) => ['labels', 'roster', String(idOrSlug)] as const,
  catalog: (idOrSlug: string | number) => ['labels', 'catalog', String(idOrSlug)] as const,
} as const
