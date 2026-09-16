'use client'

import { useSyncExternalStore } from 'react'

/**
 * Viewports on which focusing a text field raises a software keyboard: at or
 * below Tailwind's `md` breakpoint, plus every coarse pointer. Both halves
 * carry weight - a tablet in landscape is wider than `md` and still raises a
 * keyboard, and a narrow desktop window raises none but is the same geometry.
 *
 * The query is deliberately wider than "has a keyboard": a touch laptop and a
 * narrow mouse-driven window both match and get the touch treatment.
 */
export const SOFT_KEYBOARD_VIEWPORT_QUERY =
  '(max-width: 767px), (pointer: coarse)'

function getSnapshot(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false
  }
  return window.matchMedia(SOFT_KEYBOARD_VIEWPORT_QUERY).matches
}

function getServerSnapshot(): boolean {
  return false
}

function subscribe(onChange: () => void): () => void {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return () => {}
  }
  const query = window.matchMedia(SOFT_KEYBOARD_VIEWPORT_QUERY)
  query.addEventListener('change', onChange)
  return () => query.removeEventListener('change', onChange)
}

/**
 * True on viewports where a filter overlay opens as a bottom sheet rather than
 * as a popover anchored to its trigger.
 *
 * Subscribed rather than read once at open, because the value also decides the
 * trigger's ARIA contract (`aria-haspopup="dialog"` versus `role="combobox"`),
 * which a reader has to be able to trust before the control is activated. The
 * price is one listener and one extra render at hydration per page that renders
 * the filter. `false` on the server, and the trigger renders the same box
 * either way, so nothing around it moves when the client value lands.
 */
export function useSoftKeyboardViewport(): boolean {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot)
}
