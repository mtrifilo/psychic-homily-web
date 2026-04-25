'use client'

import { useSyncExternalStore } from 'react'

// SSR-safe: the server snapshot is always `false` so the server HTML
// matches; after hydration the hook reads the live MediaQueryList via
// `useSyncExternalStore`, which also wires up the change listener and
// cleans it up on unmount without the "setState inside an effect"
// double-render that ESLint flags.
//
// Used by ArtistGraph to pause the continuous force simulation for
// `prefers-reduced-motion: reduce` users — tap, zoom, and pan still
// work; only the background motion stops.
function subscribeReducedMotion(onChange: () => void): () => void {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return () => {}
  }
  const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
  // Older Safari versions only ship addListener/removeListener; prefer
  // the modern API when available so we don't trigger deprecation
  // warnings in evergreen browsers.
  if (typeof mq.addEventListener === 'function') {
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }
  mq.addListener(onChange)
  return () => mq.removeListener(onChange)
}

function getReducedMotionSnapshot(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false
  }
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

function getReducedMotionServerSnapshot(): boolean {
  return false
}

/**
 * Reads the user's `prefers-reduced-motion` setting and re-renders when
 * it changes mid-session.
 */
export function useReducedMotion(): boolean {
  return useSyncExternalStore(
    subscribeReducedMotion,
    getReducedMotionSnapshot,
    getReducedMotionServerSnapshot
  )
}
