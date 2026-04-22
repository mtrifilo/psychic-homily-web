'use client'

/**
 * Venue Edit Hooks
 *
 * Admin-only direct update + delete. Non-admin users go through the unified
 * suggest-edit flow (EntityEditDrawer / useSuggestEdit), not these hooks —
 * the legacy pending_venue_edits queue was retired in PSY-503.
 */

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { createInvalidateQueries } from '@/lib/queryClient'
import { venueEndpoints } from '@/features/venues/api'
import type { Venue, VenueEditRequest } from '../types'

/**
 * Admin-only: PUT /venues/{id} — direct venue update.
 * Non-admin users must use the unified suggest-edit endpoint via
 * `useSuggestEdit({ entityType: 'venue', ... })` instead; this hook
 * will return 403 for them.
 */
export function useVenueUpdate() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      venueId,
      data,
    }: {
      venueId: number
      data: VenueEditRequest
    }) => {
      return apiRequest<Venue>(
        venueEndpoints.UPDATE(venueId),
        {
          method: 'PUT',
          body: JSON.stringify(data),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.venues()
    },
  })
}

/**
 * DELETE /venues/{id}
 * - Admin: can delete any venue
 * - Non-admin: can delete venues they submitted
 * - Constraint: venues with associated shows cannot be deleted
 */
export function useVenueDelete() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (venueId: number): Promise<void> => {
      await apiRequest(venueEndpoints.DELETE(venueId), {
        method: 'DELETE',
      })
    },
    onSuccess: () => {
      invalidateQueries.venues()
    },
  })
}
