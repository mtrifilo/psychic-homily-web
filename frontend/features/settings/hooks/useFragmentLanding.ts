'use client'

import { useEffect, useRef, type RefObject } from 'react'
import { jumpToAnchor } from '../anchorTargets'

/**
 * Lands the viewer on the element inside `rootRef` that the address bar's
 * fragment names when the page mounts, and reports the fragment to `onLand`.
 *
 * When the server rendered the content, the browser has already scrolled to
 * the fragment before hydration; this landing re-applies that scroll, moves
 * focus to the target and marks it. When the page first showed the auth
 * gate's loading state, the browser found nothing to scroll to, and this
 * landing is the only one. Resolving inside the root keeps it off a hidden,
 * still-mounted route that carries the same id.
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
