/**
 * A stand-in for MapLibre's `queryRenderedFeatures` over circle layers, for
 * map stubs: each layer draws point features whose coordinates are screen px
 * (the stubs' `project` is the identity) and whose `radiusPx` is their drawn
 * radius.
 */
export interface RenderedCircle {
  geometry: { type: 'Point'; coordinates: [number, number] }
  properties: { radiusPx: number } & Record<string, unknown>
  id?: number | string
}

/** A circle drawn centred on screen point (x, y). */
export function renderedCircle(
  properties: Record<string, unknown>,
  x: number,
  y: number,
  radiusPx = 7,
  id?: number | string,
): RenderedCircle {
  return {
    geometry: { type: 'Point', coordinates: [x, y] },
    properties: { ...properties, radiusPx },
    ...(id === undefined ? {} : { id }),
  }
}

/**
 * The circles of `options.layers` the region (a point, or a box as two
 * corners) touches, in each layer's listed order: a stub lists them top of
 * draw first, as MapLibre returns them.
 */
export function queryRenderedCircles(
  rendered: Record<string, readonly RenderedCircle[]>,
  region: [number, number] | [[number, number], [number, number]],
  options: { layers: string[] },
): RenderedCircle[] {
  const [[x1, y1], [x2, y2]] = (
    typeof region[0] === 'number' ? [region, region] : region
  ) as [[number, number], [number, number]]
  return options.layers.flatMap((layer) =>
    (rendered[layer] ?? []).filter((circle) => {
      const [x, y] = circle.geometry.coordinates
      const dx = x - Math.max(x1, Math.min(x, x2))
      const dy = y - Math.max(y1, Math.min(y, y2))
      return Math.hypot(dx, dy) <= circle.properties.radiusPx
    }),
  )
}
