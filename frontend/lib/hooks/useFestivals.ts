'use client'

/**
 * Festival Hooks
 *
 * TanStack Query hooks for fetching festival data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys } from '../queryClient'
import type {
  FestivalsListResponse,
  FestivalDetail,
  FestivalArtistsResponse,
  FestivalVenuesResponse,
  ArtistFestivalsResponse,
} from '../types/festival'

interface UseFestivalsOptions {
  status?: string
  city?: string
  state?: string
  year?: number
  seriesSlug?: string
}

/**
 * Hook to fetch list of festivals with optional filtering
 */
export function useFestivals(options: UseFestivalsOptions = {}) {
  const { status, city, state, year, seriesSlug } = options

  const params = new URLSearchParams()
  if (status) params.set('status', status)
  if (city) params.set('city', city)
  if (state) params.set('state', state)
  if (year) params.set('year', String(year))
  if (seriesSlug) params.set('series_slug', seriesSlug)

  const queryString = params.toString()
  const endpoint = queryString
    ? `${API_ENDPOINTS.FESTIVALS.LIST}?${queryString}`
    : API_ENDPOINTS.FESTIVALS.LIST

  return useQuery({
    queryKey: queryKeys.festivals.list(
      status || city || state || year || seriesSlug
        ? { status, city, state, year, seriesSlug }
        : undefined
    ),
    queryFn: async (): Promise<FestivalsListResponse> => {
      return apiRequest<FestivalsListResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 5 * 60 * 1000,
    placeholderData: keepPreviousData,
  })
}

interface UseFestivalOptions {
  idOrSlug: string | number
  enabled?: boolean
}

/**
 * Hook to fetch a single festival by ID or slug
 */
export function useFestival(options: UseFestivalOptions) {
  const { idOrSlug, enabled = true } = options

  return useQuery({
    queryKey: queryKeys.festivals.detail(idOrSlug),
    queryFn: async (): Promise<FestivalDetail> => {
      return apiRequest<FestivalDetail>(
        API_ENDPOINTS.FESTIVALS.GET(idOrSlug),
        { method: 'GET' }
      )
    },
    enabled:
      enabled &&
      (typeof idOrSlug === 'string' ? Boolean(idOrSlug) : idOrSlug > 0),
    staleTime: 5 * 60 * 1000,
  })
}

interface UseFestivalLineupOptions {
  festivalId: string | number
  dayDate?: string
  enabled?: boolean
}

/**
 * Hook to fetch a festival's artist lineup
 */
export function useFestivalLineup(options: UseFestivalLineupOptions) {
  const { festivalId, dayDate, enabled = true } = options

  const params = new URLSearchParams()
  if (dayDate) params.set('day_date', dayDate)
  const queryString = params.toString()

  const endpoint = queryString
    ? `${API_ENDPOINTS.FESTIVALS.ARTISTS(festivalId)}?${queryString}`
    : API_ENDPOINTS.FESTIVALS.ARTISTS(festivalId)

  return useQuery({
    queryKey: queryKeys.festivals.artists(festivalId),
    queryFn: async (): Promise<FestivalArtistsResponse> => {
      return apiRequest<FestivalArtistsResponse>(endpoint, {
        method: 'GET',
      })
    },
    enabled:
      enabled &&
      (typeof festivalId === 'string'
        ? Boolean(festivalId)
        : festivalId > 0),
    staleTime: 5 * 60 * 1000,
  })
}

interface UseFestivalVenuesOptions {
  festivalId: string | number
  enabled?: boolean
}

/**
 * Hook to fetch a festival's venues
 */
export function useFestivalVenues(options: UseFestivalVenuesOptions) {
  const { festivalId, enabled = true } = options

  return useQuery({
    queryKey: queryKeys.festivals.venues(festivalId),
    queryFn: async (): Promise<FestivalVenuesResponse> => {
      return apiRequest<FestivalVenuesResponse>(
        API_ENDPOINTS.FESTIVALS.VENUES(festivalId),
        { method: 'GET' }
      )
    },
    enabled:
      enabled &&
      (typeof festivalId === 'string'
        ? Boolean(festivalId)
        : festivalId > 0),
    staleTime: 5 * 60 * 1000,
  })
}

interface UseArtistFestivalsOptions {
  artistIdOrSlug: string | number
  enabled?: boolean
}

/**
 * Hook to fetch festivals for a specific artist
 */
export function useArtistFestivals(options: UseArtistFestivalsOptions) {
  const { artistIdOrSlug, enabled = true } = options

  return useQuery({
    queryKey: queryKeys.festivals.artistFestivals(artistIdOrSlug),
    queryFn: async (): Promise<ArtistFestivalsResponse> => {
      return apiRequest<ArtistFestivalsResponse>(
        API_ENDPOINTS.FESTIVALS.ARTIST_FESTIVALS(artistIdOrSlug),
        { method: 'GET' }
      )
    },
    enabled:
      enabled &&
      (typeof artistIdOrSlug === 'string'
        ? Boolean(artistIdOrSlug)
        : artistIdOrSlug > 0),
    staleTime: 5 * 60 * 1000,
  })
}
