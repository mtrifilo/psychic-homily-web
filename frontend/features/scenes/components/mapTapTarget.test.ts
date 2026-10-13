import { describe, it, expect, vi } from 'vitest'
import type { MapGeoJSONFeature, Map as MapLibreMap } from 'maplibre-gl'
import {
  queryRenderedCircles,
  renderedCircle,
  type RenderedCircle,
} from '@/test/renderedCircles'
import { circleMarkAt } from './mapTapTarget'

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

/** A map whose query returns `features` whatever the region, so the rule sees every one. */
function everythingMap(features: unknown[]) {
  return {
    getLayer: () => ({}),
    project: ([x, y]: [number, number]) => ({ x, y }),
    queryRenderedFeatures: () => features,
  } as unknown as MapLibreMap
}

const idOf = (feature: MapGeoJSONFeature | null) => feature?.properties?.id ?? null
const pin = (id: number | string, x: number, y: number, radiusPx = 7, strokePx = 0, extra = {}) =>
  renderedCircle({ x, y, radiusPx, strokePx, properties: { id, ...extra } })

describe('circleMarkAt', () => {
  const PINS = { pins: [pin(1, 100, 100), pin(2, 124, 100)] }

  it('queries the point for a fine pointer and a 24px square centred on it for a coarse one, in the named layer only', () => {
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

  it('selects nothing where no mark is in reach', () => {
    const { map } = fakeMap(PINS)
    expect(circleMarkAt(map, { x: 0, y: 0 }, 'pins', true)).toBeNull()
    expect(circleMarkAt(map, { x: 0, y: 0 }, 'pins', false)).toBeNull()
  })

  it('reaches a mark up to 12px off its edge on a coarse pointer only', () => {
    const { map } = fakeMap(PINS)
    // Pin 1's edge is at 93; a tap at 81 is 12px off it.
    expect(idOf(circleMarkAt(map, { x: 81, y: 100 }, 'pins', true))).toBe(1)
    expect(idOf(circleMarkAt(map, { x: 80, y: 100 }, 'pins', true))).toBeNull()
    expect(idOf(circleMarkAt(map, { x: 92, y: 100 }, 'pins', false))).toBeNull()
    expect(idOf(circleMarkAt(map, { x: 94, y: 100 }, 'pins', false))).toBe(1)
  })

  it('reaches 12px off the edge on the diagonal too, not to the corner of the query square', () => {
    const { map } = fakeMap(PINS)
    // 19px from pin 1's centre on the diagonal is 12px off its edge; a tap
    // whose square corner still touches the pin (about 16px off) selects nothing.
    const d = 19 / Math.SQRT2
    expect(idOf(circleMarkAt(map, { x: 100 - d, y: 100 - d }, 'pins', true))).toBe(1)
    expect(circleMarkAt(map, { x: 84, y: 84 }, 'pins', true)).toBeNull()
  })

  it('measures from the outer rim, so the stroke is part of the mark', () => {
    // A 7px pin with a 1.5px rim: the rim's outer edge is at 91.5.
    const { map } = fakeMap({ pins: [pin(1, 100, 100, 7, 1.5)] })
    expect(idOf(circleMarkAt(map, { x: 92, y: 100 }, 'pins', false))).toBe(1)
    expect(idOf(circleMarkAt(map, { x: 79.5, y: 100 }, 'pins', true))).toBe(1)
    expect(circleMarkAt(map, { x: 79, y: 100 }, 'pins', true)).toBeNull()
  })

  it('selects the mark whose drawn edge is nearer for a coarse tap between two', () => {
    const { map } = fakeMap(PINS)
    expect(idOf(circleMarkAt(map, { x: 111, y: 100 }, 'pins', true))).toBe(1)
    expect(idOf(circleMarkAt(map, { x: 113, y: 100 }, 'pins', true))).toBe(2)
  })

  it('selects the mark a coarse tap lands on over one whose centre is nearer', () => {
    // The tap at 109 is inside the 11px mark (centre 9px away) and 4px off
    // the edge of the 3px mark (centre 7px away).
    const { map } = fakeMap({ pins: [pin(3, 100, 100, 11), pin(4, 116, 100, 3)] })
    expect(idOf(circleMarkAt(map, { x: 109, y: 100 }, 'pins', true))).toBe(3)
  })

  it('selects the top mark for a tap on its rim over a lower mark, on either pointer', () => {
    // The small mark draws on top; the tap at 104 is on its 1px rim (4px from
    // its centre, radius 3.4) and inside the big mark's fill.
    const { map } = fakeMap({ pins: [pin('small', 108, 100, 3.4, 1), pin('big', 100, 100, 6, 1)] })
    for (const coarse of [false, true]) {
      expect(idOf(circleMarkAt(map, { x: 104, y: 100 }, 'pins', coarse))).toBe('small')
    }
  })

  it('keeps the earliest of marks equally near a coarse tap', () => {
    const { map } = fakeMap({ pins: [pin(1, 110, 100, 4), pin(2, 90, 100, 4)] })
    expect(idOf(circleMarkAt(map, { x: 100, y: 100 }, 'pins', true))).toBe(1)
  })

  it('resolves a coarse tap on stacked marks as a fine click there does, through the same order', () => {
    const { map } = fakeMap({
      dots: [pin('low', 50, 50, 6, 0, { sortKey: 1 }), pin('high', 52, 50, 6, 0, { sortKey: 9 })],
    })
    const bySortKey = (features: MapGeoJSONFeature[]) =>
      [...features].sort((a, b) => b.properties.sortKey - a.properties.sortKey)
    for (const coarse of [false, true]) {
      expect(idOf(circleMarkAt(map, { x: 51, y: 50 }, 'dots', coarse, bySortKey))).toBe('high')
      expect(idOf(circleMarkAt(map, { x: 51, y: 50 }, 'dots', coarse))).toBe('low')
    }
  })

  it('measures a mark at the size MapLibre evaluated for it, such as a hover-scaled pin', () => {
    // The same 7px pin, once as drawn and once evaluated at 1.2x for a hover.
    const at = { x: 80, y: 100 }
    const plain = everythingMap([pin(1, 100, 100, 7)])
    const hovered = everythingMap([pin(1, 100, 100, 8.4)])
    expect(circleMarkAt(plain, at, 'pins', true)).toBeNull()
    expect(idOf(circleMarkAt(hovered, at, 'pins', true))).toBe(1)
  })

  it('measures a layer that leaves its rim unset at its radius alone', () => {
    // MapLibre's evaluated paint has no circle-stroke-width for such a layer.
    const radiusOnly = renderedCircle({ x: 100, y: 100, radiusPx: 7, properties: { id: 1 } })
    expect(radiusOnly.layer.paint).not.toHaveProperty('circle-stroke-width')
    const { map } = fakeMap({ pins: [radiusOnly] })
    expect(idOf(circleMarkAt(map, { x: 81, y: 100 }, 'pins', true))).toBe(1)
    expect(circleMarkAt(map, { x: 80, y: 100 }, 'pins', true)).toBeNull()
  })

  it('measures a layer that sets neither size at the style defaults, radius 5 and no rim', () => {
    const map = everythingMap([
      { geometry: { type: 'Point', coordinates: [100, 100] }, properties: { id: 1 }, layer: { paint: {} } },
    ])
    expect(idOf(circleMarkAt(map, { x: 83, y: 100 }, 'pins', true))).toBe(1)
    expect(circleMarkAt(map, { x: 82, y: 100 }, 'pins', true)).toBeNull()
  })

  it('reads a mark whose size is not a number as a point, and skips a feature that is not a point', () => {
    const map = everythingMap([
      { geometry: { type: 'LineString', coordinates: [[108, 100], [109, 100]] }, properties: { id: 0 } },
      pin(1, 100, 100, 7),
      {
        geometry: { type: 'Point', coordinates: [110, 100] },
        properties: { id: 2 },
        layer: { paint: { 'circle-radius': 'x' } },
      },
    ])
    // At 108: pin 1's edge is 1px away; pin 2, read as a point, is 2px away.
    expect(idOf(circleMarkAt(map, { x: 108, y: 100 }, 'pins', true))).toBe(1)
    // At 109.5: pin 1's edge is 2.5px away; pin 2 is 0.5px away.
    expect(idOf(circleMarkAt(map, { x: 109.5, y: 100 }, 'pins', true))).toBe(2)
  })
})
