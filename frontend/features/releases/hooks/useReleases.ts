'use client'

/**
 * Releases Hooks
 *
 * TanStack Query hooks for fetching release data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { createDetailHook, createNamedDetailHook } from '@/lib/hooks/factories'
import { releaseEndpoints, releaseQueryKeys } from '@/features/releases/api'
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
    ? `${releaseEndpoints.LIST}?${queryString}`
    : releaseEndpoints.LIST

  return useQuery({
    queryKey: releaseQueryKeys.list(
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
  releaseEndpoints.GET,
  releaseQueryKeys.detail,
)

/**
 * Hook to fetch releases for a specific artist
 */
export const useArtistReleases = createNamedDetailHook<ArtistReleasesResponse, 'artistIdOrSlug'>(
  'artistIdOrSlug',
  releaseEndpoints.ARTIST_RELEASES,
  releaseQueryKeys.artistReleases,
)
