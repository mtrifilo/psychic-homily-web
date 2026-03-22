'use client'

/**
 * Labels Hooks
 *
 * TanStack Query hooks for fetching label data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { createDetailHook, createNamedDetailHook } from '@/lib/hooks/factories'
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
    ? `${API_ENDPOINTS.LABELS.LIST}?${queryString}`
    : API_ENDPOINTS.LABELS.LIST

  return useQuery({
    queryKey: queryKeys.labels.list(
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
  API_ENDPOINTS.LABELS.GET,
  queryKeys.labels.detail,
)

/**
 * Hook to fetch labels for a specific artist
 */
export const useArtistLabels = createNamedDetailHook<ArtistLabelsResponse, 'artistIdOrSlug'>(
  'artistIdOrSlug',
  API_ENDPOINTS.ARTISTS.LABELS,
  queryKeys.artists.labels,
)

/**
 * Hook to fetch artists on a label (roster)
 */
export const useLabelRoster = createNamedDetailHook<LabelArtistsResponse, 'labelIdOrSlug'>(
  'labelIdOrSlug',
  API_ENDPOINTS.LABELS.ARTISTS,
  queryKeys.labels.roster,
)

/**
 * Hook to fetch releases on a label (catalog)
 */
export const useLabelCatalog = createNamedDetailHook<LabelReleasesResponse, 'labelIdOrSlug'>(
  'labelIdOrSlug',
  API_ENDPOINTS.LABELS.RELEASES,
  queryKeys.labels.catalog,
)
