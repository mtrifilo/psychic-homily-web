/**
 * `/atlas?city=City,ST`: the Atlas's one URL entry point.
 *
 * READ ONCE, never written. The Atlas keeps no camera state in the URL: a
 * visitor who pans and zooms is not editing an address, and writing the camera
 * back would give one view a new URL on every gesture. This param exists so
 * ANOTHER surface can hand the Atlas a starting city, and it stops being true
 * the moment the visitor moves the camera.
 *
 * Deliberately dependency-light: the surfaces that only LINK into the Atlas
 * import this, and must not pay for the globe's logic to build an href. The
 * rule that turns a named city into a camera focus lives in `cityView.ts`,
 * beside the rest of the city-view geometry.
 */

import {
  buildCitiesParam,
  parseCitiesParam,
} from '@/components/filters/cityParams'
import type { CityState } from '@/components/filters/CityFilters'

/** The query key an entry link writes, and the one the Atlas reads. */
export const ATLAS_CITY_PARAM = 'city'

/**
 * A link into the Atlas opened on one city, in the `City,ST` wire format the
 * `?cities=` family already uses.
 */
export function atlasCityHref(city: string, state: string): string {
  // Through `buildCitiesParam`, not a template: that module is the single
  // source of truth for the `City,ST` wire format, and this file already parses
  // with its counterpart. Serializing by hand is how the two halves drift.
  const value = buildCitiesParam([{ city, state }])
  return `/atlas?${ATLAS_CITY_PARAM}=${encodeURIComponent(value)}`
}

/**
 * The one city `?city=` names, or null.
 *
 * Null for an absent param, a malformed one, and for a value naming more than
 * one city: this entry point opens on a place, and "several places" is not one.
 */
export function parseAtlasCityParam(
  param: string | null | undefined,
): CityState | null {
  const parsed = parseCitiesParam(param)
  return parsed.length === 1 ? parsed[0] : null
}
