import { describe, it, expect } from 'vitest'
import { API_BASE_URL } from '@/lib/api-base'
import { artistEndpoints, artistQueryKeys } from './api'

describe('artistEndpoints', () => {
  it('exposes static collection endpoints rooted at the API base URL', () => {
    expect(artistEndpoints.LIST).toBe(`${API_BASE_URL}/artists`)
    expect(artistEndpoints.CITIES).toBe(`${API_BASE_URL}/artists/cities`)
    expect(artistEndpoints.SEARCH).toBe(`${API_BASE_URL}/artists/search`)
  })

  it('builds a detail endpoint from a numeric id or a slug', () => {
    expect(artistEndpoints.GET(42)).toBe(`${API_BASE_URL}/artists/42`)
    expect(artistEndpoints.GET('gatecreeper')).toBe(
      `${API_BASE_URL}/artists/gatecreeper`
    )
  })

  it('builds nested relation endpoints from an id or slug', () => {
    expect(artistEndpoints.SHOWS('gatecreeper')).toBe(
      `${API_BASE_URL}/artists/gatecreeper/shows`
    )
    expect(artistEndpoints.LABELS('gatecreeper')).toBe(
      `${API_BASE_URL}/artists/gatecreeper/labels`
    )
    expect(artistEndpoints.ALIASES(7)).toBe(`${API_BASE_URL}/artists/7/aliases`)
    expect(artistEndpoints.GRAPH(7)).toBe(`${API_BASE_URL}/artists/7/graph`)
    expect(artistEndpoints.BILL_COMPOSITION(7)).toBe(
      `${API_BASE_URL}/artists/7/bill-composition`
    )
    expect(artistEndpoints.RELATED(7)).toBe(`${API_BASE_URL}/artists/7/related`)
  })

  it('builds mutation + report endpoints from an artist id', () => {
    expect(artistEndpoints.DELETE(7)).toBe(`${API_BASE_URL}/artists/7`)
    expect(artistEndpoints.REPORT(7)).toBe(`${API_BASE_URL}/artists/7/report`)
    expect(artistEndpoints.MY_REPORT(7)).toBe(
      `${API_BASE_URL}/artists/7/my-report`
    )
  })

  it('builds relationship endpoints, interpolating both ids in VOTE', () => {
    expect(artistEndpoints.RELATIONSHIPS.CREATE).toBe(
      `${API_BASE_URL}/artists/relationships`
    )
    expect(artistEndpoints.RELATIONSHIPS.VOTE(3, 9)).toBe(
      `${API_BASE_URL}/artists/relationships/3/9/vote`
    )
  })
})

describe('artistQueryKeys', () => {
  it('uses a stable root key for cache invalidation', () => {
    expect(artistQueryKeys.all).toEqual(['artists'])
  })

  it('namespaces list keys with the filters object', () => {
    expect(artistQueryKeys.list()).toEqual(['artists', 'list', undefined])
    expect(artistQueryKeys.list({ city: 'Phoenix' })).toEqual([
      'artists',
      'list',
      { city: 'Phoenix' },
    ])
  })

  it('exposes a static cities key', () => {
    expect(artistQueryKeys.cities).toEqual(['artists', 'cities'])
  })

  it('lower-cases the query in search keys so case variants share a cache entry', () => {
    expect(artistQueryKeys.search('Gatecreeper')).toEqual([
      'artists',
      'search',
      'gatecreeper',
    ])
  })

  it('stringifies ids in detail / shows / labels keys for stable equality', () => {
    expect(artistQueryKeys.detail(42)).toEqual(['artists', 'detail', '42'])
    expect(artistQueryKeys.detail('gatecreeper')).toEqual([
      'artists',
      'detail',
      'gatecreeper',
    ])
    expect(artistQueryKeys.shows(42)).toEqual(['artists', 'shows', '42'])
    expect(artistQueryKeys.labels(42)).toEqual(['artists', 'labels', '42'])
  })

  it('produces identical detail keys for a numeric id and its string form', () => {
    expect(artistQueryKeys.detail(42)).toEqual(artistQueryKeys.detail('42'))
  })

  it('keeps the aliases key numeric (no stringify)', () => {
    expect(artistQueryKeys.aliases(7)).toEqual(['artists', 'aliases', 7])
  })

  it('carries the optional types array in the graph key', () => {
    expect(artistQueryKeys.graph(7)).toEqual([
      'artists',
      'graph',
      '7',
      undefined,
    ])
    expect(artistQueryKeys.graph(7, ['radio_cooccurrence'])).toEqual([
      'artists',
      'graph',
      '7',
      ['radio_cooccurrence'],
    ])
  })

  it('carries the months window in the billComposition key', () => {
    expect(artistQueryKeys.billComposition(7, 12)).toEqual([
      'artists',
      'billComposition',
      '7',
      12,
    ])
  })
})
