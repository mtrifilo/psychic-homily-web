import { describe, expect, it } from 'vitest'
import { safeDecodeQueryParam, sanitizeReturnTo } from './auth-redirect-utils'

describe('sanitizeReturnTo', () => {
  it('returns fallback for empty values', () => {
    expect(sanitizeReturnTo(undefined)).toBe('/')
    expect(sanitizeReturnTo(null)).toBe('/')
    expect(sanitizeReturnTo('')).toBe('/')
  })

  it('allows safe internal routes', () => {
    expect(sanitizeReturnTo('/library')).toBe('/library')
    expect(sanitizeReturnTo('/library?tab=venues')).toBe(
      '/library?tab=venues'
    )
    expect(sanitizeReturnTo('/shows/slug#details')).toBe('/shows/slug#details')
  })

  it('blocks external and protocol-relative values', () => {
    expect(sanitizeReturnTo('https://example.com/evil')).toBe('/')
    expect(sanitizeReturnTo('javascript:alert(1)')).toBe('/')
    expect(sanitizeReturnTo('//evil.com/path')).toBe('/')
  })

  it('blocks auth-loop destinations', () => {
    expect(sanitizeReturnTo('/auth')).toBe('/')
    expect(sanitizeReturnTo('/auth?returnTo=/library')).toBe('/')
    expect(sanitizeReturnTo('/auth/magic-link?token=abc')).toBe('/')
  })
  it('refuses a destination that normalizes into a protocol-relative URL', () => {
    // The raw `//` check cannot see this one: URL normalization collapses the
    // `..` segment, so the parsed origin is still ours while the pathname
    // becomes `//evil.com`, which every sink navigates cross-origin.
    expect(sanitizeReturnTo('/..//evil.com')).toBe('/')
    expect(sanitizeReturnTo('/./..//evil.com')).toBe('/')
    expect(sanitizeReturnTo('/a/..//evil.com')).toBe('/')
    expect(sanitizeReturnTo('/..//evil.com/phish?a=1')).toBe('/')
  })
})

describe('safeDecodeQueryParam', () => {
  it('returns null for empty values', () => {
    expect(safeDecodeQueryParam(undefined)).toBeNull()
    expect(safeDecodeQueryParam(null)).toBeNull()
    expect(safeDecodeQueryParam('')).toBeNull()
  })

  it('decodes encoded values', () => {
    expect(safeDecodeQueryParam('Email%20already%20exists')).toBe(
      'Email already exists'
    )
  })

  it('returns raw value when decode fails', () => {
    expect(safeDecodeQueryParam('%E0%A4%A')).toBe('%E0%A4%A')
  })
})
