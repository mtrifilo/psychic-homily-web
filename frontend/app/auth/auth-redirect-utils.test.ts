import { describe, expect, it } from 'vitest'

import { FALLBACK_RETURN_TO } from '@/lib/auth-href'
import { loadBackendCorpus } from '@/test/loadBackendCorpus'

import { safeDecodeQueryParam, sanitizeReturnTo } from './auth-redirect-utils'

// The Go sanitizer's test reads the same file; its _comment states the contract.
const corpus = loadBackendCorpus<{
  fallback: string
  cases: { name: string; input: string; want: string; rule: string }[]
}>('internal/api/handlers/auth/testdata/return_to_corpus.json')

describe('sanitizeReturnTo', () => {
  // Named explicitly: an empty array turns its `it.each` into zero tests.
  it('reads cases from the shared corpus', () => {
    expect(corpus.cases.length).toBeGreaterThan(0)
  })

  it('falls back to the path the backend falls back to', () => {
    expect(FALLBACK_RETURN_TO).toBe(corpus.fallback)
  })

  it.each(corpus.cases)('$name', ({ input, want, rule }) => {
    expect(sanitizeReturnTo(input), rule).toBe(want)
  })

  // The Go sanitizer takes a string, so the shared corpus has no case for these.
  it('returns fallback for null and undefined', () => {
    expect(sanitizeReturnTo(undefined)).toBe(FALLBACK_RETURN_TO)
    expect(sanitizeReturnTo(null)).toBe(FALLBACK_RETURN_TO)
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
