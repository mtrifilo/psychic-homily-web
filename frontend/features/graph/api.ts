/**
 * Graph feature API configuration — co-located endpoints + query keys, the
 * convention every other feature module follows (see features/artists/api.ts).
 */

import { API_BASE_URL } from '@/lib/api-base'

export const graphEndpoints = {
  /** The nightly precomputed "Map of the Scene" (PSY-1723). */
  OVERVIEW: `${API_BASE_URL}/graph/overview`,
  /**
   * Connectivity-ranked artists the zero state suggests as a starting point
   * (PSY-1749).
   *
   * A SEPARATE RESOURCE FROM THE MAP, and that is the point: the hero asks for
   * these exactly when the overview is unavailable, so they cannot live in a
   * payload that is missing in that state.
   */
  STARTING_POINTS: `${API_BASE_URL}/graph/starting-points`,
} as const

/**
 * HTTP status the overview endpoint answers with before the first nightly build
 * has ever run (a cold database).
 *
 * A legitimate steady state on a fresh install, a dev seed, or a restored
 * database — NOT a failure. The client hook reads it to decide not to retry; the
 * server fetch reads it to decide not to page Sentry. It lives HERE, in the
 * framework-free endpoint module both already import, rather than in either of
 * them: the client hook is `'use client'`, so importing it from the server fetch
 * would drag React Query into an OG edge bundle to learn one number.
 */
export const GRAPH_OVERVIEW_NOT_BUILT_STATUS = 503

export const graphQueryKeys = {
  all: ['graph'] as const,
  overview: () => [...graphQueryKeys.all, 'overview'] as const,
  startingPoints: () => [...graphQueryKeys.all, 'starting-points'] as const,
}
