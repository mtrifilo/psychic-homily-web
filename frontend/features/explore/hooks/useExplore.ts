'use client'

/**
 * Explore feature hooks (PSY-837)
 *
 * TanStack Query hooks for /explore read endpoints. Upcoming shows is a
 * page-load read; shuffle-target is interaction-driven via useShuffleTarget.
 * Featured Bill/Collection editorial slots were retired in PSY-1480.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { buildCitiesParam, type CityState } from '@/components/filters'
import type { ExploreUpcomingShowsResponse } from '../types'

export { useRandomArtistTarget as useShuffleTarget } from '@/features/discovery/useRandomArtistTarget'

interface UseExploreUpcomingShowsOptions {
  limit?: number
  offset?: number
  /** Multi-city filter (PSY-840). Omitted/empty ⇒ all cities. */
  cities?: CityState[]
}

/**
 * Upcoming shows for the /explore landing — chronological
 * (event_date ASC, id ASC), deterministic pagination, no algorithmic
 * ranking. Default limit matches the page section design (5 rows).
 *
 * The cities filter is included in the query key (and request) ONLY when
 * non-empty, so the unfiltered default key still matches the page-level
 * SSR prefetch (`{ limit }`) and resolves from the seeded cache without a
 * client refetch.
 */
export function useExploreUpcomingShows(
  options: UseExploreUpcomingShowsOptions = {},
) {
  const { limit, offset, cities } = options
  const hasCities = cities != null && cities.length > 0

  const params = new URLSearchParams()
  if (limit != null) params.set('limit', String(limit))
  if (offset != null) params.set('offset', String(offset))
  if (hasCities) params.set('cities', buildCitiesParam(cities))
  const queryString = params.toString()
  const endpoint = queryString
    ? `${API_ENDPOINTS.EXPLORE.UPCOMING_SHOWS}?${queryString}`
    : API_ENDPOINTS.EXPLORE.UPCOMING_SHOWS

  return useQuery({
    queryKey: queryKeys.explore.upcomingShows(
      hasCities ? { limit, offset, cities } : { limit, offset },
    ),
    queryFn: () =>
      apiRequest<ExploreUpcomingShowsResponse>(endpoint, { method: 'GET' }),
    staleTime: 60 * 1000,
    // Keep the prior city's rows visible (dimmed by the consumer) while a
    // new city filter fetches, instead of flashing the full-area spinner —
    // matches ShowList's dim-in-place behavior.
    placeholderData: keepPreviousData,
  })
}
