import type { MapGeoJSONFeature, Map as MapLibreMap, PointLike } from 'maplibre-gl'

/**
 * The side, in CSS px, of the square a coarse-pointer click queries: the
 * WCAG 2.5.8 minimum target size. A mark whose drawn edge is within half of
 * it (12px, in any direction) of the tap is in reach; the mark draws at its
 * own size.
 */
const COARSE_TAP_BOX_PX = 24
const COARSE_TAP_REACH_PX = COARSE_TAP_BOX_PX / 2

interface ScreenPoint {
  x: number
  y: number
}

// The style spec's defaults, for a layer that leaves either property unset
// (MapLibre's evaluated paint carries only the properties a layer sets).
const DEFAULT_CIRCLE_RADIUS_PX = 5
const DEFAULT_CIRCLE_STROKE_WIDTH_PX = 0

/**
 * A circle feature's paint extent from its centre, in CSS px: its
 * `circle-radius` plus its `circle-stroke-width`, as MapLibre evaluated them
 * for this feature and its feature-state when it answered the query (so a
 * hover-scaled mark measures at its hovered size). On the globe an off-centre
 * mark draws slightly smaller than this. 0 when a value is not a number,
 * which reads the mark as a point.
 */
function drawnExtentPx(feature: MapGeoJSONFeature): number {
  const paint = feature.layer?.paint as Record<string, unknown> | undefined
  const extent =
    Number(paint?.['circle-radius'] ?? DEFAULT_CIRCLE_RADIUS_PX) +
    Number(paint?.['circle-stroke-width'] ?? DEFAULT_CIRCLE_STROKE_WIDTH_PX)
  return Number.isFinite(extent) ? extent : 0
}

/**
 * The mark a click at `point` selects in the circle layer `layerId`, or null.
 *
 * The one hit rule for map marks drawn as circles from point features.
 * `topFirst` orders the layer's features top of draw first, as the layer's
 * own hover pick reads them (default: the order MapLibre returns them in).
 * - Fine pointer: the topmost feature drawn under the point.
 * - Coarse pointer: of the features whose drawn edge (rim included) is within
 *   COARSE_TAP_REACH_PX of the point, the one whose edge is nearest, the
 *   earliest in `topFirst` order among equals. A tap on a drawn mark is at
 *   distance 0 from it, so it resolves as a fine pointer's click there does
 *   (to within the globe's off-centre shrink above); the reach only extends
 *   a tap that misses every mark.
 *
 * A layer the style does not (yet) have selects nothing.
 */
export function circleMarkAt(
  map: MapLibreMap,
  point: ScreenPoint,
  layerId: string,
  coarse: boolean,
  topFirst: (
    features: MapGeoJSONFeature[],
  ) => MapGeoJSONFeature[] = (features) => features,
): MapGeoJSONFeature | null {
  if (!map.getLayer(layerId)) return null
  // The square bounds the query; the reach check below makes it a circle.
  const region: PointLike | [PointLike, PointLike] = coarse
    ? [
        [point.x - COARSE_TAP_REACH_PX, point.y - COARSE_TAP_REACH_PX],
        [point.x + COARSE_TAP_REACH_PX, point.y + COARSE_TAP_REACH_PX],
      ]
    : [point.x, point.y]
  const features = topFirst(map.queryRenderedFeatures(region, { layers: [layerId] }))
  if (!coarse) return features[0] ?? null

  let nearest: MapGeoJSONFeature | null = null
  let nearestDistance = Infinity
  for (const feature of features) {
    if (feature.geometry.type !== 'Point') continue
    const [lng, lat] = feature.geometry.coordinates
    const centre = map.project([lng, lat])
    const distance = Math.max(
      0,
      Math.hypot(centre.x - point.x, centre.y - point.y) - drawnExtentPx(feature),
    )
    if (distance <= COARSE_TAP_REACH_PX && distance < nearestDistance) {
      nearest = feature
      nearestDistance = distance
    }
  }
  return nearest
}
