'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys } from '../queryClient'
import type { MySubmissionsResponse } from '../types/show'

interface UseMySubmissionsOptions {
  limit?: number
  offset?: number
  enabled?: boolean
}

/**
 * Hook to fetch user's submitted shows
 * Returns all shows the authenticated user has submitted
 * Requires authentication
 */
export const useMySubmissions = (options: UseMySubmissionsOptions = {}) => {
  const { limit = 50, offset = 0, enabled = true } = options

  const params = new URLSearchParams()
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())

  const endpoint = `${API_ENDPOINTS.SHOWS.MY_SUBMISSIONS}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.mySubmissions.list(),
    queryFn: async (): Promise<MySubmissionsResponse> => {
      return apiRequest<MySubmissionsResponse>(endpoint, {
        method: 'GET',
      })
    },
    enabled,
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}
