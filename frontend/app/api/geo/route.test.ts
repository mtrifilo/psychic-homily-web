import { describe, it, expect } from 'vitest'
import { NextRequest } from 'next/server'
import { GET } from './route'

/**
 * Build a NextRequest carrying the given (lowercase) Vercel geo headers.
 * Header names are normalized by the Headers object, so case is irrelevant.
 */
function geoRequest(headers: Record<string, string>): NextRequest {
  return new NextRequest('https://psychichomily.com/api/geo', { headers })
}

describe('GET /api/geo', () => {
  it('decodes US geo headers into { geo: { city, state } }', async () => {
    const res = GET(
      geoRequest({
        'x-vercel-ip-city': 'Phoenix',
        'x-vercel-ip-country-region': 'AZ',
        'x-vercel-ip-country': 'US',
      }),
    )
    await expect(res.json()).resolves.toEqual({
      geo: { city: 'Phoenix', state: 'AZ' },
    })
  })

  it('URL-decodes the city header (e.g. San%20Diego)', async () => {
    const res = GET(
      geoRequest({
        'x-vercel-ip-city': 'San%20Diego',
        'x-vercel-ip-country-region': 'CA',
        'x-vercel-ip-country': 'US',
      }),
    )
    await expect(res.json()).resolves.toEqual({
      geo: { city: 'San Diego', state: 'CA' },
    })
  })

  it('returns { geo: null } for a non-US/CA country', async () => {
    const res = GET(
      geoRequest({
        'x-vercel-ip-city': 'S%C3%A3o%20Paulo',
        'x-vercel-ip-country-region': 'SP',
        'x-vercel-ip-country': 'BR',
      }),
    )
    await expect(res.json()).resolves.toEqual({ geo: null })
  })

  it('returns { geo: null } when no geo headers are present (local dev / VPN)', async () => {
    const res = GET(geoRequest({}))
    await expect(res.json()).resolves.toEqual({ geo: null })
  })

  it('returns { geo: null } when the region is missing (bare city is ambiguous)', async () => {
    const res = GET(
      geoRequest({
        'x-vercel-ip-city': 'Phoenix',
        'x-vercel-ip-country': 'US',
      }),
    )
    await expect(res.json()).resolves.toEqual({ geo: null })
  })

  it('allows Canada (province codes can match PH data)', async () => {
    const res = GET(
      geoRequest({
        'x-vercel-ip-city': 'Toronto',
        'x-vercel-ip-country-region': 'ON',
        'x-vercel-ip-country': 'CA',
      }),
    )
    await expect(res.json()).resolves.toEqual({
      geo: { city: 'Toronto', state: 'ON' },
    })
  })

  it('surfaces the visitor lat/long when the coordinate headers are present (PSY-981)', async () => {
    const res = GET(
      geoRequest({
        'x-vercel-ip-city': 'Paradise Valley',
        'x-vercel-ip-country-region': 'AZ',
        'x-vercel-ip-country': 'US',
        'x-vercel-ip-latitude': '33.5312',
        'x-vercel-ip-longitude': '-111.9426',
      }),
    )
    await expect(res.json()).resolves.toEqual({
      geo: {
        city: 'Paradise Valley',
        state: 'AZ',
        latitude: 33.5312,
        longitude: -111.9426,
      },
    })
  })

  it('omits coords when the lat/long headers are absent (pre-PSY-981 shape)', async () => {
    const res = GET(
      geoRequest({
        'x-vercel-ip-city': 'Phoenix',
        'x-vercel-ip-country-region': 'AZ',
        'x-vercel-ip-country': 'US',
      }),
    )
    await expect(res.json()).resolves.toEqual({
      geo: { city: 'Phoenix', state: 'AZ' },
    })
  })

  it('sets a non-shared-cacheable Cache-Control header (no cross-visitor poisoning)', () => {
    const res = GET(
      geoRequest({
        'x-vercel-ip-city': 'Phoenix',
        'x-vercel-ip-country-region': 'AZ',
        'x-vercel-ip-country': 'US',
      }),
    )
    const cacheControl = res.headers.get('Cache-Control') ?? ''
    // `private` forbids shared caches (CDN/proxy); `no-store` forbids storing
    // at all. Either alone would prevent cross-visitor leakage; we assert both
    // so a future edit can't quietly weaken the contract to `public`.
    expect(cacheControl).toContain('private')
    expect(cacheControl).toContain('no-store')
    expect(cacheControl).not.toContain('public')
  })
})
