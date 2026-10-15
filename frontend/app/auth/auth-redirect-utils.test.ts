import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

import { FALLBACK_RETURN_TO } from '@/lib/auth-href'

import { safeDecodeQueryParam, sanitizeReturnTo } from './auth-redirect-utils'

// Read at runtime, not imported: an import would pull a backend file into the
// program `next build` typechecks. The Go sanitizer's test reads the same file;
// its _comment states the contract.
const corpus = JSON.parse(
  readFileSync(
    resolve(
      __dirname,
      '../../../backend/internal/api/handlers/auth/testdata/return_to_corpus.json'
    ),
    'utf8'
  )
) as {
  fallback: string
  cases: { name: string; input: string; sanitizeReturnTo: string; rule: string }[]
}

describe('sanitizeReturnTo', () => {
  // Named explicitly: an empty array turns its `it.each` into zero tests.
  it('reads cases from the shared corpus', () => {
    expect(corpus.cases.length).toBeGreaterThan(0)
  })

  it('falls back to the path the backend falls back to', () => {
    expect(FALLBACK_RETURN_TO).toBe(corpus.fallback)
  })

  it.each(corpus.cases)('$name', ({ input, sanitizeReturnTo: want, rule }) => {
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
