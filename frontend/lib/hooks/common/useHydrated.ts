'use client'

import { useSyncExternalStore } from 'react'

// Whether React has hydrated cannot change back, and there is no event to
// listen for — the snapshot swap IS the notification. A no-op unsubscribe
// keeps the store contract.
function subscribe(): () => void {
  return () => {}
}

function getBrowserSnapshot(): boolean {
  return true
}

function getServerSnapshot(): boolean {
  return false
}

/**
 * `false` on the server and through the hydration render, `true` from the
 * commit after it.
 *
 * The gate for markup derived from something only the browser knows —
 * `sessionStorage`, `navigator`, a client cache the server never saw. Read such
 * a value straight into a render and the server HTML and the hydration render
 * disagree, which React reports as a hydration error and repairs by throwing
 * the server's markup away. Gate it here and both renders agree by
 * construction, whatever the browser turns out to know.
 *
 * The two-phase shape is the point, not a limitation: the pre-hydration render
 * has to be the answer that is right for a reader who never runs JavaScript at
 * all, and the refined answer arrives a commit later.
 *
 * `useSyncExternalStore`, not an effect: React uses `getServerSnapshot` for
 * BOTH the server render and the hydration render, which is exactly the
 * guarantee needed here, and it is a documented contract rather than a race
 * between an effect and the first paint. Both snapshots must be module-level
 * and return primitives, or the store re-renders forever.
 */
export function useHydrated(): boolean {
  return useSyncExternalStore(subscribe, getBrowserSnapshot, getServerSnapshot)
}
