'use client'

import { useMediaQuery } from '@/lib/hooks/common/useMediaQuery'

/**
 * The Atlas's compact viewport: narrower than Tailwind's `lg` breakpoint
 * (64rem), so phones in either orientation and tablets in portrait.
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
 * Whether the Atlas is on a compact viewport, re-rendering when the viewport
 * crosses the `lg` breakpoint (a window resize, a tablet rotating). False on
 * the server snapshot and where there is no `matchMedia`, per useMediaQuery's
 * contract.
 */
export function useAtlasCompactViewport(): boolean {
  return useMediaQuery(ATLAS_COMPACT_VIEWPORT_QUERY)
}
