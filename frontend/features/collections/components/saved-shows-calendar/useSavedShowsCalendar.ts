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
 * since this mount, the saved-shows reads have settled, and the device is
 * known. A status cached by another surface is never shown; once read, a
 * later refetch that fails keeps the last good read on screen.
 *
 * Failed first reads: the link or the status says the link could not be
 * loaded (with its retry), while the status keeps retrying on its own and the
 * panel becomes ready when one succeeds. A failed saved-shows read blocks
 * nothing: the panel states only what it knows, so each check line drops its
 * comparison with the newest save and the download row waits for the count.
 */
export function useSavedShowsCalendar(): SavedShowsCalendarState {
  const { user } = useAuthContext()
  const userId = user?.id
  const link = useSavedShowsCalendarLink(userId)
  const status = useCalendarFeedStatus(!!userId, { live: true })
  // Sorted newest save first when no time filter is given.
  const newest = useSavedShows({ limit: 1, userId, retryUntilRead: true })
  const upcoming = useSavedShows({
    limit: 1,
    userId,
    timeFilter: 'upcoming',
    retryUntilRead: true,
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
  // "Failed since this open", which survives the pending frame of each retry
  // (a retry clears the query's error until it settles) and ignores a failure
  // another surface left in the shared cache before this open.
  const failedSinceOpen = (query: { errorUpdatedAt: number }) =>
    query.errorUpdatedAt >= openedAt
  if (link.data && !currentStatus && failedSinceOpen(status)) {
    return {
      kind: 'link_failed',
      failure: 'load',
      retry: () => void status.refetch(),
    }
  }

  const newestSettled = !!newest.data || failedSinceOpen(newest)
  const upcomingSettled = !!upcoming.data || failedSinceOpen(upcoming)
  if (!link.data || !currentStatus || !newestSettled || !upcomingSettled || !device) {
    return { kind: 'loading' }
  }

  const ready = link.data.kind === 'ready' ? link.data : null
  const model = buildSavedShowsCalendarModel({
    status: currentStatus,
    unrecoverable: !ready,
    resetReasonBeforeCreate: ready?.resetReasonBeforeCreate ?? null,
    // Unknown reads as no saved shows: the line then names only the check.
    newestSavedAt: newest.data?.shows[0]?.saved_at ?? null,
    // Unknown hides the download row until the count arrives.
    upcomingCount: upcoming.data?.total ?? 0,
    device,
    // Read at render, so an age is as fresh as the panel's last render; the
    // lines do not tick while the panel sits open.
    now: new Date(),
  })
  return { kind: 'ready', model, device, feedUrl: ready?.feedUrl ?? null }
}
