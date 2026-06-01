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
})
