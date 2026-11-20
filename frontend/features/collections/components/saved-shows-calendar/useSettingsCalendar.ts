'use client'

import { useEffect, useState } from 'react'
import {
  useMutation,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { createInvalidateQueries, queryKeys } from '@/lib/queryClient'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useCalendarTokenStatus } from '@/features/auth/hooks/useCalendarFeed'
import { useSavedShows } from '@/features/shows/hooks/useSavedShows'
import { calendarResetReason } from '@/features/collections/lib/calendarFeedStatus'
import {
  useCalendarDevice,
  type CalendarDevice,
} from '@/features/collections/lib/calendarDevice'
import type {
  CalendarLinkResetReason,
  CalendarTokenCreateResponse,
  CalendarTokenStatusResponse,
} from '@/features/shows/types'
import {
  buildSavedShowsCalendarModel,
  type SavedShowsCalendarModel,
} from './savedShowsCalendarModel'

/** The two private addresses one key makes. */
export interface CalendarLinks {
  feedUrl: string
  followsFeedUrl: string
}

export type SettingsCalendarState =
  | { kind: 'loading' }
  /** The status could not be read; nothing can be shown or changed. */
  | { kind: 'load_failed'; retry: () => void }
  /** No link: never set up, turned off, or deleted by a security action. */
  | { kind: 'no_link'; resetReason: CalendarLinkResetReason | null }
  | {
      kind: 'link'
      model: SavedShowsCalendarModel
      device: CalendarDevice
      /** The addresses, or null when the link's address cannot be shown. */
      links: CalendarLinks | null
    }

/** The token status a new link has: its addresses, and nothing read yet. */
export function statusOfNewLink(
  created: CalendarTokenCreateResponse
): CalendarTokenStatusResponse {
  return {
    has_token: true,
    created_at: created.created_at,
    feed_url: created.feed_url,
    follows_feed_url: created.follows_feed_url,
    url_recoverable: true,
  }
}

interface AddedCalendar {
  status: CalendarTokenStatusResponse
  /** The reset marker the account carried before this click created a link. */
  resetReason: CalendarLinkResetReason | null
}

/**
 * Reads the link fresh and creates one only when that read says there is none.
 * Creating replaces any link the account holds, so the read keeps this click
 * from cutting off a calendar another tab set up since the card last read the
 * status; a link made between the read and the create is still replaced. Any
 * failure is the one approved line for this control, so the request's own
 * error is thrown as it is, which keeps an expired session recoverable.
 */
async function readOrCreateLink(): Promise<AddedCalendar> {
  const current = await apiRequest<CalendarTokenStatusResponse>(
    API_ENDPOINTS.CALENDAR.TOKEN,
    { method: 'GET' }
  )
  if (current.has_token) return { status: current, resetReason: null }

  const created = await apiRequest<CalendarTokenCreateResponse>(
    API_ENDPOINTS.CALENDAR.TOKEN,
    { method: 'POST' }
  )
  return {
    status: statusOfNewLink(created),
    resetReason: calendarResetReason(current),
  }
}

/**
 * Puts the status a write answered with in the cache, so the card shows the
 * new state at once. The caller refetches every calendar read after it, which
 * confirms it and refreshes the feed status other surfaces read.
 */
export function showCalendarStatus(
  queryClient: QueryClient,
  status: CalendarTokenStatusResponse
) {
  queryClient.setQueryData(queryKeys.calendar.tokenStatus, status)
}

/**
 * "Add to calendar": the one action that makes a link from Settings. Opening
 * Settings never does.
 */
function useAddCalendar(onAdded: (resetReason: CalendarLinkResetReason | null) => void) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: readOrCreateLink,
    onSuccess: ({ status, resetReason }) => {
      showCalendarStatus(queryClient, status)
      void createInvalidateQueries(queryClient).calendar()
      onAdded(resetReason)
    },
  })
}

/**
 * Everything the Settings calendar card reads, reduced to its state.
 *
 * Only a status read since this mount is shown, so no address held in the
 * cache from before (one since replaced, or another account's from earlier in
 * the tab) is ever painted. The status is read again whenever the page becomes
 * visible or the window regains focus: a calendar app's first check is made
 * while the person is in that app, and the address may have been replaced in
 * another window. A connected app's line compares its last check with the
 * newest save; a failed saved-shows read drops that comparison rather than
 * blocking the card, as the panel does.
 *
 * The reset sentence keeps showing after "Add to calendar" made a new link in
 * this visit, because creating a link clears the marker on the server and the
 * person still has the old calendar to remove. Making a new link or turning
 * the link off ends it.
 */
export function useSettingsCalendar() {
  const { user } = useAuthContext()
  const userId = user?.id
  const status = useCalendarTokenStatus(!!userId, { live: true })
  const refetchStatus = status.refetch
  // The live option follows page visibility; a window that only loses and
  // regains focus (a calendar app opened beside the browser) is caught here.
  useEffect(() => {
    const onFocus = () => void refetchStatus({ cancelRefetch: false })
    window.addEventListener('focus', onFocus)
    return () => window.removeEventListener('focus', onFocus)
  }, [refetchStatus])
  // Sorted newest save first when no time filter is given.
  const newest = useSavedShows({ limit: 1, userId, retryUntilRead: true })
  const device = useCalendarDevice()
  const [mountedAt] = useState(() => Date.now())
  const [heldResetReason, setHeldResetReason] =
    useState<CalendarLinkResetReason | null>(null)
  const add = useAddCalendar(setHeldResetReason)
  // A new link or a turn-off ends the visit's reset sentence, and an earlier
  // failed "Add to calendar" is no longer news.
  const onLinkChanged = () => {
    setHeldResetReason(null)
    add.reset()
  }

  return {
    state: settingsCalendarState(),
    add,
    onLinkChanged,
  }

  function settingsCalendarState(): SettingsCalendarState {
    const current =
      status.data && status.dataUpdatedAt >= mountedAt ? status.data : undefined
    if (!current) {
      // A failure since this mount, which survives the pending frame of each
      // retry and ignores one another surface left in the shared cache.
      return status.errorUpdatedAt >= mountedAt
        ? { kind: 'load_failed', retry: () => void status.refetch() }
        : { kind: 'loading' }
    }
    if (!current.has_token) {
      return { kind: 'no_link', resetReason: calendarResetReason(current) }
    }

    const newestSettled =
      !!newest.data || newest.errorUpdatedAt >= mountedAt
    if (!device || !newestSettled) return { kind: 'loading' }

    const links =
      current.url_recoverable && current.feed_url && current.follows_feed_url
        ? { feedUrl: current.feed_url, followsFeedUrl: current.follows_feed_url }
        : null
    const model = buildSavedShowsCalendarModel({
      status: current,
      unrecoverable: !links,
      resetReasonBeforeCreate: heldResetReason,
      // Unknown reads as no saved shows: the line then names only the check.
      newestSavedAt: newest.data?.shows[0]?.saved_at ?? null,
      // The card has no download row.
      upcomingCount: 0,
      device,
      // Read at render, so an age is as fresh as the card's last render.
      now: new Date(),
    })
    return { kind: 'link', model, device, links }
  }
}
