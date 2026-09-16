/**
 * How a room reads as a mark on a map: its size, its colors, and the feature
 * properties the paint reads.
 *
 * One BASE definition, so the Atlas city view and the `/venues` mini Atlas
 * start from the same affordance ramp for the same room, and so the paint and
 * the properties it reads stay in one file. A caller may layer its own
 * per-surface treatment over the paint (the mini Atlas mutes quiet rooms that
 * way); the layer's placement, `minzoom` and source id are the caller's too.
 */

import type { CircleLayerSpecification } from 'maplibre-gl'
import {
  DOT_COLOR_BASE,
  DOT_COLOR_HOVERED,
  DOT_COLOR_SELECTED,
  DOT_HOVER_RADIUS_SCALE,
} from './globeScale'

// ── Pin size ──────────────────────────────────────────────────────────────
// Same shape as the globe's dot scale (sqrt, capped) for the same reason: a
// 40-show venue must read as busier than a 4-show one without ballooning over
// its neighbours on a street map, where venues sit blocks apart. Bigger than
// the globe dots in absolute px because a city map has far fewer marks
// competing for the frame. Retune HERE, not inline in a canvas.
export const VENUE_PIN_BASE_RADIUS_PX = 5
export const VENUE_PIN_VARIABLE_MAX_PX = 6
// Every venue at or above this count draws the same max pin.
export const VENUE_PIN_CAP_COUNT = 20

/** Pin radius in CSS px for a venue's upcoming-show count. */
export function venuePinRadiusPx(upcomingShowCount: number): number {
  // Non-finite guard, matching sceneDotRadius: a NaN radius poisons the layer.
  const count = Number.isFinite(upcomingShowCount)
    ? Math.max(0, upcomingShowCount)
    : 0
  const variable =
    (Math.sqrt(count) / Math.sqrt(VENUE_PIN_CAP_COUNT)) *
    VENUE_PIN_VARIABLE_MAX_PX
  return (
    VENUE_PIN_BASE_RADIUS_PX + Math.min(variable, VENUE_PIN_VARIABLE_MAX_PX)
  )
}

/** The minimum a pin has to carry for the layer below to draw it. */
export interface VenuePinPlacement {
  id: number
  lng: number
  lat: number
  upcomingShowCount: number
}

/**
 * Every property a pin surface may read, written once for all of them.
 *
 * Built here rather than at each call site so a property and the expressions
 * that consume it cannot drift. Not all of them are read everywhere: the base
 * paint below reads `radiusPx`, `color` and `isSelected`, while `isQuiet` is
 * read only by a caller that mutes quiet rooms. A surface that ignores one
 * pays a property it does not draw, which is cheaper than two builders.
 */
export function venuePinFeatures(
  pins: readonly VenuePinPlacement[],
  selectedVenueId: number | null = null,
): GeoJSON.FeatureCollection {
  return {
    type: 'FeatureCollection',
    features: pins.map((pin) => ({
      type: 'Feature',
      properties: {
        id: pin.id,
        color:
          pin.id === selectedVenueId ? DOT_COLOR_SELECTED : DOT_COLOR_BASE,
        radiusPx: venuePinRadiusPx(pin.upcomingShowCount),
        isSelected: pin.id === selectedVenueId,
        // Nothing booked. Read by the surfaces that mute a quiet room; drawing
        // it is the caller's call, deriving it is not.
        isQuiet: pin.upcomingShowCount === 0,
      },
      geometry: { type: 'Point', coordinates: [pin.lng, pin.lat] },
    })),
  }
}

// A dark rim, not the globe dots' cream one: on a street basemap a light halo
// reads as a second mark rather than an outline.
export const VENUE_PIN_STROKE = 'rgba(23,16,11,0.85)'

/**
 * Paint for a venue-pin circle layer, driven by the properties
 * `venuePinFeatures` above writes and the `hover` feature-state the caller
 * sets.
 *
 * A FUNCTION, not a shared object literal: MapLibre keeps a reference to the
 * style it is handed, and two maps sharing one mutable paint object would let
 * either one's internal edits reach the other.
 */
export function venuePinPaint(): CircleLayerSpecification['paint'] {
  return {
    'circle-radius': [
      '*',
      ['get', 'radiusPx'],
      [
        'case',
        ['boolean', ['feature-state', 'hover'], false],
        DOT_HOVER_RADIUS_SCALE,
        1,
      ],
    ],
    'circle-color': [
      'case',
      [
        'all',
        ['boolean', ['feature-state', 'hover'], false],
        ['!', ['get', 'isSelected']],
      ],
      DOT_COLOR_HOVERED,
      ['get', 'color'],
    ],
    'circle-stroke-width': 1.5,
    'circle-stroke-color': VENUE_PIN_STROKE,
  }
}
