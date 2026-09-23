'use client'

import { useEffect, type RefObject } from 'react'
import { jumpToAnchor } from '../anchorTargets'

/** The history-entry state key recording the fragment an entry landed on. */
const LANDED_FRAGMENT_KEY = 'landedFragment'

function currentEntryState(): Record<string, unknown> {
  const state: unknown = window.history.state
  return typeof state === 'object' && state !== null
    ? (state as Record<string, unknown>)
    : {}
}

function entryHasLanded(fragment: string): boolean {
  return currentEntryState()[LANDED_FRAGMENT_KEY] === fragment
}

/**
 * Records the landing on the current history entry. No URL argument, so the
 * App Router's patched `replaceState` keeps its own state on the entry and
 * does not navigate.
 */
function markEntryLanded(fragment: string) {
  window.history.replaceState(
    { ...currentEntryState(), [LANDED_FRAGMENT_KEY]: fragment },
    ''
  )
}

/**
 * Lands the viewer on the element inside `rootRef` that the address bar's
 * fragment names, and reports the fragment to `onLand`: once per history
 * entry, and again when the fragment is edited while the page is open.
 *
 * The page's content mounts after the auth gate settles, which is after the
 * browser has already tried the fragment and found nothing, so the landing has
 * to be the page's own. Resolving inside the root also keeps it off a hidden,
 * still-mounted route that carries the same id.
 *
 * Once per history entry, recorded on the entry itself: Back or Forward to an
 * entry mounts or shows the page again with the fragment the viewer arrived
 * with, not where they were, and landing again would pull them off the place
 * the browser restored.
 */
export function useFragmentLanding(
  rootRef: RefObject<HTMLElement | null>,
  onLand: (fragment: string) => void
) {
  useEffect(() => {
    const land = () => {
      const fragment = window.location.hash.replace(/^#/, '')
      if (fragment === '' || entryHasLanded(fragment)) return
      jumpToAnchor(rootRef.current, fragment)
      onLand(fragment)
      markEntryLanded(fragment)
    }
    const onHashChange = (event: HashChangeEvent) => {
      // Only a fragment edited on this page; not a traversal from elsewhere.
      if (
        event.oldURL &&
        new URL(event.oldURL).pathname !== window.location.pathname
      ) {
        return
      }
      land()
    }

    land()
    window.addEventListener('hashchange', onHashChange)
    return () => window.removeEventListener('hashchange', onHashChange)
  }, [rootRef, onLand])
}
