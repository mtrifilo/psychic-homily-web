'use client'

/**
 * Venues Hooks
 *
 * TanStack Query hooks for fetching venue data from the API.
 */

import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys } from '../queryClient'
import type {
  VenuesListResponse,
  VenueShowsResponse,
  VenueCitiesResponse,
} from '../types/venue'

interface UseVenuesOptions {
  state?: string
  city?: string
  limit?: number
  offset?: number
}

/**
 * Hook to fetch list of venues with show counts
 */
export const useVenues = (options: UseVenuesOptions = {}) => {
  const { state, city, limit = 50, offset = 0 } = options

  // Build query params
  const params = new URLSearchParams()
  if (state) params.set('state', state)
  if (city) params.set('city', city)
  if (limit) params.set('limit', limit.toString())
  if (offset) params.set('offset', offset.toString())

  const queryString = params.toString()
  const endpoint = queryString
    ? `${API_ENDPOINTS.VENUES.LIST}?${queryString}`
    : API_ENDPOINTS.VENUES.LIST

  return useQuery({
    queryKey: queryKeys.venues.list({ state, city, limit, offset }),
    queryFn: async (): Promise<VenuesListResponse> => {
      return apiRequest<VenuesListResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

interface UseVenueShowsOptions {
  venueId: number
  timezone?: string
  limit?: number
  enabled?: boolean
}

/**
 * Hook to fetch upcoming shows for a specific venue (lazy-loaded)
 */
export const useVenueShows = (options: UseVenueShowsOptions) => {
  const { venueId, timezone, limit = 20, enabled = true } = options

  // Build query params
  const params = new URLSearchParams()
  if (timezone) params.set('timezone', timezone)
  if (limit) params.set('limit', limit.toString())

  const queryString = params.toString()
  const endpoint = queryString
    ? `${API_ENDPOINTS.VENUES.SHOWS(venueId)}?${queryString}`
    : API_ENDPOINTS.VENUES.SHOWS(venueId)

  return useQuery({
    queryKey: queryKeys.venues.shows(venueId),
    queryFn: async (): Promise<VenueShowsResponse> => {
      return apiRequest<VenueShowsResponse>(endpoint, {
        method: 'GET',
      })
    },
    enabled: enabled && venueId > 0,
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * Hook to fetch distinct cities with venue counts for filtering
 */
export const useVenueCities = () => {
  return useQuery({
    queryKey: queryKeys.venues.cities,
    queryFn: async (): Promise<VenueCitiesResponse> => {
      return apiRequest<VenueCitiesResponse>(API_ENDPOINTS.VENUES.CITIES, {
        method: 'GET',
      })
    },
    staleTime: 10 * 60 * 1000, // 10 minutes - cities don't change often
  })
}
