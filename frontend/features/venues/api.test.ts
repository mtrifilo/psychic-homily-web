import { describe, it, expect } from 'vitest'
import { API_BASE_URL } from '@/lib/api-base'
import { venueEndpoints, venueQueryKeys } from './api'

describe('venueEndpoints', () => {
  it('exposes static collection endpoints rooted at the API base URL', () => {
    expect(venueEndpoints.LIST).toBe(`${API_BASE_URL}/venues`)
    expect(venueEndpoints.CITIES).toBe(`${API_BASE_URL}/venues/cities`)
    expect(venueEndpoints.SEARCH).toBe(`${API_BASE_URL}/venues/search`)
  })

  it('builds a detail endpoint from a numeric id or a slug', () => {
    expect(venueEndpoints.GET(42)).toBe(`${API_BASE_URL}/venues/42`)
    expect(venueEndpoints.GET('the-rebel-lounge')).toBe(
      `${API_BASE_URL}/venues/the-rebel-lounge`
    )
  })

  it('builds nested relation endpoints from an id or slug', () => {
    expect(venueEndpoints.SHOWS('the-rebel-lounge')).toBe(
      `${API_BASE_URL}/venues/the-rebel-lounge/shows`
    )
    expect(venueEndpoints.GENRES('the-rebel-lounge')).toBe(
      `${API_BASE_URL}/venues/the-rebel-lounge/genres`
    )
    expect(venueEndpoints.BILL_NETWORK('the-rebel-lounge')).toBe(
      `${API_BASE_URL}/venues/the-rebel-lounge/bill-network`
    )
  })

  it('builds mutation endpoints from an id or slug', () => {
    expect(venueEndpoints.UPDATE(42)).toBe(`${API_BASE_URL}/venues/42`)
    expect(venueEndpoints.DELETE(42)).toBe(`${API_BASE_URL}/venues/42`)
  })
})

describe('venueQueryKeys', () => {
  it('uses a stable root key for cache invalidation', () => {
    expect(venueQueryKeys.all).toEqual(['venues'])
  })

  it('namespaces list keys with the filters object', () => {
    expect(venueQueryKeys.list()).toEqual(['venues', 'list', undefined])
    expect(venueQueryKeys.list({ city: 'Phoenix' })).toEqual([
      'venues',
      'list',
      { city: 'Phoenix' },
    ])
  })

  it('exposes a static cities key', () => {
    expect(venueQueryKeys.cities).toEqual(['venues', 'cities'])
  })

  it('lower-cases the query in search keys so case variants share a cache entry', () => {
    expect(venueQueryKeys.search('The Rebel Lounge')).toEqual([
      'venues',
      'search',
      'the rebel lounge',
    ])
  })

  it('stringifies ids in detail / shows / genres keys for stable equality', () => {
    expect(venueQueryKeys.detail(42)).toEqual(['venues', 'detail', '42'])
    expect(venueQueryKeys.detail('the-rebel-lounge')).toEqual([
      'venues',
      'detail',
      'the-rebel-lounge',
    ])
    expect(venueQueryKeys.shows(42)).toEqual(['venues', 'shows', '42'])
    expect(venueQueryKeys.genres(42)).toEqual(['venues', 'genres', '42'])
  })

  it('keys a shows PAGE on every parameter that changes the response body', () => {
    // PSY-1698: limit and timezone used to be absent from the key while
    // callers still varied them, so a 20-row preview and the venue page's
    // 50-row list shared one entry.
    expect(
      venueQueryKeys.showsPage(42, {
        timeFilter: 'upcoming',
        limit: 50,
        timezone: 'America/Phoenix',
      }),
    ).toEqual([
      'venues',
      'shows',
      '42',
      'upcoming',
      50,
      'America/Phoenix',
      null,
      null,
    ])
  })

  it('keys a shows page on its year and offset (PSY-1753)', () => {
    // The past archive pages by offset within a year; without both in the key
    // every page of every year would answer for the first one fetched.
    const base = {
      timeFilter: 'past',
      limit: 50,
      timezone: 'America/Phoenix',
    } as const
    const firstPage2025 = venueQueryKeys.showsPage(42, { ...base, year: 2025 })
    const secondPage2025 = venueQueryKeys.showsPage(42, {
      ...base,
      year: 2025,
      offset: 50,
    })
    const firstPage2024 = venueQueryKeys.showsPage(42, { ...base, year: 2024 })
    const allYears = venueQueryKeys.showsPage(42, base)

    expect(new Set([
      JSON.stringify(firstPage2025),
      JSON.stringify(secondPage2025),
      JSON.stringify(firstPage2024),
      JSON.stringify(allYears),
    ]).size).toBe(4)
  })

  it('nests the year histogram under the shows() prefix without colliding with a page', () => {
    const prefix = venueQueryKeys.shows(42)
    const years = venueQueryKeys.showYears(42, 'past')
    expect(years.slice(0, prefix.length)).toEqual([...prefix])
    // The slot 'years' occupies on a page key holds a time filter, which is
    // only ever upcoming/past/all — so the two shapes can never collide.
    expect(years[prefix.length]).toBe('years')
    expect(
      venueQueryKeys.showsPage(42, { timeFilter: 'past' })[prefix.length],
    ).toBe('past')
  })

  it('gives differently-parameterized shows requests distinct keys', () => {
    const base = { timeFilter: 'upcoming', timezone: 'America/Phoenix' } as const
    const venuePage = venueQueryKeys.showsPage(42, { ...base, limit: 50 })
    const preview = venueQueryKeys.showsPage(42, { ...base, limit: 20 })
    expect(preview).not.toEqual(venuePage)

    const utc = venueQueryKeys.showsPage(42, {
      timeFilter: 'upcoming',
      limit: 50,
      timezone: 'UTC',
    })
    expect(utc).not.toEqual(venuePage)

    const past = venueQueryKeys.showsPage(42, {
      ...base,
      timeFilter: 'past',
      limit: 50,
    })
    expect(past).not.toEqual(venuePage)
  })

  it('normalizes omitted params to null rather than a key hole', () => {
    expect(
      venueQueryKeys.showsPage(42, { timeFilter: 'upcoming', limit: 20 }),
    ).toEqual(['venues', 'shows', '42', 'upcoming', 20, null, null, null])
    expect(venueQueryKeys.showsPage(42, { timeFilter: 'upcoming' })).toEqual([
      'venues',
      'shows',
      '42',
      'upcoming',
      null,
      null,
      null,
      null,
    ])
  })

  it('nests every shows page under the shows() invalidation prefix', () => {
    // createInvalidateQueries fans out from a prefix, so the extra segments
    // have to be appended rather than woven in.
    const prefix = venueQueryKeys.shows(42)
    const page = venueQueryKeys.showsPage(42, {
      timeFilter: 'upcoming',
      limit: 50,
      timezone: 'America/Phoenix',
    })
    expect(page.slice(0, prefix.length)).toEqual([...prefix])
  })

  it('produces identical detail keys for a numeric id and its string form', () => {
    expect(venueQueryKeys.detail(42)).toEqual(venueQueryKeys.detail('42'))
  })

  it('keys billNetwork by venue + window, coercing a missing year to null', () => {
    expect(venueQueryKeys.billNetwork(42, 'all')).toEqual([
      'venues',
      'bill-network',
      '42',
      'all',
      null,
    ])
    expect(venueQueryKeys.billNetwork(42, 'year', 2025)).toEqual([
      'venues',
      'bill-network',
      '42',
      'year',
      2025,
    ])
  })
})
