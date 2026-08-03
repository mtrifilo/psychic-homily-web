/**
 * Graph feature API configuration — co-located endpoints + query keys, the
 * convention every other feature module follows (see features/artists/api.ts).
 */

import { API_BASE_URL } from '@/lib/api-base'

export const graphEndpoints = {
  /** The nightly precomputed "Map of the Scene" (PSY-1723). */
  OVERVIEW: `${API_BASE_URL}/graph/overview`,
} as const

export const graphQueryKeys = {
  all: ['graph'] as const,
  overview: () => [...graphQueryKeys.all, 'overview'] as const,
}
