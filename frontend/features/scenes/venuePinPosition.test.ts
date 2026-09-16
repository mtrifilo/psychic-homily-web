import { describe, it, expect } from 'vitest'
import type { VenueWithShowCount } from '@/features/venues/types'
import { venuePinPosition } from './venuePinPosition'

function venue(overrides: Partial<VenueWithShowCount> = {}): VenueWithShowCount {
  return {
    id: 1,
    slug: 'mohawk-austin-tx',
    name: 'Mohawk',
    address: null,
    city: 'Austin',
    state: 'TX',
    verified: true,
    latitude: 30.2672,
    longitude: -97.7431,
    upcoming_show_count: 14,
    shows_this_week: 3,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-07-25T00:00:00Z',
    ...overrides,
  } as VenueWithShowCount
}

describe('venuePinPosition (PSY-1536 privacy gate)', () => {
  it('uses street coordinates when the API served them', () => {
    expect(
      venuePinPosition(
        venue({ street_latitude: 30.2686, street_longitude: -97.7376 }),
      ),
    ).toEqual({ lat: 30.2686, lng: -97.7376, precision: 'street' })
  })

  it('falls back to the city centroid when street coords are ABSENT', () => {
    // The API omits street coords for unverified venues and stale geocodes.
    // The pin must land on the centroid, never be reconstructed some other way.
    expect(venuePinPosition(venue())).toEqual({
      lat: 30.2672,
      lng: -97.7431,
      precision: 'centroid',
    })
  })

  it('falls back to the centroid when street coords are explicitly null', () => {
    expect(
      venuePinPosition(
        venue({ street_latitude: null, street_longitude: null }),
      ),
    ).toEqual({ lat: 30.2672, lng: -97.7431, precision: 'centroid' })
  })

  it('does not pin on a half-present street geocode', () => {
    expect(
      venuePinPosition(
        venue({ street_latitude: 30.2686, street_longitude: null }),
      )?.precision,
    ).toBe('centroid')
  })

  it('returns null when the venue has no coordinates at all', () => {
    expect(
      venuePinPosition(venue({ latitude: null, longitude: null })),
    ).toBeNull()
  })
})
