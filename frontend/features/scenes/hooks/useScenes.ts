'use client'

/**
 * Scenes Hooks
 *
 * TanStack Query hooks for fetching scene data from the API.
 */

import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type {
  SceneListResponse,
  SceneDetail,
  SceneArtistsResponse,
  SceneGenreResponse,
  SceneGraphResponse,
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
 * Hook to fetch active artists for a scene
 */
export function useSceneArtists(options: UseSceneArtistsOptions) {
  const { slug, period = 90, limit = 20, offset = 0 } = options

  const params = new URLSearchParams()
  if (period) params.set('period', period.toString())
  if (limit) params.set('limit', limit.toString())
  if (offset) params.set('offset', offset.toString())

  const queryString = params.toString()
  const endpoint = queryString
    ? `${API_ENDPOINTS.SCENES.ARTISTS(slug)}?${queryString}`
    : API_ENDPOINTS.SCENES.ARTISTS(slug)

  return useQuery({
    queryKey: queryKeys.scenes.artists(slug, period),
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

interface UseSceneGraphOptions {
  slug: string
  types?: string[]
  enabled?: boolean
}

/**
 * Hook to fetch the scene-scale relationship graph (PSY-367).
 *
 * Cluster IDs are computed by the backend at query time from each artist's
 * most-frequent in-scene venue; the response is read-only (no vote data) and
 * includes derived `is_isolate` and `is_cross_cluster` flags so the frontend
 * doesn't have to recompute them every render.
 */
export function useSceneGraph(options: UseSceneGraphOptions) {
  const { slug, types, enabled = true } = options

  const params = new URLSearchParams()
  if (types && types.length > 0) {
    params.set('types', types.join(','))
  }
  const queryString = params.toString()
  const endpoint = queryString
    ? `${API_ENDPOINTS.SCENES.GRAPH(slug)}?${queryString}`
    : API_ENDPOINTS.SCENES.GRAPH(slug)

  return useQuery({
    queryKey: queryKeys.scenes.graph(slug, types),
    queryFn: async (): Promise<SceneGraphResponse> => {
      return apiRequest<SceneGraphResponse>(endpoint, { method: 'GET' })
    },
    enabled: enabled && Boolean(slug),
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}
