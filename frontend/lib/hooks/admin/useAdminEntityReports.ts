'use client'

/**
 * Admin Entity Report Hooks
 *
 * TanStack Query hooks for the unified moderation queue — entity reports.
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../../api'
import { queryKeys, createInvalidateQueries } from '../../queryClient'

// ─── Types ───────────────────────────────────────────────────────────────────

export interface EntityReportResponse {
  id: number
  entity_type: string
  entity_id: number
  reported_by: number
  reporter_name?: string
  report_type: string
  details?: string
  status: string
  admin_notes?: string
  reviewed_by?: number
  reviewer_name?: string
  reviewed_at?: string
  created_at: string
}

export interface EntityReportsListResponse {
  reports: EntityReportResponse[]
  total: number
}

// ─── Filters ─────────────────────────────────────────────────────────────────

export interface EntityReportFilters {
  status?: string
  entity_type?: string
  limit?: number
  offset?: number
}

// ─── Hooks ───────────────────────────────────────────────────────────────────

/**
 * Hook to fetch entity reports for admin review.
 */
export function useAdminEntityReports(filters: EntityReportFilters = {}) {
  const { status = 'pending', entity_type, limit = 50, offset = 0 } = filters

  const params = new URLSearchParams()
  if (status) params.set('status', status)
  if (entity_type) params.set('entity_type', entity_type)
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())

  const endpoint = `${API_ENDPOINTS.ADMIN.ENTITY_REPORTS.LIST}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.admin.entityReports({ status, entity_type, limit, offset }),
    queryFn: async (): Promise<EntityReportsListResponse> => {
      return apiRequest<EntityReportsListResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 30 * 1000, // 30 seconds
  })
}

/**
 * Hook to resolve an entity report.
 */
export function useResolveEntityReport() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      reportId,
      notes,
    }: {
      reportId: number
      notes?: string
    }): Promise<EntityReportResponse> => {
      return apiRequest<EntityReportResponse>(
        API_ENDPOINTS.ADMIN.ENTITY_REPORTS.RESOLVE(reportId),
        {
          method: 'POST',
          body: JSON.stringify({ notes: notes || '' }),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.adminEntityReports()
    },
  })
}

/**
 * Hook to dismiss an entity report.
 */
export function useDismissEntityReport() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      reportId,
      notes,
    }: {
      reportId: number
      notes?: string
    }): Promise<EntityReportResponse> => {
      return apiRequest<EntityReportResponse>(
        API_ENDPOINTS.ADMIN.ENTITY_REPORTS.DISMISS(reportId),
        {
          method: 'POST',
          body: JSON.stringify({ notes: notes || '' }),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.adminEntityReports()
    },
  })
}
