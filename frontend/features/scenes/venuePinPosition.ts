/**
 * Where a venue pins on a map, and the privacy rule that decides it.
 *
 * A leaf module with no runtime dependencies, so every map surface can bind
 * the rule without pulling the Atlas city-view logic (and its formatters and
 * genre tables) in with it. Callers import it from here; it is the only path.
 */

import type { VenueWithShowCount } from '@/features/venues/types'

/** Which coordinate source a venue's pin came from. */
export type VenuePinPrecision = 'street' | 'centroid'

export interface VenuePinPosition {
  lng: number
  lat: number
  precision: VenuePinPrecision
}

/**
 * A venue's map position, and how precise it is.
 *
 * PRIVACY GATE (a locked user decision): street coordinates exist only for
 * verified venues whose geocode still matches their current address. The API
 * enforces that (it omits street_latitude/street_longitude for everyone else)
 * and this function's ONLY job is to honor the omission by falling back to
 * the venue's city centroid. It must never reconstruct a street position from
 * any other field (address, zipcode), because that would street-map the DIY
 * and house venues the gate exists to protect.
 *
 * Returns null when the venue has no usable coordinates at all; such a venue
 * still lists, it just doesn't pin.
 */
export function venuePinPosition(
  venue: Pick<
    VenueWithShowCount,
    'latitude' | 'longitude' | 'street_latitude' | 'street_longitude'
  >,
): VenuePinPosition | null {
  if (
    Number.isFinite(venue.street_latitude) &&
    Number.isFinite(venue.street_longitude)
  ) {
    return {
      lat: venue.street_latitude as number,
      lng: venue.street_longitude as number,
      precision: 'street',
    }
  }
  if (Number.isFinite(venue.latitude) && Number.isFinite(venue.longitude)) {
    return {
      lat: venue.latitude as number,
      lng: venue.longitude as number,
      precision: 'centroid',
    }
  }
  return null
}
