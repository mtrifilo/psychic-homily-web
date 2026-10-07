'use client'

import type { ComponentProps } from 'react'
import { useSyncExternalStore } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { isAtlasMapReady, subscribeAtlasMapReady } from '@/lib/atlasMapReady'
import { atlasItem } from './navData'

/**
 * When a chrome link may prefetch while the visitor is on the Atlas:
 * - `'after-map'`: once the Atlas-ready signal releases (lib/atlasMapReady.ts).
 *   The primary navigation uses it: the bottom tab bar, PrimaryNav (top-nav
 *   mode) and the side rail's links (side-nav mode). So does every link the
 *   Atlas draws in its own pane (features/scenes/components: the map's
 *   overlays, the scene preview, the venue and artist panels).
 * - `'never'` (the default): not on the Atlas at all; the link loads on click.
 *   On a phone each prefetch fetches that route's own JS, and the long tail of
 *   menu, footer and banner links would spend that data on routes most Atlas
 *   visitors never open.
 * Off the Atlas every chrome link prefetches as it normally would.
 */
export type AtlasPrefetch = 'after-map' | 'never'

/**
 * Whether a chrome link holds its prefetch right now. Until the Atlas map is
 * up every link holds, because prefetches share the phone's connection and
 * main thread with the map's own downloads.
 */
const noSubscription = () => () => {}

function useChromeHoldsPrefetch(atlasPrefetch: AtlasPrefetch): boolean {
  // Read at render, not in an effect: Next registers a Link's prefetch when it
  // first mounts, so the hold has to be in place on the first commit.
  const pathname = usePathname()
  // A 'never' link holds on the Atlas whatever the map does, so it does not
  // re-render when the signal releases.
  const released = useSyncExternalStore(
    atlasPrefetch === 'after-map' ? subscribeAtlasMapReady : noSubscription,
    isAtlasMapReady,
    () => false
  )
  if (pathname !== atlasItem.href) return false
  return atlasPrefetch === 'never' || !released
}

/**
 * `next/link` for the app chrome and the Atlas pane's own links; the lint rule
 * on `components/layout` and the Atlas pane's files keeps them from importing
 * `next/link` directly. `atlasPrefetch` says when the
 * link may prefetch on the Atlas (see {@link AtlasPrefetch}). While
 * {@link useChromeHoldsPrefetch} holds, the link renders with
 * `prefetch={false}`, which in the App Router also turns off its hover and
 * touchstart prefetch. When the hold releases, the link takes the caller's
 * `prefetch` again and Next starts observing it for viewport prefetch (pinned
 * against the real Link in ChromeLink.realLink.test.tsx).
 */
export function ChromeLink({
  prefetch,
  atlasPrefetch = 'never',
  ...props
}: ComponentProps<typeof Link> & { atlasPrefetch?: AtlasPrefetch }) {
  const holdsPrefetch = useChromeHoldsPrefetch(atlasPrefetch)
  return <Link {...props} prefetch={holdsPrefetch ? false : prefetch} />
}
