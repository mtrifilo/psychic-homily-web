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
export const useFollowStatus = (
  entityType: string,
  // number for id-keyed entities; a scene SLUG for "scenes" (PSY-1339/1340 —
  // same route shape: GET /scenes/{slug}/followers).
  entityId: number | string,
  enabled = true
) => {
  const { isAuthenticated, user } = useAuthContext()
  const viewerId = isAuthenticated ? user?.id : undefined
  return useQuery({
    queryKey: queryKeys.follows.entity(entityType, entityId, viewerId),
    queryFn: async (): Promise<FollowStatus> => {
      return apiRequest<FollowStatus>(
        API_ENDPOINTS.FOLLOW.FOLLOWERS(entityType, entityId),
        { method: 'GET' }
      )
    },
    enabled:
      enabled &&
      (typeof entityId === 'number' ? entityId > 0 : entityId.length > 0) &&
      !!entityType &&
      (!isAuthenticated || viewerId !== undefined),
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
  const { isAuthenticated, user } = useAuthContext()
  const viewerId = isAuthenticated ? user?.id : undefined
  return useQuery({
    queryKey: queryKeys.follows.batch(entityType, entityIds, viewerId),
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
    enabled:
      entityIds.length > 0 &&
      !!entityType &&
      (!isAuthenticated || viewerId !== undefined),
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
  const { user } = useAuthContext()

  return useMutation({
    mutationFn: async ({
      entityType,
      entityId,
    }: {
      entityType: string
      // number, or a scene SLUG for entityType "scenes" (PSY-1340).
      entityId: number | string
    }): Promise<{ success: boolean; message: string }> => {
      return apiRequest(API_ENDPOINTS.FOLLOW.ENTITY(entityType, entityId), {
        method: 'POST',
      })
    },
    onMutate: async ({ entityType, entityId }) => {
      const entityKey = queryKeys.follows.entity(entityType, entityId, user?.id)
      const batchPrefix = queryKeys.follows.batchPrefix(entityType, user?.id)
      await Promise.all([
        queryClient.cancelQueries({ queryKey: entityKey }),
        queryClient.cancelQueries({ queryKey: batchPrefix }),
      ])

      // Snapshot previous value
      const previousData = queryClient.getQueryData<FollowStatus>(entityKey)
      const previousBatches = queryClient.getQueriesData<
        Record<string, BatchFollowEntry>
      >({ queryKey: batchPrefix })

      // Optimistically update
      if (previousData) {
        queryClient.setQueryData(entityKey, {
          ...previousData,
          follower_count: previousData.follower_count + 1,
          is_following: true,
        })
      }
      if (typeof entityId === 'number') {
        queryClient.setQueriesData<Record<string, BatchFollowEntry>>(
          { queryKey: batchPrefix },
          current => {
            const entry = current?.[String(entityId)]
            if (!current || !entry) return current
            return {
              ...current,
              [String(entityId)]: {
                follower_count: entry.follower_count + 1,
                is_following: true,
              },
            }
          }
        )
      }

      return { previousData, previousBatches }
    },
    onError: (_err, { entityType, entityId }, context) => {
      // Rollback on error
      if (context?.previousData) {
        queryClient.setQueryData(
          queryKeys.follows.entity(entityType, entityId, user?.id),
          context.previousData
        )
      }
      if (typeof entityId === 'number') {
        for (const [key, snapshot] of context?.previousBatches ?? []) {
          queryClient.setQueryData<Record<string, BatchFollowEntry>>(
            key,
            current => {
              const priorEntry = snapshot?.[String(entityId)]
              if (!current || !priorEntry) return current
              return { ...current, [String(entityId)]: priorEntry }
            }
          )
        }
      }
    },
    onSuccess: (_data, { entityType }) => {
      if (entityType === 'artists') invalidateQueries.personalCharts()
    },
    onSettled: (_data, _error, { entityType, entityId }) => {
      // Refetch to ensure consistency
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.entity(entityType, entityId, user?.id),
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
  const { user } = useAuthContext()

  return useMutation({
    mutationFn: async ({
      entityType,
      entityId,
    }: {
      entityType: string
      // number for the id-keyed entities; a scene's SLUG for entityType
      // "scenes" (PSY-1339 — scenes are slug-addressed, the route shape is
      // identical: DELETE /scenes/{slug}/follow).
      entityId: number | string
    }): Promise<{ success: boolean; message: string }> => {
      return apiRequest(API_ENDPOINTS.FOLLOW.ENTITY(entityType, entityId), {
        method: 'DELETE',
      })
    },
    onMutate: async ({ entityType, entityId }) => {
      const entityKey = queryKeys.follows.entity(entityType, entityId, user?.id)
      const batchPrefix = queryKeys.follows.batchPrefix(entityType, user?.id)
      await Promise.all([
        queryClient.cancelQueries({ queryKey: entityKey }),
        queryClient.cancelQueries({ queryKey: batchPrefix }),
      ])

      const previousData = queryClient.getQueryData<FollowStatus>(entityKey)
      const previousBatches = queryClient.getQueriesData<
        Record<string, BatchFollowEntry>
      >({ queryKey: batchPrefix })

      if (previousData) {
        queryClient.setQueryData(entityKey, {
          ...previousData,
          follower_count: Math.max(0, previousData.follower_count - 1),
          is_following: false,
        })
      }
      if (typeof entityId === 'number') {
        queryClient.setQueriesData<Record<string, BatchFollowEntry>>(
          { queryKey: batchPrefix },
          current => {
            const entry = current?.[String(entityId)]
            if (!current || !entry) return current
            return {
              ...current,
              [String(entityId)]: {
                follower_count: Math.max(0, entry.follower_count - 1),
                is_following: false,
              },
            }
          }
        )
      }

      return { previousData, previousBatches }
    },
    onError: (_err, { entityType, entityId }, context) => {
      if (context?.previousData) {
        queryClient.setQueryData(
          queryKeys.follows.entity(entityType, entityId, user?.id),
          context.previousData
        )
      }
      if (typeof entityId === 'number') {
        for (const [key, snapshot] of context?.previousBatches ?? []) {
          queryClient.setQueryData<Record<string, BatchFollowEntry>>(
            key,
            current => {
              const priorEntry = snapshot?.[String(entityId)]
              if (!current || !priorEntry) return current
              return { ...current, [String(entityId)]: priorEntry }
            }
          )
        }
      }
    },
    onSuccess: (_data, { entityType }) => {
      if (entityType === 'artists') invalidateQueries.personalCharts()
    },
    onSettled: (_data, _error, { entityType, entityId }) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.entity(entityType, entityId, user?.id),
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
  const { isAuthenticated, user } = useAuthContext()
  const viewerId = isAuthenticated ? user?.id : undefined
  const { type = 'all', limit = 20, offset = 0 } = options

  const params = new URLSearchParams()
  if (type && type !== 'all') params.set('type', type)
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())

  const endpoint = `${API_ENDPOINTS.FOLLOW.MY_FOLLOWING}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.follows.myFollowing({
      type,
      limit,
      offset,
      userId: viewerId,
    }),
    queryFn: async (): Promise<FollowingListResponse> => {
      return apiRequest<FollowingListResponse>(endpoint, { method: 'GET' })
    },
    enabled: isAuthenticated && viewerId !== undefined,
    staleTime: 2 * 60 * 1000,
  })
}

/**
 * Fetch a complete following bucket for management surfaces that need a
 * stable, global sort. The API caps pages at 100, so this query walks every
 * page instead of presenting each recency-ordered page as if it were the
 * complete alphabetical list.
 */
export const useAllMyFollowing = (type: string) => {
  const { isAuthenticated, user } = useAuthContext()
  const viewerId = isAuthenticated ? user?.id : undefined

  return useQuery({
    queryKey: queryKeys.follows.myFollowing({
      type,
      scope: 'all',
      userId: viewerId,
    }),
    queryFn: async (): Promise<FollowingListResponse> => {
      const limit = 100
      const following: FollowingListResponse['following'] = []
      let offset = 0
      let total = 0

      do {
        const params = new URLSearchParams({
          type,
          limit: limit.toString(),
          offset: offset.toString(),
        })
        const page = await apiRequest<FollowingListResponse>(
          `${API_ENDPOINTS.FOLLOW.MY_FOLLOWING}?${params.toString()}`,
          { method: 'GET' }
        )
        following.push(...page.following)
        total = page.total
        if (page.following.length === 0) break
        offset += page.following.length
      } while (offset < total)

      return { following, total, limit: following.length, offset: 0 }
    },
    enabled: isAuthenticated && viewerId !== undefined && type.length > 0,
    staleTime: 2 * 60 * 1000,
  })
}
