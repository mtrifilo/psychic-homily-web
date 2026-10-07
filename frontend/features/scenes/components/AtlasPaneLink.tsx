'use client'

import type { ComponentProps } from 'react'
import { ChromeLink } from '@/components/layout/nav/ChromeLink'

/**
 * `next/link` for every link the Atlas draws in its own pane: the map's
 * overlays, the scene preview, the venue and artist panels. On `/atlas` it
 * prefetches only once the Atlas-ready signal releases (ChromeLink's
 * `'after-map'` rule); elsewhere it is a plain Link.
 *
 * The links that can mount while the signal still holds are the ones drawn
 * with the map, before any input: the "not on the map" link and the My Scenes
 * overflow link. The preview and the panels open on a tap or a key, which
 * releases the signal first, so for them the rule is the pane's one link
 * policy rather than a hold that changes anything today.
 *
 * The lint rule on the Atlas pane's files and atlasPaneLinks.test.ts keep
 * the pane from importing `next/link` directly.
 */
export function AtlasPaneLink(
  props: Omit<ComponentProps<typeof ChromeLink>, 'atlasPrefetch'>,
) {
  return <ChromeLink {...props} atlasPrefetch="after-map" />
}
