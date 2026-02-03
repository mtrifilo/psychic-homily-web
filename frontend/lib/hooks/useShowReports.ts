'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys, createInvalidateQueries } from '../queryClient'
import type {
  ShowReportResponse,
  MyShowReportResponse,
  CreateShowReportRequest,
} from '../types/show'

/**
 * Hook to check if the current user has already reported a show
 * Returns the user's report for the show if one exists
 */
export const useMyShowReport = (showId: number | string | null) => {
  return useQuery({
    queryKey: queryKeys.showReports.myReport(String(showId)),
    queryFn: async (): Promise<MyShowReportResponse> => {
      return apiRequest<MyShowReportResponse>(
        API_ENDPOINTS.SHOWS.MY_REPORT(showId!),
        {
          method: 'GET',
        }
      )
    },
    enabled: Boolean(showId),
    staleTime: 30 * 1000, // 30 seconds
  })
}

/**
 * Hook to report a show issue
 * Creates a new report for the specified show
 */
export const useReportShow = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      showId,
      reportType,
      details,
    }: {
      showId: number
      reportType: CreateShowReportRequest['report_type']
      details?: string
    }): Promise<ShowReportResponse> => {
      return apiRequest<ShowReportResponse>(
        API_ENDPOINTS.SHOWS.REPORT(showId),
        {
          method: 'POST',
          body: JSON.stringify({
            report_type: reportType,
            details: details || null,
          }),
        }
      )
    },
    onSuccess: (_data, { showId }) => {
      // Invalidate the user's report check for this show
      queryClient.invalidateQueries({
        queryKey: queryKeys.showReports.myReport(String(showId)),
      })
      // Invalidate admin reports list (if user is admin viewing their own reports)
      invalidateQueries.showReports()
    },
  })
}
