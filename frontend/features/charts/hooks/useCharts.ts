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
  FeaturedCollectionResponse,
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

/**
 * Retains the previous response across an OFFSET change and drops it across
 * every other key change.
 *
 * Holding the outgoing page on screen while the next one loads is what keeps
 * the drilldown's results region, and with it the pager's live region,
 * MOUNTED across a page change. An unmounted live region announces nothing, so
 * without this a screen-reader user gets no feedback at all when they page
 * (PSY-1768). Callers must dim on `isPlaceholderData` and suppress anything
 * derived from the outgoing rows, so stale content is never presented as
 * current.
 *
 * Scoped to `offset` because a `window` or `scene` change asks a DIFFERENT
 * question: retaining there would paint the quarter chart under the "All Time"
 * heading. The homepage modules never move `offset`, so they are unaffected.
 *
 * Deliberately NOT a caller opt-in flag. That spelling is fail-open, correct
 * only until someone copies the drilldown's options onto a window-varying
 * call. Stating the invariant here makes every caller right by construction
 * (`useScenes` precedent).
 *
 * Reads `offset` as the LAST segment of the key rather than by a named index,
 * so inserting a new scope segment ahead of it keeps the retention correct
 * instead of silently widening it across scenes. Both directions are pinned by
 * `useCharts.test.tsx`: one test demands retention across an offset change,
 * the other demands the opposite across a window change.
 */
function keepPreviousChartPage<T>(currentKey: readonly unknown[]) {
  const currentScope = currentKey.slice(0, -1)
  return (
    previous: T | undefined,
    previousQuery: { queryKey: readonly unknown[] } | undefined
  ): T | undefined => {
    const previousKey = previousQuery?.queryKey
    if (!previousKey || previousKey.length !== currentKey.length) {
      return undefined
    }
    const sameScope = currentScope.every(
      (part, index) => part === previousKey[index]
    )
    return sameScope ? previous : undefined
  }
}

export function useMostActiveArtists(
  window: ChartWindow,
  limit = 7,
  { scene = '', enabled = true, offset = 0 }: ChartQueryOptions = {}
) {
  const queryKey = chartQueryKeys.mostActiveArtists(window, scene, limit, offset)
  return useQuery({
    queryKey,
    placeholderData: keepPreviousChartPage<MostActiveArtistsResponse>(queryKey),
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
  const queryKey = chartQueryKeys.onTheRadio(window, scene, limit, offset)
  return useQuery({
    queryKey,
    placeholderData: keepPreviousChartPage<OnTheRadioResponse>(queryKey),
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
  const queryKey = chartQueryKeys.mostAnticipated(window, scene, limit, offset)
  return useQuery({
    queryKey,
    placeholderData: keepPreviousChartPage<MostAnticipatedResponse>(queryKey),
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
  const queryKey = chartQueryKeys.busiestVenues(window, scene, limit, offset)
  return useQuery({
    queryKey,
    placeholderData: keepPreviousChartPage<BusiestVenuesResponse>(queryKey),
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
  const queryKey = chartQueryKeys.newReleases(window, scene, limit, offset)
  return useQuery({
    queryKey,
    placeholderData: keepPreviousChartPage<NewReleasesResponse>(queryKey),
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
  const queryKey = chartQueryKeys.openersToWatch(window, scene, limit, offset)
  return useQuery({
    queryKey,
    placeholderData: keepPreviousChartPage<OpenersToWatchResponse>(queryKey),
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
 * Live featured-collection pick for the Broadsheet card (PSY-1411 / PSY-1500):
 * the open run with the newest `featured_at`, or `{ featured: null }` when
 * nothing is currently featured. Folds into the masthead cache tier server-side.
 */
export function useFeaturedCollection(enabled = true) {
  return useQuery({
    queryKey: chartQueryKeys.featuredCollection,
    queryFn: () =>
      apiRequest<FeaturedCollectionResponse>(
        chartEndpoints.FEATURED_COLLECTION,
        { method: 'GET' }
      ),
    enabled,
  })
}

/**
 * Featured-collection picks archive (PSY-1500 / PSY-1501). Returns every
 * featuring stint newest-first, so the archive page can peel the newest run
 * off as the lead editorial card and render closed runs as the ledger. The
 * Broadsheet card (PSY-1411) uses it only to decide whether a closed run
 * exists (gating "previously featured →"; limit 100). Optional `enabled` lets
 * the card skip the request when nothing is featured.
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
