'use client'

/**
 * Explore feature hooks (PSY-837)
 *
 * TanStack Query hooks for the three /explore read endpoints. Two are
 * page-load reads (upcoming shows, featured slots) — the route's server
 * component prefetches and seeds the cache via `prefetchEntity` so
 * these hooks resolve from the seeded cache and the client never
 * refetches on first paint.
 *
 * The shuffle-target endpoint is interaction-driven (button click);
 * it's wrapped in `useShuffleTarget` (manually triggered via the
 * returned `refetch`) rather than running on mount.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { buildCitiesParam, type CityState } from '@/components/filters'
import type {
  ExploreFeaturedResponse,
  ExploreShuffleTargetResponse,
  ExploreUpcomingShowsResponse,
} from '../types'

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

/**
 * Admin-curated Featured Bill + Featured Collection. Both fields can
 * independently be null — the consumer collapses the matching section.
 */
export function useExploreFeatured() {
  return useQuery({
    queryKey: queryKeys.explore.featured,
    queryFn: () =>
      apiRequest<ExploreFeaturedResponse>(API_ENDPOINTS.EXPLORE.FEATURED, {
        method: 'GET',
      }),
    staleTime: 60 * 1000,
  })
}

/**
 * Random artist from the ±90-day show pool. Disabled on mount — the
 * shuffle CTA calls `refetch()` then navigates with the returned slug
 * so each click pulls a fresh pick rather than reusing a cached one.
 */
export function useShuffleTarget() {
  return useQuery({
    queryKey: queryKeys.explore.shuffleTarget,
    queryFn: () =>
      apiRequest<ExploreShuffleTargetResponse>(
        API_ENDPOINTS.EXPLORE.SHUFFLE_TARGET,
        { method: 'GET' },
      ),
    enabled: false,
    // staleTime: 0 — every click should hit the backend for a fresh pick.
    staleTime: 0,
    gcTime: 0,
  })
}
