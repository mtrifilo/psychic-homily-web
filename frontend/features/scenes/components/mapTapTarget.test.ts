import { describe, it, expect, vi } from 'vitest'
import type { MapGeoJSONFeature, Map as MapLibreMap } from 'maplibre-gl'
import {
  queryRenderedCircles,
  renderedCircle,
  type RenderedCircle,
} from '@/test/renderedCircles'
import {
  COARSE_TAP_BOX_PX,
  circleMarkAt,
  nearestCircleIndex,
  tapHitRegion,
} from './mapTapTarget'

describe('tapHitRegion', () => {
  it('is the point itself for a fine pointer', () => {
    expect(tapHitRegion({ x: 40, y: 60 }, false)).toEqual([40, 60])
  })

  it('is a 24px square centred on the point for a coarse pointer', () => {
    expect(COARSE_TAP_BOX_PX).toBe(24)
    expect(tapHitRegion({ x: 40, y: 60 }, true)).toEqual([
      [28, 48],
      [52, 72],
    ])
  })
})

describe('nearestCircleIndex', () => {
  it('is -1 with no circles', () => {
    expect(nearestCircleIndex({ x: 0, y: 0 }, [])).toBe(-1)
  })

  it('picks the circle whose drawn edge is nearest', () => {
    const circles = [
      // Centre 20px away, edge 9px away.
      { centre: { x: 20, y: 0 }, radiusPx: 11 },
      // Centre 12px away, edge 9.5px away.
      { centre: { x: 0, y: 12 }, radiusPx: 2.5 },
      // Centre 10px away, edge 5px away.
      { centre: { x: -10, y: 0 }, radiusPx: 5 },
    ]
    expect(nearestCircleIndex({ x: 0, y: 0 }, circles)).toBe(2)
  })

  it('picks a circle under the point over one whose centre is nearer', () => {
    const circles = [
      { centre: { x: 4, y: 0 }, radiusPx: 1 },
      { centre: { x: -9, y: 0 }, radiusPx: 11 },
    ]
    expect(nearestCircleIndex({ x: 0, y: 0 }, circles)).toBe(1)
  })

  it('keeps the earliest of circles equally near, and of circles under the point', () => {
    expect(
      nearestCircleIndex({ x: 0, y: 0 }, [
        { centre: { x: 10, y: 0 }, radiusPx: 4 },
        { centre: { x: -10, y: 0 }, radiusPx: 4 },
      ]),
    ).toBe(0)
    expect(
      nearestCircleIndex({ x: 0, y: 0 }, [
        { centre: { x: 3, y: 0 }, radiusPx: 6 },
        { centre: { x: 0, y: 0 }, radiusPx: 6 },
      ]),
    ).toBe(0)
  })
})

/** A map that draws `rendered` (screen px coordinates, identity projection). */
function fakeMap(rendered: Record<string, RenderedCircle[]>) {
  const queryRenderedFeatures = vi.fn(
    (
      region: [number, number] | [[number, number], [number, number]],
      options: { layers: string[] },
    ) => queryRenderedCircles(rendered, region, options),
  )
  const map = {
    getLayer: (id: string) => (id in rendered ? {} : undefined),
    queryRenderedFeatures,
    project: ([x, y]: [number, number]) => ({ x, y }),
  }
  return { map: map as unknown as MapLibreMap, queryRenderedFeatures }
}

const idOf = (feature: MapGeoJSONFeature | null) => feature?.properties?.id ?? null

describe('circleMarkAt', () => {
  const PINS = {
    pins: [renderedCircle({ id: 1 }, 100, 100), renderedCircle({ id: 2 }, 124, 100)],
  }

  it('queries the point for a fine pointer and the box for a coarse one, in the named layer only', () => {
    const { map, queryRenderedFeatures } = fakeMap(PINS)
    circleMarkAt(map, { x: 10, y: 20 }, 'pins', false)
    circleMarkAt(map, { x: 10, y: 20 }, 'pins', true)
    expect(queryRenderedFeatures.mock.calls).toEqual([
      [[10, 20], { layers: ['pins'] }],
      [
        [
          [-2, 8],
          [22, 32],
        ],
        { layers: ['pins'] },
      ],
    ])
  })

  it('selects nothing, and queries nothing, for a layer the style does not have', () => {
    const { map, queryRenderedFeatures } = fakeMap(PINS)
    expect(circleMarkAt(map, { x: 100, y: 100 }, 'dots', true)).toBeNull()
    expect(queryRenderedFeatures).not.toHaveBeenCalled()
  })

  it('reaches a mark up to 12px off its edge on a coarse pointer only', () => {
    const { map } = fakeMap(PINS)
    // Pin 1's edge is at 93; a tap at 81 is 12px off it.
    expect(idOf(circleMarkAt(map, { x: 81, y: 100 }, 'pins', true))).toBe(1)
    expect(idOf(circleMarkAt(map, { x: 80, y: 100 }, 'pins', true))).toBeNull()
    expect(idOf(circleMarkAt(map, { x: 92, y: 100 }, 'pins', false))).toBeNull()
    expect(idOf(circleMarkAt(map, { x: 94, y: 100 }, 'pins', false))).toBe(1)
  })

  it('selects the nearer of two marks for a coarse tap between them', () => {
    const { map } = fakeMap(PINS)
    expect(idOf(circleMarkAt(map, { x: 111, y: 100 }, 'pins', true))).toBe(1)
    expect(idOf(circleMarkAt(map, { x: 113, y: 100 }, 'pins', true))).toBe(2)
  })

  it('resolves a coarse tap on a mark as a fine click there does, through the same order', () => {
    const { map } = fakeMap({
      dots: [
        renderedCircle({ id: 'low', sortKey: 1 }, 50, 50, 6),
        renderedCircle({ id: 'high', sortKey: 9 }, 52, 50, 6),
      ],
    })
    const bySortKey = (features: MapGeoJSONFeature[]) =>
      [...features].sort((a, b) => b.properties.sortKey - a.properties.sortKey)
    for (const coarse of [false, true]) {
      expect(idOf(circleMarkAt(map, { x: 51, y: 50 }, 'dots', coarse, bySortKey))).toBe('high')
      expect(idOf(circleMarkAt(map, { x: 51, y: 50 }, 'dots', coarse))).toBe('low')
    }
  })

  it('reads a mark with no usable radius as a point, and skips a feature that is not a point', () => {
    // A map that returns whatever it holds, so the rule sees every feature.
    const map = {
      getLayer: () => ({}),
      project: ([x, y]: [number, number]) => ({ x, y }),
      queryRenderedFeatures: () => [
        {
          geometry: { type: 'LineString', coordinates: [[108, 100], [109, 100]] },
          properties: { id: 0 },
        },
        { geometry: { type: 'Point', coordinates: [100, 100] }, properties: { id: 1, radiusPx: 7 } },
        { geometry: { type: 'Point', coordinates: [110, 100] }, properties: { id: 2, radiusPx: 'x' } },
      ],
    } as unknown as MapLibreMap
    // At 108: pin 1's edge is 1px away; pin 2, read as a point, is 2px away.
    expect(idOf(circleMarkAt(map, { x: 108, y: 100 }, 'pins', true))).toBe(1)
    // At 109.5: pin 1's edge is 2.5px away; pin 2 is 0.5px away.
    expect(idOf(circleMarkAt(map, { x: 109.5, y: 100 }, 'pins', true))).toBe(2)
  })
})
