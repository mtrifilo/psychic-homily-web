/**
 * Which scene, if any, belongs to the visitor looking at the Observatory.
 *
 * Pure selection logic, no React — the component owns fetching and state; this
 * owns the rule. Separately testable, like the sibling escape-hatch picker.
 */

import { matchByGeo } from '@/lib/geo-client'
import type { GeoLocation } from '@/lib/geo-default'
import type { SceneListItem } from '@/features/scenes/types'

/**
 * The visitor's own scene, or null when we cannot honestly name one.
 *
 * Resolution runs through the shared two-tier `matchByGeo` (exact city/state,
 * else nearest scene by haversine over the scene centroids) so this surface
 * agrees with the shows city filter and the homepage graph about what "the
 * visitor's place" means. That second tier has no distance cap, so "cannot
 * place" means a visitor with no coordinates against scenes with no
 * centroids, not a visitor who is merely far from one. The `/api/geo` route
 * only answers for US/CA addresses, which is what keeps the radius sane.
 *
 * Two deliberate differences from the homepage graph's `pickDefaultScene`,
 * both because the caller here is a NAVIGATION target rather than a rendered
 * default:
 *
 * - **No liveliest-scene fallback.** A visitor we cannot place gets null, and
 *   the caller keeps the global listing. Sending someone to another region's
 *   nightly page because it happens to be the busiest one would be a worse
 *   answer than the listing they asked for.
 * - **A quiet week is also null.** `shows_this_week` is the closest thing the
 *   scenes list carries to "is anything on soon" — it does NOT promise the
 *   destination night has a show on it, and no field here could. It buys the
 *   weaker guarantee worth having: a scene that has been dark all week is not
 *   somewhere to send a reader asking what is on tonight.
 */
export function pickViewerScene(
  scenes: readonly SceneListItem[],
  geo: GeoLocation | null | undefined,
): SceneListItem | null {
  if (!geo || scenes.length === 0) return null
  const local = matchByGeo(scenes, geo, {
    city: s => s.city,
    state: s => s.state,
    lat: s => s.latitude,
    lng: s => s.longitude,
  })
  if (!local || local.shows_this_week <= 0) return null
  return local
}
