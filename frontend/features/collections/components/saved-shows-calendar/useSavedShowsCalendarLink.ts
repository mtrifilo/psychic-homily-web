'use client'

import { useState } from 'react'
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

/** Which step of preparing the link failed. */
export type PanelLinkFailure = 'load' | 'create'

const FAILED_STEP = Symbol('calendarPanelLinkFailedStep')

/**
 * Marks the original error with the step that failed rather than wrapping it,
 * so the query cache still sees an expired session's AuthError and recovers
 * from it, and the request id and status stay attached.
 */
function failedAt(step: PanelLinkFailure, error: unknown): unknown {
  if (error !== null && typeof error === 'object') {
    Object.assign(error, { [FAILED_STEP]: step })
    return error
  }
  return Object.assign(new Error(String(error)), { [FAILED_STEP]: step })
}

export function panelLinkFailure(error: unknown): PanelLinkFailure {
  const step =
    error !== null && typeof error === 'object'
      ? (error as { [FAILED_STEP]?: PanelLinkFailure })[FAILED_STEP]
      : undefined
  return step ?? 'load'
}

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
    throw failedAt('load', error)
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
    throw failedAt('create', error)
  }
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
  // The viewer this open belongs to. A different viewer never prepares a
  // link through it: that could create one for an account that never asked.
  const [openedBy] = useState(userId)
  return useQuery({
    queryKey: queryKeys.calendar.panelLink(openedBy),
    queryFn: () => preparePanelLink(queryClient),
    enabled: !!openedBy && userId === openedBy,
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
}
