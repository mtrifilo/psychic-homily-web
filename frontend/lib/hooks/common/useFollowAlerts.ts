'use client'

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { useAuthContext } from '@/lib/context/AuthContext'
import type {
  FollowAlertSettings,
  FollowAlertUpdate,
} from '@/lib/types/follow'

/**
 * The alert subscription carried by a follow (PSY-1893).
 *
 * There is no separate subscribe call: following an artist or a venue IS
 * subscribing, so both endpoints 404 when the viewer does not follow the
 * entity. Callers therefore gate this query on follow state rather than
 * treating a 404 as an error worth showing.
 *
 * Entity type is the PLURAL path segment ("artists", "venues") that every
 * other follow endpoint takes. Follow types that carry no alert subscription
 * (label, festival, tag, scene, radio show) answer 422 and must not be passed.
 */
export const useFollowAlerts = (
  entityType: string,
  entityId: number | string,
  enabled = true
) => {
  const { isAuthenticated, user } = useAuthContext()
  const viewerId = isAuthenticated ? user?.id : undefined

  return useQuery({
    queryKey: queryKeys.follows.alerts(entityType, entityId, viewerId),
    queryFn: () =>
      apiRequest<FollowAlertSettings>(
        API_ENDPOINTS.FOLLOW.ALERTS(entityType, entityId),
        { method: 'GET' }
      ),
    enabled: enabled && isAuthenticated && viewerId !== undefined,
    // The endpoint is no-store: it is per-user state a control flips
    // optimistically, and a stale read would show a chip the server disagrees
    // with. Retry stays off because the expected failure is a 404 meaning
    // "not following", which retrying cannot turn into a success.
    staleTime: 0,
    retry: false,
  })
}

/**
 * Adjust one axis of a follow's alert subscription.
 *
 * The response is the full RESOLVED subscription, so it seeds the cache
 * directly: a PATCH body only ever carries the axes it changed, and the merged
 * result is what the server read back.
 */
export const useUpdateFollowAlerts = () => {
  const queryClient = useQueryClient()
  const { user } = useAuthContext()

  return useMutation({
    mutationFn: async ({
      entityType,
      entityId,
      update,
    }: {
      entityType: string
      entityId: number | string
      update: FollowAlertUpdate
    }): Promise<FollowAlertSettings> =>
      apiRequest<FollowAlertSettings>(
        API_ENDPOINTS.FOLLOW.ALERTS(entityType, entityId),
        { method: 'PATCH', body: JSON.stringify(update) }
      ),
    // Optimistic for the same reason SceneNotifyModeToggle is: without it the
    // chip snaps back to the pre-click value between the PATCH resolving and
    // the refetch landing, which reads as "the click didn't take".
    onMutate: async ({ entityType, entityId, update }) => {
      const key = queryKeys.follows.alerts(entityType, entityId, user?.id)
      await queryClient.cancelQueries({ queryKey: key })
      const previous = queryClient.getQueryData<FollowAlertSettings>(key)
      if (previous) {
        queryClient.setQueryData<FollowAlertSettings>(key, {
          ...previous,
          shows: { ...previous.shows, ...update.shows },
          releases: previous.releases
            ? { ...previous.releases, ...update.releases }
            : previous.releases,
        })
      }
      return { previous, key }
    },
    onError: (_error, _variables, context) => {
      if (context?.previous) {
        queryClient.setQueryData(context.key, context.previous)
      }
    },
    onSuccess: (settings, { entityType, entityId }) => {
      queryClient.setQueryData(
        queryKeys.follows.alerts(entityType, entityId, user?.id),
        settings
      )
      // Library rows carry their own copy of this subscription, served with
      // the row so the per-row control renders without a request per row.
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.libraryFollowingRoot(user?.id),
      })
    },
  })
}
