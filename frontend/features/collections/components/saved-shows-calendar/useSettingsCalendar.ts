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

/** Which step of "Add to calendar" failed. */
export type AddCalendarFailure = 'load' | 'create'

const FAILED_STEP = Symbol('settingsCalendarAddFailedStep')

/**
 * Marks the original error with the step that failed rather than wrapping it,
 * so the mutation cache still sees an expired session's AuthError and
 * recovers from it.
 */
function failedAt(step: AddCalendarFailure, error: unknown): unknown {
  if (error !== null && typeof error === 'object') {
    return Object.assign(error, { [FAILED_STEP]: step })
  }
  return Object.assign(new Error(String(error)), { [FAILED_STEP]: step })
}

interface AddedCalendar {
  status: CalendarTokenStatusResponse
  /** The reset marker the account carried before this click created a link. */
  resetReason: CalendarLinkResetReason | null
}

/**
 * Reads the link fresh and creates one only when that read says there is none:
 * creating replaces any link the account holds, which would cut off a calendar
 * that another tab set up since this card last read the status.
 */
async function readOrCreateLink(): Promise<AddedCalendar> {
  let current: CalendarTokenStatusResponse
  try {
    current = await apiRequest<CalendarTokenStatusResponse>(
      API_ENDPOINTS.CALENDAR.TOKEN,
      { method: 'GET' }
    )
  } catch (error) {
    throw failedAt('load', error)
  }
  if (current.has_token) return { status: current, resetReason: null }

  try {
    const created = await apiRequest<CalendarTokenCreateResponse>(
      API_ENDPOINTS.CALENDAR.TOKEN,
      { method: 'POST' }
    )
    return {
      status: statusOfNewLink(created),
      resetReason: calendarResetReason(current),
    }
  } catch (error) {
    throw failedAt('create', error)
  }
}

export function addCalendarFailure(error: unknown): AddCalendarFailure {
  const step =
    error !== null && typeof error === 'object'
      ? (error as { [FAILED_STEP]?: AddCalendarFailure })[FAILED_STEP]
      : undefined
  return step ?? 'create'
}

/**
 * Puts the status a mutation answered with in the cache, so the card shows
 * the new state at once, then refetches every calendar read (the refetch
 * confirms it, and refreshes the feed status other surfaces read).
 */
export function applyCalendarStatus(
  queryClient: QueryClient,
  status: CalendarTokenStatusResponse
) {
  queryClient.setQueryData(queryKeys.calendar.tokenStatus, status)
  void createInvalidateQueries(queryClient).calendar()
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
      applyCalendarStatus(queryClient, status)
      onAdded(resetReason)
    },
  })
}

/**
 * Everything the Settings calendar card reads, reduced to its state.
 *
 * The status is read fresh on every mount and whenever the window regains
 * focus (see useCalendarTokenStatus `live`). A connected app's line compares
 * its last check with the newest save; a failed saved-shows read drops that
 * comparison rather than blocking the card, as the panel does.
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
  // Sorted newest save first when no time filter is given.
  const newest = useSavedShows({ limit: 1, userId, retryUntilRead: true })
  const device = useCalendarDevice()
  const [mountedAt] = useState(() => Date.now())
  const [heldResetReason, setHeldResetReason] =
    useState<CalendarLinkResetReason | null>(null)
  const add = useAddCalendar(setHeldResetReason)
  const forgetResetReason = () => setHeldResetReason(null)

  return {
    state: settingsCalendarState(),
    add,
    forgetResetReason,
  }

  function settingsCalendarState(): SettingsCalendarState {
    const current = status.data
    if (!current) {
      return status.isError && !status.isFetching
        ? { kind: 'load_failed', retry: () => void status.refetch() }
        : { kind: 'loading' }
    }
    if (!current.has_token) {
      return { kind: 'no_link', resetReason: calendarResetReason(current) }
    }

    // A failure since this mount, which survives the pending frame of each
    // retry and ignores one another surface left in the shared cache.
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
