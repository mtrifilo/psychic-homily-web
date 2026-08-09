import { describe, it, expect } from 'vitest'
import { API_BASE_URL } from '@/lib/api-base'
import {
  ARTIST_PAST_SHOWS_PAGE_LIMIT,
  artistEndpoints,
  artistPastShowsPageParams,
  artistQueryKeys,
} from './api'

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
    expect(artistEndpoints.SHOW_YEARS('gatecreeper')).toBe(
      `${API_BASE_URL}/artists/gatecreeper/shows/years`
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
    // PSY-1633: submitting goes through useReportEntity, so there is
    // deliberately no artist-specific REPORT endpoint here.
    expect(artistEndpoints).not.toHaveProperty('REPORT')
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

  it('records every response-shaping param in the showsPage key', () => {
    expect(
      artistQueryKeys.showsPage(42, {
        timeFilter: 'past',
        limit: 50,
        timezone: 'America/Phoenix',
        year: 2025,
        offset: 100,
      }),
    ).toEqual([
      'artists',
      'shows',
      '42',
      'past',
      50,
      'America/Phoenix',
      2025,
      100,
    ])
  })

  it('normalizes every unsent param to one null slot', () => {
    // "Not sent" has to hash as ONE key. An omitted `offset` and an `offset` the
    // caller passed as undefined describe the same request, and page 1's zero
    // offset is never sent at all.
    expect(artistQueryKeys.showsPage(42, { timeFilter: 'upcoming' })).toEqual([
      'artists',
      'shows',
      '42',
      'upcoming',
      null,
      null,
      null,
      null,
    ])
  })

  it('nests showsPage and showYears under the shows() invalidation prefix', () => {
    // `createInvalidateQueries` reaches one artist's pages through `shows()`.
    // Both of these are only reachable while they EXTEND it.
    const prefix = artistQueryKeys.shows(42)
    for (const key of [
      artistQueryKeys.showsPage(42, { timeFilter: 'past', limit: 50 }),
      artistQueryKeys.showYears(42, 'past'),
    ]) {
      expect(key.slice(0, prefix.length)).toEqual([...prefix])
    }
  })

  it('builds the past-archive page params page 1 sends and page 3 sends', () => {
    // Page 1's offset must be `undefined`, not 0: the request omits it, and the
    // key distinguishes the two. A peek that disagreed here would silently
    // never find page 1 in the cache.
    expect(artistPastShowsPageParams(1, null)).toMatchObject({
      timeFilter: 'past',
      limit: ARTIST_PAST_SHOWS_PAGE_LIMIT,
      year: undefined,
      offset: undefined,
    })
    expect(artistPastShowsPageParams(3, 2025)).toMatchObject({
      year: 2025,
      offset: 2 * ARTIST_PAST_SHOWS_PAGE_LIMIT,
    })
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
