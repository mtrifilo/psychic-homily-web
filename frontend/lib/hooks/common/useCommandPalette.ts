'use client'

import { useState, useEffect, useCallback } from 'react'

const RECENT_SEARCHES_KEY = 'command-palette-recent'
const MAX_RECENT_SEARCHES = 5

/**
 * The stored recents, or none. Storage is outside this app's control (an
 * extension or a hand edit can leave any JSON there), so anything that is not
 * a list of strings reads as no recents rather than reaching render.
 */
function readStoredRecents(): string[] {
  const stored = localStorage.getItem(RECENT_SEARCHES_KEY)
  const parsed: unknown = stored ? JSON.parse(stored) : []
  return Array.isArray(parsed)
    ? parsed.filter((s): s is string => typeof s === 'string')
    : []
}

/**
 * Hook for managing the command palette open/close state and keyboard shortcut.
 * Handles Cmd+K (Mac) / Ctrl+K (Windows/Linux) to toggle the palette.
 * Also manages recent searches persisted in localStorage.
 *
 * Should only be used once in the component tree (in CommandPalette).
 * Other components can trigger the palette by dispatching a custom event:
 *   window.dispatchEvent(new Event('open-command-palette'))
 */
export function useCommandPalette() {
  const [open, setOpen] = useState(false)

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'k' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        setOpen(prev => !prev)
      }
    }

    const handleCustomOpen = () => {
      setOpen(true)
    }

    document.addEventListener('keydown', handleKeyDown)
    window.addEventListener('open-command-palette', handleCustomOpen)
    return () => {
      document.removeEventListener('keydown', handleKeyDown)
      window.removeEventListener('open-command-palette', handleCustomOpen)
    }
  }, [])

  const getRecentSearches = useCallback((): string[] => {
    if (typeof window === 'undefined') return []
    try {
      return readStoredRecents()
    } catch {
      return []
    }
  }, [])

  /**
   * Moves `search` to the front of the recents. `isSameEntry` names stored
   * entries that are the same destination under another string (a route's
   * former label), which are dropped so they do not hold a slot.
   */
  const addRecentSearch = useCallback(
    (search: string, isSameEntry?: (stored: string) => boolean) => {
      if (typeof window === 'undefined') return
      const trimmed = search.trim()
      if (!trimmed) return

      try {
        const filtered = readStoredRecents().filter(
          s => s !== trimmed && !isSameEntry?.(s)
        )
        const updated = [trimmed, ...filtered].slice(0, MAX_RECENT_SEARCHES)
        localStorage.setItem(RECENT_SEARCHES_KEY, JSON.stringify(updated))
      } catch {
        // Ignore storage errors
      }
    },
    []
  )

  const clearRecentSearches = useCallback(() => {
    if (typeof window === 'undefined') return
    try {
      localStorage.removeItem(RECENT_SEARCHES_KEY)
    } catch {
      // Ignore storage errors
    }
  }, [])

  return {
    open,
    setOpen,
    getRecentSearches,
    addRecentSearch,
    clearRecentSearches,
  }
}

/**
 * Utility to open the command palette from any component.
 * Dispatches a custom DOM event that useCommandPalette listens for.
 */
export function openCommandPalette() {
  window.dispatchEvent(new Event('open-command-palette'))
}
