'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys, createInvalidateQueries } from '../queryClient'
import type { Venue, UnverifiedVenuesResponse } from '../types/venue'

/**
 * Hook to fetch unverified venues (admin only)
 */
export function useUnverifiedVenues(options?: {
  limit?: number
  offset?: number
}) {
  const limit = options?.limit ?? 50
  const offset = options?.offset ?? 0

  return useQuery({
    queryKey: queryKeys.admin.unverifiedVenues(limit, offset),
    queryFn: async () => {
      const params = new URLSearchParams({
        limit: limit.toString(),
        offset: offset.toString(),
      })
      return apiRequest<UnverifiedVenuesResponse>(
        `${API_ENDPOINTS.ADMIN.VENUES.UNVERIFIED}?${params}`
      )
    },
    staleTime: 30 * 1000, // 30 seconds - shorter for admin data
  })
}

/**
 * Hook for verifying a venue (admin only)
 */
export function useVerifyVenue() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (venueId: number) => {
      return apiRequest<Venue>(API_ENDPOINTS.ADMIN.VENUES.VERIFY(venueId), {
        method: 'POST',
      })
    },
    onSuccess: () => {
      // Invalidate venue queries
      invalidateQueries.venues()
      // Also invalidate pending shows since venue verification status may have changed
      queryClient.invalidateQueries({ queryKey: ['admin', 'shows', 'pending'] })
      // Invalidate unverified venues list
      queryClient.invalidateQueries({ queryKey: ['admin', 'venues', 'unverified'] })
    },
  })
}
