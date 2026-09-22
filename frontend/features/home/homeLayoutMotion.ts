'use client'

import { useCallback, useLayoutEffect, useRef } from 'react'

/** One step of reorder, matched between the popover list and the page behind
 *  it so the two read as the same gesture. */
export const HOME_REORDER_DURATION_MS = 180

/**
 * Whether motion is allowed at all.
 *
 * Read at the moment of the gesture rather than subscribed to: this decides a
 * single animation that is about to start, and a viewer who changes the OS
 * setting mid-click is not a case worth a listener.
 */
export function prefersReducedMotion(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
}

/** Web Animations is absent in jsdom and in older Safari. Motion is a garnish
 *  here, so its absence must degrade to an instant, correct layout. */
function canAnimate(node: Element): boolean {
  return typeof (node as HTMLElement).animate === 'function'
}

/**
 * FLIP for a keyed list whose reorders originate in an event handler.
 *
 * `orderKey` is a signature of the current order. It is what makes the capture
 * survive the renders BETWEEN the click and the reorder: the write is
 * optimistic but not synchronous, and the click itself re-renders (the live
 * region's text changes), so an effect that consumed the capture on the next
 * render would measure a list that had not moved yet and then have nothing
 * left when it did.
 *
 * Positions are captured on demand rather than every render because these
 * lists sit inside sections whose own content resizes constantly; measuring
 * every render would animate a reorder that never happened every time a show
 * list settled.
 */
export function useFlipReorder(orderKey: string) {
  const nodes = useRef(new Map<string, HTMLElement>())
  const pending = useRef<{
    tops: Map<string, number>
    orderKey: string
  } | null>(null)

  // A fresh closure per render is deliberate: React detaches the previous ref
  // and reattaches this one during commit, which runs before the layout effect
  // below, so the map is complete by the time it reads.
  const register = useCallback(
    (key: string) => (node: HTMLElement | null) => {
      if (node) nodes.current.set(key, node)
      else nodes.current.delete(key)
    },
    []
  )

  const capture = useCallback(() => {
    if (prefersReducedMotion()) {
      pending.current = null
      return
    }
    const tops = new Map<string, number>()
    for (const [key, node] of nodes.current) {
      tops.set(key, node.getBoundingClientRect().top)
    }
    pending.current = { tops, orderKey }
  }, [orderKey])

  useLayoutEffect(() => {
    const captured = pending.current
    // Nothing captured, or the order has not moved yet: keep waiting. A capture
    // that is never followed by a move is dropped by the next capture.
    if (!captured || captured.orderKey === orderKey) return
    pending.current = null
    for (const [key, node] of nodes.current) {
      const previousTop = captured.tops.get(key)
      if (previousTop === undefined || !canAnimate(node)) continue
      const delta = previousTop - node.getBoundingClientRect().top
      if (Math.abs(delta) < 1) continue
      node.animate(
        [{ transform: `translateY(${delta}px)` }, { transform: 'none' }],
        { duration: HOME_REORDER_DURATION_MS, easing: 'ease-out' }
      )
    }
  })

  return { register, capture }
}

/**
 * Animate a section slot's height between zero and its natural height.
 *
 * Returns the running animation so the caller can cancel it, or `null` when no
 * animation started and the caller should settle immediately. A collapse is
 * `fill: 'forwards'`, which HOLDS the slot at zero after it finishes; the
 * caller must cancel it if the section turns out to be staying (a failed write
 * that rolled back), or the slot stays invisible at full height.
 */
export function animateSlotHeight(
  node: HTMLElement,
  direction: 'collapse' | 'expand',
  onFinish: () => void
): Animation | null {
  if (prefersReducedMotion() || !canAnimate(node)) return null
  const naturalHeight = node.getBoundingClientRect().height
  if (naturalHeight < 1) return null

  const frames =
    direction === 'collapse'
      ? [
          { height: `${naturalHeight}px`, opacity: 1 },
          { height: '0px', opacity: 0 },
        ]
      : [
          { height: '0px', opacity: 0 },
          { height: `${naturalHeight}px`, opacity: 1 },
        ]

  const animation = node.animate(frames, {
    duration: HOME_REORDER_DURATION_MS,
    easing: 'ease-out',
    // A collapse holds at zero until the caller unmounts the slot; an expand
    // must release so the section can grow with its own content afterwards.
    fill: direction === 'collapse' ? 'forwards' : 'none',
  })
  animation.onfinish = onFinish
  return animation
}
