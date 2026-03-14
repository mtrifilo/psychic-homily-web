'use client'

/**
 * Request Hooks
 *
 * TanStack Query hooks for request CRUD, voting, and fulfillment.
 */

import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type { Request, RequestListResponse } from '../types'

// ──────────────────────────────────────────────
// Queries
// ──────────────────────────────────────────────

interface UseRequestsParams {
  status?: string
  entity_type?: string
  sort_by?: string
  limit?: number
  offset?: number
}

/** Fetch requests list with optional filters */
export function useRequests(params?: UseRequestsParams) {
  const searchParams = new URLSearchParams()
  if (params?.status) searchParams.set('status', params.status)
  if (params?.entity_type) searchParams.set('entity_type', params.entity_type)
  if (params?.sort_by) searchParams.set('sort_by', params.sort_by)
  if (params?.limit) searchParams.set('limit', String(params.limit))
  if (params?.offset) searchParams.set('offset', String(params.offset))

  const queryString = searchParams.toString()
  const url = queryString
    ? `${API_ENDPOINTS.REQUESTS.LIST}?${queryString}`
    : API_ENDPOINTS.REQUESTS.LIST

  return useQuery({
    queryKey: queryKeys.requests.list(params as Record<string, unknown> | undefined),
    queryFn: () => apiRequest<RequestListResponse>(url),
    staleTime: 5 * 60 * 1000,
    placeholderData: keepPreviousData,
  })
}

/** Fetch a single request by ID */
export function useRequest(requestId: number, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.requests.detail(requestId),
    queryFn: () =>
      apiRequest<Request>(API_ENDPOINTS.REQUESTS.GET(requestId)),
    enabled: (options?.enabled ?? true) && requestId > 0,
    staleTime: 5 * 60 * 1000,
  })
}

// ──────────────────────────────────────────────
// Mutations
// ──────────────────────────────────────────────

/** Create a new request */
export function useCreateRequest() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: {
      title: string
      description?: string
      entity_type: string
      requested_entity_id?: number
    }) =>
      apiRequest<Request>(API_ENDPOINTS.REQUESTS.LIST, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.requests.all })
    },
  })
}

/** Update an existing request */
export function useUpdateRequest() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      requestId,
      ...data
    }: {
      requestId: number
      title?: string
      description?: string
    }) =>
      apiRequest<Request>(API_ENDPOINTS.REQUESTS.GET(requestId), {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.requests.all })
      queryClient.invalidateQueries({
        queryKey: queryKeys.requests.detail(variables.requestId),
      })
    },
  })
}

/** Delete a request */
export function useDeleteRequest() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ requestId }: { requestId: number }) =>
      apiRequest<void>(API_ENDPOINTS.REQUESTS.GET(requestId), {
        method: 'DELETE',
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.requests.all })
    },
  })
}

/** Vote on a request (with optimistic updates) */
export function useVoteRequest() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      requestId,
      is_upvote,
    }: {
      requestId: number
      is_upvote: boolean
    }) =>
      apiRequest<void>(API_ENDPOINTS.REQUESTS.VOTE(requestId), {
        method: 'POST',
        body: JSON.stringify({ is_upvote }),
      }),
    onMutate: async ({ requestId, is_upvote }) => {
      // Cancel outgoing refetches
      await queryClient.cancelQueries({
        queryKey: queryKeys.requests.detail(requestId),
      })

      // Snapshot the previous value
      const previousRequest = queryClient.getQueryData<Request>(
        queryKeys.requests.detail(requestId)
      )

      // Optimistically update the detail cache
      if (previousRequest) {
        const oldVote = previousRequest.user_vote ?? 0
        const newVote = is_upvote ? 1 : -1

        let upvoteDelta = 0
        let downvoteDelta = 0

        if (oldVote === 1) {
          // Was upvoted
          upvoteDelta = -1
          if (newVote === -1) downvoteDelta = 1
        } else if (oldVote === -1) {
          // Was downvoted
          downvoteDelta = -1
          if (newVote === 1) upvoteDelta = 1
        } else {
          // No previous vote
          if (newVote === 1) upvoteDelta = 1
          else downvoteDelta = 1
        }

        // If clicking the same vote direction, this is a toggle (remove vote)
        // But the API is POST /vote which sets the vote, not toggles.
        // Removing a vote uses DELETE /vote. So we always set the new vote.
        queryClient.setQueryData<Request>(
          queryKeys.requests.detail(requestId),
          {
            ...previousRequest,
            user_vote: newVote,
            upvotes: previousRequest.upvotes + upvoteDelta,
            downvotes: previousRequest.downvotes + downvoteDelta,
            vote_score: previousRequest.vote_score + upvoteDelta - downvoteDelta,
          }
        )
      }

      return { previousRequest }
    },
    onError: (_err, variables, context) => {
      // Roll back on error
      if (context?.previousRequest) {
        queryClient.setQueryData(
          queryKeys.requests.detail(variables.requestId),
          context.previousRequest
        )
      }
    },
    onSettled: (_data, _error, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.requests.detail(variables.requestId),
      })
      queryClient.invalidateQueries({
        queryKey: queryKeys.requests.all,
      })
    },
  })
}

/** Remove vote from a request (with optimistic updates) */
export function useRemoveVoteRequest() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ requestId }: { requestId: number }) =>
      apiRequest<void>(API_ENDPOINTS.REQUESTS.VOTE(requestId), {
        method: 'DELETE',
      }),
    onMutate: async ({ requestId }) => {
      await queryClient.cancelQueries({
        queryKey: queryKeys.requests.detail(requestId),
      })

      const previousRequest = queryClient.getQueryData<Request>(
        queryKeys.requests.detail(requestId)
      )

      if (previousRequest) {
        const oldVote = previousRequest.user_vote ?? 0
        let upvoteDelta = 0
        let downvoteDelta = 0

        if (oldVote === 1) upvoteDelta = -1
        else if (oldVote === -1) downvoteDelta = -1

        queryClient.setQueryData<Request>(
          queryKeys.requests.detail(requestId),
          {
            ...previousRequest,
            user_vote: null,
            upvotes: previousRequest.upvotes + upvoteDelta,
            downvotes: previousRequest.downvotes + downvoteDelta,
            vote_score: previousRequest.vote_score + upvoteDelta - downvoteDelta,
          }
        )
      }

      return { previousRequest }
    },
    onError: (_err, variables, context) => {
      if (context?.previousRequest) {
        queryClient.setQueryData(
          queryKeys.requests.detail(variables.requestId),
          context.previousRequest
        )
      }
    },
    onSettled: (_data, _error, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.requests.detail(variables.requestId),
      })
      queryClient.invalidateQueries({
        queryKey: queryKeys.requests.all,
      })
    },
  })
}

/** Fulfill a request (admin) */
export function useFulfillRequest() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      requestId,
      fulfilled_entity_id,
    }: {
      requestId: number
      fulfilled_entity_id?: number
    }) =>
      apiRequest<Request>(API_ENDPOINTS.REQUESTS.FULFILL(requestId), {
        method: 'POST',
        body: JSON.stringify({ fulfilled_entity_id }),
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.requests.all })
      queryClient.invalidateQueries({
        queryKey: queryKeys.requests.detail(variables.requestId),
      })
    },
  })
}

/** Close a request (admin) */
export function useCloseRequest() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ requestId }: { requestId: number }) =>
      apiRequest<Request>(API_ENDPOINTS.REQUESTS.CLOSE(requestId), {
        method: 'POST',
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.requests.all })
      queryClient.invalidateQueries({
        queryKey: queryKeys.requests.detail(variables.requestId),
      })
    },
  })
}
