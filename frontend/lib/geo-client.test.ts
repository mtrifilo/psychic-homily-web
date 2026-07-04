import { describe, it, expect } from 'vitest'
import { toGeoLocation, matchByGeo } from './geo-client'

describe('toGeoLocation', () => {
  it('keeps a valid {city,state} with no coords', () => {
    expect(toGeoLocation({ city: 'Phoenix', state: 'AZ' })).toEqual({
      city: 'Phoenix',
      state: 'AZ',
    })
  })

  it('attaches coords when both are finite and in range (incl. the ±90/±180 boundary)', () => {
    expect(
      toGeoLocation({ city: 'X', state: 'Y', latitude: 90, longitude: -180 }),
    ).toEqual({ city: 'X', state: 'Y', latitude: 90, longitude: -180 })
  })

  it('drops out-of-range coords but keeps the {city,state} (PSY-1346 hardening)', () => {
    // Matches the server-side parseCoordinate bound so the cache-restore path
    // is as strict as the header-decode path.
    expect(
      toGeoLocation({ city: 'X', state: 'Y', latitude: 999, longitude: 10 }),
    ).toEqual({ city: 'X', state: 'Y' })
    expect(
      toGeoLocation({ city: 'X', state: 'Y', latitude: 10, longitude: -200 }),
    ).toEqual({ city: 'X', state: 'Y' })
  })

  it('drops a half-coordinate pair (lat only) and non-finite coords', () => {
    expect(
      toGeoLocation({ city: 'X', state: 'Y', latitude: 33.4 }),
    ).toEqual({ city: 'X', state: 'Y' })
    expect(
      toGeoLocation({ city: 'X', state: 'Y', latitude: NaN, longitude: 10 }),
    ).toEqual({ city: 'X', state: 'Y' })
  })

  it('rejects malformed values (non-object, empty/non-string halves)', () => {
    expect(toGeoLocation(null)).toBeNull()
    expect(toGeoLocation('nope')).toBeNull()
    expect(toGeoLocation({ city: '', state: 'AZ' })).toBeNull()
    expect(toGeoLocation({ city: 'Phoenix' })).toBeNull()
    expect(toGeoLocation({ city: 'Phoenix', state: 42 })).toBeNull()
  })
})

describe('matchByGeo', () => {
  interface Row {
    city: string
    state: string
    lat: number | null
    lng: number | null
  }
  const get = {
    city: (r: Row) => r.city,
    state: (r: Row) => r.state,
    lat: (r: Row) => r.lat,
    lng: (r: Row) => r.lng,
  }
  const rows: Row[] = [
    { city: 'Chicago', state: 'IL', lat: 41.88, lng: -87.63 },
    { city: 'Phoenix', state: 'AZ', lat: 33.45, lng: -112.07 },
  ]

  it('returns the exact city/state match (case/whitespace-insensitive)', () => {
    expect(matchByGeo(rows, { city: ' phoenix ', state: 'az' }, get)?.city).toBe(
      'Phoenix',
    )
  })

  it('falls back to the nearest by haversine when no exact match', () => {
    expect(
      matchByGeo(
        rows,
        { city: 'Tucson', state: 'AZ', latitude: 32.22, longitude: -110.97 },
        get,
      )?.city,
    ).toBe('Phoenix')
  })

  it('returns null on an exact miss with no coords', () => {
    expect(matchByGeo(rows, { city: 'Nowhere', state: 'XX' }, get)).toBeNull()
  })

  it('skips items the geocoder could not place when computing the nearest', () => {
    const withUnplaced: Row[] = [
      { city: 'Chicago', state: 'IL', lat: null, lng: null },
      { city: 'Phoenix', state: 'AZ', lat: 33.45, lng: -112.07 },
    ]
    expect(
      matchByGeo(
        withUnplaced,
        { city: 'Evanston', state: 'IL', latitude: 42.05, longitude: -87.69 },
        get,
      )?.city,
    ).toBe('Phoenix')
  })
})
