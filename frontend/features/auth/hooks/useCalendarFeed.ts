'use client'

import {
  useQuery,
  useMutation,
  useQueryClient,
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

/**
 * The calendar feed's full status for a session: whether a link exists, its
 * two addresses when they can be shown, the calendar apps seen reading it, and
 * the reset marker. Reading it never creates a link.
 */
export const useCalendarTokenStatus = (
  enabled: boolean = true,
  options: {
    /**
     * Refetch whenever an observer mounts and whenever the window regains
     * focus, and retry a failed read on its own, for the surface that shows
     * the addresses: an address held from before can be one that was since
     * replaced, and a calendar app's first check is made while the person is
     * in that app.
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
  } = {}
) => {
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
    ...(options.live
      ? {
          refetchOnMount: 'always',
          refetchOnWindowFocus: 'always',
          // A failed read is retried on its own until one succeeds. Whether
          // the data still held from before is fit to show is the surface's
          // call (the calendar panel shows only a read made since it opened),
          // so the retry does not depend on it. A refused session is not
          // retried: it will not change by waiting.
          refetchInterval: retryWhileFailed,
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
 * card keeps its content on screen until the new answer lands.
 *
 * Never rejects.
 */
export function forgetRevokedCalendarLink(
  queryClient: QueryClient
): Promise<unknown> {
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
