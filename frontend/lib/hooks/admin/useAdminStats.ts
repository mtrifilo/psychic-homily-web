'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../../api'
import { queryKeys } from '../../queryClient'
import type { AdminDashboardStats, ActivityEvent } from '../../types/adminStats'

/**
 * Hook to fetch admin dashboard statistics
 */
export const useAdminStats = () => {
  return useQuery({
    queryKey: queryKeys.admin.stats,
    queryFn: async (): Promise<AdminDashboardStats> => {
      return apiRequest<AdminDashboardStats>(API_ENDPOINTS.ADMIN.STATS, {
        method: 'GET',
      })
    },
    staleTime: 30 * 1000, // 30 seconds
  })
}

/**
 * Hook to fetch admin activity feed (recent audit log events)
 */
export const useAdminActivity = () => {
  return useQuery({
    queryKey: queryKeys.admin.activity,
    queryFn: async (): Promise<{ events: ActivityEvent[] }> => {
      return apiRequest<{ events: ActivityEvent[] }>(API_ENDPOINTS.ADMIN.ACTIVITY, {
        method: 'GET',
      })
    },
    staleTime: 30 * 1000, // 30 seconds
  })
}
