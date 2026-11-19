'use client'

import { useCallback, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  calendarTokenStatusQuery,
  useCreateCalendarToken,
} from '@/features/auth/hooks/useCalendarFeed'
import {
  GOOGLE_CALENDAR_ADD_BY_URL_PAGE,
  outlookComSubscribeUrl,
  outlookWorkSubscribeUrl,
  webcalUrl,
} from '@/lib/utils/calendarFeedUrls'
import { PERSONAL_FEED_TOKEN_ROTATED_EVENT } from '../personalFeedTokenEvents'
import {
  ClipboardRefusedError,
  copyWhenResolved,
  openInCalendarApp,
  openPendingTab,
  openTabNow,
} from './calendarPanelBrowser'
import type { CalendarRow } from './savedShowsCalendarModel'
import type { CalendarAddTarget } from '@/features/collections/lib/calendarFeedStatus'

/** What a row reports under itself after it was chosen. */
export type CalendarRowNote =
  | 'google_pending'
  | 'apple_opened'
  | 'outlook_opened'
  | 'copied'
  | 'create_failed'
  | 'load_failed'
  | 'copy_failed'

type CalendarRowNotes = Partial<Record<CalendarRow, CalendarRowNote>>

class LinkCreateError extends Error {}
class LinkLoadError extends Error {}
/** The link exists but its address cannot be shown; the panel says so itself. */
class LinkUnrecoverableError extends Error {}

function noteForFailure(error: unknown): CalendarRowNote | null {
  if (error instanceof LinkUnrecoverableError) return null
  if (error instanceof LinkCreateError) return 'create_failed'
  if (error instanceof ClipboardRefusedError) return 'copy_failed'
  return 'load_failed'
}

/**
 * The row actions and the notes they leave.
 *
 * Every row needs the link's address, which this panel never holds until a
 * row is chosen: with no link it creates one (the "mint on pick"), otherwise
 * it reads the token status. The tab-opening and copying rows start their
 * browser side effect in the click itself (a tab opened blank, a clipboard
 * write whose content is still a promise) and complete it when the address
 * arrives, so a popup blocker or Safari's clipboard rule never sees them come
 * after an await. The Apple row cannot: a webcal hand-off has no blank form to
 * open early, so it navigates once the address arrives.
 */
export function useSavedShowsCalendarActions(hasToken: boolean) {
  const queryClient = useQueryClient()
  const { mutateAsync: createToken } = useCreateCalendarToken()
  const [notes, setNotes] = useState<CalendarRowNotes>({})
  // The address this panel created. Held so a second choice before the status
  // refetch lands reuses it instead of creating a link that replaces it.
  const mintedAddress = useRef<string | null>(null)
  const pendingAddress = useRef<Promise<string> | null>(null)

  const loadAddress = useCallback(async (): Promise<string> => {
    let status
    try {
      // Never from cache: a link replaced elsewhere minutes ago must not be
      // handed out as this one.
      status = await queryClient.fetchQuery({
        ...calendarTokenStatusQuery,
        staleTime: 0,
      })
    } catch (error) {
      throw new LinkLoadError(String(error))
    }
    if (status.has_token && status.url_recoverable && status.feed_url) {
      return status.feed_url
    }
    if (status.has_token) throw new LinkUnrecoverableError()
    throw new LinkLoadError('The calendar link no longer exists.')
  }, [queryClient])

  const mintAddress = useCallback(async (): Promise<string> => {
    let created
    try {
      created = await createToken()
    } catch (error) {
      throw new LinkCreateError(String(error))
    }
    mintedAddress.current = created.feed_url
    // The Library and Settings cards hold an address they minted; they swap
    // it for this one, which replaced it.
    window.dispatchEvent(
      new CustomEvent(PERSONAL_FEED_TOKEN_ROTATED_EVENT, { detail: created })
    )
    return created.feed_url
  }, [createToken])

  const resolveAddress = useCallback((): Promise<string> => {
    if (mintedAddress.current) return Promise.resolve(mintedAddress.current)
    if (pendingAddress.current) return pendingAddress.current
    const address = hasToken ? loadAddress() : mintAddress()
    pendingAddress.current = address
    address
      .finally(() => {
        if (pendingAddress.current === address) pendingAddress.current = null
      })
      .catch(() => {})
    return address
  }, [hasToken, loadAddress, mintAddress])

  const setNote = useCallback(
    (row: CalendarRow, note: CalendarRowNote | null) =>
      setNotes(current => {
        const next = { ...current }
        if (note) next[row] = note
        else delete next[row]
        return next
      }),
    []
  )

  /**
   * Runs `act` for `row`, clearing its old note first and recording the
   * outcome. `cleanUp` undoes a side effect already started in the click.
   */
  const run = useCallback(
    async (
      row: CalendarRow,
      success: CalendarRowNote,
      act: () => Promise<void>,
      cleanUp?: () => void
    ) => {
      setNote(row, null)
      try {
        await act()
        setNote(row, success)
      } catch (error) {
        cleanUp?.()
        setNote(row, noteForFailure(error))
      }
    },
    [setNote]
  )

  /** Copies the link and opens Google's Add from URL page beside it. */
  const addToGoogle = useCallback(() => {
    const tab = openTabNow(GOOGLE_CALENDAR_ADD_BY_URL_PAGE)
    return run(
      'google',
      'google_pending',
      () => copyWhenResolved(resolveAddress()),
      tab.close
    )
  }, [resolveAddress, run])

  const addToApple = useCallback(
    () =>
      run('apple', 'apple_opened', async () => {
        openInCalendarApp(webcalUrl(await resolveAddress()))
      }),
    [resolveAddress, run]
  )

  const addToOutlook = useCallback(
    (row: 'outlook_com' | 'outlook_work') => {
      const tab = openPendingTab()
      const build =
        row === 'outlook_com' ? outlookComSubscribeUrl : outlookWorkSubscribeUrl
      return run(
        row,
        'outlook_opened',
        async () => {
          tab.navigate(build(await resolveAddress()))
        },
        tab.close
      )
    },
    [resolveAddress, run]
  )

  const copyLink = useCallback(
    () => run('copy', 'copied', () => copyWhenResolved(resolveAddress())),
    [resolveAddress, run]
  )

  return {
    notes,
    /** The one-step add for each app with a row of its own. */
    addTo: { google: addToGoogle, apple: addToApple } satisfies Record<
      CalendarAddTarget,
      () => void
    >,
    addToOutlook,
    copyLink,
  }
}

export type CalendarActions = ReturnType<typeof useSavedShowsCalendarActions>
