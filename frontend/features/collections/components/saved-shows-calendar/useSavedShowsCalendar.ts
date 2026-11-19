'use client'

import {
  useCalendarFeedStatus,
  useCalendarTokenStatus,
} from '@/features/auth/hooks/useCalendarFeed'
import { useSavedShows } from '@/features/shows/hooks/useSavedShows'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  useCalendarDevice,
  type CalendarDevice,
} from '@/features/collections/lib/calendarDevice'
import {
  buildSavedShowsCalendarModel,
  type SavedShowsCalendarModel,
} from './savedShowsCalendarModel'

export type SavedShowsCalendarState =
  | { kind: 'loading' }
  | {
      kind: 'ready'
      model: SavedShowsCalendarModel
      device: CalendarDevice
      hasToken: boolean
    }

/**
 * Everything the open panel reads, reduced to its model. Mounted only while
 * the panel is open, so the status refetches each time it opens and again
 * whenever the window regains focus: that is how a first check made while the
 * person was in their calendar app shows up when they come back.
 *
 * Loading until the status has been read since this mount, and the newest
 * save, the upcoming count and the device are known, since every row and line
 * depends on one of them. A status cached by another surface is never acted
 * on: whether a row creates a link or loads the existing one is decided by
 * it, and creating one replaces any link a calendar already uses. A failed
 * read stays loading and is retried on the next focus; the panel has no
 * wording for a status it could not read, and never guesses one.
 */
export function useSavedShowsCalendar(): SavedShowsCalendarState {
  const { user } = useAuthContext()
  const userId = user?.id
  const status = useCalendarFeedStatus(true, { live: true })
  // Read from the cache only; it is filled when a row loads the address.
  const tokenStatus = useCalendarTokenStatus(false)
  // Sorted newest save first when no time filter is given.
  const newest = useSavedShows({ limit: 1, userId, retryOnFocus: true })
  const upcoming = useSavedShows({
    limit: 1,
    userId,
    timeFilter: 'upcoming',
    retryOnFocus: true,
  })
  const device = useCalendarDevice()

  const currentStatus =
    status.isFetchedAfterMount && !status.isError ? status.data : undefined
  if (!currentStatus || !newest.data || !upcoming.data || !device) {
    return { kind: 'loading' }
  }

  const newestSavedAt = newest.data.shows[0]?.saved_at ?? null
  const model = buildSavedShowsCalendarModel({
    status: currentStatus,
    urlRecoverable: tokenStatus.data?.has_token
      ? tokenStatus.data.url_recoverable
      : undefined,
    newestSavedAt,
    upcomingCount: upcoming.data.total,
    device,
    // Read at render, so an age is as fresh as the panel's last render; the
    // lines do not tick while the panel sits open.
    now: new Date(),
  })
  return {
    kind: 'ready',
    model,
    device,
    hasToken: currentStatus.has_token,
  }
}
