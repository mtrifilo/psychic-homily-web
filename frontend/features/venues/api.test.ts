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
