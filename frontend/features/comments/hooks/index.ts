'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, type ApiError } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import {
  commentEndpoints,
  commentQueryKeys,
  commentPreferencesEndpoints,
  fieldNoteEndpoints,
  fieldNoteQueryKeys,
} from '../api'
import type {
  Comment,
  CommentListResponse,
  CommentThreadResponse,
  CreateFieldNoteInput,
  ReplyPermission,
} from '../types'

// ============================================================================
// Error formatting (PSY-589)
// ============================================================================

/**
 * Capitalize the first character of a non-empty string. Backend service
 * messages use lowercase ("please wait 60 seconds...") to keep substring
 * routing in handlers simple; the project copy convention is to capitalize
 * the first word in user-facing text, so we normalize at the display
 * boundary.
 */
function capitalizeFirst(s: string): string {
  if (!s) return s
  return s.charAt(0).toUpperCase() + s.slice(1)
}

/**
 * Format a submission error into a user-facing inline-banner string. 429
 * gets countdown copy populated from the `Retry-After` header (or the
 * service message body as a fallback). Any other status falls back to the
 * raw message. Returns null if there is no error.
 *
 * Exported for unit testing — the hook test asserts that 429 with a
 * Retry-After header produces "Please wait Ns before commenting again."
 */
export function formatCommentSubmissionError(error: unknown): string | null {
  if (!error) return null
  const apiErr = error as ApiError
  if (apiErr.status === 429) {
    if (apiErr.retryAfter && Number.isFinite(apiErr.retryAfter)) {
      return `Please wait ${apiErr.retryAfter}s before commenting again.`
    }
    if (apiErr.message) {
      return capitalizeFirst(apiErr.message)
    }
    return 'Please wait a minute before commenting again.'
  }
  if (apiErr.message) return capitalizeFirst(apiErr.message)
  if (error instanceof Error) return capitalizeFirst(error.message)
  return 'Something went wrong. Please try again.'
}

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
  // Backend returns `{ comments: [root, ...descendants] }` as a flat list;
  // split here so consumers get the { comment, replies } shape they expect.
  return useQuery<CommentThreadResponse>({
    queryKey: commentQueryKeys.thread(commentId),
    queryFn: async () => {
      const data = await apiRequest<{ comments: Comment[] }>(
        commentEndpoints.THREAD(commentId)
      )
      const comments = data.comments ?? []
      const root = comments.find((c) => c.id === commentId) ?? comments[0]
      const replies = comments.filter((c) => c.id !== commentId)
      return { comment: root, replies }
    },
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
      replyPermission,
    }: {
      entityType: string
      entityId: number
      body: string
      replyPermission?: ReplyPermission
    }) =>
      apiRequest<Comment>(commentEndpoints.CREATE(entityType, entityId), {
        method: 'POST',
        body: JSON.stringify(
          replyPermission
            ? { body, reply_permission: replyPermission }
            : { body }
        ),
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
      replyPermission,
    }: {
      commentId: number
      body: string
      entityType: string
      entityId: number
      replyPermission?: ReplyPermission
    }) =>
      apiRequest<Comment>(commentEndpoints.REPLY(commentId), {
        method: 'POST',
        body: JSON.stringify(
          replyPermission
            ? { body, reply_permission: replyPermission }
            : { body }
        ),
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

// PSY-296: owner-only mutation to change a comment's reply permission.
export function useUpdateReplyPermission() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({
      commentId,
      permission,
    }: {
      commentId: number
      permission: ReplyPermission
      entityType: string
      entityId: number
    }) =>
      apiRequest<Comment>(commentEndpoints.REPLY_PERMISSION(commentId), {
        method: 'PUT',
        body: JSON.stringify({ permission }),
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

// PSY-296: mutation to update the user's default reply permission.
export function useSetDefaultReplyPermission() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (permission: ReplyPermission) =>
      apiRequest<{ success: boolean; default_reply_permission: string }>(
        commentPreferencesEndpoints.DEFAULT_REPLY_PERMISSION,
        {
          method: 'PATCH',
          body: JSON.stringify({ permission }),
        }
      ),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.auth.profile })
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
      // `useComments` caches under [...entity(type, id), sort], so the exact
      // entity key is never populated — only prefix-matching via filters sees
      // the data. Use getQueriesData/setQueriesData consistently; plain
      // getQueryData(shortKey) would return undefined and skip the update.
      const queryKey = commentQueryKeys.entity(variables.entityType, variables.entityId)
      await queryClient.cancelQueries({ queryKey })

      const snapshot = queryClient.getQueriesData<CommentListResponse>({ queryKey })

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

      return { snapshot }
    },
    onError: (_err, _variables, context) => {
      // Restore every cached variant we snapshotted.
      if (context?.snapshot) {
        for (const [key, data] of context.snapshot) {
          queryClient.setQueryData(key, data)
        }
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

      const snapshot = queryClient.getQueriesData<CommentListResponse>({ queryKey })

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

      return { snapshot }
    },
    onError: (_err, _variables, context) => {
      if (context?.snapshot) {
        for (const [key, data] of context.snapshot) {
          queryClient.setQueryData(key, data)
        }
      }
    },
    onSettled: (_data, _err, variables) => {
      queryClient.invalidateQueries({
        queryKey: commentQueryKeys.entity(variables.entityType, variables.entityId),
      })
    },
  })
}

// ============================================================================
// Field Note Queries & Mutations
// ============================================================================

export function useFieldNotes(showId: number, options?: { enabled?: boolean }) {
  return useQuery<CommentListResponse>({
    queryKey: fieldNoteQueryKeys.show(showId),
    queryFn: () =>
      apiRequest<CommentListResponse>(fieldNoteEndpoints.LIST(showId)),
    enabled: (options?.enabled ?? true) && showId > 0,
  })
}

export function useCreateFieldNote() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({
      showId,
      input,
    }: {
      showId: number
      input: CreateFieldNoteInput
    }) =>
      apiRequest<Comment>(fieldNoteEndpoints.CREATE(showId), {
        method: 'POST',
        body: JSON.stringify(input),
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: fieldNoteQueryKeys.show(variables.showId),
      })
    },
  })
}
