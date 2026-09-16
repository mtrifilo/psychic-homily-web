'use client'

import { useEffect, type RefObject } from 'react'

/** Distance in px from the layout viewport's bottom edge to the keyboard's top. */
export const KEYBOARD_INSET_VAR = '--keyboard-inset-bottom'
/** Height in px the keyboard leaves visible. */
export const KEYBOARD_VISIBLE_HEIGHT_VAR = '--keyboard-visible-height'

/**
 * Writes the geometry a bottom-anchored fixed surface needs to stay above a
 * software keyboard onto `target` as {@link KEYBOARD_INSET_VAR} and
 * {@link KEYBOARD_VISIBLE_HEIGHT_VAR}. The surface reads them through `var()`
 * with its own fallbacks, so neither variable existing is a supported state.
 *
 * A fixed element is laid out against the LAYOUT viewport, which
 * `interactive-widget=resizes-visual` (the browser default, and the only
 * behaviour iOS Safari implements) leaves at full height when the keyboard
 * rises, so `bottom: 0` puts the surface behind the keyboard. The visual
 * viewport is the part still on screen, and the difference between the two is
 * the keyboard. Where `interactive-widget=resizes-content` is honoured the two
 * measurements converge and the inset reads 0.
 *
 * Geometry goes to the DOM rather than to state because `scroll` fires per
 * frame while a keyboard is up: a state write would re-render the surface and
 * every row inside it at frame rate on exactly the low-power devices this
 * exists for. One `requestAnimationFrame` per burst keeps the two layout reads
 * to once a frame as well.
 *
 * `enabled` is the caller's "this surface is open" gate: a closed surface
 * installs no listeners.
 */
export function usePinAboveSoftKeyboard(
  enabled: boolean,
  target: RefObject<HTMLElement | null>
): void {
  useEffect(() => {
    if (!enabled || typeof window === 'undefined') return
    const viewport = window.visualViewport
    if (!viewport) return

    let frame = 0

    const write = () => {
      frame = 0
      const node = target.current
      if (!node) return
      const inset = Math.max(
        0,
        Math.round(window.innerHeight - viewport.height - viewport.offsetTop)
      )
      node.style.setProperty(KEYBOARD_INSET_VAR, `${inset}px`)
      node.style.setProperty(
        KEYBOARD_VISIBLE_HEIGHT_VAR,
        `${Math.round(viewport.height)}px`
      )
    }

    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(write)
    }

    write()
    // `scroll` carries `offsetTop`, which moves under a keyboard that is
    // already up when the page scrolls.
    viewport.addEventListener('resize', schedule)
    viewport.addEventListener('scroll', schedule)
    return () => {
      if (frame) cancelAnimationFrame(frame)
      viewport.removeEventListener('resize', schedule)
      viewport.removeEventListener('scroll', schedule)
    }
  }, [enabled, target])
}
