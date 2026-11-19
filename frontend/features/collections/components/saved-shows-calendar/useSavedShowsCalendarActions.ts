'use client'

import { useCallback, useState } from 'react'
import { GOOGLE_CALENDAR_ADD_BY_URL_PAGE } from '@/lib/utils/calendarFeedUrls'
import { copyText, openTab } from './calendarPanelBrowser'
import type { CalendarRow } from './savedShowsCalendarModel'

/** What a row reports under itself after it was chosen. */
export type CalendarRowNote =
  | 'google_pending'
  | 'apple_opened'
  | 'outlook_opened'
  | 'copied'
  | 'copy_failed'

type CalendarRowNotes = Partial<Record<CalendarRow, CalendarRowNote>>

/**
 * The rows' click handlers and the notes they leave. The address is ready
 * before any row can be chosen, so every side effect starts in the click
 * itself; the Apple and Outlook rows are plain links and only record their
 * note here.
 */
export function useSavedShowsCalendarActions(feedUrl: string) {
  const [notes, setNotes] = useState<CalendarRowNotes>({})

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
   * Google's documented two-step for a private link: copy it, and open
   * Google's Add from URL page to paste it into. The copy starts before the
   * tab opens, while this document still has focus, which a clipboard write
   * needs; a refused copy closes the tab again, since there is nothing to
   * paste. When the browser blocked the tab, the note says only that the
   * link was copied, never that Google is open.
   */
  const addToGoogle = useCallback(() => {
    setNote('google', null)
    const copying = copyText(feedUrl)
    const tab = openTab(GOOGLE_CALENDAR_ADD_BY_URL_PAGE)
    copying.then(
      () => setNote('google', tab ? 'google_pending' : 'copied'),
      () => {
        tab?.close()
        setNote('google', 'copy_failed')
      }
    )
  }, [feedUrl, setNote])

  const copyLink = useCallback(() => {
    setNote('copy', null)
    copyText(feedUrl).then(
      () => setNote('copy', 'copied'),
      () => setNote('copy', 'copy_failed')
    )
  }, [feedUrl, setNote])

  const noteOpened = useCallback(
    (row: 'apple' | 'outlook_com' | 'outlook_work') =>
      setNote(row, row === 'apple' ? 'apple_opened' : 'outlook_opened'),
    [setNote]
  )

  return { notes, addToGoogle, copyLink, noteOpened }
}

export type CalendarActions = ReturnType<typeof useSavedShowsCalendarActions>
