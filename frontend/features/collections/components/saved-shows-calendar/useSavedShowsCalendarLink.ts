'use client'

import { useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { refetchAfterMutation } from '@/lib/refetchAfterMutation'
import { calendarResetReason } from '@/features/collections/lib/calendarFeedStatus'
import type {
  CalendarLinkResetReason,
  CalendarTokenCreateResponse,
  CalendarTokenStatusResponse,
} from '@/features/shows/types'
import { PERSONAL_FEED_TOKEN_ROTATED_EVENT } from '../personalFeedTokenEvents'

/** The panel's link for this open. */
export type PanelLink =
  | {
      kind: 'ready'
      /** The private address. Rows act on it; the panel never shows it. */
      feedUrl: string
      /**
       * The reset marker the account carried before this open created its
       * new link. Creating a link clears the marker on the server, so the
       * panel keeps it to go on saying why the old calendar stopped.
       */
      resetReasonBeforeCreate: CalendarLinkResetReason | null
    }
  /** A link exists, but its address was never stored and cannot be shown. */
  | { kind: 'unrecoverable' }

export class LinkLoadError extends Error {}
export class LinkCreateError extends Error {}

/**
 * Reads the link fresh from the server and creates one only when that read
 * says there is none. Never from a cached status: creating a link replaces
 * any the account holds, which would cut off a calendar that is working.
 */
async function preparePanelLink(queryClient: QueryClient): Promise<PanelLink> {
  let current: CalendarTokenStatusResponse
  try {
    current = await apiRequest<CalendarTokenStatusResponse>(
      API_ENDPOINTS.CALENDAR.TOKEN,
      { method: 'GET' }
    )
  } catch (error) {
    throw new LinkLoadError(String(error))
  }

  if (current.has_token) {
    return current.url_recoverable && current.feed_url
      ? { kind: 'ready', feedUrl: current.feed_url, resetReasonBeforeCreate: null }
      : { kind: 'unrecoverable' }
  }

  let created: CalendarTokenCreateResponse
  try {
    created = await apiRequest<CalendarTokenCreateResponse>(
      API_ENDPOINTS.CALENDAR.TOKEN,
      { method: 'POST' }
    )
  } catch (error) {
    throw new LinkCreateError(String(error))
  }
  // The Library and Settings cards hold an address they created; they swap
  // it for this one, which replaced it.
  window.dispatchEvent(
    new CustomEvent(PERSONAL_FEED_TOKEN_ROTATED_EVENT, { detail: created })
  )
  void refetchAfterMutation(queryClient, queryKeys.calendar.feedStatus)
  void refetchAfterMutation(queryClient, queryKeys.calendar.tokenStatus)
  return {
    kind: 'ready',
    feedUrl: created.feed_url,
    resetReasonBeforeCreate: calendarResetReason(current),
  }
}

/**
 * The link the open panel acts on, prepared once per open: mounted with the
 * panel's contents and dropped from the cache as soon as they unmount, so
 * every open reads afresh and a closed panel holds no address.
 *
 * A query rather than a mutation fired from an effect, because the query
 * shares one request between observers that mount together (a development
 * double mount included); two requests could create two links, the second
 * replacing the first. Never retried on its own, for the same reason: a
 * failure is shown, and the person retries.
 */
export function useSavedShowsCalendarLink(userId: string | undefined) {
  const queryClient = useQueryClient()
  return useQuery({
    queryKey: queryKeys.calendar.panelLink(userId),
    queryFn: () => preparePanelLink(queryClient),
    enabled: !!userId,
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
}
