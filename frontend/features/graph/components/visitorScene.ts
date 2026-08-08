/**
 * Which scene, if any, belongs to the visitor looking at the Observatory.
 *
 * Pure selection logic, no React — the component owns fetching and state; this
 * owns the rule. Separately testable, like the sibling escape-hatch picker.
 */

import type { GeoLocation } from '@/lib/geo-default'
import type { SceneListItem } from '@/features/scenes/types'

function normalize(value: string | null | undefined): string {
  return (value ?? '').trim().toLowerCase()
}

/**
 * The visitor's own scene, or null when we cannot honestly name one.
 *
 * **Exact city and state only.** The shared `matchByGeo` that the city filter
 * and the homepage graph use adds a second tier — nearest scene by haversine,
 * deliberately uncapped — and this rule deliberately does not. Those surfaces
 * RENDER the place they picked, so a visitor in Anchorage reads "Seattle" and
 * can override it. The caller here silently rewrites where a link goes, under
 * a label that names no city, so the same fallback would send that visitor two
 * thousand kilometres away with nothing on screen having said so. The cost is
 * real and accepted: a visitor whose suburb is not itself a scene keeps the
 * global listing rather than being sent to the metro next door. Missing a
 * scene we could have named is the failure worth having here.
 *
 * A match also has to be alive. `shows_this_week` is the closest thing the
 * scenes list carries to "is anything on soon". It does NOT promise the
 * destination night has a show on it, and no field here could; it buys the
 * weaker guarantee worth having, that a scene dark all week is not somewhere
 * to send a reader asking what is on tonight.
 *
 * Case and surrounding whitespace are normalized on both sides: the geo
 * header's spelling of a city need not match the catalog's.
 */
export function pickVisitorScene(
  scenes: readonly SceneListItem[],
  geo: GeoLocation | null | undefined,
): SceneListItem | null {
  if (!geo) return null
  const city = normalize(geo.city)
  const state = normalize(geo.state)
  if (city === '' || state === '') return null

  const local = scenes.find(
    scene => normalize(scene.city) === city && normalize(scene.state) === state,
  )
  if (!local || local.shows_this_week <= 0) return null
  return local
}
