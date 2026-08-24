'use client'

import {
  useMutation,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { useAuthContext } from '@/lib/context/AuthContext'
import { toSingularFollowType } from './useFollow'
import type { ApiError } from '@/lib/api'
import type {
  FollowAlertSettings,
  FollowAlertUpdate,
  LibraryFollowingPage,
} from '@/lib/types/follow'

/** A 404 from the alerts sub-resource means "not following", not "broken". */
const isNotFound = (error: unknown) => (error as ApiError)?.status === 404

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
 * (label, festival, tag, radio show) answer 422 and must not be passed; scenes
 * answer 400, being outside the follow-alert entity vocabulary entirely.
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
    // Matches the sibling follow-status query. A shorter window buys nothing:
    // every path that can change this value already reaches the cache without
    // a refetch (the PATCH seeds it, follow/unfollow invalidate it, and both
    // account-level writes stale the whole branch), so `staleTime: 0` would
    // only mean re-fetching on every remount and back-navigation.
    //
    staleTime: 2 * 60 * 1000,
    // A 404 here means "not following", which retrying cannot turn into a
    // success. Everything else can: this query is only enabled once the follow
    // is KNOWN to exist, so a 5xx or a dropped connection is anomalous rather
    // than expected, and giving up on it silently removes the whole alerts
    // control from a page that still says [Following].
    retry: (failureCount, error) =>
      !isNotFound(error) && failureCount < 2,
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
      // NUMBER, deliberately narrower than the follow endpoints' `number |
      // string`. Every alert-capable follow type is id-addressed (scenes, the
      // one slug-addressed type, carry no alert subscription), and the Library
      // cache patch below compares this strictly against a numeric row id: a
      // string would match nothing and leave the row silently stale.
      entityId: number
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
      // the row so the list needs no request per row. Patch that copy in place
      // rather than invalidating the list: the Library query is INFINITE, and
      // invalidating it refetches every loaded page in sequence, so flipping
      // one chip on a list the user has paged through three times would cost
      // three full 50-row round trips to change one field on one row. The
      // response is the full resolved subscription, so there is nothing to
      // re-derive from the server anyway.
      const singularType = toSingularFollowType(entityType)
      queryClient.setQueryData<InfiniteData<LibraryFollowingPage>>(
        queryKeys.follows.libraryFollowing(singularType, user?.id),
        current =>
          current && {
            ...current,
            pages: current.pages.map(page => ({
              ...page,
              following: page.following.map(row =>
                row.entity_type === singularType && row.entity_id === entityId
                  ? { ...row, alerts: settings }
                  : row
              ),
            })),
          }
      )
    },
  })
}
