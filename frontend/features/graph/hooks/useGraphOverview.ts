'use client'

/**
 * Read hook for the nightly "Map of the Scene" snapshot (PSY-1725).
 */

import { useQuery } from '@tanstack/react-query'

import { apiRequest, type ApiError } from '@/lib/api'
import {
  GRAPH_OVERVIEW_NOT_BUILT_STATUS,
  graphEndpoints,
  graphQueryKeys,
} from '@/features/graph/api'
import type { GraphOverview } from '@/features/graph/sceneMap'

/**
 * The snapshot changes exactly once a night and the response is a few hundred
 * KB, so a short staleTime would spend a full payload to learn nothing. An hour
 * matches the `max-age` the endpoint itself sets, keeping the in-memory cache
 * and the HTTP cache on the same clock instead of having React Query ask for a
 * revalidation the browser would answer from disk anyway.
 */
const OVERVIEW_STALE_TIME = 60 * 60 * 1000

/**
 * Re-exported so the surfaces that already import this hook keep one import.
 * The constant itself lives in `features/graph/api.ts`, which the server-side
 * fetch can reach without dragging React Query into an OG edge bundle.
 */
export { GRAPH_OVERVIEW_NOT_BUILT_STATUS }

export function isGraphOverviewNotBuilt(error: unknown): boolean {
  return (error as ApiError | null)?.status === GRAPH_OVERVIEW_NOT_BUILT_STATUS
}

export function useGraphOverview() {
  return useQuery<GraphOverview>({
    queryKey: graphQueryKeys.overview(),
    queryFn: () => apiRequest<GraphOverview>(graphEndpoints.OVERVIEW, { method: 'GET' }),
    staleTime: OVERVIEW_STALE_TIME,
    // "Not built yet" is an answer, not an outage: retrying it burns three
    // round-trips to re-learn a fact that only a nightly job can change, and
    // delays the fallback hero by the backoff. Every other failure keeps the
    // default retry behaviour.
    // ONE retry, not the default three. The fallback for a failed map is the
    // search-first hero, which is instant and fully usable — so three rounds of
    // exponential backoff would keep a visitor looking at a skeleton for
    // several seconds to reach something that was always available. Fail fast
    // and hand them a working surface.
    retry: (failureCount, error) => !isGraphOverviewNotBuilt(error) && failureCount < 1,
  })
}
