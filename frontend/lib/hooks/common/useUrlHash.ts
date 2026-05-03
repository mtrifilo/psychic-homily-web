'use client'

/**
 * useUrlHash (PSY-548)
 *
 * Subscribes to `window.location.hash` and returns it as a string. Re-renders
 * the consumer when the hash changes (e.g. an in-page anchor link is clicked).
 * Returns the empty string on the server.
 *
 * Why useSyncExternalStore instead of useEffect or lazy useState:
 *
 * - **vs `useEffect` + `setState`**: the effect approach renders once with
 *   the initial state, fires the effect, sets state, then re-renders — a
 *   visible flash of the wrong UI. `useSyncExternalStore` derives the value
 *   during render, so the first paint is already correct.
 *
 * - **vs lazy `useState(() => window.location.hash === ...)`**: that pattern
 *   hydration-mismatches under Next.js App Router because `'use client'`
 *   components still pre-render on the server, where `typeof window` is
 *   `'undefined'`. Server emits one value; client emits another; React
 *   warns + re-renders. `useSyncExternalStore`'s third argument
 *   (`getServerSnapshot`) is the official escape hatch — server returns
 *   `""`, client reads the real hash, no warning.
 *
 * Reference: React docs explicitly call out browser APIs with subscribable
 * mutable values as the canonical use case
 * (https://react.dev/reference/react/useSyncExternalStore).
 */

import { useSyncExternalStore } from 'react'

const subscribe = (callback: () => void): (() => void) => {
  if (typeof window === 'undefined') return () => {}
  window.addEventListener('hashchange', callback)
  return () => window.removeEventListener('hashchange', callback)
}

const getSnapshot = (): string =>
  typeof window === 'undefined' ? '' : window.location.hash

// Server-side render: hash is unknowable from the server, so report empty.
// On hydration, useSyncExternalStore re-renders with the real client value
// (no hydration mismatch warning).
const getServerSnapshot = (): string => ''

/**
 * Returns the current `window.location.hash` (including the leading `#`),
 * subscribing the calling component to `hashchange` events. Returns `""` on
 * the server.
 *
 * Common use: derive a boolean — `useUrlHash() === '#graph'` — to control
 * UI based on the URL hash without an effect or hydration mismatch.
 */
export function useUrlHash(): string {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot)
}
