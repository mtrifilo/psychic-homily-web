'use client'

/**
 * Read hook for the connectivity-ranked starting suggestions the /graph zero
 * state offers (PSY-1749).
 *
 * The list replaces a hardcoded editorial trio. It is ranked by the nightly
 * build using the SAME betweenness centrality that tiers the map's labels — so
 * the band the hero suggests is the band the map would have drawn largest — and
 * every entry is resolved against the live catalog server-side before it is
 * returned, which is what makes each suggestion clickable rather than merely
 * plausible.
 */

import { useQuery } from '@tanstack/react-query'

import { apiRequest } from '@/lib/api'
import { graphEndpoints, graphQueryKeys } from '@/features/graph/api'
import type { components } from '@/types/api'

export type GraphStartingPoint = components['schemas']['GraphStartingPoint']

/**
 * NOTE the generated `artists` is nullable: Go marshals a nil slice as `null`,
 * so the schema has to allow it even though this handler never emits one.
 * Callers narrow it rather than trusting the handler.
 */
export type GraphStartingPointsResponse =
  components['schemas']['GraphStartingPointsResponse']

/**
 * Matches the endpoint's own `max-age`, for the same reason the overview hook
 * does: the ranking moves once a night, so a shorter window would spend a
 * request to re-learn an unchanged answer while the browser served it from disk
 * anyway.
 *
 * It does NOT freeze what the visitor sees. The rotation draws a fresh random
 * subset on every mount, so a cached list still produces different names on the
 * next visit — the variation is in the pick, not in the fetch.
 */
const STARTING_POINTS_STALE_TIME = 60 * 60 * 1000

export function useGraphStartingPoints() {
  return useQuery<GraphStartingPointsResponse>({
    queryKey: graphQueryKeys.startingPoints(),
    queryFn: () =>
      apiRequest<GraphStartingPointsResponse>(graphEndpoints.STARTING_POINTS, {
        method: 'GET',
      }),
    staleTime: STARTING_POINTS_STALE_TIME,
    // ONE retry, matching useGraphOverview. The fallback for a failed list is a
    // random catalog artist, which is instant and fully usable, so three rounds
    // of exponential backoff would hold the hero's sentence on a skeleton to
    // reach something that was always available.
    retry: 1,
  })
}
