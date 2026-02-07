'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys } from '../queryClient'
import type { AuditLogsResponse } from '../types/audit'

interface UseAuditLogsOptions {
  limit?: number
  offset?: number
}

/**
 * Hook to fetch audit logs for admin review
 */
export const useAuditLogs = (options: UseAuditLogsOptions = {}) => {
  const { limit = 50, offset = 0 } = options

  const params = new URLSearchParams()
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())

  const endpoint = `${API_ENDPOINTS.ADMIN.AUDIT_LOGS.LIST}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.admin.auditLogs(limit, offset),
    queryFn: async (): Promise<AuditLogsResponse> => {
      return apiRequest<AuditLogsResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 30 * 1000, // 30 seconds
  })
}
