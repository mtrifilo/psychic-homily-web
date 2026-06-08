import { describe, it, expect } from 'vitest'
import { haversineDistanceKm } from './haversine'

// Reference coordinates (decimal degrees), matching the offline-geocoder
// centroids the backend surfaces for these cities (PSY-981).
const PHOENIX = { lat: 33.4484, lng: -112.074 }
const TUCSON = { lat: 32.2217, lng: -110.9265 }
const PARADISE_VALLEY = { lat: 33.5312, lng: -111.9426 } // Phoenix suburb

describe('haversineDistanceKm', () => {
  it('is 0 for identical points (no NaN from float noise)', () => {
    expect(
      haversineDistanceKm(PHOENIX.lat, PHOENIX.lng, PHOENIX.lat, PHOENIX.lng),
    ).toBe(0)
  })

  it('matches the known Phoenix↔Tucson distance (~173 km) within 2 km', () => {
    const d = haversineDistanceKm(
      PHOENIX.lat,
      PHOENIX.lng,
      TUCSON.lat,
      TUCSON.lng,
    )
    // Great-circle distance between these two centroids is ~173.5 km
    // (road distance via I-10 is ~180 km; the straight-line arc is shorter).
    expect(d).toBeGreaterThan(171)
    expect(d).toBeLessThan(176)
  })

  it('is symmetric (a→b equals b→a)', () => {
    const ab = haversineDistanceKm(
      PHOENIX.lat,
      PHOENIX.lng,
      TUCSON.lat,
      TUCSON.lng,
    )
    const ba = haversineDistanceKm(
      TUCSON.lat,
      TUCSON.lng,
      PHOENIX.lat,
      PHOENIX.lng,
    )
    expect(ab).toBeCloseTo(ba, 9)
  })

  it('ranks Paradise Valley as closer to Phoenix than to Tucson', () => {
    const toPhoenix = haversineDistanceKm(
      PARADISE_VALLEY.lat,
      PARADISE_VALLEY.lng,
      PHOENIX.lat,
      PHOENIX.lng,
    )
    const toTucson = haversineDistanceKm(
      PARADISE_VALLEY.lat,
      PARADISE_VALLEY.lng,
      TUCSON.lat,
      TUCSON.lng,
    )
    expect(toPhoenix).toBeLessThan(toTucson)
    // Paradise Valley is a Phoenix suburb (~15 km away), nowhere near Tucson.
    expect(toPhoenix).toBeLessThan(20)
  })

  it('handles the antimeridian: -179.9° and +179.9° are ~22 km apart, not half the globe', () => {
    // Same latitude, longitudes straddling ±180°. Naive degree subtraction
    // would give ~359.8° (≈ half the Earth); haversine gives the true short arc.
    const d = haversineDistanceKm(0, -179.9, 0, 179.9)
    expect(d).toBeLessThan(30)
    // The long way round at the equator would be ~40,000 km — assert we did NOT
    // take it.
    expect(d).toBeLessThan(100)
  })

  it('computes the equatorial quarter-arc (~10,000 km) for a 90° longitude gap', () => {
    const d = haversineDistanceKm(0, 0, 0, 90)
    // A quarter of the Earth's ~40,030 km circumference.
    expect(d).toBeGreaterThan(9900)
    expect(d).toBeLessThan(10100)
  })

  it('computes pole-to-pole (~20,000 km) for antipodal latitudes', () => {
    const d = haversineDistanceKm(-90, 0, 90, 0)
    // Half the meridian circumference.
    expect(d).toBeGreaterThan(19900)
    expect(d).toBeLessThan(20100)
  })
})
