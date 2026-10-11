import { ATLAS_PATHNAME } from '@/lib/atlasMapReady'

/**
 * Routes whose page fills the viewport between the top bar and the fixed
 * BottomTabBar wherever that bar is shown (below `xl`). The site footer would
 * sit under such a page and show through between it and the bar, so it does
 * not render on these routes below `xl`; at `xl` it renders as on every other
 * route. Matched against the exact pathname.
 */
const VIEWPORT_FILLING_ROUTES_BELOW_XL: ReadonlySet<string> = new Set([ATLAS_PATHNAME])

/** Whether the site footer is hidden below `xl` on `pathname`. */
export function isFooterHiddenBelowXl(pathname: string | null): boolean {
  return pathname !== null && VIEWPORT_FILLING_ROUTES_BELOW_XL.has(pathname)
}
