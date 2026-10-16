'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../../api'
import { queryKeys } from '../../queryClient'
import { refetchAfterMutation } from '@/lib/refetchAfterMutation'
import type { Venue, UnverifiedVenuesResponse } from '@/features/venues'

/**
 * Hook to fetch unverified venues (admin only)
 */
export function useUnverifiedVenues(options?: {
  limit?: number
  offset?: number
  enabled?: boolean
}) {
  const limit = options?.limit ?? 50
  const offset = options?.offset ?? 0
  const enabled = options?.enabled ?? true

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
    enabled,
    staleTime: 30 * 1000, // 30 seconds - shorter for admin data
  })
}

/**
 * Hook for verifying a venue (admin only)
 */
export function useVerifyVenue() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (venueId: number) => {
      return apiRequest<Venue>(API_ENDPOINTS.ADMIN.VENUES.VERIFY(venueId), {
        method: 'POST',
      })
    },
    onSuccess: () => {
      // The venue itself; pending shows, which display their venue's
      // verification status; and the unverified list, which loses this venue
      void refetchAfterMutation(
        queryClient,
        queryKeys.venues.all,
        ['admin', 'shows', 'pending'],
        ['admin', 'venues', 'unverified']
      )
    },
  })
}
