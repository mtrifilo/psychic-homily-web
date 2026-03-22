'use client'

/**
 * Festival Hooks
 *
 * TanStack Query hooks for fetching festival data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { createDetailHook, createNamedDetailHook } from '@/lib/hooks/factories'
import type {
  FestivalsListResponse,
  FestivalDetail,
  FestivalArtistsResponse,
  FestivalVenuesResponse,
  ArtistFestivalsResponse,
  SimilarFestivalsResponse,
  FestivalBreakouts,
  ArtistTrajectory,
  SeriesComparison,
} from '../types'

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

/**
 * Hook to fetch a single festival by ID or slug
 */
export const useFestival = createDetailHook<FestivalDetail>(
  API_ENDPOINTS.FESTIVALS.GET,
  queryKeys.festivals.detail,
)

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

/**
 * Hook to fetch venues for a festival
 */
export const useFestivalVenues = createNamedDetailHook<FestivalVenuesResponse, 'festivalIdOrSlug'>(
  'festivalIdOrSlug',
  API_ENDPOINTS.FESTIVALS.VENUES,
  queryKeys.festivals.venues,
)

/**
 * Hook to fetch festivals for a specific artist
 */
export const useArtistFestivals = createNamedDetailHook<ArtistFestivalsResponse, 'artistIdOrSlug'>(
  'artistIdOrSlug',
  API_ENDPOINTS.FESTIVALS.ARTIST_FESTIVALS,
  queryKeys.festivals.artistFestivals,
)

// ──────────────────────────────────────────────
// Festival Intelligence hooks
// ──────────────────────────────────────────────

/**
 * Hook to fetch similar festivals based on lineup overlap
 */
export function useSimilarFestivals(options: { festivalIdOrSlug: string | number; limit?: number; enabled?: boolean }) {
  const { festivalIdOrSlug, limit = 10, enabled = true } = options

  const params = new URLSearchParams()
  if (limit) params.set('limit', String(limit))
  const queryString = params.toString()
  const baseUrl = API_ENDPOINTS.FESTIVALS.SIMILAR(festivalIdOrSlug)
  const endpoint = queryString ? `${baseUrl}?${queryString}` : baseUrl

  return useQuery({
    queryKey: queryKeys.festivals.similar(festivalIdOrSlug),
    queryFn: async (): Promise<SimilarFestivalsResponse> => {
      return apiRequest<SimilarFestivalsResponse>(endpoint, { method: 'GET' })
    },
    enabled: enabled && (typeof festivalIdOrSlug === 'string' ? Boolean(festivalIdOrSlug) : festivalIdOrSlug > 0),
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Hook to fetch breakout artists at a festival
 */
export const useFestivalBreakouts = createNamedDetailHook<FestivalBreakouts, 'festivalIdOrSlug'>(
  'festivalIdOrSlug',
  API_ENDPOINTS.FESTIVALS.BREAKOUTS,
  queryKeys.festivals.breakouts,
)

/**
 * Hook to fetch an artist's festival billing trajectory
 */
export const useArtistFestivalTrajectory = createNamedDetailHook<ArtistTrajectory, 'artistIdOrSlug'>(
  'artistIdOrSlug',
  API_ENDPOINTS.FESTIVALS.ARTIST_TRAJECTORY,
  queryKeys.festivals.artistTrajectory,
)

/**
 * Hook to fetch year-over-year comparison for a festival series
 */
export function useSeriesComparison(options: { seriesSlug: string; years: number[]; enabled?: boolean }) {
  const { seriesSlug, years, enabled = true } = options

  const params = new URLSearchParams()
  params.set('years', years.join(','))
  const endpoint = `${API_ENDPOINTS.FESTIVALS.SERIES_COMPARE(seriesSlug)}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.festivals.seriesCompare(seriesSlug, years),
    queryFn: async (): Promise<SeriesComparison> => {
      return apiRequest<SeriesComparison>(endpoint, { method: 'GET' })
    },
    enabled: enabled && Boolean(seriesSlug) && years.length >= 2,
    staleTime: 5 * 60 * 1000,
  })
}
