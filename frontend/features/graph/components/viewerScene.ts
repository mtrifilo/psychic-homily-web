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
 * visitor's place" means.
 *
 * Two deliberate differences from the homepage graph's `pickDefaultScene`,
 * both because the caller here is a NAVIGATION target rather than a rendered
 * default:
 *
 * - **No liveliest-scene fallback.** A visitor we cannot place gets null, and
 *   the caller keeps the global listing. Sending someone to another region's
 *   nightly page because it happens to be the busiest one would be a worse
 *   answer than the listing they asked for.
 * - **An inactive match is also null.** A scene with nothing upcoming has a
 *   nightly page that is correct and empty; the global listing is the more
 *   useful destination for that visitor.
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
  if (!local || local.upcoming_show_count <= 0) return null
  return local
}
