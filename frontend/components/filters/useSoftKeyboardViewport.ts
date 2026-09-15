'use client'

import { useSyncExternalStore } from 'react'

/**
 * Viewports on which focusing a text field raises a software keyboard: at or
 * below Tailwind's `md` breakpoint, plus every coarse pointer. Both halves
 * carry weight - a tablet in landscape is wider than `md` and still raises a
 * keyboard, and a narrow desktop window raises none but is the same geometry.
 */
export const SOFT_KEYBOARD_VIEWPORT_QUERY =
  '(max-width: 767px), (pointer: coarse)'

function subscribe(onChange: () => void): () => void {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return () => {}
  }
  const mq = window.matchMedia(SOFT_KEYBOARD_VIEWPORT_QUERY)
  // Older Safari versions only ship addListener/removeListener; prefer the
  // modern API when available so we don't trigger deprecation warnings in
  // evergreen browsers.
  if (typeof mq.addEventListener === 'function') {
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }
  mq.addListener(onChange)
  return () => mq.removeListener(onChange)
}

function getSnapshot(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false
  }
  return window.matchMedia(SOFT_KEYBOARD_VIEWPORT_QUERY).matches
}

function getServerSnapshot(): boolean {
  return false
}

/**
 * True when a software keyboard can shrink the visual viewport under an open
 * popover, which is the condition under which a combobox popover has to be
 * pinned below its trigger and bounded to the space that is left.
 *
 * The server snapshot is `false`, so server HTML carries the desktop treatment
 * and the value settles during hydration - before any popover can be opened,
 * since the popover content only exists while open.
 */
export function useSoftKeyboardViewport(): boolean {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot)
}
