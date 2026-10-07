'use client'

import type { ComponentProps } from 'react'
import { useSyncExternalStore } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { ATLAS_PATHNAME, isAtlasMapReady, subscribeAtlasMapReady } from './atlasMapReady'

/**
 * When a link may prefetch while the visitor is on the Atlas:
 * - `'after-map'`: once the Atlas-ready signal releases (lib/atlasMapReady.ts).
 * - `'never'`: not on the Atlas at all; the link loads on click.
 * Off the Atlas every link prefetches as it normally would.
 */
export type AtlasPrefetch = 'after-map' | 'never'

const noSubscription = () => () => {}

/**
 * Whether a link holds its prefetch right now. On the Atlas a `'never'` link
 * always holds and an `'after-map'` link holds until the signal releases,
 * because prefetches share the phone's connection and main thread with the
 * map's own downloads.
 */
function useHoldsPrefetch(atlasPrefetch: AtlasPrefetch): boolean {
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
  if (pathname !== ATLAS_PATHNAME) return false
  return atlasPrefetch === 'never' || !released
}

/**
 * `next/link` with the Atlas prefetch hold; `atlasPrefetch` says when the link
 * may prefetch on the Atlas (see {@link AtlasPrefetch}). While the hold is on,
 * the link renders with `prefetch={false}`, which in the App Router also turns
 * off its hover and touchstart prefetch. When it releases, the link takes the
 * caller's `prefetch` again and Next starts observing it for viewport
 * prefetch (pinned against the real Link in
 * components/layout/nav/ChromeLink.realLink.test.tsx).
 *
 * This module imports only React, Next and the signal, so the chrome
 * (ChromeLink) and the Atlas pane (AtlasPaneLink) can both wrap it without
 * either depending on the other.
 */
export function AtlasMapReadyLink({
  prefetch,
  atlasPrefetch,
  ...props
}: ComponentProps<typeof Link> & { atlasPrefetch: AtlasPrefetch }) {
  const holdsPrefetch = useHoldsPrefetch(atlasPrefetch)
  return <Link {...props} prefetch={holdsPrefetch ? false : prefetch} />
}
