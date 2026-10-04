'use client'

import { useMediaQuery } from '@/lib/hooks/common/useMediaQuery'

/**
 * The Atlas's compact viewport: any viewport narrower than Tailwind's `lg`
 * breakpoint (64rem). AtlasGlobe mounts the map only in a container at least
 * 640px wide and renders a scene list below that, so this query decides the
 * map's treatment between 640px and 64rem, and at every width below 64rem
 * wherever the map does mount.
 *
 * Width rather than pointer type: the phone boards are drawn by width, and a
 * viewport at `lg` or wider is desktop-sized whether a finger or a mouse
 * drives it.
 *
 * The exact complement of Tailwind's `lg:` variant (`min-width: 64rem`), so
 * an `lg:` utility and a branch on this query switch at the same width;
 * `not all and (...)` rather than range syntax for browsers that predate
 * Media Queries Level 4 ranges.
 */
export const ATLAS_COMPACT_VIEWPORT_QUERY = 'not all and (min-width: 64rem)'

/**
 * Whether the viewport is compact right now, read straight from `matchMedia`:
 * for a decision made once, such as a map style built at construction, where
 * a hook's first value could be its server snapshot. False where there is no
 * `matchMedia` (the server, jsdom without a shim).
 */
export function isAtlasCompactViewport(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false
  }
  return window.matchMedia(ATLAS_COMPACT_VIEWPORT_QUERY).matches
}

/**
 * {@link isAtlasCompactViewport} as a subscription, re-rendering when the
 * viewport crosses the `lg` breakpoint (a window resize, a tablet rotating).
 * False on the server snapshot, per useMediaQuery's contract.
 */
export function useAtlasCompactViewport(): boolean {
  return useMediaQuery(ATLAS_COMPACT_VIEWPORT_QUERY)
}
