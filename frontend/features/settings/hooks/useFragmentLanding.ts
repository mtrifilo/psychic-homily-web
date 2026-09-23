'use client'

import { useEffect, type RefObject } from 'react'
import { jumpToAnchor } from '../anchorTargets'

/**
 * Lands the viewer on the element inside `rootRef` that the address bar's
 * fragment names: once when the page mounts, and again whenever the fragment
 * changes from outside the page (an edited address, history traversal).
 *
 * The page's content mounts after the auth gate settles, which is after the
 * browser has already tried the fragment and found nothing, so the landing has
 * to be the page's own. Resolving inside the root also keeps it off a hidden,
 * still-mounted route that carries the same id.
 */
export function useFragmentLanding(rootRef: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const land = () => {
      const fragment = window.location.hash.replace(/^#/, '')
      if (fragment !== '') jumpToAnchor(rootRef.current, fragment)
    }
    land()
    window.addEventListener('hashchange', land)
    return () => window.removeEventListener('hashchange', land)
  }, [rootRef])
}
