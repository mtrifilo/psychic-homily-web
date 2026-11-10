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
import type {
  CalendarFeedStatusResponse,
  CalendarLinkResetReason,
  CalendarTokenStatusResponse,
  CalendarTokenCreateResponse,
  CalendarTokenDeleteResponse,
} from '@/features/shows'

/**
 * Hook to check if the user has a calendar feed token
 */
export const useCalendarTokenStatus = (enabled: boolean = true) => {
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
  })
}

/**
 * The calendar feed's status with no feed address in it: whether a link
 * exists, the calendar apps seen reading it, and the marker a security action
 * leaves when it reset a link an app had been reading. For surfaces that talk
 * about the calendar without showing its address.
 */
export const useCalendarFeedStatus = (enabled: boolean = true) => {
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
  })
}

/**
 * Whether `status` says `reason` reset a calendar link that an app had been
 * reading. Unknown status (loading or failed) is false: the reader is never
 * told about a reset that was not confirmed.
 */
export function calendarLinkWasResetBy(
  status: CalendarFeedStatusResponse | undefined,
  reason: CalendarLinkResetReason
): boolean {
  return !!status?.link_reset_at && status.link_reset_reason === reason
}

/**
 * Calendar caches after a security action (signing out everywhere else, or a
 * password reset), which deletes the account's feed link and can leave a
 * link-reset marker.
 *
 * The feed status is RESET rather than refetched, so its pre-action value is
 * gone the moment the action succeeds: a surface reporting what the action did
 * to the link can only ever read a status fetched after it. Resetting cancels
 * any fetch in flight, and an active query refetches at once. The token status
 * refetches in place instead, so the Settings calendar card keeps its content
 * on screen until the new answer lands.
 *
 * Never rejects.
 */
export function refreshCalendarAfterLinkRevocation(
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
