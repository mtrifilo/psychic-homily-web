'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { commentEndpoints, commentQueryKeys } from '../api'
import type { Comment, CommentListResponse, CommentThreadResponse } from '../types'

// ============================================================================
// Queries
// ============================================================================

export function useComments(
  entityType: string,
  entityId: number,
  sort: 'best' | 'new' | 'top' = 'best'
) {
  return useQuery<CommentListResponse>({
    queryKey: [...commentQueryKeys.entity(entityType, entityId), sort],
    queryFn: () =>
      apiRequest<CommentListResponse>(
        `${commentEndpoints.LIST(entityType, entityId)}?sort=${sort}`
      ),
    enabled: !!entityType && entityId > 0,
  })
}

export function useCommentThread(commentId: number, enabled = false) {
  return useQuery<CommentThreadResponse>({
    queryKey: commentQueryKeys.thread(commentId),
    queryFn: () =>
      apiRequest<CommentThreadResponse>(commentEndpoints.THREAD(commentId)),
    enabled: enabled && commentId > 0,
  })
}

// ============================================================================
// Mutations
// ============================================================================

export function useCreateComment() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({
      entityType,
      entityId,
      body,
    }: {
      entityType: string
      entityId: number
      body: string
    }) =>
      apiRequest<Comment>(commentEndpoints.CREATE(entityType, entityId), {
        method: 'POST',
        body: JSON.stringify({ body }),
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: commentQueryKeys.entity(variables.entityType, variables.entityId),
      })
    },
  })
}

export function useReplyToComment() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({
      commentId,
      body,
    }: {
      commentId: number
      body: string
      entityType: string
      entityId: number
    }) =>
      apiRequest<Comment>(commentEndpoints.REPLY(commentId), {
        method: 'POST',
        body: JSON.stringify({ body }),
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: commentQueryKeys.entity(variables.entityType, variables.entityId),
      })
      queryClient.invalidateQueries({
        queryKey: commentQueryKeys.thread(variables.commentId),
      })
    },
  })
}

export function useUpdateComment() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({
      commentId,
      body,
    }: {
      commentId: number
      body: string
      entityType: string
      entityId: number
    }) =>
      apiRequest<Comment>(commentEndpoints.UPDATE(commentId), {
        method: 'PUT',
        body: JSON.stringify({ body }),
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: commentQueryKeys.entity(variables.entityType, variables.entityId),
      })
    },
  })
}

export function useDeleteComment() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({
      commentId,
    }: {
      commentId: number
      entityType: string
      entityId: number
    }) =>
      apiRequest<void>(commentEndpoints.DELETE(commentId), {
        method: 'DELETE',
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: commentQueryKeys.entity(variables.entityType, variables.entityId),
      })
    },
  })
}

export function useVoteComment() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({
      commentId,
      direction,
    }: {
      commentId: number
      direction: 1 | -1
      entityType: string
      entityId: number
    }) =>
      apiRequest<void>(commentEndpoints.VOTE(commentId), {
        method: 'POST',
        body: JSON.stringify({ direction }),
      }),
    onMutate: async (variables) => {
      // Optimistic update: adjust vote counts in the cached comment list
      const queryKey = commentQueryKeys.entity(variables.entityType, variables.entityId)
      await queryClient.cancelQueries({ queryKey })

      const previous = queryClient.getQueryData<CommentListResponse>(queryKey)

      if (previous) {
        const updateComment = (comment: Comment): Comment => {
          if (comment.id !== variables.commentId) return comment

          const prevVote = comment.user_vote ?? null
          let ups = comment.ups
          let downs = comment.downs

          // Remove previous vote
          if (prevVote === 1) ups--
          if (prevVote === -1) downs--

          // Apply new vote
          if (variables.direction === 1) ups++
          if (variables.direction === -1) downs++

          return {
            ...comment,
            ups,
            downs,
            score: ups - downs,
            user_vote: variables.direction,
          }
        }

        // Update all sort variants of this entity's comments
        queryClient.setQueriesData<CommentListResponse>(
          { queryKey },
          (old) => {
            if (!old) return old
            return {
              ...old,
              comments: old.comments.map(updateComment),
            }
          }
        )
      }

      return { previous }
    },
    onError: (_err, variables, context) => {
      // Roll back on error
      if (context?.previous) {
        const queryKey = commentQueryKeys.entity(variables.entityType, variables.entityId)
        queryClient.setQueryData(queryKey, context.previous)
      }
    },
    onSettled: (_data, _err, variables) => {
      queryClient.invalidateQueries({
        queryKey: commentQueryKeys.entity(variables.entityType, variables.entityId),
      })
    },
  })
}

export function useUnvoteComment() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({
      commentId,
    }: {
      commentId: number
      entityType: string
      entityId: number
    }) =>
      apiRequest<void>(commentEndpoints.VOTE(commentId), {
        method: 'DELETE',
      }),
    onMutate: async (variables) => {
      const queryKey = commentQueryKeys.entity(variables.entityType, variables.entityId)
      await queryClient.cancelQueries({ queryKey })

      const previous = queryClient.getQueryData<CommentListResponse>(queryKey)

      if (previous) {
        const updateComment = (comment: Comment): Comment => {
          if (comment.id !== variables.commentId) return comment

          const prevVote = comment.user_vote ?? null
          let ups = comment.ups
          let downs = comment.downs

          if (prevVote === 1) ups--
          if (prevVote === -1) downs--

          return {
            ...comment,
            ups,
            downs,
            score: ups - downs,
            user_vote: null,
          }
        }

        queryClient.setQueriesData<CommentListResponse>(
          { queryKey },
          (old) => {
            if (!old) return old
            return {
              ...old,
              comments: old.comments.map(updateComment),
            }
          }
        )
      }

      return { previous }
    },
    onError: (_err, variables, context) => {
      if (context?.previous) {
        const queryKey = commentQueryKeys.entity(variables.entityType, variables.entityId)
        queryClient.setQueryData(queryKey, context.previous)
      }
    },
    onSettled: (_data, _err, variables) => {
      queryClient.invalidateQueries({
        queryKey: commentQueryKeys.entity(variables.entityType, variables.entityId),
      })
    },
  })
}
