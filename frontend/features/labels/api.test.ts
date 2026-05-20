import { describe, it, expect } from 'vitest'
import { API_BASE_URL } from '@/lib/api-base'
import { labelEndpoints, labelQueryKeys } from './api'

describe('labelEndpoints', () => {
  it('exposes static collection endpoints rooted at the API base URL', () => {
    expect(labelEndpoints.LIST).toBe(`${API_BASE_URL}/labels`)
    expect(labelEndpoints.SEARCH).toBe(`${API_BASE_URL}/labels/search`)
    expect(labelEndpoints.CREATE).toBe(`${API_BASE_URL}/labels`)
  })

  it('builds a detail endpoint from a numeric id', () => {
    expect(labelEndpoints.GET(42)).toBe(`${API_BASE_URL}/labels/42`)
  })

  it('builds a detail endpoint from a slug', () => {
    expect(labelEndpoints.GET('sub-pop')).toBe(`${API_BASE_URL}/labels/sub-pop`)
  })

  it('builds update and delete endpoints from a label id', () => {
    expect(labelEndpoints.UPDATE(7)).toBe(`${API_BASE_URL}/labels/7`)
    expect(labelEndpoints.DELETE(7)).toBe(`${API_BASE_URL}/labels/7`)
  })

  it('builds nested roster and catalog endpoints', () => {
    expect(labelEndpoints.ARTISTS('sub-pop')).toBe(
      `${API_BASE_URL}/labels/sub-pop/artists`
    )
    expect(labelEndpoints.RELEASES(7)).toBe(`${API_BASE_URL}/labels/7/releases`)
  })
})

describe('labelQueryKeys', () => {
  it('uses a stable root key for cache invalidation', () => {
    expect(labelQueryKeys.all).toEqual(['labels'])
  })

  it('namespaces list keys with the filters object', () => {
    expect(labelQueryKeys.list({ status: 'active' })).toEqual([
      'labels',
      'list',
      { status: 'active' },
    ])
  })

  it('produces a list key with undefined filters when none are passed', () => {
    expect(labelQueryKeys.list()).toEqual(['labels', 'list', undefined])
  })

  it('lower-cases the query in search keys so case variants share a cache entry', () => {
    expect(labelQueryKeys.search('Sub POP')).toEqual([
      'labels',
      'search',
      'sub pop',
    ])
  })

  it('stringifies ids in detail / roster / catalog keys', () => {
    expect(labelQueryKeys.detail(42)).toEqual(['labels', 'detail', '42'])
    expect(labelQueryKeys.roster('sub-pop')).toEqual([
      'labels',
      'roster',
      'sub-pop',
    ])
    expect(labelQueryKeys.catalog(7)).toEqual(['labels', 'catalog', '7'])
  })

  it('produces identical detail keys for the numeric id and its string form', () => {
    expect(labelQueryKeys.detail(42)).toEqual(labelQueryKeys.detail('42'))
  })
})
