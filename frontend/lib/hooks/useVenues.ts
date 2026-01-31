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
  Venue,
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

interface UseVenueOptions {
  venueId: string | number
  enabled?: boolean
}

/**
 * Hook to fetch a single venue by ID or slug
 */
export const useVenue = (options: UseVenueOptions) => {
  const { venueId, enabled = true } = options

  return useQuery({
    queryKey: queryKeys.venues.detail(String(venueId)),
    queryFn: async (): Promise<Venue> => {
      return apiRequest<Venue>(API_ENDPOINTS.VENUES.GET(venueId), {
        method: 'GET',
      })
    },
    enabled: enabled && Boolean(venueId),
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

export type TimeFilter = 'upcoming' | 'past' | 'all'

interface UseVenueShowsOptions {
  venueId: string | number
  timezone?: string
  limit?: number
  enabled?: boolean
  timeFilter?: TimeFilter
}

/**
 * Hook to fetch shows for a specific venue by ID or slug (lazy-loaded)
 * @param timeFilter - Filter by time: 'upcoming' (default), 'past', or 'all'
 */
export const useVenueShows = (options: UseVenueShowsOptions) => {
  const {
    venueId,
    timezone,
    limit = 20,
    enabled = true,
    timeFilter = 'upcoming',
  } = options

  // Build query params
  const params = new URLSearchParams()
  if (timezone) params.set('timezone', timezone)
  if (limit) params.set('limit', limit.toString())
  if (timeFilter) params.set('time_filter', timeFilter)

  const queryString = params.toString()
  const endpoint = queryString
    ? `${API_ENDPOINTS.VENUES.SHOWS(venueId)}?${queryString}`
    : API_ENDPOINTS.VENUES.SHOWS(venueId)

  return useQuery({
    queryKey: [...queryKeys.venues.shows(venueId), timeFilter],
    queryFn: async (): Promise<VenueShowsResponse> => {
      return apiRequest<VenueShowsResponse>(endpoint, {
        method: 'GET',
      })
    },
    enabled: enabled && Boolean(venueId),
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
