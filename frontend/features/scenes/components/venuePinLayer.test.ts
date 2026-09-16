import { describe, it, expect } from 'vitest'
import {
  VENUE_PIN_CAP_COUNT,
  venuePinFeatures,
  venuePinRadiusPx,
} from './venuePinLayer'
import {
  DOT_COLOR_BASE,
  DOT_COLOR_SELECTED,
} from './globeScale'

const PINS = [
  { id: 1, lng: -112.07, lat: 33.45, upcomingShowCount: 12 },
  { id: 2, lng: -112.03, lat: 33.49, upcomingShowCount: 0 },
]

describe('venuePinFeatures', () => {
  it('writes every property the pin paint reads', () => {
    const [busy] = venuePinFeatures(PINS).features
    expect(Object.keys(busy.properties ?? {}).sort()).toEqual([
      'color',
      'id',
      'isQuiet',
      'isSelected',
      'radiusPx',
    ])
    expect(busy.geometry).toEqual({
      type: 'Point',
      coordinates: [-112.07, 33.45],
    })
  })

  it('sizes each pin by its upcoming count', () => {
    const [busy, quiet] = venuePinFeatures(PINS).features
    expect(busy.properties?.radiusPx).toBe(venuePinRadiusPx(12))
    expect(quiet.properties?.radiusPx).toBe(venuePinRadiusPx(0))
  })

  it('marks a room with nothing booked as quiet', () => {
    const [busy, quiet] = venuePinFeatures(PINS).features
    expect(busy.properties?.isQuiet).toBe(false)
    expect(quiet.properties?.isQuiet).toBe(true)
  })

  it('marks only the selected room, and nothing at all with no selection', () => {
    const selected = venuePinFeatures(PINS, 2).features
    expect(selected[0].properties?.isSelected).toBe(false)
    expect(selected[0].properties?.color).toBe(DOT_COLOR_BASE)
    expect(selected[1].properties?.isSelected).toBe(true)
    expect(selected[1].properties?.color).toBe(DOT_COLOR_SELECTED)

    const none = venuePinFeatures(PINS).features
    expect(none.every(f => f.properties?.isSelected === false)).toBe(true)
  })

  it('is empty for no pins', () => {
    expect(venuePinFeatures([]).features).toEqual([])
  })
})

describe('venuePinRadiusPx', () => {
  it('grows with the upcoming count', () => {
    expect(venuePinRadiusPx(10)).toBeGreaterThan(venuePinRadiusPx(1))
  })

  it('caps, so one huge venue cannot swallow the block', () => {
    expect(venuePinRadiusPx(VENUE_PIN_CAP_COUNT * 20)).toBe(
      venuePinRadiusPx(VENUE_PIN_CAP_COUNT),
    )
  })

  it('never returns NaN for malformed counts', () => {
    expect(Number.isFinite(venuePinRadiusPx(Number.NaN))).toBe(true)
    expect(Number.isFinite(venuePinRadiusPx(-5))).toBe(true)
  })
})
