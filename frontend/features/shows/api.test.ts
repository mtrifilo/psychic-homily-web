import { describe, it, expect } from 'vitest'
import { API_BASE_URL } from '@/lib/api-base'
import { showEndpoints, showQueryKeys } from './api'

describe('showEndpoints', () => {
  it('exposes static collection endpoints rooted at the API base URL', () => {
    expect(showEndpoints.SUBMIT).toBe(`${API_BASE_URL}/shows`)
    expect(showEndpoints.UPCOMING).toBe(`${API_BASE_URL}/shows/upcoming`)
    expect(showEndpoints.CITIES).toBe(`${API_BASE_URL}/shows/cities`)
    expect(showEndpoints.SEARCH).toBe(`${API_BASE_URL}/shows/search`)
    expect(showEndpoints.MY_SUBMISSIONS).toBe(
      `${API_BASE_URL}/shows/my-submissions`
    )
  })

  it('builds detail + mutation endpoints from a show id', () => {
    expect(showEndpoints.GET(42)).toBe(`${API_BASE_URL}/shows/42`)
    expect(showEndpoints.GET('some-show-slug')).toBe(
      `${API_BASE_URL}/shows/some-show-slug`
    )
    expect(showEndpoints.UPDATE(42)).toBe(`${API_BASE_URL}/shows/42`)
    expect(showEndpoints.DELETE(42)).toBe(`${API_BASE_URL}/shows/42`)
  })

  it('builds status-transition endpoints from a show id', () => {
    expect(showEndpoints.UNPUBLISH(42)).toBe(
      `${API_BASE_URL}/shows/42/unpublish`
    )
    expect(showEndpoints.MAKE_PRIVATE(42)).toBe(
      `${API_BASE_URL}/shows/42/make-private`
    )
    expect(showEndpoints.PUBLISH(42)).toBe(`${API_BASE_URL}/shows/42/publish`)
    expect(showEndpoints.SET_SOLD_OUT(42)).toBe(
      `${API_BASE_URL}/shows/42/sold-out`
    )
    expect(showEndpoints.SET_CANCELLED(42)).toBe(
      `${API_BASE_URL}/shows/42/cancelled`
    )
  })

  it('builds the also-tonight rail endpoint from a show id or slug', () => {
    expect(showEndpoints.ALSO_TONIGHT(42)).toBe(
      `${API_BASE_URL}/shows/42/also-tonight`
    )
    // The rails address the show by slug when it has one, so the slug form is
    // the one actually exercised in the browser.
    expect(showEndpoints.ALSO_TONIGHT('some-show-slug')).toBe(
      `${API_BASE_URL}/shows/some-show-slug/also-tonight`
    )
  })

  it('builds export + report endpoints from a show id', () => {
    expect(showEndpoints.EXPORT(42)).toBe(`${API_BASE_URL}/shows/42/export`)
    expect(showEndpoints.REPORT(42)).toBe(`${API_BASE_URL}/shows/42/report`)
    expect(showEndpoints.MY_REPORT(42)).toBe(
      `${API_BASE_URL}/shows/42/my-report`
    )
  })
})

describe('showQueryKeys', () => {
  it('uses a stable root key for cache invalidation', () => {
    expect(showQueryKeys.all).toEqual(['shows'])
  })

  it('namespaces list keys with the filters object', () => {
    expect(showQueryKeys.list()).toEqual(['shows', 'list', undefined])
    expect(showQueryKeys.list({ city: 'Phoenix' })).toEqual([
      'shows',
      'list',
      { city: 'Phoenix' },
    ])
  })

  // No per-viewer segment (PSY-1678): /shows/cities counts one venue-local
  // partition for everyone, so a timezone segment could only fragment the cache
  // across entries holding identical data — and would break the server-seeded
  // first screen, which keys on exactly this.
  it('keys cities on nothing but the collection', () => {
    expect(showQueryKeys.cities()).toEqual(['shows', 'cities'])
  })

  it('scopes the detail key by id', () => {
    expect(showQueryKeys.detail('42')).toEqual(['shows', 'detail', '42'])
  })

  // Same no-per-viewer-segment rule as `cities` above, for the same reason:
  // the rail is about the SHOW's own night read on the venue's clock, so every
  // viewer of one show gets one answer and a timezone segment could only
  // fragment the cache across identical entries.
  it('scopes the also-tonight key by show, and by nothing else', () => {
    expect(showQueryKeys.alsoTonight('42')).toEqual([
      'shows',
      'also-tonight',
      '42',
    ])
  })

  it('shares the shows root, so a show edit invalidates the rail with it', () => {
    // `createInvalidateQueries(queryClient).shows()` (lib/queryClient.ts)
    // invalidates the bare `['shows']` prefix, which reaches this key. That is
    // correct rather than incidental: editing a show can move it to a different
    // night or venue, which changes what belongs on its rail.
    expect(showQueryKeys.alsoTonight('42')[0]).toBe(showQueryKeys.all[0])
  })

  it('sits OUTSIDE the detail key, so refetching the show alone keeps the rail', () => {
    // react-query matches on key PREFIX, so this is the assertion that matters
    // — comparing the two arrays for inequality could never fail.
    const detail = showQueryKeys.detail('42')
    const alsoTonight = showQueryKeys.alsoTonight('42')
    expect(alsoTonight.slice(0, detail.length)).not.toEqual([...detail])
  })

  it('scopes the userShows key by user id', () => {
    expect(showQueryKeys.userShows('user-7')).toEqual([
      'shows',
      'user',
      'user-7',
    ])
  })

  it('lower-cases the query in search keys so case variants share a cache entry', () => {
    expect(showQueryKeys.search('Gatecreeper FEST')).toEqual([
      'shows',
      'search',
      'gatecreeper fest',
    ])
  })
})
