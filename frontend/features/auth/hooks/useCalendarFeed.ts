'use client'

import {
  useQuery,
  useMutation,
  useQueryClient,
  type Query,
  type QueryClient,
} from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys, createInvalidateQueries } from '@/lib/queryClient'
import { refetchAfterMutation } from '@/lib/refetchAfterMutation'
import { isAuthError } from '@/lib/errors'
import { retryWhileFailed } from '@/lib/retryFailedRead'
import type { ApiError } from '@/lib/api'
import type { CalendarFeedClient } from '@/features/shows/types'
import type {
  CalendarFeedStatusResponse,
  ForgetCalendarFeedReadResponse,
  CalendarTokenStatusResponse,
  CalendarTokenCreateResponse,
  CalendarTokenDeleteResponse,
} from '@/features/shows'
import { PERSONAL_FEED_TOKEN_REVOKED_EVENT } from '@/features/collections/components/personalFeedTokenEvents'

/**
 * Hook to check if the user has a calendar feed token
 */
export const useCalendarTokenStatus = (
  enabled: boolean = true,
  options: {
    /**
     * Read again whenever an observer mounts and whenever the window regains
     * focus, and retry a failed read on its own: for a surface that acts on
     * the address and must follow it being replaced or turned off elsewhere.
     */
    live?: boolean
  } = {}
) => {
  return useQuery({
    queryKey: queryKeys.calendar.tokenStatus,
    queryFn: async (): Promise<CalendarTokenStatusResponse> => {
      return apiRequest<CalendarTokenStatusResponse>(
        API_ENDPOINTS.CALENDAR.TOKEN,
        { method: 'GET' }
      )
    },
    enabled,
    staleTime: 5 * 60 * 1000, // 5 minutes
    ...(options.live
      ? {
          refetchOnMount: 'always',
          refetchOnWindowFocus: 'always',
          refetchInterval: retryWhileFailed,
        }
      : {}),
  })
}

/**
 * How often a live status read that is waiting for a calendar app's first
 * check (`pollUntil`) reads again while its page is visible. A calendar app
 * usually fetches a newly added link within a minute or two; the window-focus
 * refetch covers the person coming back from that app's tab.
 */
export const CALENDAR_STATUS_POLL_MS = 15_000

/**
 * The calendar feed's status with no feed address in it: whether a link
 * exists, the calendar apps seen reading it, and the marker a security action
 * leaves when it reset a link an app had been reading. For surfaces that talk
 * about the calendar without showing its address.
 */
export const useCalendarFeedStatus = (
  enabled: boolean = true,
  options: {
    /**
     * Refetch whenever an observer mounts and whenever the window regains
     * focus, for a surface opened to watch a calendar app's first check: the
     * person adds the calendar in another tab or app and comes back.
     */
    live?: boolean
    /**
     * With `live`: also read again every CALENDAR_STATUS_POLL_MS until a read
     * satisfies this. React Query evaluates it on every render and after
     * every read, so it must be pure and cheap. The interval belongs to the
     * observer, so it ends when the surface unmounts, and React Query skips
     * it while the page is hidden. A failed read follows the retry rule
     * instead, so a refused session ends it.
     */
    pollUntil?: (status: CalendarFeedStatusResponse) => boolean
  } = {}
) => {
  const { live, pollUntil } = options
  return useQuery({
    queryKey: queryKeys.calendar.feedStatus,
    queryFn: async (): Promise<CalendarFeedStatusResponse> => {
      return apiRequest<CalendarFeedStatusResponse>(
        API_ENDPOINTS.CALENDAR.FEED_STATUS,
        { method: 'GET' }
      )
    },
    enabled,
    staleTime: 5 * 60 * 1000, // 5 minutes
    ...(live
      ? {
          refetchOnMount: 'always',
          refetchOnWindowFocus: 'always',
          // A failed read is retried on its own until one succeeds, and a
          // refused session is not retried: it will not change by waiting.
          // Whether the data still held from before is fit to show is the
          // surface's call (the calendar panel shows only a read made since it
          // opened), so the retry does not depend on it. Otherwise the
          // interval polls until `pollUntil` holds.
          refetchInterval: (query: Query<CalendarFeedStatusResponse, Error>) => {
            if (!pollUntil || query.state.status === 'error') {
              return retryWhileFailed(query)
            }
            const { data } = query.state
            return data && pollUntil(data) ? false : CALENDAR_STATUS_POLL_MS
          },
        }
      : {}),
  })
}

/**
 * Forgets one calendar client family on the current link: the person removed
 * that calendar, so it stops being reported as stopped. It reappears if it
 * checks the link again.
 */
export const useForgetCalendarClient = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (
      client: CalendarFeedClient
    ): Promise<ForgetCalendarFeedReadResponse> => {
      return apiRequest<ForgetCalendarFeedReadResponse>(
        API_ENDPOINTS.CALENDAR.FORGET_READS(client),
        { method: 'DELETE' }
      )
    },
    onSuccess: () => {
      invalidateQueries.calendar()
    },
  })
}

/**
 * Asks the server to email the signed-in person the steps for adding the
 * calendar to Google on a computer. Resolves on the 204; rejects with the
 * request's error, which `emailCalendarStepsFailure` sorts.
 */
export function sendCalendarStepsEmail(): Promise<void> {
  return apiRequest<void>(API_ENDPOINTS.CALENDAR.EMAIL_STEPS, {
    method: 'POST',
  })
}

export const useEmailCalendarSteps = () =>
  useMutation({ mutationFn: sendCalendarStepsEmail })

/**
 * Why sending the steps failed, as far as the reader is concerned: the address
 * is not verified (or the account has none), the hourly allowance is used up,
 * or anything else, including email being unavailable on the server.
 */
export type EmailCalendarStepsFailure = 'unverified' | 'rate_limited' | 'failed'

export function emailCalendarStepsFailure(
  error: unknown
): EmailCalendarStepsFailure {
  const code = isAuthError(error)
    ? error.code
    : (error as ApiError | null | undefined)?.errorCode
  if (code === 'EMAIL_VERIFICATION_REQUIRED') return 'unverified'
  if (code === 'CALENDAR_STEPS_RATE_LIMITED_HOURLY') return 'rate_limited'
  return 'failed'
}

/**
 * Calendar caches after a security action (signing out everywhere else, or a
 * password reset), which deletes the account's feed link and can leave a
 * link-reset marker.
 *
 * The feed status is RESET rather than refetched, so its pre-action value is
 * gone the moment the action succeeds: no surface goes on saying there is a
 * link, the one the action deleted, while the new read is in flight.
 * Resetting cancels any fetch in flight, and an active query refetches at
 * once. The token status refetches in place instead, so the Settings calendar
 * card keeps its content on screen until the new answer lands. The feed cards
 * hold a link they just minted outside the cache, so they are told the link is
 * gone as well.
 *
 * Never rejects.
 */
export function forgetRevokedCalendarLink(
  queryClient: QueryClient
): Promise<unknown> {
  window.dispatchEvent(new CustomEvent(PERSONAL_FEED_TOKEN_REVOKED_EVENT))
  return Promise.all([
    queryClient.resetQueries({ queryKey: queryKeys.calendar.feedStatus }),
    refetchAfterMutation(queryClient, queryKeys.calendar.tokenStatus),
  ])
}

/**
 * Hook to create (or regenerate) a calendar feed token
 */
export const useCreateCalendarToken = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (): Promise<CalendarTokenCreateResponse> => {
      return apiRequest<CalendarTokenCreateResponse>(
        API_ENDPOINTS.CALENDAR.TOKEN,
        { method: 'POST' }
      )
    },
    onSuccess: () => {
      invalidateQueries.calendar()
    },
  })
}

/**
 * Hook to delete (disable) a calendar feed token
 */
export const useDeleteCalendarToken = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (): Promise<CalendarTokenDeleteResponse> => {
      return apiRequest<CalendarTokenDeleteResponse>(
        API_ENDPOINTS.CALENDAR.TOKEN,
        { method: 'DELETE' }
      )
    },
    onSuccess: () => {
      invalidateQueries.calendar()
    },
  })
}
