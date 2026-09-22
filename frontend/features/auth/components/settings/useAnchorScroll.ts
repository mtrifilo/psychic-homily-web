'use client'

import { useCallback, useRef } from 'react'
import { useUrlHash } from '@/lib/hooks/common/useUrlHash'

/** Keeps a linked-to card clear of the sticky TopBar once it is scrolled to. */
export const SETTINGS_ANCHOR_SCROLL_MT =
  'scroll-mt-[calc(var(--topbar-height)+1rem)]'

const prefersReducedMotion = () =>
  typeof window !== 'undefined' &&
  window.matchMedia?.('(prefers-reduced-motion: reduce)').matches === true

/**
 * A ref that scrolls its card into view when a link carrying that card's
 * fragment lands here.
 *
 * A callback REF rather than an effect keyed on the hash. Settings cards live
 * inside a Radix TabsContent that mounts only after the client navigation
 * commits, and the cards themselves can mount later still, so on a cold load
 * (bookmark, refresh, opened from an email, or the home page's "All settings
 * →") nothing with that id exists when the browser, or an effect that only
 * re-runs on `hashchange`, resolves the fragment. Firing when the NODE arrives
 * is the one signal that is always available at the right moment. `once` keeps
 * a later re-render from yanking the page back after the user has scrolled
 * away.
 *
 * Moving the VIEWPORT is only half of following a link. Without focus, a
 * keyboard or screen-reader user arriving from a deep link gets the page
 * scrolled to the card while focus stays at the document start, so their next
 * Tab lands in the top nav rather than on the control they were sent to. The
 * card is not otherwise focusable, hence the -1 tabindex the caller applies.
 */
export function useAnchorScroll(anchorId: string) {
  const urlHash = useUrlHash()
  const scrolled = useRef(false)

  return useCallback(
    (node: HTMLElement | null) => {
      if (!node || scrolled.current) return
      if (urlHash.replace(/^#/, '') !== anchorId) return
      scrolled.current = true
      node.scrollIntoView({
        behavior: prefersReducedMotion() ? 'auto' : 'smooth',
        block: 'start',
      })
      node.focus({ preventScroll: true })
    },
    [urlHash, anchorId]
  )
}
