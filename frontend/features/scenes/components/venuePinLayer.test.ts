import { describe, it, expect } from 'vitest'
import {
  VENUE_PIN_CAP_COUNT,
  VENUE_PIN_STROKE,
  VENUE_PIN_STROKE_PX,
  venuePinFeatures,
  venuePinPaint,
  venuePinRadiusPx,
} from './venuePinLayer'
import { MARKER_RAMPS, type MarkerTheme } from './globeScale'

const PINS = [
  { id: 1, lng: -112.07, lat: 33.45, upcomingShowCount: 12 },
  { id: 2, lng: -112.03, lat: 33.49, upcomingShowCount: 0 },
]

const THEMES: MarkerTheme[] = ['dark', 'light']
const DARK = MARKER_RAMPS.dark

describe('venuePinFeatures', () => {
  it('writes every property the pin paint reads', () => {
    const [busy] = venuePinFeatures(PINS, DARK).features
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
    const [busy, quiet] = venuePinFeatures(PINS, DARK).features
    expect(busy.properties?.radiusPx).toBe(venuePinRadiusPx(12))
    expect(quiet.properties?.radiusPx).toBe(venuePinRadiusPx(0))
  })

  it('marks a room with nothing booked as quiet', () => {
    const [busy, quiet] = venuePinFeatures(PINS, DARK).features
    expect(busy.properties?.isQuiet).toBe(false)
    expect(quiet.properties?.isQuiet).toBe(true)
  })

  it.each(THEMES)(
    'marks only the selected room, in the %s ramp, and nothing at all with no selection',
    (theme) => {
      const ramp = MARKER_RAMPS[theme]
      const selected = venuePinFeatures(PINS, ramp, 2).features
      expect(selected[0].properties?.isSelected).toBe(false)
      expect(selected[0].properties?.color).toBe(ramp.base)
      expect(selected[1].properties?.isSelected).toBe(true)
      expect(selected[1].properties?.color).toBe(ramp.selected)

      const none = venuePinFeatures(PINS, ramp).features
      expect(none.every(f => f.properties?.isSelected === false)).toBe(true)
    },
  )

  it('is empty for no pins', () => {
    expect(venuePinFeatures([], DARK).features).toEqual([])
  })
})

describe('venuePinPaint', () => {
  it.each(THEMES)('hovers in the %s ramp, never over the selected pin', (theme) => {
    const ramp = MARKER_RAMPS[theme]
    expect(venuePinPaint(ramp)?.['circle-color']).toEqual([
      'case',
      [
        'all',
        ['boolean', ['feature-state', 'hover'], false],
        ['!', ['get', 'isSelected']],
      ],
      ramp.hovered,
      ['get', 'color'],
    ])
  })

  it('rims every pin dark on the dark ramp', () => {
    const paint = venuePinPaint(DARK)
    expect(paint?.['circle-stroke-color']).toBe(VENUE_PIN_STROKE)
    expect(paint?.['circle-stroke-width']).toBe(VENUE_PIN_STROKE_PX)
  })

  it('rings the selected pin in paper on the light ramp, keeping the dark rim on the rest', () => {
    const paint = venuePinPaint(MARKER_RAMPS.light)
    expect(paint?.['circle-stroke-color']).toEqual([
      'case',
      ['get', 'isSelected'],
      '#f4f1ea',
      VENUE_PIN_STROKE,
    ])
    expect(paint?.['circle-stroke-width']).toEqual([
      'case',
      ['get', 'isSelected'],
      1.5,
      VENUE_PIN_STROKE_PX,
    ])
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
