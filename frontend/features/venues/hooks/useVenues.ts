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
  VenueBillNetworkResponse,
  VenueBillNetworkWindow,
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
  /** Multi-tag filter (PSY-309). Slugs applied with AND by default. */
  tags?: string[]
  /** Set to 'any' to switch the tag filter to OR semantics. */
  tagMatch?: 'all' | 'any'
}

/**
 * Hook to fetch list of venues with show counts
 */
export const useVenues = (options: UseVenuesOptions = {}) => {
  const { state, city, cities, limit = 50, offset = 0, tags, tagMatch } = options

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
  if (tags && tags.length > 0) {
    params.set('tags', tags.join(','))
    if (tagMatch === 'any') params.set('tag_match', 'any')
  }

  const queryString = params.toString()
  const endpoint = queryString
    ? `${venueEndpoints.LIST}?${queryString}`
    : venueEndpoints.LIST

  return useQuery({
    queryKey: venueQueryKeys.list({
      state,
      city,
      cities,
      limit,
      offset,
      tags: tags && tags.length > 0 ? tags : undefined,
      tagMatch: tagMatch === 'any' ? 'any' : undefined,
    }),
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

interface UseVenueBillNetworkOptions {
  venueIdOrSlug: string | number
  /** All-time (default) / rolling 12 months / specific calendar year. */
  window?: VenueBillNetworkWindow
  /** Required when window === 'year'. Hook returns disabled if missing. */
  year?: number
  enabled?: boolean
}

/**
 * Hook to fetch a venue's co-bill network (PSY-365). Mirrors `useSceneGraph`
 * — same shape on the wire (nodes/links/clusters), narrower scope.
 *
 * Edge weights are AT-VENUE shared shows (not global), within the active
 * time window. The default window is "all" (matches the scene graph's
 * "all approved shows" precedent).
 */
export const useVenueBillNetwork = (options: UseVenueBillNetworkOptions) => {
  const { venueIdOrSlug, window = 'all', year, enabled = true } = options

  // Build query params. The backend accepts: window=all|12m|year, year=YYYY.
  const params = new URLSearchParams()
  if (window !== 'all') {
    params.set('window', window)
  }
  if (window === 'year' && year !== undefined) {
    params.set('year', String(year))
  }
  const queryString = params.toString()
  const endpoint = queryString
    ? `${venueEndpoints.BILL_NETWORK(venueIdOrSlug)}?${queryString}`
    : venueEndpoints.BILL_NETWORK(venueIdOrSlug)

  // year is required when window=year; if missing, gate the request rather
  // than send an invalid query that the backend would reject with a 400.
  const hasValidYear = window !== 'year' || (year !== undefined && year > 0)

  return useQuery({
    queryKey: venueQueryKeys.billNetwork(venueIdOrSlug, window, year),
    queryFn: async (): Promise<VenueBillNetworkResponse> => {
      return apiRequest<VenueBillNetworkResponse>(endpoint, { method: 'GET' })
    },
    enabled:
      enabled &&
      hasValidYear &&
      (typeof venueIdOrSlug === 'string'
        ? Boolean(venueIdOrSlug)
        : venueIdOrSlug > 0),
    staleTime: 5 * 60 * 1000, // 5 minutes — match useSceneGraph
  })
}
