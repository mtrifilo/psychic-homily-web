'use client'

import { useSyncExternalStore } from 'react'

// The viewer's zone cannot change mid-session without a reload, so there is
// nothing to subscribe to. A no-op unsubscribe keeps the store contract.
function subscribe(): () => void {
  return () => {}
}

function getBrowserSnapshot(): string | undefined {
  return Intl.DateTimeFormat().resolvedOptions().timeZone
}

function getServerSnapshot(): string | undefined {
  return undefined
}

/**
 * The viewer's IANA timezone — `undefined` on the server and through the
 * hydration render, the real zone from the commit after it.
 *
 * `Intl.DateTimeFormat().resolvedOptions().timeZone` looks like a constant and
 * is not: on the server it resolves to the SERVER's zone (UTC in production),
 * on the client to the viewer's. Read directly during render, it therefore
 * makes any value derived from it — a cache key, a request URL, a date label —
 * differ between the server HTML and the hydration pass. Nothing surfaced that
 * while the surfaces reading it rendered a skeleton server-side; a
 * server-rendered list (PSY-1624) reads it for real.
 *
 * The two-phase shape is the point, not a limitation. It gives the server and
 * the client's first render one agreed answer to key a request on, and hands
 * the viewer's actual zone to every render after. The two answers differ only
 * in where "today" starts, and `keepPreviousData` holds the first screen in
 * place while the corrected one arrives.
 *
 * Measured cost, stated plainly because it is not free: on a cold `/shows` the
 * page issues FOUR client requests where it used to issue two. The seeded
 * entries are stale by construction, so the hydration commit revalidates the
 * zone-less `/shows/upcoming` and `/shows/cities` pair, and the commit after
 * it — once this hook reports the real zone — fetches the zone-keyed pair.
 * The first pair's results are discarded. `/venues` and `/scenes` do not pay
 * this; nothing there keys on a timezone. Reducing it means giving the server
 * a zone to key on (an `x-vercel-ip-timezone` read, or a cookie written on
 * first visit), which trades a Data Cache entry per zone for the saved pair —
 * a worthwhile follow-up, not a silent default.
 *
 * `useSyncExternalStore`, not an effect: React uses `getServerSnapshot` for
 * BOTH the server render and the hydration render, which is exactly the
 * guarantee needed, and it is a documented contract rather than a race between
 * an effect and a first paint.
 */
export function useBrowserTimezone(): string | undefined {
  return useSyncExternalStore(
    subscribe,
    getBrowserSnapshot,
    getServerSnapshot,
  )
}
