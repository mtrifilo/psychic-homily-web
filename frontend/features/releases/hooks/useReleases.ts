'use client'

/**
 * Releases Hooks
 *
 * TanStack Query hooks for fetching release data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { createDetailHook, createNamedDetailHook } from '@/lib/hooks/factories'
import type {
  ReleaseDetail,
  ReleasesListResponse,
  ArtistReleasesResponse,
} from '../types'

interface UseReleasesOptions {
  releaseType?: string
  year?: number
  artistId?: string | number
}

/**
 * Hook to fetch list of releases with optional filtering
 */
export function useReleases(options: UseReleasesOptions = {}) {
  const { releaseType, year, artistId } = options

  const params = new URLSearchParams()
  if (releaseType) params.set('release_type', releaseType)
  if (year) params.set('year', year.toString())
  if (artistId) params.set('artist_id', artistId.toString())

  const queryString = params.toString()
  const endpoint = queryString
    ? `${API_ENDPOINTS.RELEASES.LIST}?${queryString}`
    : API_ENDPOINTS.RELEASES.LIST

  return useQuery({
    queryKey: queryKeys.releases.list(
      releaseType || year || artistId
        ? { releaseType, year, artistId }
        : undefined
    ),
    queryFn: async (): Promise<ReleasesListResponse> => {
      return apiRequest<ReleasesListResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 5 * 60 * 1000,
    placeholderData: keepPreviousData,
  })
}

/**
 * Hook to fetch a single release by ID or slug
 */
export const useRelease = createDetailHook<ReleaseDetail>(
  API_ENDPOINTS.RELEASES.GET,
  queryKeys.releases.detail,
)

/**
 * Hook to fetch releases for a specific artist
 */
export const useArtistReleases = createNamedDetailHook<ArtistReleasesResponse, 'artistIdOrSlug'>(
  'artistIdOrSlug',
  API_ENDPOINTS.RELEASES.ARTIST_RELEASES,
  queryKeys.releases.artistReleases,
)
