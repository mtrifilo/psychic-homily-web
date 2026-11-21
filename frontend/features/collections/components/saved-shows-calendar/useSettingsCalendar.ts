'use client'

import { useState } from 'react'
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
 * `shownReason` is the reset the card was stating when the person clicked: a
 * link found already made (by another window, or by an earlier attempt whose
 * answer was lost) still follows that reset.
 * Creating replaces any link the account holds, so the read keeps this click
 * from cutting off a calendar another tab set up since the card last read the
 * status; a link made between the read and the create is still replaced. Any
 * failure is the one approved line for this control, so the request's own
 * error is thrown as it is, which keeps an expired session recoverable.
 */
async function readOrCreateLink(
  shownReason: CalendarLinkResetReason | null
): Promise<AddedCalendar> {
  const current = await apiRequest<CalendarTokenStatusResponse>(
    API_ENDPOINTS.CALENDAR.TOKEN,
    { method: 'GET' }
  )
  if (current.has_token) return { status: current, resetReason: shownReason }

  const created = await apiRequest<CalendarTokenCreateResponse>(
    API_ENDPOINTS.CALENDAR.TOKEN,
    { method: 'POST' }
  )
  return {
    status: statusOfNewLink(created),
    resetReason: calendarResetReason(current) ?? shownReason,
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
 * A reset marker kept for the link "Add to calendar" made after it. `feedUrl`
 * is unknown after a failed attempt (its create may still have committed); it
 * is bound to the next link the card reads.
 */
interface HeldResetReason {
  reason: CalendarLinkResetReason
  feedUrl: string | undefined
}

/**
 * "Add to calendar": the one action that makes a link from Settings. Opening
 * Settings never does. Every calendar read is refetched when it settles, on
 * failure too, since a create whose answer was lost may have committed.
 */
function useAddCalendar(onHeld: (held: HeldResetReason) => void) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: readOrCreateLink,
    onSuccess: ({ status, resetReason }) => {
      showCalendarStatus(queryClient, status)
      if (resetReason) onHeld({ reason: resetReason, feedUrl: status.feed_url })
    },
    onError: (_error, shownReason) => {
      if (shownReason) onHeld({ reason: shownReason, feedUrl: undefined })
    },
    onSettled: () => {
      void createInvalidateQueries(queryClient).calendar()
    },
  })
}

/**
 * Everything the Settings calendar card reads, reduced to its state.
 *
 * Only status data written to the cache since this mount is shown (counted,
 * not timed, so a write in the same millisecond as the mount does not pass),
 * so no address held from before (one since replaced, or another account's
 * from earlier in the tab) is ever painted. The status is read live (see
 * useCalendarTokenStatus): again on every mount and whenever the page is
 * shown again, since a calendar app's first check is made while the person is
 * in that app and the address may have been replaced in another window. A connected app's line compares its last
 * check with the newest save once that read lands; the saved-shows read never
 * holds the card back, and while it is pending or after it fails the line
 * names only the check, as the panel's does.
 *
 * The reset sentence keeps showing after "Add to calendar" made a new link in
 * this visit, because creating a link clears the marker on the server and the
 * person still has the old calendar to remove. It is kept for that link only:
 * once the address changes (a new link, a turn-off, or a change another
 * window made) it ends.
 */
export function useSettingsCalendar() {
  const { user } = useAuthContext()
  const userId = user?.id
  const queryClient = useQueryClient()
  const statusState = () =>
    queryClient.getQueryState(queryKeys.calendar.tokenStatus)
  const [countsAtMount] = useState(() => ({
    data: statusState()?.dataUpdateCount ?? 0,
    error: statusState()?.errorUpdateCount ?? 0,
  }))
  const status = useCalendarTokenStatus(!!userId, { live: true })
  // Sorted newest save first when no time filter is given.
  const newest = useSavedShows({ limit: 1, userId, retryUntilRead: true })
  const device = useCalendarDevice()
  const [heldResetReason, setHeldResetReason] =
    useState<HeldResetReason | null>(null)
  const add = useAddCalendar(setHeldResetReason)
  // After New link or Turn off settles, an earlier failed "Add to calendar"
  // is no longer news.
  const onLinkWritten = () => add.reset()

  return {
    state: settingsCalendarState(),
    add,
    onLinkWritten,
  }

  function settingsCalendarState(): SettingsCalendarState {
    // React Query re-renders a component only for the result properties it
    // reads. `isFetchedAfterMount` flips on the first read to settle after
    // this mount, even one that returns the data already cached, and
    // `status` and `errorUpdateCount` change with every failure and recovery,
    // so reading them here is what lets the count gate below open.
    const fetchedSinceMount = status.isFetchedAfterMount
    const dataWrittenSinceMount =
      (statusState()?.dataUpdateCount ?? 0) > countsAtMount.data
    const current =
      fetchedSinceMount && dataWrittenSinceMount ? status.data : undefined
    if (!current) {
      // A failure since this mount, kept while its retries run, and never one
      // another surface left in the shared cache.
      return status.status === 'error' &&
        status.errorUpdateCount > countsAtMount.error
        ? { kind: 'load_failed', retry: () => void status.refetch() }
        : { kind: 'loading' }
    }
    if (!current.has_token) {
      return { kind: 'no_link', resetReason: calendarResetReason(current) }
    }
    // A reason held from a failed attempt belongs to the first link read
    // after it. Adjusted during render, the house way to follow new data
    // without an effect.
    if (heldResetReason && heldResetReason.feedUrl === undefined && current.feed_url) {
      setHeldResetReason({ ...heldResetReason, feedUrl: current.feed_url })
    }

    if (!device) return { kind: 'loading' }

    const links =
      current.url_recoverable && current.feed_url && current.follows_feed_url
        ? { feedUrl: current.feed_url, followsFeedUrl: current.follows_feed_url }
        : null
    const model = buildSavedShowsCalendarModel({
      status: current,
      unrecoverable: !links,
      // An unbound reason (feedUrl unknown) never matches, not even a link
      // whose address cannot be shown.
      resetReasonBeforeCreate:
        heldResetReason?.feedUrl !== undefined &&
        heldResetReason.feedUrl === current.feed_url
          ? heldResetReason.reason
          : null,
      // Pending or failed reads as no saved shows: the line then names only
      // the check, and gains its comparison when the read lands.
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
