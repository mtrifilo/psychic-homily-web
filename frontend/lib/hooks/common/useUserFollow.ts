'use client'

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { UserFollowStatus } from '@/lib/types/follow'
import type { PublicProfileResponse } from '@/features/auth'

/**
 * Fetch follow status for a user addressed by username.
 * Optional auth — when authenticated, includes is_following.
 */
export const useUserFollowStatus = (username: string, enabled = true) => {
  const { isAuthenticated, user } = useAuthContext()
  const viewerId = isAuthenticated ? user?.id : undefined
  return useQuery({
    queryKey: queryKeys.follows.user(username, viewerId),
    queryFn: async (): Promise<UserFollowStatus> => {
      return apiRequest<UserFollowStatus>(
        API_ENDPOINTS.USERS.FOLLOWERS(username),
        { method: 'GET' }
      )
    },
    enabled:
      enabled &&
      Boolean(username) &&
      (!isAuthenticated || viewerId !== undefined),
    staleTime: 2 * 60 * 1000,
  })
}

const bumpProfileFollowers = (
  profile: PublicProfileResponse | undefined,
  delta: number
): PublicProfileResponse | undefined => {
  if (!profile?.stats) return profile
  return {
    ...profile,
    stats: {
      ...profile.stats,
      followers_count: Math.max(0, profile.stats.followers_count + delta),
    },
  }
}

/**
 * Follow a user by username. Optimistic is_following + follower_count,
 * and bumps profile.stats.followers_count when that cache is present.
 */
export const useUserFollow = () => {
  const queryClient = useQueryClient()
  const { user } = useAuthContext()

  return useMutation({
    mutationFn: async (
      username: string
    ): Promise<{ success: boolean; message: string }> => {
      return apiRequest(API_ENDPOINTS.USERS.FOLLOW(username), {
        method: 'POST',
      })
    },
    onMutate: async username => {
      const statusKey = queryKeys.follows.user(username, user?.id)
      const profileKey = queryKeys.contributor.profile(username)
      await Promise.all([
        queryClient.cancelQueries({ queryKey: statusKey }),
        queryClient.cancelQueries({ queryKey: profileKey }),
      ])

      const previousStatus =
        queryClient.getQueryData<UserFollowStatus>(statusKey)
      const previousProfile =
        queryClient.getQueryData<PublicProfileResponse>(profileKey)

      if (previousStatus) {
        queryClient.setQueryData<UserFollowStatus>(statusKey, {
          ...previousStatus,
          follower_count:
            previousStatus.follower_count +
            (previousStatus.is_following ? 0 : 1),
          is_following: true,
        })
      }
      if (previousProfile && previousStatus && !previousStatus.is_following) {
        queryClient.setQueryData(
          profileKey,
          bumpProfileFollowers(previousProfile, 1)
        )
      }

      return { previousStatus, previousProfile, statusKey, profileKey }
    },
    onError: (_err, _username, context) => {
      if (context?.previousStatus) {
        queryClient.setQueryData(context.statusKey, context.previousStatus)
      }
      if (context?.previousProfile) {
        queryClient.setQueryData(context.profileKey, context.previousProfile)
      }
    },
    onSettled: (_data, _error, username) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.user(username, user?.id),
      })
      queryClient.invalidateQueries({
        queryKey: queryKeys.contributor.profile(username),
      })
    },
  })
}

/**
 * Unfollow a user by username. Optimistic rollback of follow state + count.
 */
export const useUserUnfollow = () => {
  const queryClient = useQueryClient()
  const { user } = useAuthContext()

  return useMutation({
    mutationFn: async (
      username: string
    ): Promise<{ success: boolean; message: string }> => {
      return apiRequest(API_ENDPOINTS.USERS.FOLLOW(username), {
        method: 'DELETE',
      })
    },
    onMutate: async username => {
      const statusKey = queryKeys.follows.user(username, user?.id)
      const profileKey = queryKeys.contributor.profile(username)
      await Promise.all([
        queryClient.cancelQueries({ queryKey: statusKey }),
        queryClient.cancelQueries({ queryKey: profileKey }),
      ])

      const previousStatus =
        queryClient.getQueryData<UserFollowStatus>(statusKey)
      const previousProfile =
        queryClient.getQueryData<PublicProfileResponse>(profileKey)

      if (previousStatus) {
        queryClient.setQueryData<UserFollowStatus>(statusKey, {
          ...previousStatus,
          follower_count: Math.max(
            0,
            previousStatus.follower_count -
              (previousStatus.is_following ? 1 : 0)
          ),
          is_following: false,
        })
      }
      if (previousProfile && previousStatus?.is_following) {
        queryClient.setQueryData(
          profileKey,
          bumpProfileFollowers(previousProfile, -1)
        )
      }

      return { previousStatus, previousProfile, statusKey, profileKey }
    },
    onError: (_err, _username, context) => {
      if (context?.previousStatus) {
        queryClient.setQueryData(context.statusKey, context.previousStatus)
      }
      if (context?.previousProfile) {
        queryClient.setQueryData(context.profileKey, context.previousProfile)
      }
    },
    onSettled: (_data, _error, username) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.user(username, user?.id),
      })
      queryClient.invalidateQueries({
        queryKey: queryKeys.contributor.profile(username),
      })
    },
  })
}
