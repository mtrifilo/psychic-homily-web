import type { SceneListItem } from '../types'

/**
 * Atlas globe types + guard, kept in their own module (no maplibre-gl import)
 * so AtlasGlobe and its tests can use them without pulling in the WebGL map —
 * only GlobeCanvas (dynamic-imported, ssr:false) actually loads the library.
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

/**
 * Initial camera focus for the globe. Resolved ONCE by AtlasGlobe
 * (first-resolution-wins) and identity-stable for the canvas's lifetime —
 * GlobeCanvas aims the map at it only at construction (a saved camera from a
 * previous show takes precedence), and its map-creation effect depends on
 * this identity, so a NEW pov object tears the map down and rebuilds it.
 * Post-mount camera moves go through the flyToRef seam, never through pov.
 */
export interface GlobePov {
  lat: number
  lng: number
  altitude: number
}
