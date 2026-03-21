'use client'

/**
 * Festival Hooks
 *
 * TanStack Query hooks for fetching festival data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { festivalEndpoints, festivalQueryKeys } from '../api'
import type {
  FestivalsListResponse,
  FestivalDetail,
  FestivalArtistsResponse,
  FestivalVenuesResponse,
  ArtistFestivalsResponse,
  SimilarFestivalsResponse,
  FestivalOverlap,
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
    ? `${festivalEndpoints.LIST}?${queryString}`
    : festivalEndpoints.LIST

  return useQuery({
    queryKey: festivalQueryKeys.list(
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
    queryKey: festivalQueryKeys.detail(idOrSlug),
    queryFn: async (): Promise<FestivalDetail> => {
      return apiRequest<FestivalDetail>(
        festivalEndpoints.GET(idOrSlug),
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
  const baseUrl = festivalEndpoints.ARTISTS(festivalIdOrSlug)
  const endpoint = queryString ? `${baseUrl}?${queryString}` : baseUrl

  return useQuery({
    queryKey: festivalQueryKeys.artists(festivalIdOrSlug, dayDate),
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
    queryKey: festivalQueryKeys.venues(festivalIdOrSlug),
    queryFn: async (): Promise<FestivalVenuesResponse> => {
      return apiRequest<FestivalVenuesResponse>(
        festivalEndpoints.VENUES(festivalIdOrSlug),
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
    queryKey: festivalQueryKeys.artistFestivals(artistIdOrSlug),
    queryFn: async (): Promise<ArtistFestivalsResponse> => {
      return apiRequest<ArtistFestivalsResponse>(
        festivalEndpoints.ARTIST_FESTIVALS(artistIdOrSlug),
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
  const baseUrl = festivalEndpoints.SIMILAR(festivalIdOrSlug)
  const endpoint = queryString ? `${baseUrl}?${queryString}` : baseUrl

  return useQuery({
    queryKey: festivalQueryKeys.similar(festivalIdOrSlug),
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
export function useFestivalBreakouts(options: { festivalIdOrSlug: string | number; enabled?: boolean }) {
  const { festivalIdOrSlug, enabled = true } = options

  return useQuery({
    queryKey: festivalQueryKeys.breakouts(festivalIdOrSlug),
    queryFn: async (): Promise<FestivalBreakouts> => {
      return apiRequest<FestivalBreakouts>(
        festivalEndpoints.BREAKOUTS(festivalIdOrSlug),
        { method: 'GET' }
      )
    },
    enabled: enabled && (typeof festivalIdOrSlug === 'string' ? Boolean(festivalIdOrSlug) : festivalIdOrSlug > 0),
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Hook to fetch an artist's festival billing trajectory
 */
export function useArtistFestivalTrajectory(options: { artistIdOrSlug: string | number; enabled?: boolean }) {
  const { artistIdOrSlug, enabled = true } = options

  return useQuery({
    queryKey: festivalQueryKeys.artistTrajectory(artistIdOrSlug),
    queryFn: async (): Promise<ArtistTrajectory> => {
      return apiRequest<ArtistTrajectory>(
        festivalEndpoints.ARTIST_TRAJECTORY(artistIdOrSlug),
        { method: 'GET' }
      )
    },
    enabled: enabled && (typeof artistIdOrSlug === 'string' ? Boolean(artistIdOrSlug) : artistIdOrSlug > 0),
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Hook to fetch year-over-year comparison for a festival series
 */
export function useSeriesComparison(options: { seriesSlug: string; years: number[]; enabled?: boolean }) {
  const { seriesSlug, years, enabled = true } = options

  const params = new URLSearchParams()
  params.set('years', years.join(','))
  const endpoint = `${festivalEndpoints.SERIES_COMPARE(seriesSlug)}?${params.toString()}`

  return useQuery({
    queryKey: festivalQueryKeys.seriesCompare(seriesSlug, years),
    queryFn: async (): Promise<SeriesComparison> => {
      return apiRequest<SeriesComparison>(endpoint, { method: 'GET' })
    },
    enabled: enabled && Boolean(seriesSlug) && years.length >= 2,
    staleTime: 5 * 60 * 1000,
  })
}
