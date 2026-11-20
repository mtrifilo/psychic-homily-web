'use client'

import { useState } from 'react'
import {
  useCalendarFeedStatus,
  useCalendarTokenStatus,
  useCreateCalendarToken,
} from '@/features/auth/hooks/useCalendarFeed'
import {
  calendarResetReason,
  classifyCalendarApps,
  type CalendarApps,
} from '@/features/collections/lib/calendarFeedStatus'
import type {
  CalendarFeedStatusResponse,
  CalendarLinkResetReason,
} from '@/features/shows/types'

/**
 * How long the page reads the status on an interval while it waits for
 * Google's first check. After that only a return to the window reads it
 * again, so a page left open does not poll for hours.
 */
export const GOOGLE_CHECK_POLL_LIMIT_MS = 10 * 60 * 1000

/** The link the page acts on. */
export type CalendarStepsLink =
  /**
   * The account has no link. Only the person's click on Add to calendar
   * makes one: arriving here never does.
   */
  | {
      kind: 'none'
      create: () => void
      creating: boolean
      createFailed: boolean
    }
  /** The private address. The page acts on it and never shows it. */
  | { kind: 'ready'; feedUrl: string }
  /** A link exists, but its address was never stored and cannot be shown. */
  | { kind: 'unrecoverable' }

export type CalendarStepsPageState =
  /** A read has not finished since the page opened. */
  | { kind: 'loading' }
  /** A read failed since the page opened, with nothing read before it. */
  | { kind: 'read_failed'; retry: () => void }
  | {
      kind: 'ready'
      link: CalendarStepsLink
      apps: CalendarApps
      /**
       * Why a security action reset the link: set while the account has no
       * link and the marker says why, and kept for the rest of this visit
       * after Add to calendar clears the marker on the server.
       */
      resetReason: CalendarLinkResetReason | null
      /** When the lines were computed; ages read from it. */
      now: Date
    }

/**
 * Everything the page reads: the link (read only, never created on arrival)
 * and the status, each read live, so a link replaced or turned off in another
 * tab is followed when the person comes back to this one. A read cached by
 * another surface before the page opened is never shown.
 *
 * With `pollGoogle`, while the address is ready and Google is not connected,
 * the status is also read on an interval, for GOOGLE_CHECK_POLL_LIMIT_MS; the
 * interval ends with the page.
 */
export function useCalendarStepsPage({
  pollGoogle,
}: {
  pollGoogle: boolean
}): CalendarStepsPageState {
  const [openedAt] = useState(() => Date.now())
  const token = useCalendarTokenStatus(true, { live: true })
  const create = useCreateCalendarToken()
  const [keptResetReason, setKeptResetReason] =
    useState<CalendarLinkResetReason | null>(null)
  // When this visit's Add to calendar succeeded; 0 until it has.
  const [createdAt, setCreatedAt] = useState(0)

  const readSinceOpen = <T>(query: {
    data: T | undefined
    dataUpdatedAt: number
  }): T | undefined =>
    query.data !== undefined && query.dataUpdatedAt >= openedAt
      ? query.data
      : undefined

  const currentToken = readSinceOpen(token)
  // A link this visit created stands until a read finished after it says
  // otherwise (a later turn-off or replacement elsewhere).
  const created =
    create.data && createdAt > token.dataUpdatedAt ? create.data : null
  const feedUrl = created
    ? created.feed_url
    : currentToken?.has_token && currentToken.url_recoverable
      ? currentToken.feed_url
      : undefined

  const status = useCalendarFeedStatus(true, {
    live: true,
    pollUntil:
      pollGoogle && feedUrl
        ? (read: CalendarFeedStatusResponse) =>
            googleConnected(classifyCalendarApps(read, new Date())) ||
            Date.now() - openedAt >= GOOGLE_CHECK_POLL_LIMIT_MS
        : undefined,
  })
  const currentStatus = readSinceOpen(status)

  const failedSinceOpen = (query: { errorUpdatedAt: number }) =>
    query.errorUpdatedAt >= openedAt
  if (
    (!currentToken && !created && failedSinceOpen(token)) ||
    (!currentStatus && failedSinceOpen(status))
  ) {
    return {
      kind: 'read_failed',
      retry: () => {
        void token.refetch()
        void status.refetch()
      },
    }
  }
  if ((!currentToken && !created) || !currentStatus) return { kind: 'loading' }

  const link: CalendarStepsLink = feedUrl
    ? { kind: 'ready', feedUrl }
    : currentToken?.has_token
      ? { kind: 'unrecoverable' }
      : {
          kind: 'none',
          create: () => {
            // Making the link clears the marker on the server; the page goes
            // on saying why the old calendar stopped for this visit.
            setKeptResetReason(
              reason => reason ?? calendarResetReason(currentStatus)
            )
            create.mutate(undefined, {
              onSuccess: () => setCreatedAt(Date.now()),
            })
          },
          creating: create.isPending,
          createFailed: create.isError,
        }

  // Read at render: the lines are as fresh as the page's last render.
  const now = new Date()
  return {
    kind: 'ready',
    link,
    apps: classifyCalendarApps(currentStatus, now),
    resetReason:
      keptResetReason ??
      (link.kind === 'none' ? calendarResetReason(currentStatus) : null),
    now,
  }
}

export function googleConnected(apps: CalendarApps): boolean {
  return apps.connected.some(app => app.client === 'google')
}
