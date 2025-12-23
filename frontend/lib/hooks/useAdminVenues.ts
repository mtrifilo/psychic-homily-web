'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { createInvalidateQueries } from '../queryClient'
import type { Venue } from '../types/venue'

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
    },
  })
}
