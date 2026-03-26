'use client'

/**
 * Shows Hooks
 *
 * TanStack Query hooks for fetching show data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { showEndpoints, showQueryKeys } from '@/features/shows/api'
import type { UpcomingShowsResponse, ShowResponse, ShowCitiesResponse } from '../types'

interface UseUpcomingShowsOptions {
  timezone?: string
  cursor?: string
  limit?: number
  /** Legacy single-city filter */
  city?: string
  /** Legacy single-state filter */
  state?: string
  /** Multi-city filter (takes priority over city/state) */
  cities?: Array<{ city: string; state: string }>
}

/**
 * Build pipe-delimited cities param: "Phoenix,AZ|Mesa,AZ"
 */
function buildCitiesParam(cities: Array<{ city: string; state: string }>): string {
  return cities.map(c => `${c.city},${c.state}`).join('|')
}

/**
 * Hook to fetch upcoming shows with cursor-based pagination
 */
export const useUpcomingShows = (options: UseUpcomingShowsOptions = {}) => {
  const { timezone, cursor, limit, city, state, cities } = options

  // Build query params
  const params = new URLSearchParams()
  if (timezone) params.set('timezone', timezone)
  if (cursor) params.set('cursor', cursor)
  if (limit) params.set('limit', limit.toString())

  // Multi-city takes priority over legacy single-city
  if (cities && cities.length > 0) {
    params.set('cities', buildCitiesParam(cities))
  } else {
    if (city) params.set('city', city)
    if (state) params.set('state', state)
  }

  const queryString = params.toString()
  const endpoint = queryString
    ? `${showEndpoints.UPCOMING}?${queryString}`
    : showEndpoints.UPCOMING

  return useQuery({
    queryKey: showQueryKeys.list({ timezone, cursor, limit, city, state, cities }),
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

interface UseShowCitiesOptions {
  timezone?: string
}

/**
 * Hook to fetch cities that have upcoming shows with counts
 */
export const useShowCities = (options: UseShowCitiesOptions = {}) => {
  const { timezone } = options

  // Build query params
  const params = new URLSearchParams()
  if (timezone) params.set('timezone', timezone)

  const queryString = params.toString()
  const endpoint = queryString
    ? `${showEndpoints.CITIES}?${queryString}`
    : showEndpoints.CITIES

  return useQuery({
    queryKey: showQueryKeys.cities(timezone),
    queryFn: async (): Promise<ShowCitiesResponse> => {
      return apiRequest<ShowCitiesResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    placeholderData: keepPreviousData, // Keep old data visible while fetching
  })
}
