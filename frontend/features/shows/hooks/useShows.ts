'use client'

/**
 * Shows Hooks
 *
 * TanStack Query hooks for fetching show data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import {
  showEndpoints,
  showQueryKeys,
  SHOW_CITIES_FIRST_SCREEN_URL,
  TRANSITIONAL_TIMEZONE_PARAM,
} from '@/features/shows/api'
import type { UpcomingShowsResponse, ShowResponse, ShowCitiesResponse } from '../types'
import { buildCitiesParam } from '@/components/filters/cityParams'

interface UseUpcomingShowsOptions {
  cursor?: string
  limit?: number
  /** Legacy single-city filter */
  city?: string
  /** Legacy single-state filter */
  state?: string
  /** Multi-city filter (takes priority over city/state) */
  cities?: Array<{ city: string; state: string }>
  /** Multi-tag filter (PSY-309). Slugs applied with AND by default. */
  tags?: string[]
  /** Set to 'any' to switch the tag filter to OR semantics. */
  tagMatch?: 'all' | 'any'
}

/**
 * Hook to fetch upcoming shows with cursor-based pagination.
 *
 * No PER-VIEWER input: whether a show is still upcoming is decided against its
 * own venue's zone, so the response is the same for every viewer (PSY-1678).
 * That is what lets the server-seeded first screen be a cache HIT rather than an
 * approximation the hydration commit has to refetch — see the seeding contract
 * in `features/shows/api.ts`. Do not reintroduce a per-viewer key segment here
 * without changing that contract too.
 *
 * The request does still carry a FIXED `timezone`, which the current backend
 * ignores; it exists only so this release is safe to deploy in either order and
 * safe to roll back. It is set FIRST so a no-filter call produces exactly
 * `UPCOMING_SHOWS_FIRST_SCREEN_URL`, and it is deliberately absent from the key
 * below. Both facts are asserted by `useShowsFirstScreen.test.tsx`. See
 * `TRANSITIONAL_TIMEZONE_PARAM`; PSY-1762 removes it.
 */
export const useUpcomingShows = (options: UseUpcomingShowsOptions = {}) => {
  const { cursor, limit, city, state, cities, tags, tagMatch } = options

  // Build query params. Timezone first: it is the only one a bare call sets, so
  // its position fixes the byte-for-byte match with the seeded URL.
  const params = new URLSearchParams(TRANSITIONAL_TIMEZONE_PARAM)
  if (cursor) params.set('cursor', cursor)
  if (limit) params.set('limit', limit.toString())

  // Multi-city takes priority over legacy single-city
  if (cities && cities.length > 0) {
    params.set('cities', buildCitiesParam(cities))
  } else {
    if (city) params.set('city', city)
    if (state) params.set('state', state)
  }

  if (tags && tags.length > 0) {
    params.set('tags', tags.join(','))
    if (tagMatch === 'any') params.set('tag_match', 'any')
  }

  // `params` always carries the transitional timezone, so the ternary the old
  // shape needed is gone: the query string is never empty.
  const endpoint = `${showEndpoints.UPCOMING}?${params.toString()}`

  return useQuery({
    queryKey: showQueryKeys.list({
      cursor,
      limit,
      city,
      state,
      cities,
      tags: tags && tags.length > 0 ? tags : undefined,
      tagMatch: tagMatch === 'any' ? 'any' : undefined,
    }),
    queryFn: async (): Promise<UpcomingShowsResponse> => {
      return apiRequest<UpcomingShowsResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    placeholderData: keepPreviousData, // Keep old data visible while fetching
  })
}

/**
 * Hook to fetch a single show by ID
 */
export const useShow = (showId: string | number) => {
  return useQuery({
    queryKey: showQueryKeys.detail(String(showId)),
    queryFn: async (): Promise<ShowResponse> => {
      return apiRequest<ShowResponse>(showEndpoints.GET(showId), {
        method: 'GET',
      })
    },
    enabled: Boolean(showId),
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * Hook to fetch cities that have upcoming shows with counts.
 *
 * Takes no options and keys on nothing per-viewer: the counts cover the same
 * venue-local upcoming partition `useUpcomingShows` lists, so every viewer gets
 * the same answer (PSY-1678). It requests the seeded URL directly, which carries
 * the transitional timezone the current backend ignores — same reasoning, and
 * the same one-release lifetime, as `useUpcomingShows`.
 */
export const useShowCities = () => {
  return useQuery({
    queryKey: showQueryKeys.cities(),
    queryFn: async (): Promise<ShowCitiesResponse> => {
      return apiRequest<ShowCitiesResponse>(SHOW_CITIES_FIRST_SCREEN_URL, {
        method: 'GET',
      })
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    placeholderData: keepPreviousData, // Keep old data visible while fetching
  })
}
