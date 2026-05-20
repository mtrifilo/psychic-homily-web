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

  it('carries the optional timezone segment in the cities key', () => {
    expect(showQueryKeys.cities()).toEqual(['shows', 'cities', undefined])
    expect(showQueryKeys.cities('America/Phoenix')).toEqual([
      'shows',
      'cities',
      'America/Phoenix',
    ])
  })

  it('scopes the detail key by id', () => {
    expect(showQueryKeys.detail('42')).toEqual(['shows', 'detail', '42'])
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
