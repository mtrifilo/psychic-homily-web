'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { chartEndpoints, chartQueryKeys } from '../api'
import type {
  BusiestVenuesResponse,
  ChartScenesResponse,
  ChartsSummaryResponse,
  ChartWindow,
  FreshlyAddedResponse,
  MostActiveArtistsResponse,
  MostAnticipatedResponse,
  NewReleasesResponse,
  OnTheRadioResponse,
  OpenersToWatchResponse,
} from '../types'

function withParams(
  endpoint: string,
  params: Record<string, string | number | undefined>
): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') search.set(key, String(value))
  }
  return `${endpoint}?${search.toString()}`
}

export interface ChartQueryOptions {
  scene?: string
  enabled?: boolean
}

export function useMostActiveArtists(
  window: ChartWindow,
  limit = 7,
  { scene = '', enabled = true }: ChartQueryOptions = {}
) {
  return useQuery({
    queryKey: chartQueryKeys.mostActiveArtists(window, scene, limit),
    queryFn: () =>
      apiRequest<MostActiveArtistsResponse>(
        withParams(chartEndpoints.MOST_ACTIVE_ARTISTS, {
          window,
          limit,
          scene,
        }),
        { method: 'GET' }
      ),
    enabled,
  })
}

export function useOnTheRadio(
  window: ChartWindow,
  limit = 7,
  { scene = '', enabled = true }: ChartQueryOptions = {}
) {
  return useQuery({
    queryKey: chartQueryKeys.onTheRadio(window, scene, limit),
    queryFn: () =>
      apiRequest<OnTheRadioResponse>(
        withParams(chartEndpoints.ON_THE_RADIO, { window, limit, scene }),
        { method: 'GET' }
      ),
    enabled,
  })
}

export function useMostAnticipated(
  limit = 6,
  { scene = '', enabled = true }: ChartQueryOptions = {}
) {
  return useQuery({
    queryKey: chartQueryKeys.mostAnticipated(scene, limit),
    queryFn: () =>
      apiRequest<MostAnticipatedResponse>(
        withParams(chartEndpoints.MOST_ANTICIPATED, { limit, scene }),
        { method: 'GET' }
      ),
    enabled,
  })
}

export function useBusiestVenues(
  window: ChartWindow,
  limit = 7,
  { scene = '', enabled = true }: ChartQueryOptions = {}
) {
  return useQuery({
    queryKey: chartQueryKeys.busiestVenues(window, scene, limit),
    queryFn: () =>
      apiRequest<BusiestVenuesResponse>(
        withParams(chartEndpoints.BUSIEST_VENUES, { window, limit, scene }),
        { method: 'GET' }
      ),
    enabled,
  })
}

export function useNewReleases(
  window: ChartWindow,
  limit = 6,
  { scene = '', enabled = true }: ChartQueryOptions = {}
) {
  return useQuery({
    queryKey: chartQueryKeys.newReleases(window, scene, limit),
    queryFn: () =>
      apiRequest<NewReleasesResponse>(
        withParams(chartEndpoints.NEW_RELEASES, { window, limit, scene }),
        { method: 'GET' }
      ),
    enabled,
  })
}

export function useOpenersToWatch(
  window: ChartWindow,
  limit = 6,
  { scene = '', enabled = true }: ChartQueryOptions = {}
) {
  return useQuery({
    queryKey: chartQueryKeys.openersToWatch(window, scene, limit),
    queryFn: () =>
      apiRequest<OpenersToWatchResponse>(
        withParams(chartEndpoints.OPENERS_TO_WATCH, { window, limit, scene }),
        { method: 'GET' }
      ),
    enabled,
  })
}

export function useChartsSummary(
  window: ChartWindow,
  { scene = '', enabled = true }: ChartQueryOptions = {}
) {
  return useQuery({
    queryKey: chartQueryKeys.summary(window, scene),
    queryFn: () =>
      apiRequest<ChartsSummaryResponse>(
        withParams(chartEndpoints.SUMMARY, { window, scene }),
        { method: 'GET' }
      ),
    enabled,
  })
}

export function useFreshlyAdded(
  limit = 6,
  { scene = '', enabled = true }: ChartQueryOptions = {}
) {
  return useQuery({
    queryKey: chartQueryKeys.freshlyAdded(scene, limit),
    queryFn: () =>
      apiRequest<FreshlyAddedResponse>(
        withParams(chartEndpoints.FRESHLY_ADDED, { limit, scene }),
        { method: 'GET' }
      ),
    enabled,
  })
}

export function useChartScenes(window: ChartWindow) {
  return useQuery({
    queryKey: chartQueryKeys.scenes(window),
    queryFn: () =>
      apiRequest<ChartScenesResponse>(
        withParams(chartEndpoints.SCENES, { window }),
        { method: 'GET' }
      ),
  })
}
