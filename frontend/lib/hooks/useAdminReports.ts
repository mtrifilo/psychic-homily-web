'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys, createInvalidateQueries } from '../queryClient'
import type {
  ShowReportResponse,
  ShowReportsListResponse,
  AdminReportActionRequest,
  ResolveReportRequest,
} from '../types/show'

interface UsePendingReportsOptions {
  limit?: number
  offset?: number
}

/**
 * Hook to fetch pending show reports for admin review
 */
export const usePendingReports = (options: UsePendingReportsOptions = {}) => {
  const { limit = 50, offset = 0 } = options

  const params = new URLSearchParams()
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())

  const endpoint = `${API_ENDPOINTS.ADMIN.REPORTS.LIST}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.showReports.pending(limit, offset),
    queryFn: async (): Promise<ShowReportsListResponse> => {
      return apiRequest<ShowReportsListResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 30 * 1000, // 30 seconds
  })
}

/**
 * Hook to dismiss a show report (mark as spam/invalid)
 */
export const useDismissReport = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      reportId,
      notes,
    }: {
      reportId: number
      notes?: string
    }): Promise<ShowReportResponse> => {
      const body: AdminReportActionRequest = {}
      if (notes) body.notes = notes

      return apiRequest<ShowReportResponse>(
        API_ENDPOINTS.ADMIN.REPORTS.DISMISS(reportId),
        {
          method: 'POST',
          body: JSON.stringify(body),
        }
      )
    },
    onSuccess: () => {
      // Invalidate all show reports queries
      invalidateQueries.showReports()
    },
  })
}

/**
 * Hook to resolve a show report (mark as action taken)
 * Optionally sets the corresponding show flag (is_cancelled or is_sold_out)
 */
export const useResolveReport = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      reportId,
      notes,
      setShowFlag,
    }: {
      reportId: number
      notes?: string
      setShowFlag?: boolean
    }): Promise<ShowReportResponse> => {
      const body: ResolveReportRequest = {
        set_show_flag: setShowFlag ?? false,
      }
      if (notes) body.notes = notes

      return apiRequest<ShowReportResponse>(
        API_ENDPOINTS.ADMIN.REPORTS.RESOLVE(reportId),
        {
          method: 'POST',
          body: JSON.stringify(body),
        }
      )
    },
    onSuccess: () => {
      // Invalidate all show reports queries and shows (since flag may have changed)
      invalidateQueries.showReports()
      invalidateQueries.shows()
    },
  })
}
