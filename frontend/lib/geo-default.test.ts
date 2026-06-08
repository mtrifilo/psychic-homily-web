import { describe, it, expect, vi, beforeEach } from 'vitest'
import { headers } from 'next/headers'
import { getGeoDefaultCity } from './geo-default'

// Mock next/headers; each test resolves a header store with the given values.
vi.mock('next/headers', () => ({
  headers: vi.fn(),
}))

const mockHeaders = vi.mocked(headers)

/** Resolve headers() to a store backed by the given (lowercased) map. */
function setHeaders(map: Record<string, string>) {
  const store = {
    get: (key: string) => map[key.toLowerCase()] ?? null,
  }
  mockHeaders.mockResolvedValue(
    store as unknown as Awaited<ReturnType<typeof headers>>,
  )
}

describe('getGeoDefaultCity', () => {
  beforeEach(() => {
    mockHeaders.mockReset()
  })

  it('returns {city, state} from US geo headers', async () => {
    setHeaders({
      'x-vercel-ip-city': 'Phoenix',
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
    })
    await expect(getGeoDefaultCity()).resolves.toEqual({
      city: 'Phoenix',
      state: 'AZ',
    })
  })

  it('URL-decodes the city header (e.g. São Paulo) but only for US/CA', async () => {
    // A US city with an encoded space exercises the decode path.
    setHeaders({
      'x-vercel-ip-city': 'San%20Diego',
      'x-vercel-ip-country-region': 'CA',
      'x-vercel-ip-country': 'US',
    })
    await expect(getGeoDefaultCity()).resolves.toEqual({
      city: 'San Diego',
      state: 'CA',
    })
  })

  it('allows Canada (province codes can match PH data)', async () => {
    setHeaders({
      'x-vercel-ip-city': 'Toronto',
      'x-vercel-ip-country-region': 'ON',
      'x-vercel-ip-country': 'CA',
    })
    await expect(getGeoDefaultCity()).resolves.toEqual({
      city: 'Toronto',
      state: 'ON',
    })
  })

  it('returns null for a non-US/CA country (foreign region cannot match)', async () => {
    setHeaders({
      'x-vercel-ip-city': 'S%C3%A3o%20Paulo',
      'x-vercel-ip-country-region': 'SP',
      'x-vercel-ip-country': 'BR',
    })
    await expect(getGeoDefaultCity()).resolves.toBeNull()
  })

  it('returns null when the city header is missing', async () => {
    setHeaders({
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
    })
    await expect(getGeoDefaultCity()).resolves.toBeNull()
  })

  it('returns null when the city header is empty (truthy-empty trap)', async () => {
    setHeaders({
      'x-vercel-ip-city': '',
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
    })
    await expect(getGeoDefaultCity()).resolves.toBeNull()
  })

  it('returns null when the region header is missing (bare city is ambiguous)', async () => {
    setHeaders({
      'x-vercel-ip-city': 'Phoenix',
      'x-vercel-ip-country': 'US',
    })
    await expect(getGeoDefaultCity()).resolves.toBeNull()
  })

  it('returns null when no geo headers are present at all (local dev / VPN)', async () => {
    setHeaders({})
    await expect(getGeoDefaultCity()).resolves.toBeNull()
  })

  it('allows an absent country header through (client has-shows check is the gate)', async () => {
    setHeaders({
      'x-vercel-ip-city': 'Omaha',
      'x-vercel-ip-country-region': 'NE',
    })
    await expect(getGeoDefaultCity()).resolves.toEqual({
      city: 'Omaha',
      state: 'NE',
    })
  })

  it('returns null on a malformed percent-encoding rather than throwing', async () => {
    setHeaders({
      'x-vercel-ip-city': '%E0%A4%A', // truncated → decodeURIComponent throws
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
    })
    await expect(getGeoDefaultCity()).resolves.toBeNull()
  })

  it('trims surrounding whitespace from decoded values', async () => {
    setHeaders({
      'x-vercel-ip-city': '%20Phoenix%20',
      'x-vercel-ip-country-region': '%20AZ%20',
      'x-vercel-ip-country': 'US',
    })
    await expect(getGeoDefaultCity()).resolves.toEqual({
      city: 'Phoenix',
      state: 'AZ',
    })
  })

  // --- PSY-981: visitor lat/long for the nearest-has-shows-city fallback ---

  it('attaches the visitor lat/long when both coordinate headers are present', async () => {
    setHeaders({
      'x-vercel-ip-city': 'Paradise Valley',
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
      'x-vercel-ip-latitude': '33.5312',
      'x-vercel-ip-longitude': '-111.9426',
    })
    await expect(getGeoDefaultCity()).resolves.toEqual({
      city: 'Paradise Valley',
      state: 'AZ',
      latitude: 33.5312,
      longitude: -111.9426,
    })
  })

  it('omits coords (city/state still resolves) when the lat/long headers are absent', async () => {
    // The pre-PSY-981 shape — degrades to exact city-name matching downstream.
    setHeaders({
      'x-vercel-ip-city': 'Phoenix',
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
    })
    const result = await getGeoDefaultCity()
    expect(result).toEqual({ city: 'Phoenix', state: 'AZ' })
    expect(result).not.toHaveProperty('latitude')
    expect(result).not.toHaveProperty('longitude')
  })

  it('drops coords when only ONE of lat/long is present (no half-coordinate)', async () => {
    setHeaders({
      'x-vercel-ip-city': 'Phoenix',
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
      'x-vercel-ip-latitude': '33.4484',
      // longitude omitted
    })
    await expect(getGeoDefaultCity()).resolves.toEqual({
      city: 'Phoenix',
      state: 'AZ',
    })
  })

  it('drops coords when a value is non-numeric garbage', async () => {
    setHeaders({
      'x-vercel-ip-city': 'Phoenix',
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
      'x-vercel-ip-latitude': 'not-a-number',
      'x-vercel-ip-longitude': '-111.9426',
    })
    await expect(getGeoDefaultCity()).resolves.toEqual({
      city: 'Phoenix',
      state: 'AZ',
    })
  })

  it('drops coords when out of range (latitude > 90 / longitude > 180)', async () => {
    setHeaders({
      'x-vercel-ip-city': 'Phoenix',
      'x-vercel-ip-country-region': 'AZ',
      'x-vercel-ip-country': 'US',
      'x-vercel-ip-latitude': '200',
      'x-vercel-ip-longitude': '-111.9426',
    })
    await expect(getGeoDefaultCity()).resolves.toEqual({
      city: 'Phoenix',
      state: 'AZ',
    })
  })

  it('accepts coordinates at the valid bounds and zero', async () => {
    setHeaders({
      'x-vercel-ip-city': 'Null Island',
      'x-vercel-ip-country-region': 'TX',
      'x-vercel-ip-country': 'US',
      'x-vercel-ip-latitude': '0',
      'x-vercel-ip-longitude': '180',
    })
    await expect(getGeoDefaultCity()).resolves.toEqual({
      city: 'Null Island',
      state: 'TX',
      latitude: 0,
      longitude: 180,
    })
  })

  it('still returns null for a non-US/CA country even with coords present', async () => {
    // The country gate is UNCHANGED by PSY-981 — coords don't override it.
    setHeaders({
      'x-vercel-ip-city': 'Paris',
      'x-vercel-ip-country-region': 'IDF',
      'x-vercel-ip-country': 'FR',
      'x-vercel-ip-latitude': '48.8566',
      'x-vercel-ip-longitude': '2.3522',
    })
    await expect(getGeoDefaultCity()).resolves.toBeNull()
  })
})
