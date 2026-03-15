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
