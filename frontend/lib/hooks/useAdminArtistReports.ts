'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys, createInvalidateQueries } from '../queryClient'
import type {
  ArtistReportResponse,
  ArtistReportsListResponse,
} from '../types/artist'
import type { AdminReportActionRequest } from '../types/show'

interface UsePendingArtistReportsOptions {
  limit?: number
  offset?: number
}

/**
 * Hook to fetch pending artist reports for admin review
 */
export const usePendingArtistReports = (
  options: UsePendingArtistReportsOptions = {}
) => {
  const { limit = 50, offset = 0 } = options

  const params = new URLSearchParams()
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())

  const endpoint = `${API_ENDPOINTS.ADMIN.ARTIST_REPORTS.LIST}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.artistReports.pending(limit, offset),
    queryFn: async (): Promise<ArtistReportsListResponse> => {
      return apiRequest<ArtistReportsListResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 30 * 1000, // 30 seconds
  })
}

/**
 * Hook to dismiss an artist report (mark as spam/invalid)
 */
export const useDismissArtistReport = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      reportId,
      notes,
    }: {
      reportId: number
      notes?: string
    }): Promise<ArtistReportResponse> => {
      const body: AdminReportActionRequest = {}
      if (notes) body.notes = notes

      return apiRequest<ArtistReportResponse>(
        API_ENDPOINTS.ADMIN.ARTIST_REPORTS.DISMISS(reportId),
        {
          method: 'POST',
          body: JSON.stringify(body),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.artistReports()
    },
  })
}

/**
 * Hook to resolve an artist report (mark as action taken)
 */
export const useResolveArtistReport = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      reportId,
      notes,
    }: {
      reportId: number
      notes?: string
    }): Promise<ArtistReportResponse> => {
      const body: AdminReportActionRequest = {}
      if (notes) body.notes = notes

      return apiRequest<ArtistReportResponse>(
        API_ENDPOINTS.ADMIN.ARTIST_REPORTS.RESOLVE(reportId),
        {
          method: 'POST',
          body: JSON.stringify(body),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.artistReports()
    },
  })
}
