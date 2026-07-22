'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { chartEndpoints, chartQueryKeys } from '../api'
import type {
  BusiestVenuesResponse,
  ChartEntityRank,
  ChartRankEntityType,
  ChartScenesResponse,
  ChartsSummaryResponse,
  ChartWindow,
  FeaturedCollectionHistoryResponse,
  FreshlyAddedResponse,
  MostActiveArtistsResponse,
  MostAnticipatedResponse,
  NewReleasesResponse,
  OnTheRadioResponse,
  OpenersToWatchResponse,
  PersonalChartsStats,
  TopTagsResponse,
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
  offset?: number
}

export function useMostActiveArtists(
  window: ChartWindow,
  limit = 7,
  { scene = '', enabled = true, offset = 0 }: ChartQueryOptions = {}
) {
  return useQuery({
    queryKey: chartQueryKeys.mostActiveArtists(window, scene, limit, offset),
    queryFn: () =>
      apiRequest<MostActiveArtistsResponse>(
        withParams(chartEndpoints.MOST_ACTIVE_ARTISTS, {
          window,
          limit,
          offset: offset || undefined,
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
  { scene = '', enabled = true, offset = 0 }: ChartQueryOptions = {}
) {
  return useQuery({
    queryKey: chartQueryKeys.onTheRadio(window, scene, limit, offset),
    queryFn: () =>
      apiRequest<OnTheRadioResponse>(
        withParams(chartEndpoints.ON_THE_RADIO, {
          window,
          limit,
          offset: offset || undefined,
          scene,
        }),
        { method: 'GET' }
      ),
    enabled,
  })
}

export function useMostAnticipated(
  window: ChartWindow,
  limit = 6,
  { scene = '', enabled = true, offset = 0 }: ChartQueryOptions = {}
) {
  return useQuery({
    queryKey: chartQueryKeys.mostAnticipated(window, scene, limit, offset),
    queryFn: () =>
      apiRequest<MostAnticipatedResponse>(
        withParams(chartEndpoints.MOST_ANTICIPATED, {
          window,
          limit,
          offset: offset || undefined,
          scene,
        }),
        { method: 'GET' }
      ),
    enabled,
  })
}

export function useBusiestVenues(
  window: ChartWindow,
  limit = 7,
  { scene = '', enabled = true, offset = 0 }: ChartQueryOptions = {}
) {
  return useQuery({
    queryKey: chartQueryKeys.busiestVenues(window, scene, limit, offset),
    queryFn: () =>
      apiRequest<BusiestVenuesResponse>(
        withParams(chartEndpoints.BUSIEST_VENUES, {
          window,
          limit,
          offset: offset || undefined,
          scene,
        }),
        { method: 'GET' }
      ),
    enabled,
  })
}

export function useNewReleases(
  window: ChartWindow,
  limit = 6,
  { scene = '', enabled = true, offset = 0 }: ChartQueryOptions = {}
) {
  return useQuery({
    queryKey: chartQueryKeys.newReleases(window, scene, limit, offset),
    queryFn: () =>
      apiRequest<NewReleasesResponse>(
        withParams(chartEndpoints.NEW_RELEASES, {
          window,
          limit,
          offset: offset || undefined,
          scene,
        }),
        { method: 'GET' }
      ),
    enabled,
  })
}

export function useOpenersToWatch(
  window: ChartWindow,
  limit = 6,
  { scene = '', enabled = true, offset = 0 }: ChartQueryOptions = {}
) {
  return useQuery({
    queryKey: chartQueryKeys.openersToWatch(window, scene, limit, offset),
    queryFn: () =>
      apiRequest<OpenersToWatchResponse>(
        withParams(chartEndpoints.OPENERS_TO_WATCH, {
          window,
          limit,
          offset: offset || undefined,
          scene,
        }),
        { method: 'GET' }
      ),
    enabled,
  })
}

export function useTopTags(
  window: ChartWindow,
  limit = 7,
  { scene = '', enabled = true }: ChartQueryOptions = {}
) {
  return useQuery({
    queryKey: chartQueryKeys.topTags(window, scene, limit),
    queryFn: () =>
      apiRequest<TopTagsResponse>(
        withParams(chartEndpoints.TOP_TAGS, { window, limit, scene }),
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

export function usePersonalChartsStats(
  userId?: string | number,
  enabled = true
) {
  return useQuery({
    queryKey: chartQueryKeys.personal(userId),
    queryFn: () =>
      apiRequest<PersonalChartsStats>(chartEndpoints.PERSONAL, {
        method: 'GET',
      }),
    enabled: enabled && userId != null,
  })
}

/**
 * Featured-collection picks archive (PSY-1500 / PSY-1501). Returns every
 * featuring stint newest-first, so the caller can peel the newest run off as
 * the lead editorial card and render the remainder as the closed-run ledger.
 * Defaults to the endpoint's max page size — the archive is a flat, single-page
 * ledger by design, so one generous fetch covers the current pick + history.
 * Optional `enabled` lets the Broadsheet card (PSY-1411) skip the request when
 * nothing is featured.
 */
export function useFeaturedCollectionHistory(
  limit = 100,
  offset = 0,
  { enabled = true }: { enabled?: boolean } = {}
) {
  return useQuery({
    queryKey: chartQueryKeys.featuredCollectionHistory(limit, offset),
    queryFn: () =>
      apiRequest<FeaturedCollectionHistoryResponse>(
        withParams(chartEndpoints.FEATURED_COLLECTION_HISTORY, {
          limit,
          offset: offset || undefined,
        }),
        { method: 'GET' }
      ),
    enabled,
  })
}

/**
 * Non-blocking per-entity chart rank lookup (PSY-1420). Defaults to the
 * v1 window (`quarter`). Global-scope only — no scene param.
 */
export function useChartEntityRank(
  entityType: ChartRankEntityType,
  entityId: number,
  window: ChartWindow = 'quarter',
  { enabled = true }: { enabled?: boolean } = {}
) {
  return useQuery({
    queryKey: chartQueryKeys.rank(entityType, entityId, window),
    queryFn: () =>
      apiRequest<ChartEntityRank>(
        withParams(chartEndpoints.RANK, {
          entity_type: entityType,
          entity_id: entityId,
          window,
        }),
        { method: 'GET' }
      ),
    enabled: enabled && entityId > 0,
  })
}
