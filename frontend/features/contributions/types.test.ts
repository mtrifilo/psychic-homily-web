import { describe, it, expect } from 'vitest'
import { validateUrlField } from './types'

// PSY-599: client-side URL pre-validator for the suggest-edit drawer's
// `type: 'url'` fields. Server-side validation is the source of truth (see
// `backend/internal/utils/url.go`); this is purely UX so users see an
// invalid URL before the 422 roundtrip.
describe('validateUrlField', () => {
  it('returns null for empty string (clearing is intentional)', () => {
    expect(validateUrlField('')).toBeNull()
  })

  it('returns null for whitespace-only string (treated as empty)', () => {
    expect(validateUrlField('   ')).toBeNull()
  })

  it('returns null for a valid https URL', () => {
    expect(validateUrlField('https://instagram.com/someone')).toBeNull()
  })

  it('returns null for a valid http URL', () => {
    expect(validateUrlField('http://example.com')).toBeNull()
  })

  it('returns null for surrounding whitespace around a valid URL', () => {
    expect(validateUrlField('  https://example.com  ')).toBeNull()
  })

  it('rejects strings without a scheme — the canonical PSY-599 case', () => {
    // This is the exact bug — `not-a-real-url` survived the server roundtrip
    // and produced a confusing `(got "")` message. Now the client catches
    // it before submit.
    expect(validateUrlField('not-a-real-url')).toMatch(/http/i)
  })

  it('rejects bare domains without a scheme', () => {
    expect(validateUrlField('instagram.com/someone')).toMatch(/http/i)
  })

  it('rejects bare handles', () => {
    expect(validateUrlField('@matt')).toMatch(/http/i)
  })

  it('rejects javascript: URLs', () => {
    expect(validateUrlField('javascript:alert(1)')).toMatch(/http/i)
  })

  it('rejects data: URLs', () => {
    expect(validateUrlField('data:text/html,foo')).toMatch(/http/i)
  })

  it('rejects file: URLs', () => {
    expect(validateUrlField('file:///etc/passwd')).toMatch(/http/i)
  })

  it('rejects ftp: URLs', () => {
    expect(validateUrlField('ftp://example.com')).toMatch(/http/i)
  })

  it('rejects mailto: URLs', () => {
    expect(validateUrlField('mailto:matt@example.com')).toMatch(/http/i)
  })
})
