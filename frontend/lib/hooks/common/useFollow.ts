'use client'

import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys, createInvalidateQueries } from '@/lib/queryClient'
import { useAuthContext } from '@/lib/context/AuthContext'
import type {
  FollowStatus,
  BatchFollowResponse,
  BatchFollowEntry,
  FollowingListResponse,
  LibraryFollowingCounts,
  LibraryFollowingPage,
} from '@/lib/types/follow'

const LIBRARY_FOLLOWING_PAGE_SIZE = 50

const toSingularFollowType = (entityType: string) =>
  ({
    artists: 'artist',
    venues: 'venue',
    scenes: 'scene',
    labels: 'label',
    festivals: 'festival',
    'radio-shows': 'radio_show',
  })[entityType] ?? entityType

const toLibraryCountKey = (
  entityType: string
): keyof LibraryFollowingCounts | undefined =>
  ({
    artist: 'artists',
    venue: 'venues',
    scene: 'scenes',
    label: 'labels',
    festival: 'festivals',
  })[toSingularFollowType(entityType)] as
    | keyof LibraryFollowingCounts
    | undefined

const isMyFollowingQueryForType = (
  queryKey: readonly unknown[],
  singularType: string,
  userId?: string | number
) => {
  if (queryKey[0] !== 'follows' || queryKey[1] !== 'my-following') return false
  const params = queryKey[2]
  if (!params || typeof params !== 'object') return false
  const { type, userId: cachedUserId } = params as {
    type?: string
    userId?: string | number
  }
  if (cachedUserId !== userId) return false
  return type === singularType || type === 'all' || type === undefined
}

const getCachedIsFollowing = (
  entityId: number | string,
  entityStatus: FollowStatus | undefined,
  batches: Array<
    readonly [readonly unknown[], Record<string, BatchFollowEntry> | undefined]
  >
) => {
  if (entityStatus) return entityStatus.is_following
  if (typeof entityId !== 'number') return undefined
  for (const [, batch] of batches) {
    const entry = batch?.[String(entityId)]
    if (entry) return entry.is_following
  }
  return undefined
}

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
      const libraryCountsKey = queryKeys.follows.libraryCounts(user?.id)
      await Promise.all([
        queryClient.cancelQueries({ queryKey: entityKey }),
        queryClient.cancelQueries({ queryKey: batchPrefix }),
        queryClient.cancelQueries({ queryKey: libraryCountsKey }),
      ])

      // Snapshot previous value
      const previousData = queryClient.getQueryData<FollowStatus>(entityKey)
      const previousBatches = queryClient.getQueriesData<
        Record<string, BatchFollowEntry>
      >({ queryKey: batchPrefix })
      const previousLibraryCounts =
        queryClient.getQueryData<LibraryFollowingCounts>(libraryCountsKey)
      const wasFollowing = getCachedIsFollowing(
        entityId,
        previousData,
        previousBatches
      )

      // Optimistically update
      if (previousData) {
        queryClient.setQueryData(entityKey, {
          ...previousData,
          follower_count:
            previousData.follower_count + (previousData.is_following ? 0 : 1),
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
                follower_count:
                  entry.follower_count + (entry.is_following ? 0 : 1),
                is_following: true,
              },
            }
          }
        )
      }
      const countKey = toLibraryCountKey(entityType)
      if (previousLibraryCounts && countKey && wasFollowing === false) {
        queryClient.setQueryData<LibraryFollowingCounts>(libraryCountsKey, {
          ...previousLibraryCounts,
          [countKey]: previousLibraryCounts[countKey] + 1,
        })
      }

      return { previousData, previousBatches, previousLibraryCounts }
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
      if (context?.previousLibraryCounts) {
        queryClient.setQueryData(
          queryKeys.follows.libraryCounts(user?.id),
          context.previousLibraryCounts
        )
      }
    },
    onSettled: (_data, _error, { entityType, entityId }) => {
      const singularType = toSingularFollowType(entityType)
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.entity(entityType, entityId, user?.id),
      })
      invalidateQueries.follows()
      // Reconcile broad first_activity_at semantics without making the core
      // follow mutation wait on an optional /charts/me request.
      void invalidateQueries.personalCharts()
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.batchPrefix(entityType, user?.id),
      })
      queryClient.invalidateQueries({
        predicate: query =>
          isMyFollowingQueryForType(query.queryKey, singularType, user?.id),
      })
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.libraryFollowing(singularType, user?.id),
      })
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.libraryCounts(user?.id),
      })
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
      const libraryCountsKey = queryKeys.follows.libraryCounts(user?.id)
      const singularType = toSingularFollowType(entityType)
      await Promise.all([
        queryClient.cancelQueries({ queryKey: entityKey }),
        queryClient.cancelQueries({ queryKey: batchPrefix }),
        queryClient.cancelQueries({ queryKey: libraryCountsKey }),
      ])

      const previousData = queryClient.getQueryData<FollowStatus>(entityKey)
      const previousBatches = queryClient.getQueriesData<
        Record<string, BatchFollowEntry>
      >({ queryKey: batchPrefix })
      const previousLibraryCounts =
        queryClient.getQueryData<LibraryFollowingCounts>(libraryCountsKey)
      const previousFollowingLists =
        queryClient.getQueriesData<FollowingListResponse>({
          predicate: query =>
            isMyFollowingQueryForType(query.queryKey, singularType, user?.id),
        })
      const wasFollowing = getCachedIsFollowing(
        entityId,
        previousData,
        previousBatches
      )

      if (previousData) {
        queryClient.setQueryData(entityKey, {
          ...previousData,
          follower_count: Math.max(
            0,
            previousData.follower_count - (previousData.is_following ? 1 : 0)
          ),
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
                follower_count: Math.max(
                  0,
                  entry.follower_count - (entry.is_following ? 1 : 0)
                ),
                is_following: false,
              },
            }
          }
        )
      }
      const countKey = toLibraryCountKey(entityType)
      if (previousLibraryCounts && countKey && wasFollowing === true) {
        queryClient.setQueryData<LibraryFollowingCounts>(libraryCountsKey, {
          ...previousLibraryCounts,
          [countKey]: Math.max(0, previousLibraryCounts[countKey] - 1),
        })
      }

      for (const [key, snapshot] of previousFollowingLists) {
        if (!snapshot) continue
        queryClient.setQueryData<FollowingListResponse>(key, current => {
          if (!current) return current
          const nextFollowing = current.following.filter(entity => {
            if (entity.entity_type !== singularType) return true
            return typeof entityId === 'number'
              ? entity.entity_id !== entityId
              : entity.slug !== entityId
          })
          if (nextFollowing.length === current.following.length) return current
          return {
            ...current,
            following: nextFollowing,
            total: Math.max(0, current.total - 1),
          }
        })
      }

      return {
        previousData,
        previousBatches,
        previousLibraryCounts,
        previousFollowingLists,
      }
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
      if (context?.previousLibraryCounts) {
        queryClient.setQueryData(
          queryKeys.follows.libraryCounts(user?.id),
          context.previousLibraryCounts
        )
      }
      for (const [key, snapshot] of context?.previousFollowingLists ?? []) {
        queryClient.setQueryData(key, snapshot)
      }
    },
    onSettled: (_data, _error, { entityType, entityId }) => {
      const singularType = toSingularFollowType(entityType)
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.entity(entityType, entityId, user?.id),
      })
      invalidateQueries.follows()
      void invalidateQueries.personalCharts()
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.batchPrefix(entityType, user?.id),
      })
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.libraryFollowing(singularType, user?.id),
      })
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.libraryCounts(user?.id),
      })
      queryClient.invalidateQueries({
        predicate: query =>
          isMyFollowingQueryForType(query.queryKey, singularType, user?.id),
      })
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

/** Fetch all Library follow-tab totals with one request. */
export const useLibraryFollowingCounts = () => {
  const { isAuthenticated, user } = useAuthContext()
  const viewerId = isAuthenticated ? user?.id : undefined

  return useQuery({
    queryKey: queryKeys.follows.libraryCounts(viewerId),
    queryFn: () =>
      apiRequest<LibraryFollowingCounts>(API_ENDPOINTS.FOLLOW.LIBRARY_COUNTS, {
        method: 'GET',
      }),
    enabled: isAuthenticated && viewerId !== undefined,
    staleTime: 2 * 60 * 1000,
  })
}

/** Fetch bounded, server-sorted Library following pages for one entity type. */
export const useLibraryFollowing = (type: string) => {
  const { isAuthenticated, user } = useAuthContext()
  const viewerId = isAuthenticated ? user?.id : undefined

  return useInfiniteQuery({
    queryKey: queryKeys.follows.libraryFollowing(type, viewerId),
    initialPageParam: null as string | null,
    queryFn: async ({ pageParam }): Promise<LibraryFollowingPage> => {
      const params = new URLSearchParams({
        type,
        limit: LIBRARY_FOLLOWING_PAGE_SIZE.toString(),
      })
      if (pageParam) params.set('cursor', pageParam)
      return apiRequest<LibraryFollowingPage>(
        `${API_ENDPOINTS.FOLLOW.LIBRARY_FOLLOWING}?${params.toString()}`,
        { method: 'GET' }
      )
    },
    getNextPageParam: page => page.next_cursor,
    enabled: isAuthenticated && viewerId !== undefined && type.length > 0,
    staleTime: 2 * 60 * 1000,
  })
}
