import { describe, expect, it } from 'vitest'
import {
  addressableShowSlug,
  isNumericShowSegment,
  showCanonicalPath,
  showSlugRedirectPath,
} from './showCanonical'

describe('isNumericShowSegment', () => {
  it('is true for a run of digits', () => {
    expect(isNumericShowSegment('1359')).toBe(true)
    expect(isNumericShowSegment('01359')).toBe(true)
  })

  it('is false for a slug, including one that starts with a date', () => {
    expect(isNumericShowSegment('2026-09-19-nirosta-steel-at-hideout')).toBe(false)
    expect(isNumericShowSegment('1359a')).toBe(false)
    expect(isNumericShowSegment('+1359')).toBe(false)
    expect(isNumericShowSegment('')).toBe(false)
  })
})

describe('addressableShowSlug', () => {
  it('returns a real slug', () => {
    expect(addressableShowSlug('white-denim-at-moth-club')).toBe(
      'white-denim-at-moth-club'
    )
  })

  it('is null for an absent, empty or all-digit slug', () => {
    expect(addressableShowSlug(null)).toBeNull()
    expect(addressableShowSlug(undefined)).toBeNull()
    expect(addressableShowSlug('')).toBeNull()
    expect(addressableShowSlug('2325')).toBeNull()
  })
})

describe('showCanonicalPath', () => {
  it('names the loaded slug when the request was a numeric id', () => {
    expect(showCanonicalPath('1359', 'white-denim-at-moth-club')).toBe(
      '/shows/white-denim-at-moth-club'
    )
  })

  it('names the loaded slug when the request was the slug', () => {
    expect(
      showCanonicalPath('white-denim-at-moth-club', 'white-denim-at-moth-club')
    ).toBe('/shows/white-denim-at-moth-club')
  })

  it('keeps the requested id for a show with no slug, never the bare root', () => {
    expect(showCanonicalPath('1359', null)).toBe('/shows/1359')
    expect(showCanonicalPath('1359', '')).toBe('/shows/1359')
  })

  it('keeps the requested id when the slug is all digits', () => {
    expect(showCanonicalPath('1359', '2325')).toBe('/shows/1359')
  })

  it('percent-encodes the segment', () => {
    expect(showCanonicalPath('1359', 'a/b?c')).toBe('/shows/a%2Fb%3Fc')
  })
})

describe('showSlugRedirectPath', () => {
  it('redirects a numeric request to the slug', () => {
    expect(showSlugRedirectPath('1359', 'white-denim-at-moth-club')).toBe(
      '/shows/white-denim-at-moth-club'
    )
  })

  it('never redirects a slug request, so the target cannot loop', () => {
    expect(
      showSlugRedirectPath('white-denim-at-moth-club', 'white-denim-at-moth-club')
    ).toBeNull()
    const target = showSlugRedirectPath('1359', 'white-denim-at-moth-club')
    const followed = target!.slice('/shows/'.length)
    expect(showSlugRedirectPath(followed, 'white-denim-at-moth-club')).toBeNull()
  })

  it('does not redirect a show with no slug', () => {
    expect(showSlugRedirectPath('1359', null)).toBeNull()
    expect(showSlugRedirectPath('1359', '')).toBeNull()
  })

  it('does not redirect to an all-digit slug, which would address another show', () => {
    expect(showSlugRedirectPath('1359', '2325')).toBeNull()
  })

  it('percent-encodes the target segment', () => {
    expect(showSlugRedirectPath('1359', 'a/b?c')).toBe('/shows/a%2Fb%3Fc')
  })
})
