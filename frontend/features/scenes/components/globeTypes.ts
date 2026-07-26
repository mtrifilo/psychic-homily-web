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

/**
 * One venue pin on the city view (PSY-1539) — a flat VIEW MODEL, already
 * positioned and already worded.
 *
 * GlobeCanvas deliberately takes this rather than a venue API row: the canvas
 * must not know how a pin position is chosen (that is the PSY-1536 privacy
 * gate, decided in cityView.venuePinPosition) nor how a date is worded. It
 * only draws what it is handed.
 */
export interface VenuePin {
  id: number
  name: string
  lng: number
  lat: number
  upcomingShowCount: number
  /** Preformatted tooltip line, e.g. "next Fri, Jul 31". '' when nothing booked. */
  nextShowLabel: string
}
// Deliberately no `precision` field. Which coordinate source a pin came from
// is decided (and unit-tested) in cityView.venuePinPosition; carrying it down
// here would be a value nothing reads, and DRAWING street and centroid pins
// differently is a design decision the approved mock does not make.

/** Camera state reported by GlobeCanvas once a movement settles. */
export interface CameraSettle {
  lng: number
  lat: number
  zoom: number
}
