'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys } from '../queryClient'
import type { AdminUsersResponse } from '../types/user'

interface UseAdminUsersOptions {
  limit?: number
  offset?: number
  search?: string
}

/**
 * Hook to fetch users for admin review
 */
export const useAdminUsers = (options: UseAdminUsersOptions = {}) => {
  const { limit = 50, offset = 0, search = '' } = options

  const params = new URLSearchParams()
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())
  if (search) {
    params.set('search', search)
  }

  const endpoint = `${API_ENDPOINTS.ADMIN.USERS.LIST}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.admin.users(limit, offset, search),
    queryFn: async (): Promise<AdminUsersResponse> => {
      return apiRequest<AdminUsersResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 30 * 1000, // 30 seconds
  })
}
