'use client'

import { useState } from 'react'
import { useCalendarFeedStatus } from '@/features/auth/hooks/useCalendarFeed'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  calendarCheckedLine,
  classifyCalendarApps,
  type CalendarApp,
} from '@/features/collections/lib/calendarFeedStatus'
import type { CalendarFeedStatusResponse } from '@/features/shows/types'
import {
  panelLinkFailure,
  useSavedShowsCalendarLink,
  type PanelLinkFailure,
} from './useSavedShowsCalendarLink'

/** The link the steps page acts on. */
export type CalendarStepsLink =
  | { kind: 'loading' }
  | { kind: 'failed'; failure: PanelLinkFailure; retry: () => void }
  /** The private address. The page acts on it and never shows it. */
  | { kind: 'ready'; feedUrl: string }
  /** A link exists, but its address was never stored and cannot be shown. */
  | { kind: 'unrecoverable' }

/**
 * The link, prepared once as the page mounts exactly as the calendar panel
 * prepares it on open (loaded fresh, and created only when the account has
 * none), so the copy and the calendar links act inside their own click.
 * Mounted only where the page acts on the address: never on Android, where
 * the page offers nothing that needs it.
 */
export function useCalendarStepsLink(): CalendarStepsLink {
  const { user } = useAuthContext()
  const link = useSavedShowsCalendarLink(user?.id)
  if (link.data) {
    return link.data.kind === 'ready'
      ? { kind: 'ready', feedUrl: link.data.feedUrl }
      : { kind: 'unrecoverable' }
  }
  if (link.isError && !link.isFetching) {
    return {
      kind: 'failed',
      failure: panelLinkFailure(link.error),
      retry: () => void link.refetch(),
    }
  }
  return { kind: 'loading' }
}

/** What the page knows about Google Calendar's checks of the link. */
export type GoogleCalendarCheck =
  /** No status read has finished since the page opened. */
  | { kind: 'unknown' }
  /** Every status read since the page opened failed. */
  | { kind: 'failed'; retry: () => void }
  | {
      kind: 'waiting'
      /** Some other calendar app is checking the link. */
      anyAppConnected: boolean
    }
  | {
      kind: 'connected'
      google: CalendarApp
      /** "Checked 2h ago.": the page states no comparison with saves. */
      checkedLine: string
      anyAppConnected: true
    }

function connectedGoogle(connected: CalendarApp[]): CalendarApp | undefined {
  return connected.find(app => app.client === 'google')
}

const googleIsConnected = (status: CalendarFeedStatusResponse) =>
  connectedGoogle(classifyCalendarApps(status, new Date()).connected) !==
  undefined

/**
 * Google Calendar's observed state, for the page. Only a status read since
 * the page opened counts, so a stale read cached by another surface never
 * says Google is connected or not. The read is live (again on window focus,
 * which is when a person comes back from Google's tab), and with
 * `pollUntilGoogle` it is also read on an interval until Google shows as
 * connected; the interval ends with the component that asked for it. Poll
 * only while the person can add Google from the page: with no address to copy,
 * nothing on it can bring Google's first check.
 */
export function useGoogleCalendarCheck({
  pollUntilGoogle,
}: {
  pollUntilGoogle: boolean
}): GoogleCalendarCheck {
  const status = useCalendarFeedStatus(true, {
    live: true,
    pollUntil: pollUntilGoogle ? googleIsConnected : undefined,
  })
  const [openedAt] = useState(() => Date.now())

  const current =
    status.data && status.dataUpdatedAt >= openedAt ? status.data : undefined
  if (!current) {
    // A failure before this open belongs to another surface's read.
    return status.errorUpdatedAt >= openedAt
      ? { kind: 'failed', retry: () => void status.refetch() }
      : { kind: 'unknown' }
  }

  // Read at render: the line is as fresh as the page's last render.
  const now = new Date()
  const { connected } = classifyCalendarApps(current, now)
  const google = connectedGoogle(connected)
  if (!google) {
    return { kind: 'waiting', anyAppConnected: connected.length > 0 }
  }
  return {
    kind: 'connected',
    google,
    checkedLine: calendarCheckedLine(google.lastReadAt, null, now),
    anyAppConnected: true,
  }
}
