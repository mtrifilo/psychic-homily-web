'use client'

/**
 * Venue Edit Hooks
 *
 * TanStack Query hooks for venue editing operations.
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys, createInvalidateQueries } from '../queryClient'
import type {
  VenueEditRequest,
  UpdateVenueResponse,
  MyPendingEditResponse,
} from '../types/venue'

/**
 * Hook to update a venue
 * - Admin: Updates venue directly
 * - Non-admin: Creates pending edit if user owns the venue
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
      return apiRequest<UpdateVenueResponse>(
        API_ENDPOINTS.VENUES.UPDATE(venueId),
        {
          method: 'PUT',
          body: JSON.stringify(data),
        }
      )
    },
    onSuccess: (response, { venueId }) => {
      // Invalidate venue-related queries
      invalidateQueries.venues()

      // Invalidate user's pending edit for this venue
      queryClient.invalidateQueries({
        queryKey: queryKeys.venues.myPendingEdit(venueId),
      })

      // If admin approved, also invalidate admin pending edits
      if (response.status === 'updated') {
        queryClient.invalidateQueries({
          queryKey: ['admin', 'venues', 'pendingEdits'],
        })
      }
    },
  })
}

/**
 * Hook to get user's pending edit for a specific venue
 */
export function useMyPendingVenueEdit(venueId: number, enabled = true) {
  return useQuery({
    queryKey: queryKeys.venues.myPendingEdit(venueId),
    queryFn: async (): Promise<MyPendingEditResponse> => {
      return apiRequest<MyPendingEditResponse>(
        API_ENDPOINTS.VENUES.MY_PENDING_EDIT(venueId),
        {
          method: 'GET',
        }
      )
    },
    enabled: enabled && venueId > 0,
    staleTime: 30 * 1000, // 30 seconds
  })
}

/**
 * Hook to cancel a user's pending venue edit
 */
export function useCancelPendingVenueEdit() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (venueId: number) => {
      return apiRequest<{ message: string }>(
        API_ENDPOINTS.VENUES.MY_PENDING_EDIT(venueId),
        {
          method: 'DELETE',
        }
      )
    },
    onSuccess: (_, venueId) => {
      // Invalidate user's pending edit
      queryClient.invalidateQueries({
        queryKey: queryKeys.venues.myPendingEdit(venueId),
      })
    },
  })
}
