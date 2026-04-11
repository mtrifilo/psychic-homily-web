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

export interface PendingComment {
  id: number
  entity_type: string
  entity_id: number
  entity_name?: string
  user_id: number
  author_name: string
  body: string
  body_html: string
  parent_id: number | null
  depth: number
  visibility: string
  trust_tier?: string
  created_at: string
  updated_at: string
}

export interface PendingCommentsResponse {
  comments: PendingComment[]
  total: number
}

// ─── Query Keys ─────────────────────────────────────────────────────────────

export const adminCommentQueryKeys = {
  all: ['admin', 'comments'] as const,
  pending: (params?: Record<string, unknown>) =>
    ['admin', 'comments', 'pending', params] as const,
}

// ─── Endpoints ──────────────────────────────────────────────────────────────

const ADMIN_COMMENT_ENDPOINTS = {
  PENDING: `${API_BASE_URL}/admin/comments/pending`,
  APPROVE: (id: number) => `${API_BASE_URL}/admin/comments/${id}/approve`,
  REJECT: (id: number) => `${API_BASE_URL}/admin/comments/${id}/reject`,
  HIDE: (id: number) => `${API_BASE_URL}/admin/comments/${id}/hide`,
  RESTORE: (id: number) => `${API_BASE_URL}/admin/comments/${id}/restore`,
}

// ─── Hooks ───────────────────────────────────────────────────────────────────

/**
 * Hook to fetch pending comments awaiting admin review.
 */
export function useAdminPendingComments(limit = 25, offset = 0) {
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
