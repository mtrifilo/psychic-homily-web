'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys, createInvalidateQueries } from '../queryClient'
import type {
  ArtistReportResponse,
  MyArtistReportResponse,
  CreateArtistReportRequest,
} from '../types/artist'

/**
 * Hook to check if the current user has already reported an artist
 * Returns the user's report for the artist if one exists
 */
export const useMyArtistReport = (artistId: number | string | null) => {
  return useQuery({
    queryKey: queryKeys.artistReports.myReport(String(artistId)),
    queryFn: async (): Promise<MyArtistReportResponse> => {
      return apiRequest<MyArtistReportResponse>(
        API_ENDPOINTS.ARTISTS.MY_REPORT(artistId!),
        {
          method: 'GET',
        }
      )
    },
    enabled: Boolean(artistId),
    staleTime: 30 * 1000, // 30 seconds
  })
}

/**
 * Hook to report an artist issue
 * Creates a new report for the specified artist
 */
export const useReportArtist = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      artistId,
      reportType,
      details,
    }: {
      artistId: number
      reportType: CreateArtistReportRequest['report_type']
      details?: string
    }): Promise<ArtistReportResponse> => {
      return apiRequest<ArtistReportResponse>(
        API_ENDPOINTS.ARTISTS.REPORT(artistId),
        {
          method: 'POST',
          body: JSON.stringify({
            report_type: reportType,
            details: details || null,
          }),
        }
      )
    },
    onSuccess: (_data, { artistId }) => {
      // Invalidate the user's report check for this artist
      queryClient.invalidateQueries({
        queryKey: queryKeys.artistReports.myReport(String(artistId)),
      })
      // Invalidate admin reports list
      invalidateQueries.artistReports()
    },
  })
}
