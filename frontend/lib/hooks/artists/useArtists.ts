'use client'

/**
 * Artists Hooks
 *
 * TanStack Query hooks for fetching artist data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../../api'
import { queryKeys } from '../../queryClient'
import type { CityState } from '@/components/filters'
import type {
  Artist,
  ArtistsListResponse,
  ArtistCitiesResponse,
  ArtistShowsResponse,
  ArtistTimeFilter,
} from '../../types/artist'

interface UseArtistsOptions {
  cities?: CityState[]
}

/**
 * Hook to fetch list of artists with optional city filtering
 */
export function useArtists(options: UseArtistsOptions = {}) {
  const { cities } = options

  // Build query params
  const params = new URLSearchParams()
  if (cities && cities.length > 0) {
    params.set('cities', cities.map(c => `${c.city},${c.state}`).join('|'))
  }

  const queryString = params.toString()
  const endpoint = queryString
    ? `${API_ENDPOINTS.ARTISTS.LIST}?${queryString}`
    : API_ENDPOINTS.ARTISTS.LIST

  return useQuery({
    queryKey: queryKeys.artists.list(cities ? { cities } : undefined),
    queryFn: async (): Promise<ArtistsListResponse> => {
      return apiRequest<ArtistsListResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    placeholderData: keepPreviousData,
  })
}

/**
 * Hook to fetch distinct cities with artist counts for filtering
 */
export function useArtistCities() {
  return useQuery({
    queryKey: queryKeys.artists.cities,
    queryFn: async (): Promise<ArtistCitiesResponse> => {
      return apiRequest<ArtistCitiesResponse>(API_ENDPOINTS.ARTISTS.CITIES, {
        method: 'GET',
      })
    },
    staleTime: 10 * 60 * 1000, // 10 minutes - cities don't change often
    placeholderData: keepPreviousData,
  })
}

interface UseArtistOptions {
  artistId: string | number
  enabled?: boolean
}

/**
 * Hook to fetch a single artist by ID or slug
 */
export function useArtist(options: UseArtistOptions) {
  const { artistId, enabled = true } = options

  return useQuery({
    queryKey: queryKeys.artists.detail(artistId),
    queryFn: async (): Promise<Artist> => {
      return apiRequest<Artist>(API_ENDPOINTS.ARTISTS.GET(artistId), {
        method: 'GET',
      })
    },
    enabled: enabled && (typeof artistId === 'string' ? Boolean(artistId) : artistId > 0),
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

interface UseArtistShowsOptions {
  artistId: string | number
  timezone?: string
  limit?: number
  enabled?: boolean
  timeFilter?: ArtistTimeFilter
}

/**
 * Hook to fetch shows for a specific artist by ID or slug
 * @param timeFilter - Filter by time: 'upcoming' (default), 'past', or 'all'
 */
export function useArtistShows(options: UseArtistShowsOptions) {
  const {
    artistId,
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
    ? `${API_ENDPOINTS.ARTISTS.SHOWS(artistId)}?${queryString}`
    : API_ENDPOINTS.ARTISTS.SHOWS(artistId)

  return useQuery({
    queryKey: [...queryKeys.artists.shows(artistId), timeFilter],
    queryFn: async (): Promise<ArtistShowsResponse> => {
      return apiRequest<ArtistShowsResponse>(endpoint, {
        method: 'GET',
      })
    },
    enabled: enabled && (typeof artistId === 'string' ? Boolean(artistId) : artistId > 0),
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}
