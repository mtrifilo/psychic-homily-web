'use client'

/**
 * Leaderboard Hook
 *
 * TanStack Query hook for fetching contributor leaderboard data.
 */

import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type { LeaderboardResponse, LeaderboardDimension, LeaderboardPeriod } from '../types'

/**
 * Hook to fetch the contributor leaderboard.
 */
export function useLeaderboard(
  dimension: LeaderboardDimension = 'overall',
  period: LeaderboardPeriod = 'all_time',
  limit?: number,
) {
  const params = new URLSearchParams()
  params.set('dimension', dimension)
  params.set('period', period)
  if (limit) {
    params.set('limit', String(limit))
  }

  return useQuery({
    queryKey: queryKeys.community.leaderboard(dimension, period, limit),
    queryFn: async (): Promise<LeaderboardResponse> => {
      return apiRequest<LeaderboardResponse>(
        `${API_ENDPOINTS.COMMUNITY.LEADERBOARD}?${params.toString()}`,
        { method: 'GET' },
      )
    },
    staleTime: 2 * 60 * 1000, // 2 minutes
  })
}
