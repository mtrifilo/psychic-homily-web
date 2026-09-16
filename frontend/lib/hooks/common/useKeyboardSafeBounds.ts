'use client'

import { useSyncExternalStore } from 'react'

export interface KeyboardSafeBounds {
  /**
   * Distance in px from the layout viewport's bottom edge to the top of the
   * software keyboard. `0` when no keyboard is up.
   */
  bottom: number
  /** Height in px the keyboard leaves visible. */
  maxHeight: number
}

/**
 * The last value handed out, so repeated reads of an unchanged viewport return
 * the same object. `useSyncExternalStore` compares snapshots by identity and
 * re-renders forever on a fresh object every read.
 */
let cached: KeyboardSafeBounds | null = null

function getBounds(): KeyboardSafeBounds | null {
  const viewport = typeof window === 'undefined' ? null : window.visualViewport
  if (!viewport) return null
  const bottom = Math.max(
    0,
    Math.round(window.innerHeight - viewport.height - viewport.offsetTop)
  )
  const maxHeight = Math.round(viewport.height)
  if (!cached || cached.bottom !== bottom || cached.maxHeight !== maxHeight) {
    cached = { bottom, maxHeight }
  }
  return cached
}

function getNoBounds(): null {
  return null
}

function subscribeBounds(onChange: () => void): () => void {
  const viewport = typeof window === 'undefined' ? null : window.visualViewport
  if (!viewport) return () => {}
  // `scroll` carries `offsetTop`, which moves under a keyboard that is already
  // up when the page scrolls.
  viewport.addEventListener('resize', onChange)
  viewport.addEventListener('scroll', onChange)
  return () => {
    viewport.removeEventListener('resize', onChange)
    viewport.removeEventListener('scroll', onChange)
  }
}

function subscribeNothing(): () => void {
  return () => {}
}

/**
 * Geometry a bottom-anchored fixed surface needs to stay above a software
 * keyboard.
 *
 * A fixed element is laid out against the LAYOUT viewport, which
 * `interactive-widget=resizes-visual` (the browser default, and the only
 * behaviour iOS Safari implements) leaves at full height when the keyboard
 * rises - so `bottom: 0` puts the surface behind the keyboard. The visual
 * viewport is the part still on screen, and the difference between the two is
 * the keyboard. Where `interactive-widget=resizes-content` is honoured the
 * layout viewport shrinks with the keyboard, the two measurements converge, and
 * `bottom` reads 0.
 *
 * `enabled` is the caller's "this surface is open" gate. It decides which
 * subscription is installed, so a closed surface neither listens to the
 * viewport nor re-renders as the page scrolls, and reads `null`, meaning the
 * caller keeps its own static geometry.
 */
export function useKeyboardSafeBounds(
  enabled: boolean
): KeyboardSafeBounds | null {
  return useSyncExternalStore(
    enabled ? subscribeBounds : subscribeNothing,
    enabled ? getBounds : getNoBounds,
    getNoBounds
  )
}
