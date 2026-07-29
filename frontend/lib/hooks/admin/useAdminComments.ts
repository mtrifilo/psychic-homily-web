'use client'

/**
 * Admin Comment Moderation Hooks
 *
 * TanStack Query hooks for admin comment moderation:
 * pending comment review, approve/reject/hide/restore actions.
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_BASE_URL } from '../../api'
import { commentQueryKeys } from '@/features/comments/api'

// ─── Types ───────────────────────────────────────────────────────────────────
//
// Aliased from the generated OpenAPI types, not hand-written (PSY-1550/1600).
// Regenerate with `bun run api:types`; the "API Types Drift" CI gate fails if
// the committed types drift from the backend. Exported names are kept stable
// for callers.
//
// The pending-comment queue returns the same `CommentResponse` the public
// comment endpoints do — there is no admin-specific comment shape. The
// hand-written `PendingComment` this replaces claimed two fields the wire has
// never carried, `entity_name` and `trust_tier`, so the moderation queue's
// entity link and trust-tier badge were dead UI (PSY-1600).

import type { components } from '../../../types/api'

export type PendingComment = components['schemas']['CommentResponse']
export type PendingCommentsResponse =
  components['schemas']['AdminListPendingCommentsResponseBody']
export type CommentEditHistoryEntry =
  components['schemas']['CommentEditHistoryEntry']
export type CommentEditHistoryResponse =
  components['schemas']['CommentEditHistoryResponse']

// ─── Query Keys ─────────────────────────────────────────────────────────────

export const adminCommentQueryKeys = {
  all: ['admin', 'comments'] as const,
  pending: (params?: Record<string, unknown>) =>
    ['admin', 'comments', 'pending', params] as const,
  edits: (commentId: number) =>
    ['admin', 'comments', 'edits', commentId] as const,
}

// ─── Endpoints ──────────────────────────────────────────────────────────────

const ADMIN_COMMENT_ENDPOINTS = {
  PENDING: `${API_BASE_URL}/admin/comments/pending`,
  APPROVE: (id: number) => `${API_BASE_URL}/admin/comments/${id}/approve`,
  REJECT: (id: number) => `${API_BASE_URL}/admin/comments/${id}/reject`,
  HIDE: (id: number) => `${API_BASE_URL}/admin/comments/${id}/hide`,
  RESTORE: (id: number) => `${API_BASE_URL}/admin/comments/${id}/restore`,
  EDITS: (id: number) => `${API_BASE_URL}/admin/comments/${id}/edits`,
}

// ─── Hooks ───────────────────────────────────────────────────────────────────

/**
 * Hook to fetch pending comments awaiting admin review.
 */
export function useAdminPendingComments(
  limit = 25,
  offset = 0,
  // When false, the query does not fire (e.g. the admin nav badge off-route). Defaults to true.
  options: { enabled?: boolean } = {}
) {
  const { enabled = true } = options
  const params = new URLSearchParams()
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())

  const endpoint = `${ADMIN_COMMENT_ENDPOINTS.PENDING}?${params.toString()}`

  return useQuery({
    queryKey: adminCommentQueryKeys.pending({ limit, offset }),
    queryFn: async (): Promise<PendingCommentsResponse> => {
      return apiRequest<PendingCommentsResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 30 * 1000, // 30 seconds
    enabled,
  })
}

/**
 * Hook to approve a pending comment.
 */
export function useAdminApproveComment() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (commentId: number): Promise<void> => {
      return apiRequest<void>(ADMIN_COMMENT_ENDPOINTS.APPROVE(commentId), {
        method: 'POST',
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: adminCommentQueryKeys.all })
      queryClient.invalidateQueries({ queryKey: commentQueryKeys.all })
    },
  })
}

/**
 * Hook to reject a pending comment.
 */
export function useAdminRejectComment() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      commentId,
      reason,
    }: {
      commentId: number
      reason: string
    }): Promise<void> => {
      return apiRequest<void>(ADMIN_COMMENT_ENDPOINTS.REJECT(commentId), {
        method: 'POST',
        body: JSON.stringify({ reason }),
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: adminCommentQueryKeys.all })
      queryClient.invalidateQueries({ queryKey: commentQueryKeys.all })
    },
  })
}

/**
 * Hook to hide a visible comment (moderation action).
 */
export function useAdminHideComment() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      commentId,
      reason,
    }: {
      commentId: number
      reason: string
    }): Promise<void> => {
      return apiRequest<void>(ADMIN_COMMENT_ENDPOINTS.HIDE(commentId), {
        method: 'POST',
        body: JSON.stringify({ reason }),
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: adminCommentQueryKeys.all })
      queryClient.invalidateQueries({ queryKey: commentQueryKeys.all })
      queryClient.invalidateQueries({ queryKey: ['admin', 'entityReports'] })
    },
  })
}

/**
 * Hook to restore a hidden comment.
 */
export function useAdminRestoreComment() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (commentId: number): Promise<void> => {
      return apiRequest<void>(ADMIN_COMMENT_ENDPOINTS.RESTORE(commentId), {
        method: 'POST',
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: adminCommentQueryKeys.all })
      queryClient.invalidateQueries({ queryKey: commentQueryKeys.all })
    },
  })
}

/**
 * Hook to fetch the edit history for a single comment. Admin-only (PSY-297).
 * Entries are returned oldest-first.
 *
 * @param commentId - The comment whose history to load.
 * @param enabled - When false (default), the query is not fired. Flip to true
 *                  when a viewer (modal/drawer) is opened so we don't
 *                  prefetch edit history for every comment on a page.
 */
export function useAdminCommentEditHistory(commentId: number, enabled = false) {
  return useQuery({
    queryKey: adminCommentQueryKeys.edits(commentId),
    queryFn: async (): Promise<CommentEditHistoryResponse> => {
      return apiRequest<CommentEditHistoryResponse>(
        ADMIN_COMMENT_ENDPOINTS.EDITS(commentId),
        { method: 'GET' }
      )
    },
    enabled: enabled && commentId > 0,
    staleTime: 60 * 1000,
  })
}
