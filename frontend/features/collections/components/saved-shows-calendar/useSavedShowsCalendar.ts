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
 * Loading until the status, the newest save, the upcoming count and the
 * device are all known, since every row and line depends on one of them. A
 * failed read stays loading and is retried on the next focus or open; the
 * panel has no wording for a status it could not read, and never guesses one.
 */
export function useSavedShowsCalendar(): SavedShowsCalendarState {
  const { user } = useAuthContext()
  const userId = user?.id
  const status = useCalendarFeedStatus(true, { live: true })
  // Read from the cache only; it is filled when a row loads the address.
  const tokenStatus = useCalendarTokenStatus(false)
  // Sorted newest save first when no time filter is given.
  const newest = useSavedShows({ limit: 1, userId })
  const upcoming = useSavedShows({ limit: 1, userId, timeFilter: 'upcoming' })
  const device = useCalendarDevice()

  if (!status.data || !newest.data || !upcoming.data || !device) {
    return { kind: 'loading' }
  }

  const newestSavedAt = newest.data.shows[0]?.saved_at ?? null
  const model = buildSavedShowsCalendarModel({
    status: status.data,
    urlRecoverable: tokenStatus.data?.has_token
      ? tokenStatus.data.url_recoverable
      : undefined,
    newestSavedAt,
    upcomingCount: upcoming.data.total,
    device,
    // Read at render: every render follows a query update or a focus
    // refetch, which is when the ages are worth recomputing.
    now: new Date(),
  })
  return {
    kind: 'ready',
    model,
    device,
    hasToken: status.data.has_token,
  }
}
