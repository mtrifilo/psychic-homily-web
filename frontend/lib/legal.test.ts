import { describe, it, expect } from 'vitest'
import { CURRENT_TERMS_VERSION, CURRENT_PRIVACY_VERSION } from './legal'

describe('legal constants', () => {
  it('exports CURRENT_TERMS_VERSION as a date string', () => {
    expect(CURRENT_TERMS_VERSION).toBeDefined()
    expect(typeof CURRENT_TERMS_VERSION).toBe('string')
    // Should be a valid date format YYYY-MM-DD
    expect(CURRENT_TERMS_VERSION).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })

  it('exports CURRENT_PRIVACY_VERSION as a date string', () => {
    expect(CURRENT_PRIVACY_VERSION).toBeDefined()
    expect(typeof CURRENT_PRIVACY_VERSION).toBe('string')
    expect(CURRENT_PRIVACY_VERSION).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })

  it('has valid date values that can be parsed', () => {
    const termsDate = new Date(CURRENT_TERMS_VERSION)
    const privacyDate = new Date(CURRENT_PRIVACY_VERSION)

    expect(termsDate.toString()).not.toBe('Invalid Date')
    expect(privacyDate.toString()).not.toBe('Invalid Date')
  })

  it('has terms version set to expected value', () => {
    expect(CURRENT_TERMS_VERSION).toBe('2026-01-31')
  })

  it('has privacy version set to expected value', () => {
    expect(CURRENT_PRIVACY_VERSION).toBe('2026-02-15')
  })
})
