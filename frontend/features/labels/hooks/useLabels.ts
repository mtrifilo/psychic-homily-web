'use client'

/**
 * Labels Hooks
 *
 * TanStack Query hooks for fetching label data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
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

interface UseLabelOptions {
  idOrSlug: string | number
  enabled?: boolean
}

/**
 * Hook to fetch a single label by ID or slug
 */
export function useLabel(options: UseLabelOptions) {
  const { idOrSlug, enabled = true } = options

  return useQuery({
    queryKey: queryKeys.labels.detail(idOrSlug),
    queryFn: async (): Promise<LabelDetail> => {
      return apiRequest<LabelDetail>(
        API_ENDPOINTS.LABELS.GET(idOrSlug),
        { method: 'GET' }
      )
    },
    enabled:
      enabled &&
      (typeof idOrSlug === 'string' ? Boolean(idOrSlug) : idOrSlug > 0),
    staleTime: 5 * 60 * 1000,
  })
}

interface UseArtistLabelsOptions {
  artistIdOrSlug: string | number
  enabled?: boolean
}

/**
 * Hook to fetch labels for a specific artist
 */
export function useArtistLabels(options: UseArtistLabelsOptions) {
  const { artistIdOrSlug, enabled = true } = options

  return useQuery({
    queryKey: queryKeys.artists.labels(artistIdOrSlug),
    queryFn: async (): Promise<ArtistLabelsResponse> => {
      return apiRequest<ArtistLabelsResponse>(
        API_ENDPOINTS.ARTISTS.LABELS(artistIdOrSlug),
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

interface UseLabelRosterOptions {
  labelIdOrSlug: string | number
  enabled?: boolean
}

/**
 * Hook to fetch artists on a label (roster)
 */
export function useLabelRoster(options: UseLabelRosterOptions) {
  const { labelIdOrSlug, enabled = true } = options

  return useQuery({
    queryKey: queryKeys.labels.roster(labelIdOrSlug),
    queryFn: async (): Promise<LabelArtistsResponse> => {
      return apiRequest<LabelArtistsResponse>(
        API_ENDPOINTS.LABELS.ARTISTS(labelIdOrSlug),
        { method: 'GET' }
      )
    },
    enabled:
      enabled &&
      (typeof labelIdOrSlug === 'string'
        ? Boolean(labelIdOrSlug)
        : labelIdOrSlug > 0),
    staleTime: 5 * 60 * 1000,
  })
}

interface UseLabelCatalogOptions {
  labelIdOrSlug: string | number
  enabled?: boolean
}

/**
 * Hook to fetch releases on a label (catalog)
 */
export function useLabelCatalog(options: UseLabelCatalogOptions) {
  const { labelIdOrSlug, enabled = true } = options

  return useQuery({
    queryKey: queryKeys.labels.catalog(labelIdOrSlug),
    queryFn: async (): Promise<LabelReleasesResponse> => {
      return apiRequest<LabelReleasesResponse>(
        API_ENDPOINTS.LABELS.RELEASES(labelIdOrSlug),
        { method: 'GET' }
      )
    },
    enabled:
      enabled &&
      (typeof labelIdOrSlug === 'string'
        ? Boolean(labelIdOrSlug)
        : labelIdOrSlug > 0),
    staleTime: 5 * 60 * 1000,
  })
}
