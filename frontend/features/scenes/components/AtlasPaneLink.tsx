'use client'

import type { ComponentProps } from 'react'
import { ChromeLink } from '@/components/layout/nav/ChromeLink'

/**
 * `next/link` for every link the Atlas draws in its own pane: the map's
 * overlays, the scene preview, the venue and artist panels. On `/atlas` it
 * prefetches only once the Atlas-ready signal releases (ChromeLink's
 * `'after-map'` rule), so the pane's links do not share the phone's
 * connection with the map's own downloads; elsewhere it is a plain Link. The
 * lint rule on the Atlas pane's files keeps them from importing `next/link`
 * directly.
 */
export function AtlasPaneLink(
  props: Omit<ComponentProps<typeof ChromeLink>, 'atlasPrefetch'>,
) {
  return <ChromeLink {...props} atlasPrefetch="after-map" />
}
