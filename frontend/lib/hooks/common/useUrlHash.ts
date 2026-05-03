'use client'

import { useSyncExternalStore } from 'react'

/**
 * Anchor for graph sections on entity detail pages (RelatedArtists,
 * CollectionGraph, SceneGraph, VenueBillNetwork). Cmd+K constructs
 * `${path}${GRAPH_HASH}` deep-links; consumers compare `useUrlHash() ===
 * GRAPH_HASH` to decide whether to auto-open. Centralized so a rename
 * doesn't have to update five callsites.
 */
export const GRAPH_HASH = '#graph'

const subscribe = (callback: () => void): (() => void) => {
  if (typeof window === 'undefined') return () => {}
  window.addEventListener('hashchange', callback)
  return () => window.removeEventListener('hashchange', callback)
}

const getSnapshot = (): string =>
  typeof window === 'undefined' ? '' : window.location.hash

// Returns "" so SSR + hydration agree; client re-renders with the real hash.
const getServerSnapshot = (): string => ''

/**
 * Subscribe to `window.location.hash`. Returns the hash including the
 * leading `#`, or `""` on the server. Re-renders on `hashchange`.
 *
 * Prefer this over `useEffect`-driven reads (visible flash of wrong UI on
 * mount) and over `useState(() => window.location.hash)` (hydration
 * mismatch in `'use client'` components — server returns one value, client
 * another). `useSyncExternalStore` is the React-recommended pattern for
 * browser APIs with subscribable mutable values.
 */
export function useUrlHash(): string {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot)
}
