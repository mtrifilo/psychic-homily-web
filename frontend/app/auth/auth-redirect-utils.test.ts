import { describe, expect, it } from 'vitest'
import { safeDecodeQueryParam, sanitizeReturnTo } from './auth-redirect-utils'

describe('sanitizeReturnTo', () => {
  it('returns fallback for empty values', () => {
    expect(sanitizeReturnTo(undefined)).toBe('/')
    expect(sanitizeReturnTo(null)).toBe('/')
    expect(sanitizeReturnTo('')).toBe('/')
  })

  it('allows safe internal routes', () => {
    expect(sanitizeReturnTo('/collection')).toBe('/collection')
    expect(sanitizeReturnTo('/collection?tab=favorites')).toBe(
      '/collection?tab=favorites'
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
    expect(sanitizeReturnTo('/auth?returnTo=/collection')).toBe('/')
    expect(sanitizeReturnTo('/auth/magic-link?token=abc')).toBe('/')
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
