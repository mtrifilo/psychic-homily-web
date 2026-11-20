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
import { useState } from 'react'
import {
  panelLinkFailure,
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
 * Loading until the link is ready, the status has been read successfully
 * since this mount, and the newest save, the upcoming count and the device
 * are known, since every row and line depends on one of them. A status
 * cached by another surface is never shown. Once read, a later refetch that
 * fails keeps the last good read on screen; until a first read succeeds the
 * panel stays loading while the status query retries on its own. The panel has no wording for a status it
 * could not read, and never guesses one.
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
  // When this open began; a status read before it came from another surface.
  const [openedAt] = useState(() => Date.now())

  if (!link.data && link.isError && !link.isFetching) {
    return {
      kind: 'link_failed',
      failure: panelLinkFailure(link.error),
      retry: () => void link.refetch(),
    }
  }

  const currentStatus =
    status.data && status.dataUpdatedAt >= openedAt ? status.data : undefined
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
