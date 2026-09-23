'use client'

import { useEffect, useRef, type RefObject } from 'react'
import { jumpToAnchor } from '../anchorTargets'

/**
 * Lands the viewer on the element inside `rootRef` that the address bar's
 * fragment names when the page mounts, and reports the fragment to `onLand`.
 *
 * The page's content mounts after the auth gate settles, which is after the
 * browser has already tried the fragment and found nothing, so this one
 * landing has to be the page's own. Resolving inside the root keeps it off a
 * hidden, still-mounted route that carries the same id.
 *
 * Exactly once per mount, tracked by a ref, so a route kept alive while the
 * viewer is elsewhere does not land again when it is shown. Fragment changes
 * after mount (a skip link, an edited address, Back or Forward between
 * fragments) are the browser's own fragment navigation, and this hook stays
 * out of them. It writes nothing to history.
 */
export function useFragmentLanding(
  rootRef: RefObject<HTMLElement | null>,
  onLand: (fragment: string) => void
) {
  const landed = useRef(false)

  useEffect(() => {
    if (landed.current) return
    landed.current = true
    const fragment = window.location.hash.replace(/^#/, '')
    if (fragment === '') return
    jumpToAnchor(rootRef.current, fragment)
    onLand(fragment)
  }, [rootRef, onLand])
}
