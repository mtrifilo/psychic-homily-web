// Place labels for the light globe: which Natural Earth populated places
// (globe-places-110m.geojson, loaded by globeSurface.ts) get a label at the
// current camera. GlobeCanvas draws the chosen ones as DOM markers in the
// app's mono face; this module holds the rules, free of DOM and MapLibre:
//
// - zoom: labels show from PLACE_LABEL_MIN_ZOOM up to the zoom the caller
//   passes as the ceiling (the start of the globe-to-street crossfade, so no
//   place label is left on screen while the street basemap's own labels come
//   in);
// - density: at most placeLabelBudget() labels on screen, in the file's rank
//   order (capitals and the largest cities first);
// - collisions: a place label never overlaps a scene label or a scene dot, or
//   another place label. Scene marks are fixed obstacles, so a scene always
//   wins its spot.

/** A labellable place; `rank` 0 is the first to label. */
export interface GlobePlace {
  name: string
  lng: number
  lat: number
  rank: number
}

/** A screen-space rectangle in CSS px, relative to the map container. */
export interface Box {
  left: number
  top: number
  right: number
  bottom: number
}

/** Below this zoom the globe is too small to label places on. */
export const PLACE_LABEL_MIN_ZOOM = 2

/**
 * The density rule: PLACE_LABEL_BUDGET_AT_ENTRY labels at
 * PLACE_LABEL_ENTRY_ZOOM, growing linearly to PLACE_LABEL_BUDGET_AT_FULL at
 * PLACE_LABEL_BUDGET_FULL_ZOOM and holding there. PLACE_LABEL_ENTRY_ZOOM is
 * the zoom the globe opens at for a visitor with no located city (globeScale's
 * zoomForAltitude of the default camera's altitude 1.8), the view the rule is
 * calibrated against. The budget is the same at every pane size.
 */
export const PLACE_LABEL_BUDGET_AT_ENTRY = 12
export const PLACE_LABEL_BUDGET_AT_FULL = 20
export const PLACE_LABEL_ENTRY_ZOOM = 2.36
export const PLACE_LABEL_BUDGET_FULL_ZOOM = 5

/** Clear space kept around every label, in CSS px. */
const PLACE_LABEL_GAP_PX = 2

export function placeLabelBudget(zoom: number): number {
  const t = Math.min(
    1,
    Math.max(
      0,
      (zoom - PLACE_LABEL_ENTRY_ZOOM) / (PLACE_LABEL_BUDGET_FULL_ZOOM - PLACE_LABEL_ENTRY_ZOOM),
    ),
  )
  return Math.round(
    PLACE_LABEL_BUDGET_AT_ENTRY + t * (PLACE_LABEL_BUDGET_AT_FULL - PLACE_LABEL_BUDGET_AT_ENTRY),
  )
}

/** Whether place labels show at this zoom, below the given ceiling. */
export function placeLabelsShowAt(zoom: number, maxZoom: number): boolean {
  return zoom >= PLACE_LABEL_MIN_ZOOM && zoom < maxZoom
}

/**
 * The places in a loaded collection, in rank order. Features without a
 * string name, a numeric rank and a Point inside lng/lat bounds are skipped:
 * the file is ours, but a bad entry must cost one label, not the layer.
 */
export function parseGlobePlaces(data: GeoJSON.FeatureCollection): GlobePlace[] {
  const places: GlobePlace[] = []
  for (const feature of data.features) {
    const name: unknown = feature.properties?.name
    const rank: unknown = feature.properties?.rank
    if (typeof name !== 'string' || name === '' || typeof rank !== 'number') continue
    if (feature.geometry?.type !== 'Point') continue
    const [lng, lat] = feature.geometry.coordinates
    if (!(Math.abs(lng) <= 180 && Math.abs(lat) <= 90)) continue
    places.push({ name, lng, lat, rank })
  }
  return places.sort((a, b) => a.rank - b.rank)
}

/** The slice of a MapLibre map {@link isFacing} and {@link facingPoint} need. */
export interface GlobeProjector {
  project(lngLat: [number, number]): { x: number; y: number }
  unproject(point: [number, number]): { lng: number; lat: number }
}

// How far, in degrees, a projected point's round trip may land from where it
// started and still count as facing the camera.
const FACING_TOLERANCE_DEG = 0.5

/**
 * Whether a location is on the near side of the globe. A far-side location
 * projects to a point inside the globe's disk too; unprojecting that point
 * lands on the near-side surface, far from where it started, which is how the
 * far side is told apart.
 */
export function isFacing(map: GlobeProjector, lng: number, lat: number): boolean {
  const point = map.project([lng, lat])
  const back = map.unproject([point.x, point.y])
  const dLng = ((((back.lng - lng) % 360) + 540) % 360) - 180
  const dLat = back.lat - lat
  return (
    Math.abs(dLat) <= FACING_TOLERANCE_DEG &&
    Math.abs(dLng * Math.cos((lat * Math.PI) / 180)) <= FACING_TOLERANCE_DEG
  )
}

/**
 * The screen point of a location on the near side of the globe and inside
 * the pane, or null.
 */
export function facingPoint(
  map: GlobeProjector,
  lng: number,
  lat: number,
  paneWidth: number,
  paneHeight: number,
): { x: number; y: number } | null {
  const point = map.project([lng, lat])
  if (!(point.x >= 0 && point.x <= paneWidth && point.y >= 0 && point.y <= paneHeight)) {
    return null
  }
  return isFacing(map, lng, lat) ? point : null
}

/** A square box around a scene dot of the given radius. */
export function dotBox(x: number, y: number, radiusPx: number): Box {
  return { left: x - radiusPx, top: y - radiusPx, right: x + radiusPx, bottom: y + radiusPx }
}

function overlaps(a: Box, b: Box): boolean {
  const gap = PLACE_LABEL_GAP_PX
  return (
    a.left < b.right + gap &&
    b.left < a.right + gap &&
    a.top < b.bottom + gap &&
    b.top < a.bottom + gap
  )
}

function inside(box: Box, pane: Box): boolean {
  return (
    box.left >= pane.left &&
    box.top >= pane.top &&
    box.right <= pane.right &&
    box.bottom <= pane.bottom
  )
}

/**
 * The labels to keep, from candidates already in rank order: each is kept
 * when it lies wholly inside the pane and clears every blocker (scene labels
 * and dots) and every label kept before it, until `budget` are kept.
 */
export function pickPlaceLabels<T extends { box: Box }>(
  candidates: readonly T[],
  blockers: readonly Box[],
  pane: Box,
  budget: number,
): T[] {
  const kept: T[] = []
  for (const candidate of candidates) {
    if (kept.length >= budget) break
    const { box } = candidate
    if (!inside(box, pane)) continue
    if (blockers.some((b) => overlaps(box, b))) continue
    if (kept.some((k) => overlaps(box, k.box))) continue
    kept.push(candidate)
  }
  return kept
}
