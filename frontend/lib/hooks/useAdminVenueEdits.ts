'use client'

/**
 * Admin Venue Edit Hooks
 *
 * TanStack Query hooks for admin venue edit management.
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys, createInvalidateQueries } from '../queryClient'
import type {
  PendingVenueEditsResponse,
  PendingVenueEdit,
  Venue,
} from '../types/venue'

/**
 * Hook to fetch pending venue edits (admin only)
 */
export function usePendingVenueEdits(options?: {
  limit?: number
  offset?: number
}) {
  const limit = options?.limit ?? 50
  const offset = options?.offset ?? 0

  return useQuery({
    queryKey: queryKeys.admin.pendingVenueEdits(limit, offset),
    queryFn: async () => {
      const params = new URLSearchParams({
        limit: limit.toString(),
        offset: offset.toString(),
      })
      return apiRequest<PendingVenueEditsResponse>(
        `${API_ENDPOINTS.ADMIN.VENUES.PENDING_EDITS}?${params}`
      )
    },
    staleTime: 30 * 1000, // 30 seconds - shorter for admin data
  })
}

/**
 * Hook to approve a pending venue edit (admin only)
 */
export function useApproveVenueEdit() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (editId: number) => {
      return apiRequest<Venue>(API_ENDPOINTS.ADMIN.VENUES.APPROVE_EDIT(editId), {
        method: 'POST',
      })
    },
    onSuccess: () => {
      // Invalidate pending edits list
      queryClient.invalidateQueries({
        queryKey: ['admin', 'venues', 'pendingEdits'],
      })
      // Invalidate venues since one was updated
      invalidateQueries.venues()
    },
  })
}

/**
 * Hook to reject a pending venue edit (admin only)
 */
export function useRejectVenueEdit() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      editId,
      reason,
    }: {
      editId: number
      reason: string
    }) => {
      return apiRequest<PendingVenueEdit>(
        API_ENDPOINTS.ADMIN.VENUES.REJECT_EDIT(editId),
        {
          method: 'POST',
          body: JSON.stringify({ reason }),
        }
      )
    },
    onSuccess: () => {
      // Invalidate pending edits list
      queryClient.invalidateQueries({
        queryKey: ['admin', 'venues', 'pendingEdits'],
      })
    },
  })
}
