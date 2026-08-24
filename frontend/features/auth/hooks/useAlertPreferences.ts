'use client'

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { useAuthContext } from '@/lib/context/AuthContext'

/** One alert type's account-level channels, RESOLVED against the shipped defaults. */
export interface AlertChannelDefaults {
  in_app: boolean
  email: boolean
}

/**
 * The account alert matrix every follow inherits from (PSY-1907). Resolved,
 * not raw: "unset" only has meaning against the shipped defaults, and
 * duplicating those in the client is the drift the three-layer design exists
 * to avoid.
 */
export interface AccountAlertDefaults {
  shows: AlertChannelDefaults
  releases: AlertChannelDefaults
}

/** GET /auth/preferences/alerts — one read for the whole account alerts surface. */
export interface AlertPreferences {
  success: boolean
  /** Home metro CBSA code, or null when no home area is set. */
  home_metro: string | null
  alert_defaults: AccountAlertDefaults
}

/** PATCH body: an omitted channel keeps inheriting rather than being pinned. */
export interface AlertChannelDefaultsUpdate {
  in_app?: boolean
  email?: boolean
}

export interface AlertDefaultsUpdate {
  shows?: AlertChannelDefaultsUpdate
  releases?: AlertChannelDefaultsUpdate
}

/**
 * Read the viewer's home area and resolved alert matrix.
 *
 * Both writes below return this same shape, so they seed the cache directly
 * rather than invalidating and re-fetching: the server already read the merged
 * state back, and a refetch would only re-derive what the response carries.
 */
export const useAlertPreferences = () => {
  const { isAuthenticated, user } = useAuthContext()
  const viewerId = isAuthenticated ? user?.id : undefined

  return useQuery({
    queryKey: queryKeys.auth.alertPreferences(viewerId),
    queryFn: () =>
      apiRequest<AlertPreferences>(API_ENDPOINTS.AUTH.ALERT_PREFERENCES, {
        method: 'GET',
      }),
    enabled: isAuthenticated && viewerId !== undefined,
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Replace the home area. `null` clears it, which is the state that makes an
 * artist follow's near-me scope unreachable — hence the control that calls
 * this and the control that offers "Near me" read the same query.
 */
export const useSetHomeMetro = () => {
  const queryClient = useQueryClient()
  const { user } = useAuthContext()

  return useMutation({
    mutationFn: async (metro: string | null): Promise<AlertPreferences> =>
      apiRequest<AlertPreferences>(API_ENDPOINTS.AUTH.HOME_METRO, {
        method: 'PUT',
        body: JSON.stringify({ metro }),
      }),
    onSuccess: preferences => {
      queryClient.setQueryData(
        queryKeys.auth.alertPreferences(user?.id),
        preferences
      )
      // Every follow's resolved scope depends on whether a home area exists,
      // so the per-follow subscriptions the server resolved before this write
      // are stale the moment it lands.
      queryClient.invalidateQueries({
        predicate: query =>
          query.queryKey[0] === 'follows' && query.queryKey[1] === 'alerts',
      })
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.libraryFollowingRoot(user?.id),
      })
    },
  })
}

/** Partially update the account alert matrix. A body that pins nothing is a 422. */
export const useSetAlertDefaults = () => {
  const queryClient = useQueryClient()
  const { user } = useAuthContext()

  return useMutation({
    mutationFn: async (update: AlertDefaultsUpdate): Promise<AlertPreferences> =>
      apiRequest<AlertPreferences>(API_ENDPOINTS.AUTH.ALERT_DEFAULTS, {
        method: 'PATCH',
        body: JSON.stringify(update),
      }),
    onSuccess: preferences => {
      queryClient.setQueryData(
        queryKeys.auth.alertPreferences(user?.id),
        preferences
      )
      // The matrix is the layer every un-overridden follow inherits from, so
      // editing it changes what those follows resolve to.
      queryClient.invalidateQueries({
        predicate: query =>
          query.queryKey[0] === 'follows' && query.queryKey[1] === 'alerts',
      })
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.libraryFollowingRoot(user?.id),
      })
    },
  })
}
