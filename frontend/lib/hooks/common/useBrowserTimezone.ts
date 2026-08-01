'use client'

import { useSyncExternalStore } from 'react'
import { CANONICAL_FIRST_SCREEN_TIMEZONE } from '@/lib/canonicalTimezone'

// The viewer's zone cannot change mid-session without a reload, so there is
// nothing to subscribe to. A no-op unsubscribe keeps the store contract.
function subscribe(): () => void {
  return () => {}
}

function getBrowserSnapshot(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone
}

function getServerSnapshot(): string {
  return CANONICAL_FIRST_SCREEN_TIMEZONE
}

/**
 * The viewer's IANA timezone: a fixed canonical zone on the server and through
 * the hydration render, the viewer's real zone from the commit after it.
 *
 * `Intl.DateTimeFormat().resolvedOptions().timeZone` looks like a constant and
 * is not. On the server it resolves to the SERVER's zone, in the browser to the
 * viewer's. Read directly during render it therefore makes anything derived
 * from it (a cache key, a request URL, a date label) differ between the server
 * HTML and the hydration pass. That never surfaced while the surfaces reading
 * it rendered a skeleton server-side. A server-rendered list (PSY-1624) reads
 * it for real.
 *
 * The two-phase shape is the point, not a limitation. It gives the server and
 * the client's first render one agreed answer to key a request on, and hands
 * the viewer's actual zone to every render after. The agreed answer is
 * `CANONICAL_FIRST_SCREEN_TIMEZONE`, which documents why it is US-West rather
 * than the API's UTC default; the short version is that UTC rolls over at 4 or
 * 5 PM Pacific and would drop tonight's shows from the first screen at peak
 * reading hours.
 *
 * What the refinement costs, stated plainly because it is not free: on a cold
 * `/shows` the page issues four client requests where it used to issue two.
 * The seeded entries are stale by construction, so the hydration commit
 * revalidates the canonical `/shows/upcoming` and `/shows/cities` pair, and the
 * commit after it, once this hook reports the real zone, fetches the
 * viewer-keyed pair. The first pair's results are discarded. It is also
 * VISIBLE: the key change makes `isPlaceholderData` true for one round trip, so
 * the list dims to 60% just after it hydrates. That is the accepted
 * refine-on-hydrate transition, softened by the opacity treatment the ticket
 * asks for, and for a US viewer the two answers almost always agree so the
 * swap reads as nothing happening. `/venues` and `/scenes` pay none of this;
 * nothing there keys on a timezone.
 *
 * Both costs are symptoms of asking the viewer's clock a question that belongs
 * to each show's venue. PSY-1678 moves "upcoming" to a per-show comparison
 * against the venue's own zone, which removes the parameter, the second pair of
 * requests, and this hook.
 *
 * `useSyncExternalStore`, not an effect: React uses `getServerSnapshot` for
 * BOTH the server render and the hydration render, which is exactly the
 * guarantee needed, and it is a documented contract rather than a race between
 * an effect and a first paint.
 */
export function useBrowserTimezone(): string {
  return useSyncExternalStore(
    subscribe,
    getBrowserSnapshot,
    getServerSnapshot,
  )
}
