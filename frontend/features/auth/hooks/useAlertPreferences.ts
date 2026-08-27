'use client'

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { HomeMetroState } from '@/components/shared/followAlertChoices'

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
 *
 * `enabled` is a real parameter rather than an always-on fetch because the
 * biggest consumer is a control mounted on every artist and venue detail page
 * that renders nothing for most viewers. Callers that cannot use the answer
 * should not pay for it.
 */
export const useAlertPreferences = (enabled = true) => {
  const { isAuthenticated, user } = useAuthContext()
  const viewerId = isAuthenticated ? user?.id : undefined

  return useQuery({
    queryKey: queryKeys.auth.alertPreferences(viewerId),
    queryFn: () =>
      apiRequest<AlertPreferences>(API_ENDPOINTS.AUTH.ALERT_PREFERENCES, {
        method: 'GET',
      }),
    enabled: enabled && isAuthenticated && viewerId !== undefined,
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Whether the viewer has a home area, as a TRI-STATE.
 *
 * One definition of "known", in one place. The rule was previously spelled out
 * at each of the three surfaces that need it, sharing only a type, so deciding
 * that (say) a failed read should collapse to `false` would have had to be
 * remembered in three files — and the two surfaces over one field would
 * silently disagree until someone noticed.
 *
 * `undefined` means UNKNOWN: in flight, or failed with nothing cached.
 * Callers must not render a scope as though it were `false`; see
 * `followAlertChoices`.
 *
 * Keyed on DATA PRESENCE, not on `isSuccess`. TanStack keeps `data` when a
 * BACKGROUND refetch fails and only moves `status` to error, so a query that
 * resolved fine and then hit one failed revalidation (a reconnect, or any
 * remount past the 5-minute staleTime) would report UNKNOWN while the correct
 * answer sat in the cache. That is the worst version of unknown: every Library
 * row's bracket vanishes and both entity-page and Library surfaces claim they
 * could not load something they demonstrably had.
 */
export const useHomeMetroState = (enabled = true): HomeMetroState => {
  const query = useAlertPreferences(enabled)
  return query.data ? Boolean(query.data.home_metro) : undefined
}

/**
 * What both account-level writes do with their response.
 *
 * Seed the cache from the merged state the server read back, then stale every
 * follow's resolved subscription: the account matrix is the layer an
 * un-overridden follow inherits from, and a follow's near-me scope resolves
 * against the home area, so changing either changes what those follows mean.
 */
const seedAndStaleFollows =
  (queryClient: ReturnType<typeof useQueryClient>, userId?: string | number) =>
  (preferences: AlertPreferences) => {
    queryClient.setQueryData(
      queryKeys.auth.alertPreferences(userId),
      preferences
    )
    queryClient.invalidateQueries({
      predicate: query =>
        query.queryKey[0] === 'follows' && query.queryKey[1] === 'alerts',
    })
    queryClient.invalidateQueries({
      queryKey: queryKeys.follows.libraryFollowingRoot(userId),
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
    onSuccess: seedAndStaleFollows(queryClient, user?.id),
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
    onSuccess: seedAndStaleFollows(queryClient, user?.id),
  })
}
