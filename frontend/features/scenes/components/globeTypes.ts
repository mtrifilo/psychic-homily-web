import type { SceneListItem } from '../types'

/**
 * Atlas globe types + guard, kept in their own module (no react-globe.gl import)
 * so AtlasGlobe and its tests can use them without pulling in three.js — only
 * GlobeCanvas (dynamic-imported, ssr:false) actually loads the WebGL library.
 */

/** A scene with resolved coordinates — the only kind the globe can plot. */
export interface PlaceableScene extends SceneListItem {
  latitude: number
  longitude: number
}

/** Type guard: a scene the globe can place (finite lat/lng). */
export function isPlaceableScene(s: SceneListItem): s is PlaceableScene {
  return (
    typeof s.latitude === 'number' &&
    Number.isFinite(s.latitude) &&
    typeof s.longitude === 'number' &&
    Number.isFinite(s.longitude)
  )
}

/** Camera focus for the globe (re-applied when it changes). */
export interface GlobePov {
  lat: number
  lng: number
  altitude: number
}
