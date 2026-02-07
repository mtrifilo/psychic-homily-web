'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys } from '../queryClient'
import type { AdminDashboardStats } from '../types/adminStats'

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
