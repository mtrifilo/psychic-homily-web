'use client'

import { useEffect, useRef, useState } from 'react'
import { focusManager, useMutation } from '@tanstack/react-query'
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
 * Google's first check, counted from the latest of arrival, the click that
 * made the link, and the page being shown again (the person coming back from
 * Google's tab). After that only the page being shown again reads it, so a
 * page left open does not poll for hours.
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
 * the status is also read on an interval, within GOOGLE_CHECK_POLL_LIMIT_MS;
 * the interval ends with the page.
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
  // When this visit's Add to calendar last finished (a link made, or found
  // made elsewhere); 0 until it has. Reads older than it are not trusted.
  const [addedAt, setAddedAt] = useState(0)
  // Add to calendar reads the link fresh and makes one only if that read says
  // there is none: making a link replaces any the account holds, so a link
  // made elsewhere since the page last read (another device, another tab)
  // must not be cut off by a click on a button drawn from that older read.
  // When the page was last shown again; the poll's limit counts from it too.
  const shownAt = useRef(0)
  useEffect(
    () =>
      focusManager.subscribe(focused => {
        if (focused) shownAt.current = Date.now()
      }),
    []
  )
  const add = useMutation({
    mutationFn: async () => {
      const fresh = await token.refetch({ throwOnError: true })
      if (fresh.data?.has_token) {
        // Nothing is made, so nothing refreshes the status: read it for the
        // link found.
        void status.refetch()
        return
      }
      await create.mutateAsync()
    },
    onSuccess: () => setAddedAt(Date.now()),
  })

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
    create.data && addedAt > token.dataUpdatedAt ? create.data : null
  const heldUrl = created
    ? created.feed_url
    : currentToken?.has_token && currentToken.url_recoverable
      ? currentToken.feed_url
      : undefined
  // When the link the page holds was made: it tells a status read about that
  // link from one about a link made since.
  const heldCreatedAt = created ? created.created_at : currentToken?.created_at

  const status = useCalendarFeedStatus(true, {
    live: true,
    pollUntil:
      pollGoogle && heldUrl
        ? (read: CalendarFeedStatusResponse) =>
            !read.has_token ||
            googleConnected(classifyCalendarApps(read, new Date())) ||
            Date.now() - Math.max(openedAt, addedAt, shownAt.current) >=
              GOOGLE_CHECK_POLL_LIMIT_MS
        : undefined,
  })
  const currentStatus = readSinceOpen(status)
  // The status is read more often than the link (on the interval). When a
  // status read newer than both the link read and this visit's click says
  // there is no link, or describes a link made at another time, the link was
  // turned off or replaced elsewhere: the address held is dead, and the page
  // offers the one button again, whose click reads the link fresh and makes
  // one only if there is none.
  const statusNewer =
    status.dataUpdatedAt > Math.max(token.dataUpdatedAt, addedAt)
  const statusSaysNoLink =
    statusNewer &&
    currentStatus !== undefined &&
    (!currentStatus.has_token ||
      (heldUrl !== undefined &&
        !!currentStatus.created_at &&
        !!heldCreatedAt &&
        Date.parse(currentStatus.created_at) !== Date.parse(heldCreatedAt)))
  const feedUrl = statusSaysNoLink ? undefined : heldUrl

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
    : currentToken?.has_token && !statusSaysNoLink
      ? { kind: 'unrecoverable' }
      : {
          kind: 'none',
          create: () => {
            // Making the link clears the marker on the server; the page goes
            // on saying why the old calendar stopped for this visit.
            setKeptResetReason(
              reason => reason ?? calendarResetReason(currentStatus)
            )
            add.mutate()
          },
          creating: add.isPending,
          createFailed: add.isError,
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
