'use client'

/**
 * Charts Hooks
 *
 * TanStack Query hooks for fetching top charts data from the public API.
 */

import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type {
  ChartsOverviewResponse,
  TrendingShowsResponse,
  PopularArtistsResponse,
  ActiveVenuesResponse,
  HotReleasesResponse,
} from '../types'

/**
 * Hook to fetch the charts overview (top 5 of each category)
 */
export function useChartsOverview() {
  return useQuery({
    queryKey: queryKeys.charts.overview,
    queryFn: async (): Promise<ChartsOverviewResponse> => {
      return apiRequest<ChartsOverviewResponse>(API_ENDPOINTS.CHARTS.OVERVIEW, {
        method: 'GET',
      })
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * Hook to fetch trending shows chart
 */
export function useTrendingShows(limit?: number) {
  const params = limit ? `?limit=${limit}` : ''
  return useQuery({
    queryKey: queryKeys.charts.trendingShows(limit),
    queryFn: async (): Promise<TrendingShowsResponse> => {
      return apiRequest<TrendingShowsResponse>(
        `${API_ENDPOINTS.CHARTS.TRENDING_SHOWS}${params}`,
        { method: 'GET' }
      )
    },
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Hook to fetch popular artists chart
 */
export function usePopularArtists(limit?: number) {
  const params = limit ? `?limit=${limit}` : ''
  return useQuery({
    queryKey: queryKeys.charts.popularArtists(limit),
    queryFn: async (): Promise<PopularArtistsResponse> => {
      return apiRequest<PopularArtistsResponse>(
        `${API_ENDPOINTS.CHARTS.POPULAR_ARTISTS}${params}`,
        { method: 'GET' }
      )
    },
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Hook to fetch active venues chart
 */
export function useActiveVenues(limit?: number) {
  const params = limit ? `?limit=${limit}` : ''
  return useQuery({
    queryKey: queryKeys.charts.activeVenues(limit),
    queryFn: async (): Promise<ActiveVenuesResponse> => {
      return apiRequest<ActiveVenuesResponse>(
        `${API_ENDPOINTS.CHARTS.ACTIVE_VENUES}${params}`,
        { method: 'GET' }
      )
    },
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Hook to fetch hot releases chart
 */
export function useHotReleases(limit?: number) {
  const params = limit ? `?limit=${limit}` : ''
  return useQuery({
    queryKey: queryKeys.charts.hotReleases(limit),
    queryFn: async (): Promise<HotReleasesResponse> => {
      return apiRequest<HotReleasesResponse>(
        `${API_ENDPOINTS.CHARTS.HOT_RELEASES}${params}`,
        { method: 'GET' }
      )
    },
    staleTime: 5 * 60 * 1000,
  })
}
