'use client'

/**
 * Labels Hooks
 *
 * TanStack Query hooks for fetching label data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { createDetailHook, createNamedDetailHook } from '@/lib/hooks/factories'
import { labelEndpoints, labelQueryKeys } from '@/features/labels/api'
import { artistEndpoints, artistQueryKeys } from '@/features/artists/api'
import type {
  LabelsListResponse,
  LabelDetail,
  LabelArtistsResponse,
  LabelReleasesResponse,
  ArtistLabelsResponse,
} from '../types'

interface UseLabelsOptions {
  status?: string
  city?: string
  state?: string
}

/**
 * Hook to fetch list of labels with optional filtering
 */
export function useLabels(options: UseLabelsOptions = {}) {
  const { status, city, state } = options

  const params = new URLSearchParams()
  if (status) params.set('status', status)
  if (city) params.set('city', city)
  if (state) params.set('state', state)

  const queryString = params.toString()
  const endpoint = queryString
    ? `${labelEndpoints.LIST}?${queryString}`
    : labelEndpoints.LIST

  return useQuery({
    queryKey: labelQueryKeys.list(
      status || city || state
        ? { status, city, state }
        : undefined
    ),
    queryFn: async (): Promise<LabelsListResponse> => {
      return apiRequest<LabelsListResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 5 * 60 * 1000,
    placeholderData: keepPreviousData,
  })
}

/**
 * Hook to fetch a single label by ID or slug
 */
export const useLabel = createDetailHook<LabelDetail>(
  labelEndpoints.GET,
  labelQueryKeys.detail,
)

/**
 * Hook to fetch labels for a specific artist
 */
export const useArtistLabels = createNamedDetailHook<ArtistLabelsResponse, 'artistIdOrSlug'>(
  'artistIdOrSlug',
  artistEndpoints.LABELS,
  artistQueryKeys.labels,
)

/**
 * Hook to fetch artists on a label (roster)
 */
export const useLabelRoster = createNamedDetailHook<LabelArtistsResponse, 'labelIdOrSlug'>(
  'labelIdOrSlug',
  labelEndpoints.ARTISTS,
  labelQueryKeys.roster,
)

/**
 * Hook to fetch releases on a label (catalog)
 */
export const useLabelCatalog = createNamedDetailHook<LabelReleasesResponse, 'labelIdOrSlug'>(
  'labelIdOrSlug',
  labelEndpoints.RELEASES,
  labelQueryKeys.catalog,
)
