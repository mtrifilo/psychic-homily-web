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
//
// Aliased from the generated OpenAPI types, not hand-written (PSY-1550/1600).
// Regenerate with `bun run api:types`; the "API Types Drift" CI gate fails if
// the committed types drift from the backend. Exported names are kept stable
// for callers.
//
// `summary_html` / `rejection_reason_html` are server-sanitised HTML
// (MarkdownRenderer = goldmark + bluemonday); render via
// `dangerouslySetInnerHTML` — the sanitiser is the source of truth for XSS
// safety, and legacy rows omit them entirely.
// `field_changes` is nullable on the wire: guard before iterating (PSY-1600).

import type { components } from '../../../types/api'
import type { FieldChange } from '../common/useRevisions'

export type { FieldChange }

export type PendingEditResponse = components['schemas']['PendingEditResponse']
export type PendingEditsListResponse =
  components['schemas']['AdminListPendingEditsResponseBody']

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
