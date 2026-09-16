import { describe, it, expect } from 'vitest'

import {
  appendCityCountScope,
  cityCountQueryKey,
  cityCountScopeKey,
} from './cityCountScope'

function paramsFor(scope: Parameters<typeof appendCityCountScope>[1]): string {
  const params = new URLSearchParams()
  appendCityCountScope(params, scope)
  return params.toString()
}

describe('appendCityCountScope', () => {
  it('sends nothing at all for an empty scope', () => {
    expect(paramsFor(undefined)).toBe('')
    expect(paramsFor({})).toBe('')
    expect(paramsFor({ tags: [] })).toBe('')
    // A tag match with no tags filters nothing, so it must not reach the wire
    // either: the bare endpoint is what the seeded first-screen entry holds.
    expect(paramsFor({ tags: [], tagMatch: 'any' })).toBe('')
  })

  it('spells the tag filter the way the lists spell it', () => {
    expect(paramsFor({ tags: ['post-punk', 'shoegaze'] })).toBe(
      'tags=post-punk%2Cshoegaze'
    )
    expect(paramsFor({ tags: ['post-punk'], tagMatch: 'any' })).toBe(
      'tags=post-punk&tag_match=any'
    )
    // 'all' is the default, so it is never sent.
    expect(paramsFor({ tags: ['post-punk'], tagMatch: 'all' })).toBe(
      'tags=post-punk'
    )
  })
})

describe('cityCountScopeKey', () => {
  it('is undefined for every spelling of an empty scope', () => {
    expect(cityCountScopeKey(undefined)).toBeUndefined()
    expect(cityCountScopeKey({})).toBeUndefined()
    expect(cityCountScopeKey({ tags: [] })).toBeUndefined()
    expect(cityCountScopeKey({ tags: [], tagMatch: 'any' })).toBeUndefined()
  })

  it('normalizes the default tag match away', () => {
    expect(cityCountScopeKey({ tags: ['diy'], tagMatch: 'all' })).toEqual(
      cityCountScopeKey({ tags: ['diy'] })
    )
    expect(cityCountScopeKey({ tags: ['diy'], tagMatch: 'any' })).toEqual({
      tags: ['diy'],
      tagMatch: 'any',
    })
  })
})

describe('cityCountQueryKey', () => {
  const base = ['venues', 'cities'] as const

  it('returns the base key untouched when there is nothing to scope by', () => {
    // Identity, not just equality: the unscoped facet has to land on exactly the
    // entry a server-seeded payload was written to.
    expect(cityCountQueryKey(base, undefined)).toBe(base)
    expect(cityCountQueryKey(base, { tags: [] })).toBe(base)
  })

  it('appends one fragment for a scoped read', () => {
    expect(cityCountQueryKey(base, { tags: ['diy'] })).toEqual([
      'venues',
      'cities',
      { tags: ['diy'], tagMatch: undefined },
    ])
  })

  it('keys two different tag filters apart', () => {
    const a = cityCountQueryKey(base, { tags: ['diy'] })
    const b = cityCountQueryKey(base, { tags: ['punk'] })
    expect(JSON.stringify(a)).not.toBe(JSON.stringify(b))
  })
})
