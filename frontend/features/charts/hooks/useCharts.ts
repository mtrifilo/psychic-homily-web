'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { chartEndpoints, chartQueryKeys } from '../api'
import type {
  BusiestVenuesResponse,
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
  params: Record<string, string | number>
): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params))
    search.set(key, String(value))
  return `${endpoint}?${search.toString()}`
}

export function useMostActiveArtists(window: ChartWindow, limit = 7) {
  return useQuery({
    queryKey: chartQueryKeys.mostActiveArtists(window, limit),
    queryFn: () =>
      apiRequest<MostActiveArtistsResponse>(
        withParams(chartEndpoints.MOST_ACTIVE_ARTISTS, { window, limit }),
        { method: 'GET' }
      ),
  })
}

export function useOnTheRadio(window: ChartWindow, limit = 7) {
  return useQuery({
    queryKey: chartQueryKeys.onTheRadio(window, limit),
    queryFn: () =>
      apiRequest<OnTheRadioResponse>(
        withParams(chartEndpoints.ON_THE_RADIO, { window, limit }),
        { method: 'GET' }
      ),
  })
}

export function useMostAnticipated(limit = 6) {
  return useQuery({
    queryKey: chartQueryKeys.mostAnticipated(limit),
    queryFn: () =>
      apiRequest<MostAnticipatedResponse>(
        withParams(chartEndpoints.MOST_ANTICIPATED, { limit }),
        { method: 'GET' }
      ),
  })
}

export function useBusiestVenues(window: ChartWindow, limit = 7) {
  return useQuery({
    queryKey: chartQueryKeys.busiestVenues(window, limit),
    queryFn: () =>
      apiRequest<BusiestVenuesResponse>(
        withParams(chartEndpoints.BUSIEST_VENUES, { window, limit }),
        { method: 'GET' }
      ),
  })
}

export function useNewReleases(window: ChartWindow, limit = 6) {
  return useQuery({
    queryKey: chartQueryKeys.newReleases(window, limit),
    queryFn: () =>
      apiRequest<NewReleasesResponse>(
        withParams(chartEndpoints.NEW_RELEASES, { window, limit }),
        { method: 'GET' }
      ),
  })
}

export function useOpenersToWatch(window: ChartWindow, limit = 6) {
  return useQuery({
    queryKey: chartQueryKeys.openersToWatch(window, limit),
    queryFn: () =>
      apiRequest<OpenersToWatchResponse>(
        withParams(chartEndpoints.OPENERS_TO_WATCH, { window, limit }),
        { method: 'GET' }
      ),
  })
}

export function useChartsSummary(window: ChartWindow) {
  return useQuery({
    queryKey: chartQueryKeys.summary(window),
    queryFn: () =>
      apiRequest<ChartsSummaryResponse>(
        withParams(chartEndpoints.SUMMARY, { window }),
        { method: 'GET' }
      ),
  })
}

export function useFreshlyAdded(limit = 6) {
  return useQuery({
    queryKey: chartQueryKeys.freshlyAdded(limit),
    queryFn: () =>
      apiRequest<FreshlyAddedResponse>(
        withParams(chartEndpoints.FRESHLY_ADDED, { limit }),
        { method: 'GET' }
      ),
  })
}
