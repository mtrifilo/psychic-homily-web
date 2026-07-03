'use client'

/**
 * Scenes Hooks
 *
 * TanStack Query hooks for fetching scene data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type {
  SceneListResponse,
  SceneDetail,
  SceneArtistsResponse,
  SceneGenreResponse,
  SceneGraphResponse,
  SceneShowsResponse,
} from '../types'

/**
 * Hook to fetch the list of scenes (cities meeting the threshold)
 */
export function useScenes() {
  return useQuery({
    queryKey: queryKeys.scenes.list,
    queryFn: async (): Promise<SceneListResponse> => {
      return apiRequest<SceneListResponse>(API_ENDPOINTS.SCENES.LIST, {
        method: 'GET',
      })
    },
    staleTime: 10 * 60 * 1000, // 10 minutes — scenes don't change often
  })
}

/**
 * Hook to fetch detail for a single scene by slug
 */
export function useSceneDetail(slug: string) {
  return useQuery({
    queryKey: queryKeys.scenes.detail(slug),
    queryFn: async (): Promise<SceneDetail> => {
      return apiRequest<SceneDetail>(API_ENDPOINTS.SCENES.DETAIL(slug), {
        method: 'GET',
      })
    },
    enabled: Boolean(slug),
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

interface UseSceneArtistsOptions {
  slug: string
  period?: number
  limit?: number
  offset?: number
}

/**
 * Hook to fetch a scene's roster — the bands BASED in the metro (PSY-1255 step C).
 * Each artist carries `is_active`; the endpoint returns the whole roster, active
 * ones first, not just the active subset. `period` overrides the active window
 * (days); when omitted, the backend's default (~6 months) applies — do NOT
 * re-default it here, or the FE-sent window would contradict that model.
 */
export function useSceneArtists(options: UseSceneArtistsOptions) {
  const { slug, period, limit = 20, offset = 0 } = options

  const params = new URLSearchParams()
  if (period) params.set('period', period.toString())
  if (limit) params.set('limit', limit.toString())
  if (offset) params.set('offset', offset.toString())

  const queryString = params.toString()
  const endpoint = queryString
    ? `${API_ENDPOINTS.SCENES.ARTISTS(slug)}?${queryString}`
    : API_ENDPOINTS.SCENES.ARTISTS(slug)

  return useQuery({
    // `limit` is part of the key: different limits for the same slug+period are
    // distinct results (e.g. the /atlas preview's 6 vs scene-detail's 10), so
    // they must not share a cache entry.
    queryKey: queryKeys.scenes.artists(slug, period, limit),
    queryFn: async (): Promise<SceneArtistsResponse> => {
      return apiRequest<SceneArtistsResponse>(endpoint, {
        method: 'GET',
      })
    },
    enabled: Boolean(slug),
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * Hook to fetch a scene's next upcoming shows — the preview panel's "This week"
 * row (PSY-1309). Backend defaults: 7-day window, 3 shows, soonest first;
 * metro-scoped so member-city shows count (a Tempe show shows under Phoenix).
 * Don't re-default the window here — the backend owns it (same rule as
 * useSceneArtists' period).
 */
export function useSceneShows(slug: string) {
  return useQuery({
    queryKey: queryKeys.scenes.shows(slug),
    queryFn: async (): Promise<SceneShowsResponse> => {
      return apiRequest<SceneShowsResponse>(API_ENDPOINTS.SCENES.SHOWS(slug), {
        method: 'GET',
      })
    },
    enabled: Boolean(slug),
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * Hook to fetch genre distribution for a scene
 */
export function useSceneGenres(slug: string) {
  return useQuery({
    queryKey: queryKeys.scenes.genres(slug),
    queryFn: async (): Promise<SceneGenreResponse> => {
      return apiRequest<SceneGenreResponse>(API_ENDPOINTS.SCENES.GENRES(slug), {
        method: 'GET',
      })
    },
    enabled: Boolean(slug),
    staleTime: 10 * 60 * 1000, // 10 minutes — genre data changes infrequently
  })
}

export type SceneGraphClusterBy = 'venue' | 'community'

interface UseSceneGraphOptions {
  slug: string
  types?: string[]
  clusterBy?: SceneGraphClusterBy
  enabled?: boolean
}

/**
 * Hook to fetch the scene-scale relationship graph (PSY-367).
 *
 * Cluster IDs are computed by the backend at query time — per artist's
 * most-frequent in-scene venue (`cluster_by=venue`, the default) or the
 * persisted Leiden community partition (`cluster_by=community`, PSY-1262/
 * PSY-1320). The response is read-only (no vote data) and includes derived
 * `is_isolate` and `is_cross_cluster` flags so the frontend doesn't have to
 * recompute them every render.
 *
 * `placeholderData: keepPreviousData` is load-bearing, not a nicety: the
 * cluster-by toggle changes the query key while the fullscreen overlay can be
 * open, and `useFullscreenGraphOverlay`'s `available` contract requires data
 * not to transiently disappear mid-fetch (the venue bill network's in-overlay
 * filter kick-out, PSY-1305).
 */
export function useSceneGraph(options: UseSceneGraphOptions) {
  const { slug, types, clusterBy, enabled = true } = options

  const params = new URLSearchParams()
  if (types && types.length > 0) {
    params.set('types', types.join(','))
  }
  // Only send cluster_by when it deviates from the backend default (venue),
  // keeping the common request shape unchanged.
  if (clusterBy && clusterBy !== 'venue') {
    params.set('cluster_by', clusterBy)
  }
  const queryString = params.toString()
  const endpoint = queryString
    ? `${API_ENDPOINTS.SCENES.GRAPH(slug)}?${queryString}`
    : API_ENDPOINTS.SCENES.GRAPH(slug)

  return useQuery({
    queryKey: queryKeys.scenes.graph(slug, types, clusterBy),
    queryFn: async (): Promise<SceneGraphResponse> => {
      return apiRequest<SceneGraphResponse>(endpoint, { method: 'GET' })
    },
    enabled: enabled && Boolean(slug),
    staleTime: 5 * 60 * 1000, // 5 minutes
    placeholderData: keepPreviousData,
  })
}
