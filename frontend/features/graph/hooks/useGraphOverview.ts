'use client'

/**
 * Read hook for the nightly "Map of the Scene" snapshot (PSY-1725).
 */

import { useQuery } from '@tanstack/react-query'

import { apiRequest, type ApiError } from '@/lib/api'
import { graphEndpoints, graphQueryKeys } from '@/features/graph/api'
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
 * HTTP status the endpoint answers with before the first nightly build has ever
 * run (a cold database). It is a legitimate steady state on a fresh install or
 * a dev seed, NOT a failure — the surface renders its search-first fallback
 * rather than an error card, and we do not retry into it.
 */
export const GRAPH_OVERVIEW_NOT_BUILT_STATUS = 503

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
