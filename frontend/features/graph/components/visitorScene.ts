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

/** A city+state pair, the same identity ParseSceneSlug returns for a slug. */
export interface ScenePlace {
  city: string
  state: string
}

/**
 * The slug form ParseSceneSlug / buildSceneSlug use: lowercase, spaces to
 * hyphens, last segment the state. "Tempe"+"AZ" → "tempe-az"; "Los Angeles"+
 * "CA" → "los-angeles-ca". The caller feeds this to GET /scenes/{slug}, which
 * already maps a CBSA member slug to the metro principal.
 */
export function sceneSlugFromPlace(city: string, state: string): string {
  return `${city.trim().toLowerCase().replaceAll(' ', '-')}-${state.trim().toLowerCase()}`
}

/**
 * The visitor's own scene, or null when we cannot honestly name one.
 *
 * Two honest ways to name one, in order:
 *
 *   1. Exact city and state. A Phoenix visitor lands on Phoenix.
 *   2. Census CBSA membership, via the principal ParseSceneSlug already
 *      returns for a member slug (Tempe → Phoenix). The caller resolves that
 *      principal — this function does not fetch, and it does not invent a
 *      radius. A city that is not in any scene's CBSA stays null.
 *
 * The shared `matchByGeo` that the city filter and the homepage graph use
 * adds a different second tier — nearest scene by haversine, deliberately
 * uncapped — and this rule deliberately does not. Those surfaces RENDER the
 * place they picked, so a visitor in Anchorage reads "Seattle" and can
 * override it. The caller here silently rewrites where a link goes, under a
 * label that names no city, so the same fallback would send that visitor two
 * thousand kilometres away with nothing on screen having said so.
 *
 * A match also has to be alive. `shows_this_week` is the closest thing the
 * scenes list carries to "is anything on soon". It does NOT promise the
 * destination night has a show on it, and no field here could; it buys the
 * weaker guarantee worth having, that a scene dark all week is not somewhere
 * to send a reader asking what is on tonight. Deliberately the ROLLING window
 * and not `shows_calendar_week`: this link goes to a NIGHT, so the seven days
 * ahead of the reader are the relevant ones, where the calendar week would be
 * nearly spent by Sunday evening. The calendar count is the one to reach for
 * beside a link to the weekly page, which this is not.
 *
 * Case and surrounding whitespace are normalized on both sides: the geo
 * header's spelling of a city need not match the catalog's.
 */
export function pickVisitorScene(
  scenes: readonly SceneListItem[],
  geo: GeoLocation | null | undefined,
  cbsaPrincipal: ScenePlace | null = null,
): SceneListItem | null {
  if (!geo) return null
  const city = normalize(geo.city)
  const state = normalize(geo.state)
  if (city === '' || state === '') return null

  const local =
    findScene(scenes, city, state) ??
    (cbsaPrincipal
      ? findScene(scenes, normalize(cbsaPrincipal.city), normalize(cbsaPrincipal.state))
      : undefined)
  if (!local || local.shows_this_week <= 0) return null
  return local
}

function findScene(
  scenes: readonly SceneListItem[],
  city: string,
  state: string,
): SceneListItem | undefined {
  if (city === '' || state === '') return undefined
  return scenes.find(
    scene => normalize(scene.city) === city && normalize(scene.state) === state,
  )
}
