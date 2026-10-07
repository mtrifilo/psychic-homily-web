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
  const mapReady = useSyncExternalStore(
    subscribeAtlasMapReady,
    isAtlasMapReady,
    () => false
  )
  return pathname === atlasItem.href && !mapReady
}

/**
 * `next/link` for the app chrome (top bar, bottom tab bar, menus, footer,
 * consent banner). While {@link useChromeHoldsPrefetch} holds, the link renders
 * with `prefetch={false}`; when it releases, the link takes the caller's
 * `prefetch` again, and Next re-arms its viewport prefetch because the Link's
 * ref callback depends on whether prefetch is enabled.
 */
export function ChromeLink({ prefetch, ...props }: ComponentProps<typeof Link>) {
  const holdsPrefetch = useChromeHoldsPrefetch()
  return <Link {...props} prefetch={holdsPrefetch ? false : prefetch} />
}
