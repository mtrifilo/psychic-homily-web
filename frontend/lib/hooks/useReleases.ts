'use client'

/**
 * Releases Hooks
 *
 * TanStack Query hooks for fetching release data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys } from '../queryClient'
import type {
  ReleaseDetail,
  ReleasesListResponse,
  ArtistReleasesResponse,
} from '../types/release'

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

interface UseReleaseOptions {
  idOrSlug: string | number
  enabled?: boolean
}

/**
 * Hook to fetch a single release by ID or slug
 */
export function useRelease(options: UseReleaseOptions) {
  const { idOrSlug, enabled = true } = options

  return useQuery({
    queryKey: queryKeys.releases.detail(idOrSlug),
    queryFn: async (): Promise<ReleaseDetail> => {
      return apiRequest<ReleaseDetail>(
        API_ENDPOINTS.RELEASES.GET(idOrSlug),
        { method: 'GET' }
      )
    },
    enabled:
      enabled &&
      (typeof idOrSlug === 'string' ? Boolean(idOrSlug) : idOrSlug > 0),
    staleTime: 5 * 60 * 1000,
  })
}

interface UseArtistReleasesOptions {
  artistIdOrSlug: string | number
  enabled?: boolean
}

/**
 * Hook to fetch releases for a specific artist
 */
export function useArtistReleases(options: UseArtistReleasesOptions) {
  const { artistIdOrSlug, enabled = true } = options

  return useQuery({
    queryKey: queryKeys.releases.artistReleases(artistIdOrSlug),
    queryFn: async (): Promise<ArtistReleasesResponse> => {
      return apiRequest<ArtistReleasesResponse>(
        API_ENDPOINTS.RELEASES.ARTIST_RELEASES(artistIdOrSlug),
        { method: 'GET' }
      )
    },
    enabled:
      enabled &&
      (typeof artistIdOrSlug === 'string'
        ? Boolean(artistIdOrSlug)
        : artistIdOrSlug > 0),
    staleTime: 5 * 60 * 1000,
  })
}
