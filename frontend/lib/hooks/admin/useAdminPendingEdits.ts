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
  entity_name?: string
  submitted_by: number
  submitter_name?: string
  /**
   * PSY-619: submitter's username when set, null otherwise. Pass to
   * `<UserAttribution username={...} />` to render the byline as a link to
   * /users/:username when non-null.
   */
  submitter_username: string | null
  field_changes: FieldChange[]
  summary: string
  /**
   * PSY-605: sanitised HTML of `summary` rendered server-side via the
   * shared MarkdownRenderer (goldmark + bluemonday, comment-system allowlist).
   * Render via `dangerouslySetInnerHTML` — the sanitiser is the source of
   * truth for XSS safety. Falls back to empty string for legacy rows; the
   * raw `summary` is still available alongside.
   */
  summary_html?: string
  status: 'pending' | 'approved' | 'rejected'
  reviewed_by?: number
  reviewer_name?: string
  reviewer_username?: string | null
  reviewed_at?: string
  rejection_reason?: string
  /**
   * PSY-605: sanitised HTML of `rejection_reason`. Same renderer + allowlist
   * as `summary_html`. Empty when no rejection reason has been written.
   */
  rejection_reason_html?: string
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
  /** When false, the query does not fire (e.g. the admin nav badge off-route). Defaults to true. */
  enabled?: boolean
}

// ─── Hooks ───────────────────────────────────────────────────────────────────

/**
 * Hook to fetch pending entity edits for admin review.
 */
export function useAdminPendingEdits(filters: PendingEditsFilters = {}) {
  const { status = 'pending', entity_type, limit = 50, offset = 0, enabled = true } = filters

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
    enabled,
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
