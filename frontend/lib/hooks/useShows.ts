'use client'

/**
 * Shows Hooks
 *
 * TanStack Query hooks for fetching show data from the API.
 */

import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys } from '../queryClient'
import type { UpcomingShowsResponse, ShowResponse } from '../types/show'

interface UseUpcomingShowsOptions {
  timezone?: string
  cursor?: string
  limit?: number
}

/**
 * Hook to fetch upcoming shows with cursor-based pagination
 */
export const useUpcomingShows = (options: UseUpcomingShowsOptions = {}) => {
  const { timezone, cursor, limit } = options

  // Build query params
  const params = new URLSearchParams()
  if (timezone) params.set('timezone', timezone)
  if (cursor) params.set('cursor', cursor)
  if (limit) params.set('limit', limit.toString())

  const queryString = params.toString()
  const endpoint = queryString
    ? `${API_ENDPOINTS.SHOWS.UPCOMING}?${queryString}`
    : API_ENDPOINTS.SHOWS.UPCOMING

  return useQuery({
    queryKey: queryKeys.shows.list({ timezone, cursor, limit }),
    queryFn: async (): Promise<UpcomingShowsResponse> => {
      return apiRequest<UpcomingShowsResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * Hook to fetch a single show by ID
 */
export const useShow = (showId: string | number) => {
  return useQuery({
    queryKey: queryKeys.shows.detail(String(showId)),
    queryFn: async (): Promise<ShowResponse> => {
      return apiRequest<ShowResponse>(API_ENDPOINTS.SHOWS.GET(showId), {
        method: 'GET',
      })
    },
    enabled: Boolean(showId),
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}
