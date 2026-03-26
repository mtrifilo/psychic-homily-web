'use client'

/**
 * Venues Hooks
 *
 * TanStack Query hooks for fetching venue data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { createNamedDetailHook } from '@/lib/hooks/factories'
import { venueEndpoints, venueQueryKeys } from '@/features/venues/api'
import type {
  Venue,
  VenuesListResponse,
  VenueShowsResponse,
  VenueCitiesResponse,
  VenueGenreResponse,
} from '../types'

interface CityState {
  city: string
  state: string
}

interface UseVenuesOptions {
  state?: string
  city?: string
  cities?: CityState[]
  limit?: number
  offset?: number
}

/**
 * Hook to fetch list of venues with show counts
 */
export const useVenues = (options: UseVenuesOptions = {}) => {
  const { state, city, cities, limit = 50, offset = 0 } = options

  // Build query params
  const params = new URLSearchParams()
  if (cities && cities.length > 0) {
    // Multi-city filter: "Phoenix,AZ|Tucson,AZ"
    params.set('cities', cities.map(c => `${c.city},${c.state}`).join('|'))
  } else {
    if (state) params.set('state', state)
    if (city) params.set('city', city)
  }
  if (limit) params.set('limit', limit.toString())
  if (offset) params.set('offset', offset.toString())

  const queryString = params.toString()
  const endpoint = queryString
    ? `${venueEndpoints.LIST}?${queryString}`
    : venueEndpoints.LIST

  return useQuery({
    queryKey: venueQueryKeys.list({ state, city, cities, limit, offset }),
    queryFn: async (): Promise<VenuesListResponse> => {
      return apiRequest<VenuesListResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    placeholderData: keepPreviousData, // Keep old data visible while fetching
  })
}

/**
 * Hook to fetch a single venue by ID or slug
 */
export const useVenue = createNamedDetailHook<Venue, 'venueId'>(
  'venueId',
  venueEndpoints.GET,
  venueQueryKeys.detail,
)

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
    ? `${venueEndpoints.SHOWS(venueId)}?${queryString}`
    : venueEndpoints.SHOWS(venueId)

  return useQuery({
    queryKey: [...venueQueryKeys.shows(venueId), timeFilter],
    queryFn: async (): Promise<VenueShowsResponse> => {
      return apiRequest<VenueShowsResponse>(endpoint, {
        method: 'GET',
      })
    },
    enabled: enabled && (typeof venueId === 'string' ? Boolean(venueId) : venueId > 0),
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * Hook to fetch distinct cities with venue counts for filtering
 */
export const useVenueCities = () => {
  return useQuery({
    queryKey: venueQueryKeys.cities,
    queryFn: async (): Promise<VenueCitiesResponse> => {
      return apiRequest<VenueCitiesResponse>(venueEndpoints.CITIES, {
        method: 'GET',
      })
    },
    staleTime: 10 * 60 * 1000, // 10 minutes - cities don't change often
    placeholderData: keepPreviousData, // Keep old data visible while fetching
  })
}

/**
 * Hook to fetch a venue's genre profile (top 5 genres derived from artist tags)
 */
export const useVenueGenres = (venueIdOrSlug: string | number) => {
  return useQuery({
    queryKey: venueQueryKeys.genres(venueIdOrSlug),
    queryFn: async (): Promise<VenueGenreResponse> => {
      return apiRequest<VenueGenreResponse>(
        venueEndpoints.GENRES(venueIdOrSlug),
        { method: 'GET' }
      )
    },
    enabled: typeof venueIdOrSlug === 'string' ? Boolean(venueIdOrSlug) : venueIdOrSlug > 0,
    staleTime: 10 * 60 * 1000, // 10 minutes — genre profiles change infrequently
  })
}
