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
 * A no-argument call therefore sends NO query string at all, which is exactly
 * `UPCOMING_SHOWS_FIRST_SCREEN_URL`. `useShowsFirstScreen.test.tsx` asserts that
 * pairing against the real constants.
 */
export const useUpcomingShows = (options: UseUpcomingShowsOptions = {}) => {
  const { cursor, limit, city, state, cities, tags, tagMatch } = options

  const params = new URLSearchParams()
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

  // The `?` only when there is something to put after it, so a bare call
  // produces `UPCOMING_SHOWS_FIRST_SCREEN_URL` exactly. Whether the server's
  // seed is a HIT is decided by the KEY, not by this string — but the exported
  // constant is what `app/shows/page.tsx` fetches server-side, and it is only an
  // honest description of what this hook requests while the two stay identical.
  // `useShowsFirstScreen.test.tsx` is what holds them together.
  const queryString = params.toString()
  const endpoint = queryString
    ? `${showEndpoints.UPCOMING}?${queryString}`
    : showEndpoints.UPCOMING

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
 * the same answer (PSY-1678). It requests the seeded URL directly, which is the
 * bare endpoint — there is nothing per-viewer left to put in it.
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
