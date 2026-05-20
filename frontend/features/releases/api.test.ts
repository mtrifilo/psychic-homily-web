import { describe, it, expect, vi } from 'vitest'

// Pin API_BASE_URL so endpoint assertions are deterministic regardless of env.
vi.mock('@/lib/api-base', () => ({
  API_BASE_URL: 'http://api.test',
}))

import { releaseEndpoints, releaseQueryKeys } from './api'

describe('releaseEndpoints', () => {
  it('builds static collection endpoints', () => {
    expect(releaseEndpoints.LIST).toBe('http://api.test/releases')
    expect(releaseEndpoints.SEARCH).toBe('http://api.test/releases/search')
    expect(releaseEndpoints.CREATE).toBe('http://api.test/releases')
  })

  it('builds a detail endpoint from a slug', () => {
    expect(releaseEndpoints.GET('ok-computer')).toBe(
      'http://api.test/releases/ok-computer'
    )
  })

  it('builds a detail endpoint from a numeric id', () => {
    expect(releaseEndpoints.GET(42)).toBe('http://api.test/releases/42')
  })

  it('builds update and delete endpoints by id', () => {
    expect(releaseEndpoints.UPDATE(7)).toBe('http://api.test/releases/7')
    expect(releaseEndpoints.DELETE(7)).toBe('http://api.test/releases/7')
  })

  it('builds nested link endpoints', () => {
    expect(releaseEndpoints.ADD_LINK(7)).toBe(
      'http://api.test/releases/7/links'
    )
    expect(releaseEndpoints.REMOVE_LINK(7, 99)).toBe(
      'http://api.test/releases/7/links/99'
    )
  })

  it('builds the artist-releases endpoint', () => {
    expect(releaseEndpoints.ARTIST_RELEASES('radiohead')).toBe(
      'http://api.test/artists/radiohead/releases'
    )
  })
})

describe('releaseQueryKeys', () => {
  it('exposes a stable root key', () => {
    expect(releaseQueryKeys.all).toEqual(['releases'])
  })

  it('namespaces list keys under releases with the filter object', () => {
    const filters = { releaseType: 'lp' }
    expect(releaseQueryKeys.list(filters)).toEqual([
      'releases',
      'list',
      filters,
    ])
  })

  it('allows an undefined filter for the unfiltered list', () => {
    expect(releaseQueryKeys.list()).toEqual(['releases', 'list', undefined])
  })

  it('lowercases search query keys for cache stability', () => {
    expect(releaseQueryKeys.search('Radiohead')).toEqual([
      'releases',
      'search',
      'radiohead',
    ])
  })

  it('stringifies detail keys so id and slug share a cache shape', () => {
    expect(releaseQueryKeys.detail(42)).toEqual([
      'releases',
      'detail',
      '42',
    ])
    expect(releaseQueryKeys.detail('ok-computer')).toEqual([
      'releases',
      'detail',
      'ok-computer',
    ])
  })

  it('stringifies artist-releases keys', () => {
    expect(releaseQueryKeys.artistReleases(5)).toEqual([
      'releases',
      'artist',
      '5',
    ])
  })
})
