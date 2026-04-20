'use client'

/**
 * Festival Hooks
 *
 * TanStack Query hooks for fetching festival data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { createDetailHook, createNamedDetailHook } from '@/lib/hooks/factories'
import { festivalEndpoints, festivalQueryKeys } from '@/features/festivals/api'
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
  /** Multi-tag filter (PSY-309). Slugs applied with AND by default. */
  tags?: string[]
  /** Set to 'any' to switch the tag filter to OR semantics. */
  tagMatch?: 'all' | 'any'
}

/**
 * Hook to fetch list of festivals with optional filtering
 */
export function useFestivals(options: UseFestivalsOptions = {}) {
  const { status, city, state, year, seriesSlug, tags, tagMatch } = options

  const params = new URLSearchParams()
  if (status) params.set('status', status)
  if (city) params.set('city', city)
  if (state) params.set('state', state)
  if (year) params.set('year', String(year))
  if (seriesSlug) params.set('series_slug', seriesSlug)
  if (tags && tags.length > 0) {
    params.set('tags', tags.join(','))
    if (tagMatch === 'any') params.set('tag_match', 'any')
  }

  const queryString = params.toString()
  const endpoint = queryString
    ? `${festivalEndpoints.LIST}?${queryString}`
    : festivalEndpoints.LIST

  return useQuery({
    queryKey: festivalQueryKeys.list(
      status || city || state || year || seriesSlug || (tags && tags.length > 0)
        ? { status, city, state, year, seriesSlug, tags, tagMatch }
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
  festivalEndpoints.GET,
  festivalQueryKeys.detail,
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

/**
 * Hook to fetch venues for a festival
 */
export const useFestivalVenues = createNamedDetailHook<FestivalVenuesResponse, 'festivalIdOrSlug'>(
  'festivalIdOrSlug',
  festivalEndpoints.VENUES,
  festivalQueryKeys.venues,
)

/**
 * Hook to fetch festivals for a specific artist
 */
export const useArtistFestivals = createNamedDetailHook<ArtistFestivalsResponse, 'artistIdOrSlug'>(
  'artistIdOrSlug',
  festivalEndpoints.ARTIST_FESTIVALS,
  festivalQueryKeys.artistFestivals,
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
export const useFestivalBreakouts = createNamedDetailHook<FestivalBreakouts, 'festivalIdOrSlug'>(
  'festivalIdOrSlug',
  festivalEndpoints.BREAKOUTS,
  festivalQueryKeys.breakouts,
)

/**
 * Hook to fetch an artist's festival billing trajectory
 */
export const useArtistFestivalTrajectory = createNamedDetailHook<ArtistTrajectory, 'artistIdOrSlug'>(
  'artistIdOrSlug',
  festivalEndpoints.ARTIST_TRAJECTORY,
  festivalQueryKeys.artistTrajectory,
)

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
