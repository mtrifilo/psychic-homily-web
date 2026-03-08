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

interface UseFestivalArtistsOptions {
  festivalIdOrSlug: string | number
  dayDate?: string
  enabled?: boolean
}

/**
 * Hook to fetch the lineup (artists) for a festival
 */
export function useFestivalArtists(options: UseFestivalArtistsOptions) {
  const { festivalIdOrSlug, dayDate, enabled = true } = options

  const params = new URLSearchParams()
  if (dayDate) params.set('day_date', dayDate)

  const queryString = params.toString()
  const baseUrl = API_ENDPOINTS.FESTIVALS.ARTISTS(festivalIdOrSlug)
  const endpoint = queryString ? `${baseUrl}?${queryString}` : baseUrl

  return useQuery({
    queryKey: queryKeys.festivals.artists(festivalIdOrSlug, dayDate),
    queryFn: async (): Promise<FestivalArtistsResponse> => {
      return apiRequest<FestivalArtistsResponse>(endpoint, {
        method: 'GET',
      })
    },
    enabled:
      enabled &&
      (typeof festivalIdOrSlug === 'string'
        ? Boolean(festivalIdOrSlug)
        : festivalIdOrSlug > 0),
    staleTime: 5 * 60 * 1000,
  })
}

/** Alias for backward compatibility with admin components */
export function useFestivalLineup(options: { festivalId: string | number; dayDate?: string; enabled?: boolean }) {
  return useFestivalArtists({
    festivalIdOrSlug: options.festivalId,
    dayDate: options.dayDate,
    enabled: options.enabled,
  })
}

interface UseFestivalVenuesOptions {
  festivalIdOrSlug: string | number
  enabled?: boolean
}

/**
 * Hook to fetch venues for a festival
 */
export function useFestivalVenues(options: UseFestivalVenuesOptions) {
  const { festivalIdOrSlug, enabled = true } = options

  return useQuery({
    queryKey: queryKeys.festivals.venues(festivalIdOrSlug),
    queryFn: async (): Promise<FestivalVenuesResponse> => {
      return apiRequest<FestivalVenuesResponse>(
        API_ENDPOINTS.FESTIVALS.VENUES(festivalIdOrSlug),
        { method: 'GET' }
      )
    },
    enabled:
      enabled &&
      (typeof festivalIdOrSlug === 'string'
        ? Boolean(festivalIdOrSlug)
        : festivalIdOrSlug > 0),
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
