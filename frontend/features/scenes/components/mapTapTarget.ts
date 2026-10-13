import type { MapGeoJSONFeature, Map as MapLibreMap, PointLike } from 'maplibre-gl'

/**
 * The side, in CSS px, of the square a coarse-pointer click hit-tests: the
 * WCAG 2.5.8 minimum target size. A finger that lands, or lifts, up to half
 * of it off a drawn mark still reaches the mark; the mark draws at its own
 * size.
 */
const COARSE_TAP_BOX_PX = 24

interface ScreenPoint {
  x: number
  y: number
}

/**
 * The mark a click at `point` selects in the circle layer `layerId`, or null.
 *
 * The one hit rule for map marks drawn as circles from point features whose
 * `radiusPx` property is their drawn radius. `topFirst` orders the layer's
 * features top of draw first, as the layer's own hover pick reads them
 * (default: the order MapLibre returns them in).
 * - Fine pointer: the topmost feature drawn under the point.
 * - Coarse pointer: of the features drawn within the COARSE_TAP_BOX_PX
 *   square centred on the point, the one whose drawn edge is nearest the
 *   point, the earliest in `topFirst` order among equals. A tap on a drawn
 *   mark is at distance 0 from it, so it resolves exactly as a fine
 *   pointer's click there does; the box only extends the reach of a tap that
 *   misses every mark.
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
  const half = COARSE_TAP_BOX_PX / 2
  const region: PointLike | [PointLike, PointLike] = coarse
    ? [
        [point.x - half, point.y - half],
        [point.x + half, point.y + half],
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
    const radius = Number(feature.properties?.radiusPx)
    const distance = Math.max(
      0,
      Math.hypot(centre.x - point.x, centre.y - point.y) -
        (Number.isFinite(radius) ? radius : 0),
    )
    if (distance < nearestDistance) {
      nearest = feature
      nearestDistance = distance
    }
  }
  return nearest
}
