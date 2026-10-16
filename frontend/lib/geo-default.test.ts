import { describe, it, expect } from 'vitest'
import { decodeGeoHeaders } from './geo-default'

/** Decode a header store backed by the given (lowercased) map. */
function decodeFrom(map: Record<string, string>) {
  return decodeGeoHeaders({
    get: (key: string) => map[key.toLowerCase()] ?? null,
  })
}

describe('decodeGeoHeaders', () => {
  it('returns {city, state} from US geo headers', () => {
    const result = decodeFrom({
      'x-vercel-ip-city': 'Phoenix',
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
    })
    expect(result).toEqual({
      city: 'Phoenix',
      state: 'AZ',
    })
  })

  it('URL-decodes the city header (e.g. São Paulo) but only for US/CA', () => {
    // A US city with an encoded space exercises the decode path.
    const result = decodeFrom({
      'x-vercel-ip-city': 'San%20Diego',
      'x-vercel-ip-country-region': 'CA',
      'x-vercel-ip-country': 'US',
    })
    expect(result).toEqual({
      city: 'San Diego',
      state: 'CA',
    })
  })

  it('allows Canada (province codes can match PH data)', () => {
    const result = decodeFrom({
      'x-vercel-ip-city': 'Toronto',
      'x-vercel-ip-country-region': 'ON',
      'x-vercel-ip-country': 'CA',
    })
    expect(result).toEqual({
      city: 'Toronto',
      state: 'ON',
    })
  })

  it('returns null for a non-US/CA country (foreign region cannot match)', () => {
    const result = decodeFrom({
      'x-vercel-ip-city': 'S%C3%A3o%20Paulo',
      'x-vercel-ip-country-region': 'SP',
      'x-vercel-ip-country': 'BR',
    })
    expect(result).toBeNull()
  })

  it('returns null when the city header is missing', () => {
    const result = decodeFrom({
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
    })
    expect(result).toBeNull()
  })

  it('returns null when the city header is empty (truthy-empty trap)', () => {
    const result = decodeFrom({
      'x-vercel-ip-city': '',
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
    })
    expect(result).toBeNull()
  })

  it('returns null when the region header is missing (bare city is ambiguous)', () => {
    const result = decodeFrom({
      'x-vercel-ip-city': 'Phoenix',
      'x-vercel-ip-country': 'US',
    })
    expect(result).toBeNull()
  })

  it('returns null when no geo headers are present at all (local dev / VPN)', () => {
    const result = decodeFrom({})
    expect(result).toBeNull()
  })

  it('allows an absent country header through (client has-shows check is the gate)', () => {
    const result = decodeFrom({
      'x-vercel-ip-city': 'Omaha',
      'x-vercel-ip-country-region': 'NE',
    })
    expect(result).toEqual({
      city: 'Omaha',
      state: 'NE',
    })
  })

  it('returns null on a malformed percent-encoding rather than throwing', () => {
    const result = decodeFrom({
      'x-vercel-ip-city': '%E0%A4%A', // truncated → decodeURIComponent throws
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
    })
    expect(result).toBeNull()
  })

  it('trims surrounding whitespace from decoded values', () => {
    const result = decodeFrom({
      'x-vercel-ip-city': '%20Phoenix%20',
      'x-vercel-ip-country-region': '%20AZ%20',
      'x-vercel-ip-country': 'US',
    })
    expect(result).toEqual({
      city: 'Phoenix',
      state: 'AZ',
    })
  })

  // --- PSY-981: visitor lat/long for the nearest-has-shows-city fallback ---

  it('attaches the visitor lat/long when both coordinate headers are present', () => {
    const result = decodeFrom({
      'x-vercel-ip-city': 'Paradise Valley',
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
      'x-vercel-ip-latitude': '33.5312',
      'x-vercel-ip-longitude': '-111.9426',
    })
    expect(result).toEqual({
      city: 'Paradise Valley',
      state: 'AZ',
      latitude: 33.5312,
      longitude: -111.9426,
    })
  })

  it('omits coords (city/state still resolves) when the lat/long headers are absent', () => {
    // The pre-PSY-981 shape — degrades to exact city-name matching downstream.
    const result = decodeFrom({
      'x-vercel-ip-city': 'Phoenix',
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
    })
    expect(result).toEqual({ city: 'Phoenix', state: 'AZ' })
    expect(result).not.toHaveProperty('latitude')
    expect(result).not.toHaveProperty('longitude')
  })

  it('drops coords when only ONE of lat/long is present (no half-coordinate)', () => {
    const result = decodeFrom({
      'x-vercel-ip-city': 'Phoenix',
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
      'x-vercel-ip-latitude': '33.4484',
      // longitude omitted
    })
    expect(result).toEqual({
      city: 'Phoenix',
      state: 'AZ',
    })
  })

  it('drops coords when a value is non-numeric garbage', () => {
    const result = decodeFrom({
      'x-vercel-ip-city': 'Phoenix',
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
      'x-vercel-ip-latitude': 'not-a-number',
      'x-vercel-ip-longitude': '-111.9426',
    })
    expect(result).toEqual({
      city: 'Phoenix',
      state: 'AZ',
    })
  })

  it('drops coords when out of range (latitude > 90 / longitude > 180)', () => {
    const result = decodeFrom({
      'x-vercel-ip-city': 'Phoenix',
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
      'x-vercel-ip-latitude': '200',
      'x-vercel-ip-longitude': '-111.9426',
    })
    expect(result).toEqual({
      city: 'Phoenix',
      state: 'AZ',
    })
  })

  it('accepts coordinates at the valid bounds and zero', () => {
    const result = decodeFrom({
      'x-vercel-ip-city': 'Null Island',
      'x-vercel-ip-country-region': 'TX',
      'x-vercel-ip-country': 'US',
      'x-vercel-ip-latitude': '0',
      'x-vercel-ip-longitude': '180',
    })
    expect(result).toEqual({
      city: 'Null Island',
      state: 'TX',
      latitude: 0,
      longitude: 180,
    })
  })

  it('still returns null for a non-US/CA country even with coords present', () => {
    // The country gate is UNCHANGED by PSY-981 — coords don't override it.
    const result = decodeFrom({
      'x-vercel-ip-city': 'Paris',
      'x-vercel-ip-country-region': 'IDF',
      'x-vercel-ip-country': 'FR',
      'x-vercel-ip-latitude': '48.8566',
      'x-vercel-ip-longitude': '2.3522',
    })
    expect(result).toBeNull()
  })
})
