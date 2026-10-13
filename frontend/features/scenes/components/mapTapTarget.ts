import type { MapGeoJSONFeature, Map as MapLibreMap, PointLike } from 'maplibre-gl'

/**
 * The side, in CSS px, of the square a coarse-pointer click hit-tests: the
 * WCAG 2.5.8 minimum target size. A finger that lands, or lifts, up to half
 * of it off a drawn mark still reaches the mark; the mark draws at its own
 * size.
 */
export const COARSE_TAP_BOX_PX = 24

export interface ScreenPoint {
  x: number
  y: number
}

/**
 * The region a click at `point` hit-tests: the point itself for a fine
 * pointer, and a COARSE_TAP_BOX_PX square centred on it for a coarse one.
 */
export function tapHitRegion(
  point: ScreenPoint,
  coarse: boolean,
): PointLike | [PointLike, PointLike] {
  if (!coarse) return [point.x, point.y]
  const half = COARSE_TAP_BOX_PX / 2
  return [
    [point.x - half, point.y - half],
    [point.x + half, point.y + half],
  ]
}

/** A circle as drawn on screen: its centre and its radius, in CSS px. */
export interface DrawnCircle {
  centre: ScreenPoint
  radiusPx: number
}

/**
 * The index of the circle whose drawn edge is nearest `point`, or -1 for
 * none. A point on a circle is at distance 0 from it, so among the circles
 * under the point, and among circles equally near, the earliest in the list
 * wins: callers list them top of draw first.
 */
export function nearestCircleIndex(
  point: ScreenPoint,
  circles: readonly DrawnCircle[],
): number {
  let best = -1
  let bestDistance = Infinity
  circles.forEach((circle, index) => {
    const distance = Math.max(
      0,
      Math.hypot(circle.centre.x - point.x, circle.centre.y - point.y) -
        circle.radiusPx,
    )
    if (distance < bestDistance) {
      best = index
      bestDistance = distance
    }
  })
  return best
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
 *   square around the point, the one whose drawn edge is nearest the point.
 *   A tap on a drawn mark is at distance 0 from it, so it resolves exactly as
 *   a fine pointer's click there does; the box only extends the reach of a
 *   tap that misses every mark.
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
  const features = topFirst(
    map.queryRenderedFeatures(tapHitRegion(point, coarse), {
      layers: [layerId],
    }),
  )
  if (!coarse) return features[0] ?? null
  const circles: DrawnCircle[] = []
  const candidates: MapGeoJSONFeature[] = []
  for (const feature of features) {
    if (feature.geometry.type !== 'Point') continue
    const [lng, lat] = feature.geometry.coordinates
    const radius = Number(feature.properties?.radiusPx)
    circles.push({
      centre: map.project([lng, lat]),
      radiusPx: Number.isFinite(radius) ? radius : 0,
    })
    candidates.push(feature)
  }
  return candidates[nearestCircleIndex(point, circles)] ?? null
}
