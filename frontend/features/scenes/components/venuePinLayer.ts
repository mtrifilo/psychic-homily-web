/**
 * The venue-pin circle layer's paint, shared by every map surface that draws
 * rooms as pins.
 *
 * One definition so the Atlas city view and the `/venues` mini Atlas cannot
 * drift into two affordance ramps for the same mark. The layer's placement,
 * `minzoom` and source id belong to the caller; only the paint is shared.
 */

import type { CircleLayerSpecification } from 'maplibre-gl'
import { DOT_COLOR_HOVERED, DOT_HOVER_RADIUS_SCALE } from './globeScale'

// A dark rim, not the globe dots' cream one: on a street basemap a light halo
// reads as a second mark rather than an outline.
export const VENUE_PIN_STROKE = 'rgba(23,16,11,0.85)'

/**
 * Paint for a venue-pin circle layer, driven by two feature properties the
 * source must carry (`radiusPx`, `color`, `isSelected`) and the `hover`
 * feature-state the caller sets.
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
