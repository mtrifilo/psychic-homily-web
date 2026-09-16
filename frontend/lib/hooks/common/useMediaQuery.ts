'use client'

import { useCallback, useSyncExternalStore } from 'react'

/**
 * Whether a CSS media query matches, re-rendering when it stops or starts.
 *
 * SSR-safe by contract: the server snapshot is always `false`, so the server
 * HTML is the "no match" rendering and anything gated on a query appears at
 * hydration. A component that must not SHIFT when that happens has to reserve
 * its space some other way.
 *
 * `matchMedia` is read through `useSyncExternalStore` rather than an effect:
 * the subscription and its teardown come with the store, and there is no
 * set-state-in-an-effect double render.
 */
export function useMediaQuery(query: string): boolean {
  const subscribe = useCallback(
    (onChange: () => void) => {
      const mq = matchMediaOrNull(query)
      if (!mq) return () => {}
      // Older Safari ships only addListener/removeListener; prefer the modern
      // API where it exists so evergreen browsers log no deprecation.
      if (typeof mq.addEventListener === 'function') {
        mq.addEventListener('change', onChange)
        return () => mq.removeEventListener('change', onChange)
      }
      mq.addListener(onChange)
      return () => mq.removeListener(onChange)
    },
    [query]
  )

  const getSnapshot = useCallback(
    () => matchMediaOrNull(query)?.matches ?? false,
    [query]
  )

  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot)
}

/**
 * The query's `MediaQueryList`, or null where there is no `matchMedia` to ask:
 * the server, and jsdom without the shim.
 */
function matchMediaOrNull(query: string): MediaQueryList | null {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return null
  }
  return window.matchMedia(query)
}

function getServerSnapshot(): boolean {
  return false
}
