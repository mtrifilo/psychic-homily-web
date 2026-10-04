import { describe, it, expect } from 'vitest'
import { zoomForAltitude } from '../components/globeScale'
import {
  PLACE_LABEL_BUDGET_AT_ENTRY,
  PLACE_LABEL_BUDGET_AT_FULL,
  PLACE_LABEL_BUDGET_FULL_ZOOM,
  PLACE_LABEL_ENTRY_ZOOM,
  PLACE_LABEL_MIN_ZOOM,
  dotBox,
  facingPoint,
  isFacing,
  parseGlobePlaces,
  pickPlaceLabels,
  placeLabelBudget,
  placeLabelsShowAt,
  type Box,
  type GlobeProjector,
} from './globePlaces'

const pane: Box = { left: 0, top: 0, right: 390, bottom: 731 }
const box = (left: number, top: number, width = 40, height = 12): Box => ({
  left,
  top,
  right: left + width,
  bottom: top + height,
})
const label = (id: string, b: Box) => ({ id, box: b })

describe('place label zoom range', () => {
  const CEILING = 5.5

  it('shows labels from zoom 2 up to, not including, the ceiling', () => {
    expect(PLACE_LABEL_MIN_ZOOM).toBe(2)
    expect(placeLabelsShowAt(1.99, CEILING)).toBe(false)
    expect(placeLabelsShowAt(2, CEILING)).toBe(true)
    expect(placeLabelsShowAt(PLACE_LABEL_ENTRY_ZOOM, CEILING)).toBe(true)
    expect(placeLabelsShowAt(5.49, CEILING)).toBe(true)
    expect(placeLabelsShowAt(5.5, CEILING)).toBe(false)
    expect(placeLabelsShowAt(12, CEILING)).toBe(false)
  })
})

describe('placeLabelBudget', () => {
  it('allows about 12 labels at the default entry zoom', () => {
    expect(PLACE_LABEL_BUDGET_AT_ENTRY).toBe(12)
    expect(placeLabelBudget(PLACE_LABEL_ENTRY_ZOOM)).toBe(12)
  })

  it('calibrates at the zoom the default camera opens at', () => {
    // The Atlas's default camera altitude, in globeScale's altitude units.
    expect(PLACE_LABEL_ENTRY_ZOOM).toBeCloseTo(zoomForAltitude(1.8), 2)
  })

  it('grows with zoom up to zoom 5 and holds there', () => {
    expect(placeLabelBudget(1.5)).toBe(PLACE_LABEL_BUDGET_AT_ENTRY)
    expect(placeLabelBudget(3.5)).toBeGreaterThan(placeLabelBudget(PLACE_LABEL_ENTRY_ZOOM))
    expect(placeLabelBudget(4.5)).toBeGreaterThan(placeLabelBudget(3.5))
    expect(PLACE_LABEL_BUDGET_FULL_ZOOM).toBe(5)
    expect(placeLabelBudget(5)).toBe(PLACE_LABEL_BUDGET_AT_FULL)
    expect(placeLabelBudget(5.4)).toBe(PLACE_LABEL_BUDGET_AT_FULL)
  })
})

describe('pickPlaceLabels', () => {
  it('keeps candidates in rank order until the budget is spent', () => {
    const candidates = [0, 1, 2, 3, 4].map((i) => label(`p${i}`, box(10, 10 + i * 40)))
    expect(pickPlaceLabels(candidates, [], pane, 3).map((c) => c.id)).toEqual(['p0', 'p1', 'p2'])
  })

  it('drops a label that overlaps a scene label, so the scene keeps its spot', () => {
    const sceneLabel = box(100, 100, 60, 16)
    const candidates = [label('under-scene', box(120, 104)), label('clear', box(100, 200))]
    expect(pickPlaceLabels(candidates, [sceneLabel], pane, 12).map((c) => c.id)).toEqual(['clear'])
  })

  it('drops a label that sits on a scene dot', () => {
    const dot = dotBox(150, 300, 6)
    const candidates = [label('on-dot', box(130, 295)), label('clear', box(10, 10))]
    expect(pickPlaceLabels(candidates, [dot], pane, 12).map((c) => c.id)).toEqual(['clear'])
  })

  it('keeps the gap clear: a label 1px from a blocker is dropped, one 3px away is kept', () => {
    const blocker = box(100, 100)
    const near = label('near', box(141, 100))
    const far = label('far', box(143, 200))
    const farSide = label('far-side', box(143, 100))
    expect(pickPlaceLabels([near], [blocker], pane, 12)).toEqual([])
    expect(pickPlaceLabels([farSide, far], [blocker], pane, 12).map((c) => c.id)).toEqual([
      'far-side',
      'far',
    ])
  })

  it('lets the higher-ranked of two colliding place labels win', () => {
    const candidates = [label('first', box(50, 50)), label('second', box(60, 52))]
    expect(pickPlaceLabels(candidates, [], pane, 12).map((c) => c.id)).toEqual(['first'])
  })

  it('a dropped label does not spend budget', () => {
    const candidates = [
      label('blocked', box(0, 0)),
      label('a', box(0, 100)),
      label('b', box(0, 200)),
    ]
    expect(pickPlaceLabels(candidates, [box(0, 0)], pane, 2).map((c) => c.id)).toEqual(['a', 'b'])
  })

  it('drops a label cut off by the pane edge', () => {
    const candidates = [label('edge', box(370, 100)), label('in', box(300, 100))]
    expect(pickPlaceLabels(candidates, [], pane, 12).map((c) => c.id)).toEqual(['in'])
  })
})

describe('facingPoint', () => {
  // A stand-in projection: the near hemisphere (lng in [-90, 90]) maps
  // linearly onto the pane and round-trips; a far-side location projects
  // into the pane too but unprojects onto its near-side mirror.
  const projector: GlobeProjector = {
    project: ([lng, lat]) => {
      const nearLng = Math.abs(lng) <= 90 ? lng : Math.sign(lng) * 180 - lng
      return { x: 195 + nearLng * 2, y: 365 - lat * 3 }
    },
    unproject: ([x, y]) => ({ lng: (x - 195) / 2, lat: (365 - y) / 3 }),
  }

  it('returns the screen point of a near-side location inside the pane', () => {
    expect(facingPoint(projector, 10, 20, 390, 731)).toEqual({ x: 215, y: 305 })
  })

  it('rejects a far-side location even though it projects inside the pane', () => {
    expect(facingPoint(projector, 170, 20, 390, 731)).toBeNull()
  })

  it('rejects a location that projects outside the pane', () => {
    expect(facingPoint(projector, 89, 20, 300, 731)).toBeNull()
  })

  it('tells near side from far side without a pane, for marks that may hang into it', () => {
    expect(isFacing(projector, 89, 20)).toBe(true)
    expect(isFacing(projector, 170, 20)).toBe(false)
  })
})

describe('parseGlobePlaces', () => {
  const feature = (properties: Record<string, unknown>, coordinates: number[] = [1, 2]) => ({
    type: 'Feature' as const,
    properties,
    geometry: { type: 'Point' as const, coordinates },
  })

  it('returns valid places in rank order and skips malformed entries', () => {
    const places = parseGlobePlaces({
      type: 'FeatureCollection',
      features: [
        feature({ name: 'Second', rank: 1 }),
        feature({ name: 'First', rank: 0 }, [-87.6, 41.9]),
        feature({ name: '', rank: 2 }),
        feature({ name: 'No rank' }),
        feature({ name: 'Out of bounds', rank: 3 }, [200, 0]),
        {
          type: 'Feature',
          properties: { name: 'Line', rank: 4 },
          geometry: { type: 'LineString', coordinates: [[0, 0], [1, 1]] },
        },
      ],
    })
    expect(places).toEqual([
      { name: 'First', lng: -87.6, lat: 41.9, rank: 0 },
      { name: 'Second', lng: 1, lat: 2, rank: 1 },
    ])
  })

  it('skips null features, missing or non-numeric coordinates, and a collection without features', () => {
    const places = parseGlobePlaces({
      type: 'FeatureCollection',
      features: [
        null,
        { type: 'Feature', properties: { name: 'No geometry', rank: 0 }, geometry: null },
        feature({ name: 'Null coordinates', rank: 1 }, null as unknown as number[]),
        feature({ name: 'String coordinates', rank: 2 }, ['12', '40'] as unknown as number[]),
        feature({ name: 'Kept', rank: 3 }, [12, 40]),
      ] as unknown as GeoJSON.Feature[],
    })
    expect(places).toEqual([{ name: 'Kept', lng: 12, lat: 40, rank: 3 }])
    expect(
      parseGlobePlaces({ type: 'FeatureCollection' } as unknown as GeoJSON.FeatureCollection),
    ).toEqual([])
  })
})
