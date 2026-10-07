'use client'

import type { ComponentProps } from 'react'
import { useSyncExternalStore } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { isAtlasMapReady, subscribeAtlasMapReady } from '@/lib/atlasMapReady'
import { atlasItem } from './navData'

/**
 * Whether the app chrome's links hold their prefetches right now: on the
 * Atlas until its first map is up (lib/atlasMapReady.ts), because those
 * prefetches share the phone's connection and main thread with the map's own
 * downloads. Everywhere else, and on the Atlas once the signal releases, the
 * links prefetch as they normally would.
 */
function useChromeHoldsPrefetch(): boolean {
  // Read at render, not in an effect: Next registers a Link's prefetch when it
  // first mounts, so the hold has to be in place on the first commit.
  const pathname = usePathname()
  const released = useSyncExternalStore(
    subscribeAtlasMapReady,
    isAtlasMapReady,
    () => false
  )
  return pathname === atlasItem.href && !released
}

/**
 * `next/link` for the app chrome; the lint rule on `components/layout` keeps
 * the chrome from importing `next/link` directly. While
 * {@link useChromeHoldsPrefetch} holds, the link renders with
 * `prefetch={false}`, which in the App Router also turns off its hover and
 * touchstart prefetch. When the hold releases, the link takes the caller's
 * `prefetch` again and Next starts observing it for viewport prefetch (pinned
 * against the real Link in ChromeLink.realLink.test.tsx).
 */
export function ChromeLink({ prefetch, ...props }: ComponentProps<typeof Link>) {
  const holdsPrefetch = useChromeHoldsPrefetch()
  return <Link {...props} prefetch={holdsPrefetch ? false : prefetch} />
}
