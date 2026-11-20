'use client'

import { useEffect, type RefObject } from 'react'
import dynamic from 'next/dynamic'

const loadCalendarPanel = () => import('./SavedShowsCalendarPanel')

/**
 * Starts loading the panel's code, so it is ready by the time a control asks
 * for it. Never rejects: a failed load is the lazy panel's to handle.
 */
export function preloadSavedShowsCalendarPanel(): void {
  void loadCalendarPanel().catch(() => {})
}

/**
 * The saved-shows calendar panel, loaded on demand: every surface that opens
 * it (a Save control on every list row, a section header, the home module)
 * renders it only once it has been opened, and none pays for its code before
 * that. Takes the panel's own props.
 *
 * A chunk that fails to load renders nothing, returning focus to the anchor,
 * rather than throwing into the page's error boundary and replacing the page
 * the person is on.
 */
export const LazySavedShowsCalendarPanel = dynamic(
  () =>
    loadCalendarPanel()
      .then(m => m.SavedShowsCalendarPanel)
      .catch(() => NoCalendarPanel),
  { ssr: false }
)

/**
 * Stands in for a panel whose code failed to load. The control that opened it
 * may already be gone (the first-save hint closes as it opens the panel), so
 * focus goes back to the anchor's first control rather than <body>.
 */
function NoCalendarPanel({
  anchorRef,
}: {
  anchorRef: RefObject<HTMLElement | null>
}) {
  useEffect(() => {
    // Only when focus is lost: the person may have moved on while it loaded.
    if (document.activeElement && document.activeElement !== document.body) return
    anchorRef.current
      ?.querySelector<HTMLElement>('button, a')
      ?.focus({ preventScroll: true })
  }, [anchorRef])
  return null
}
