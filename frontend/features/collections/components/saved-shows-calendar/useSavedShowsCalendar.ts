'use client'

import { useCalendarFeedStatus } from '@/features/auth/hooks/useCalendarFeed'
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
import {
  LinkCreateError,
  useSavedShowsCalendarLink,
} from './useSavedShowsCalendarLink'

export type SavedShowsCalendarState =
  | { kind: 'loading' }
  | {
      /** The link could not be loaded or created; nothing can be added. */
      kind: 'link_failed'
      failure: 'create' | 'load'
      retry: () => void
    }
  | {
      kind: 'ready'
      model: SavedShowsCalendarModel
      device: CalendarDevice
      /** The private address, or null when it cannot be shown (state H). */
      feedUrl: string | null
    }

/**
 * Everything the open panel reads, reduced to its model. Mounted only while
 * the panel is open, so each open prepares the link afresh (see
 * useSavedShowsCalendarLink) and reads the status again, and the status is
 * read again whenever the window regains focus: that is how a first check
 * made while the person was in their calendar app shows up when they return.
 *
 * Loading until the link is ready, the status has been read since this mount,
 * and the newest save, the upcoming count and the device are known, since
 * every row and line depends on one of them. A failed status or saved-shows
 * read stays loading and is retried on the next focus; the panel has no
 * wording for a status it could not read, and never guesses one.
 */
export function useSavedShowsCalendar(): SavedShowsCalendarState {
  const { user } = useAuthContext()
  const userId = user?.id
  const link = useSavedShowsCalendarLink(userId)
  const status = useCalendarFeedStatus(true, { live: true })
  // Sorted newest save first when no time filter is given.
  const newest = useSavedShows({ limit: 1, userId, retryOnFocus: true })
  const upcoming = useSavedShows({
    limit: 1,
    userId,
    timeFilter: 'upcoming',
    retryOnFocus: true,
  })
  const device = useCalendarDevice()

  if (link.isError && !link.isFetching) {
    return {
      kind: 'link_failed',
      failure: link.error instanceof LinkCreateError ? 'create' : 'load',
      retry: () => void link.refetch(),
    }
  }

  const currentStatus =
    status.isFetchedAfterMount && !status.isError ? status.data : undefined
  if (!link.data || !currentStatus || !newest.data || !upcoming.data || !device) {
    return { kind: 'loading' }
  }

  const ready = link.data.kind === 'ready' ? link.data : null
  const model = buildSavedShowsCalendarModel({
    status: currentStatus,
    unrecoverable: !ready,
    resetReasonBeforeCreate: ready?.resetReasonBeforeCreate ?? null,
    newestSavedAt: newest.data.shows[0]?.saved_at ?? null,
    upcomingCount: upcoming.data.total,
    device,
    // Read at render, so an age is as fresh as the panel's last render; the
    // lines do not tick while the panel sits open.
    now: new Date(),
  })
  return { kind: 'ready', model, device, feedUrl: ready?.feedUrl ?? null }
}
