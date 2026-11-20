'use client'

import { useEffect, useState, type ComponentType } from 'react'
import type { SavedShowsCalendarPanelProps } from './SavedShowsCalendarPanel'

type Panel = ComponentType<SavedShowsCalendarPanelProps>

// The panel once its code has loaded, shared by every opener on the page so a
// later open renders at once.
let loadedPanel: Panel | null = null
// The load in flight, or null: a failed load is forgotten, so the next open
// tries again rather than inheriting the failure for the life of the page.
let pendingLoad: Promise<Panel> | null = null

function loadPanel(): Promise<Panel> {
  pendingLoad ??= import('./SavedShowsCalendarPanel').then(
    m => {
      loadedPanel = m.SavedShowsCalendarPanel
      return loadedPanel
    },
    error => {
      pendingLoad = null
      throw error
    }
  )
  return pendingLoad
}

/**
 * Starts loading the panel's code, so it is ready by the time a control asks
 * for it. Never rejects: a failed load is the lazy panel's to handle.
 */
export function preloadSavedShowsCalendarPanel(): void {
  void loadPanel().catch(() => {})
}

/**
 * The saved-shows calendar panel, loaded on demand: every surface that opens
 * it (a Save control on every list row, a section header, the home module)
 * renders it only once it has been opened, and none pays for its code before
 * that. Takes the panel's own props.
 *
 * Code that fails to load renders nothing rather than throwing into the
 * page's error boundary and replacing the page the person is on. The open is
 * then refused through `onOpenChange(false)`, so the control that asked is not
 * left announcing an open panel that is not there, and focus goes back to the
 * anchor if it was lost. The next open loads again.
 */
export function LazySavedShowsCalendarPanel(
  props: SavedShowsCalendarPanelProps
) {
  const [Panel, setPanel] = useState<Panel | null>(() => loadedPanel)
  const { open, onOpenChange, anchorRef } = props

  // Loads in an effect, not during render: the import is a side effect, and
  // its failure has to reach the opener through onOpenChange.
  useEffect(() => {
    if (Panel || !open) return
    let current = true
    loadPanel().then(
      loaded => {
        if (current) setPanel(() => loaded)
      },
      () => {
        if (!current) return
        onOpenChange(false)
        // Only when focus is lost: the person may have moved on while it
        // loaded.
        if (document.activeElement && document.activeElement !== document.body) {
          return
        }
        anchorRef.current
          ?.querySelector<HTMLElement>('button, a')
          ?.focus({ preventScroll: true })
      }
    )
    return () => {
      current = false
    }
  }, [Panel, open, onOpenChange, anchorRef])

  return Panel ? <Panel {...props} /> : null
}
