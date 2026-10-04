'use client'

import { useMediaQuery } from '@/lib/hooks/common/useMediaQuery'

/**
 * The Atlas's compact viewport: any viewport narrower than Tailwind's `lg`
 * breakpoint (64rem, 1024px at the default root size). Phones in either
 * orientation and tablets in portrait fall below it; desktops and tablets in
 * landscape do not.
 *
 * Width rather than pointer type, because what the compact treatment trades
 * away (the night-earth raster today) is screen-size-shaped: the boards that
 * define the phone Atlas are drawn by width, a viewport below `lg` is the
 * same geometry whether a finger or a mouse drives it, and a landscape
 * tablet showing the desktop layout should get the desktop globe with it.
 * A width query is also what Playwright and jsdom can emulate without a
 * device descriptor.
 *
 * Written as the exact complement of Tailwind's `lg:` variant
 * (`min-width: 64rem`), so a component styled with `lg:` utilities and a
 * branch on this query switch at the same width. `not all and (...)` rather
 * than range syntax for browsers that predate Media Queries Level 4 ranges.
 */
export const ATLAS_COMPACT_VIEWPORT_QUERY = 'not all and (min-width: 64rem)'

/**
 * Whether the Atlas is on a compact viewport, re-rendering when the viewport
 * crosses the `lg` breakpoint (a window resize, a tablet rotating). False on
 * the server snapshot and where there is no `matchMedia`, per useMediaQuery's
 * contract.
 */
export function useAtlasCompactViewport(): boolean {
  return useMediaQuery(ATLAS_COMPACT_VIEWPORT_QUERY)
}
