'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys, createInvalidateQueries } from '@/lib/queryClient'
import { useAuthContext } from '@/lib/context/AuthContext'
import type {
  FollowStatus,
  BatchFollowResponse,
  BatchFollowEntry,
  FollowingListResponse,
} from '@/lib/types/follow'

/**
 * Hook to fetch follow status (follower count + user's follow status) for a single entity.
 * Uses optional auth -- if authenticated, includes whether the user is following.
 */
export const useFollowStatus = (entityType: string, entityId: number) => {
  return useQuery({
    queryKey: queryKeys.follows.entity(entityType, entityId),
    queryFn: async (): Promise<FollowStatus> => {
      return apiRequest<FollowStatus>(
        API_ENDPOINTS.FOLLOW.FOLLOWERS(entityType, entityId),
        { method: 'GET' }
      )
    },
    enabled: entityId > 0 && !!entityType,
    staleTime: 2 * 60 * 1000, // 2 minutes
  })
}

/**
 * Hook to fetch batch follow status for multiple entities of the same type.
 * Uses POST to send entity IDs. Returns a map of entity_id (string) -> { follower_count, is_following }.
 */
export const useBatchFollowStatus = (
  entityType: string,
  entityIds: number[]
) => {
  return useQuery({
    queryKey: queryKeys.follows.batch(entityType, entityIds),
    queryFn: async (): Promise<Record<string, BatchFollowEntry>> => {
      const response = await apiRequest<BatchFollowResponse>(
        API_ENDPOINTS.FOLLOW.BATCH,
        {
          method: 'POST',
          body: JSON.stringify({
            entity_type: entityType,
            entity_ids: entityIds,
          }),
        }
      )
      return response.follows
    },
    enabled: entityIds.length > 0 && !!entityType,
    staleTime: 2 * 60 * 1000,
  })
}

/**
 * Hook to follow an entity.
 * POST to /{entityType}/{entityId}/follow.
 * Includes optimistic update: increment follower_count, set is_following=true.
 */
export const useFollow = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      entityType,
      entityId,
    }: {
      entityType: string
      entityId: number
    }): Promise<{ success: boolean; message: string }> => {
      return apiRequest(API_ENDPOINTS.FOLLOW.ENTITY(entityType, entityId), {
        method: 'POST',
      })
    },
    onMutate: async ({ entityType, entityId }) => {
      // Cancel outgoing queries for this entity's follow status
      await queryClient.cancelQueries({
        queryKey: queryKeys.follows.entity(entityType, entityId),
      })

      // Snapshot previous value
      const previousData = queryClient.getQueryData<FollowStatus>(
        queryKeys.follows.entity(entityType, entityId)
      )

      // Optimistically update
      if (previousData) {
        queryClient.setQueryData(
          queryKeys.follows.entity(entityType, entityId),
          {
            ...previousData,
            follower_count: previousData.follower_count + 1,
            is_following: true,
          }
        )
      }

      return { previousData }
    },
    onError: (_err, { entityType, entityId }, context) => {
      // Rollback on error
      if (context?.previousData) {
        queryClient.setQueryData(
          queryKeys.follows.entity(entityType, entityId),
          context.previousData
        )
      }
    },
    onSettled: (_data, _error, { entityType, entityId }) => {
      // Refetch to ensure consistency
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.entity(entityType, entityId),
      })
      invalidateQueries.follows()
    },
  })
}

/**
 * Hook to unfollow an entity.
 * DELETE to /{entityType}/{entityId}/follow.
 * Includes optimistic update: decrement follower_count, set is_following=false.
 */
export const useUnfollow = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      entityType,
      entityId,
    }: {
      entityType: string
      entityId: number
    }): Promise<{ success: boolean; message: string }> => {
      return apiRequest(API_ENDPOINTS.FOLLOW.ENTITY(entityType, entityId), {
        method: 'DELETE',
      })
    },
    onMutate: async ({ entityType, entityId }) => {
      await queryClient.cancelQueries({
        queryKey: queryKeys.follows.entity(entityType, entityId),
      })

      const previousData = queryClient.getQueryData<FollowStatus>(
        queryKeys.follows.entity(entityType, entityId)
      )

      if (previousData) {
        queryClient.setQueryData(
          queryKeys.follows.entity(entityType, entityId),
          {
            ...previousData,
            follower_count: Math.max(0, previousData.follower_count - 1),
            is_following: false,
          }
        )
      }

      return { previousData }
    },
    onError: (_err, { entityType, entityId }, context) => {
      if (context?.previousData) {
        queryClient.setQueryData(
          queryKeys.follows.entity(entityType, entityId),
          context.previousData
        )
      }
    },
    onSettled: (_data, _error, { entityType, entityId }) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.entity(entityType, entityId),
      })
      invalidateQueries.follows()
    },
  })
}

interface UseMyFollowingOptions {
  type?: string
  limit?: number
  offset?: number
}

/**
 * Hook to fetch the authenticated user's following list.
 * Only enabled when user is authenticated.
 */
export const useMyFollowing = (options: UseMyFollowingOptions = {}) => {
  const { isAuthenticated } = useAuthContext()
  const { type = 'all', limit = 20, offset = 0 } = options

  const params = new URLSearchParams()
  if (type && type !== 'all') params.set('type', type)
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())

  const endpoint = `${API_ENDPOINTS.FOLLOW.MY_FOLLOWING}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.follows.myFollowing({ type, limit, offset }),
    queryFn: async (): Promise<FollowingListResponse> => {
      return apiRequest<FollowingListResponse>(endpoint, { method: 'GET' })
    },
    enabled: isAuthenticated,
    staleTime: 2 * 60 * 1000,
  })
}
