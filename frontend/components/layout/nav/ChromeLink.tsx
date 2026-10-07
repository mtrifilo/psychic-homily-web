'use client'

import type { ComponentProps } from 'react'
import { AtlasMapReadyLink, type AtlasPrefetch } from '@/lib/atlasMapReadyLink'

/**
 * `next/link` for the app chrome; the lint rule on `components/layout` keeps
 * the chrome from importing `next/link` directly. The Atlas prefetch hold is
 * AtlasMapReadyLink's (lib/atlasMapReadyLink.tsx); this wrapper sets the
 * chrome's policy for it:
 * - `atlasPrefetch="after-map"` for the primary navigation: the bottom tab
 *   bar, PrimaryNav (top-nav mode) and the side rail's links (side-nav mode).
 * - `'never'` (the default) for everything else. On a phone each prefetch
 *   fetches that route's own JS, and the long tail of menu, footer and banner
 *   links would spend that data on routes most Atlas visitors never open.
 */
export function ChromeLink({
  atlasPrefetch = 'never',
  ...props
}: Omit<ComponentProps<typeof AtlasMapReadyLink>, 'atlasPrefetch'> & {
  atlasPrefetch?: AtlasPrefetch
}) {
  return <AtlasMapReadyLink {...props} atlasPrefetch={atlasPrefetch} />
}
