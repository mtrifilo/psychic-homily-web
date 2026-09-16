import { describe, it, expect } from 'vitest'
import {
  miniAtlasBounds,
  miniAtlasPins,
  miniAtlasSummary,
} from './venueMiniAtlas'
import type { VenueWithShowCount } from './types'

function makeVenue(
  overrides: Partial<VenueWithShowCount> = {}
): VenueWithShowCount {
  return {
    id: 1,
    slug: 'a-room',
    name: 'A Room',
    address: '1 Main St',
    city: 'Phoenix',
    state: 'AZ',
    verified: true,
    upcoming_show_count: 3,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('miniAtlasPins', () => {
  it('pins a verified room at the street coordinates the API served', () => {
    const pins = miniAtlasPins([
      makeVenue({
        latitude: 33.4,
        longitude: -112.0,
        street_latitude: 33.45,
        street_longitude: -112.07,
      }),
    ])

    expect(pins).toEqual([
      { id: 1, lat: 33.45, lng: -112.07, upcomingShowCount: 3 },
    ])
  })

  it('falls back to the city centroid when no street coordinates were served', () => {
    // The API omits street coordinates for rooms the privacy gate protects.
    // The pane must never reconstruct a position from the address.
    const pins = miniAtlasPins([
      makeVenue({ latitude: 33.4, longitude: -112.0 }),
    ])

    expect(pins[0]).toMatchObject({ lat: 33.4, lng: -112.0 })
  })

  it('draws street and centroid rooms as the same kind of pin', () => {
    const [street, centroid] = miniAtlasPins([
      makeVenue({
        id: 1,
        latitude: 33.4,
        longitude: -112.0,
        street_latitude: 33.45,
        street_longitude: -112.07,
      }),
      makeVenue({ id: 2, latitude: 33.4, longitude: -112.0 }),
    ])

    expect(Object.keys(street).sort()).toEqual(Object.keys(centroid).sort())
  })

  it('drops a room with no coordinates at all and keeps the rest in row order', () => {
    const pins = miniAtlasPins([
      makeVenue({ id: 1, latitude: 33.4, longitude: -112.0 }),
      makeVenue({ id: 2 }),
      makeVenue({ id: 3, latitude: 33.5, longitude: -112.1 }),
    ])

    expect(pins.map(p => p.id)).toEqual([1, 3])
  })

  it('carries the upcoming count so a quiet room can be told from a busy one', () => {
    const pins = miniAtlasPins([
      makeVenue({ id: 1, upcoming_show_count: 0, latitude: 1, longitude: 2 }),
    ])

    expect(pins[0].upcomingShowCount).toBe(0)
  })
})

describe('miniAtlasBounds', () => {
  it('holds every pin', () => {
    const pins = miniAtlasPins([
      makeVenue({ id: 1, latitude: 33.4, longitude: -112.0 }),
      makeVenue({ id: 2, latitude: 33.6, longitude: -111.8 }),
      makeVenue({ id: 3, latitude: 33.2, longitude: -112.3 }),
    ])

    expect(miniAtlasBounds(pins)).toEqual([
      [-112.3, 33.2],
      [-111.8, 33.6],
    ])
  })

  it('is null with nothing to hold', () => {
    expect(miniAtlasBounds([])).toBeNull()
  })

  it('is a point for a single room', () => {
    const pins = miniAtlasPins([
      makeVenue({ latitude: 33.4, longitude: -112.0 }),
    ])

    expect(miniAtlasBounds(pins)).toEqual([
      [-112.0, 33.4],
      [-112.0, 33.4],
    ])
  })
})

describe('miniAtlasSummary', () => {
  it('says how many rooms are on the map and where they are', () => {
    expect(miniAtlasSummary(8, 8, 'Phoenix, AZ')).toBe(
      'Map of 8 rooms in Phoenix, AZ. Every room on it is a row in the table beside it.'
    )
  })

  it('says so when the map is short of the list', () => {
    expect(miniAtlasSummary(6, 8, 'Phoenix, AZ')).toContain(
      '6 rooms of the 8 listed'
    )
  })

  it('counts one room in the singular', () => {
    expect(miniAtlasSummary(1, 1, 'Mesa, AZ')).toContain('Map of 1 room in')
  })
})
