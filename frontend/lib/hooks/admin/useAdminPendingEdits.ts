'use client'

/**
 * Admin Pending Entity Edit Hooks
 *
 * TanStack Query hooks for the unified moderation queue — pending entity edits.
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../../api'
import { queryKeys, createInvalidateQueries } from '../../queryClient'

// ─── Types ───────────────────────────────────────────────────────────────────

import type { FieldChange } from '../common/useRevisions'

export type { FieldChange }

export interface PendingEditResponse {
  id: number
  entity_type: string
  entity_id: number
  submitted_by: number
  submitter_name?: string
  field_changes: FieldChange[]
  summary: string
  status: 'pending' | 'approved' | 'rejected'
  reviewed_by?: number
  reviewer_name?: string
  reviewed_at?: string
  rejection_reason?: string
  created_at: string
  updated_at: string
}

export interface PendingEditsListResponse {
  edits: PendingEditResponse[]
  total: number
}

// ─── Filters ─────────────────────────────────────────────────────────────────

export interface PendingEditsFilters {
  status?: string
  entity_type?: string
  limit?: number
  offset?: number
}

// ─── Hooks ───────────────────────────────────────────────────────────────────

/**
 * Hook to fetch pending entity edits for admin review.
 */
export function useAdminPendingEdits(filters: PendingEditsFilters = {}) {
  const { status = 'pending', entity_type, limit = 50, offset = 0 } = filters

  const params = new URLSearchParams()
  if (status) params.set('status', status)
  if (entity_type) params.set('entity_type', entity_type)
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())

  const endpoint = `${API_ENDPOINTS.ADMIN.PENDING_EDITS.LIST}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.admin.pendingEdits({ status, entity_type, limit, offset }),
    queryFn: async (): Promise<PendingEditsListResponse> => {
      return apiRequest<PendingEditsListResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 30 * 1000, // 30 seconds
  })
}

/**
 * Hook to approve a pending entity edit.
 */
export function useApprovePendingEdit() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (editId: number): Promise<PendingEditResponse> => {
      return apiRequest<PendingEditResponse>(
        API_ENDPOINTS.ADMIN.PENDING_EDITS.APPROVE(editId),
        { method: 'POST' }
      )
    },
    onSuccess: () => {
      invalidateQueries.adminPendingEdits()
      // Also invalidate related entity queries since data changed
      invalidateQueries.artists()
      invalidateQueries.venues()
      invalidateQueries.festivals()
    },
  })
}

/**
 * Hook to reject a pending entity edit.
 */
export function useRejectPendingEdit() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      editId,
      reason,
    }: {
      editId: number
      reason: string
    }): Promise<PendingEditResponse> => {
      return apiRequest<PendingEditResponse>(
        API_ENDPOINTS.ADMIN.PENDING_EDITS.REJECT(editId),
        {
          method: 'POST',
          body: JSON.stringify({ reason }),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.adminPendingEdits()
    },
  })
}
