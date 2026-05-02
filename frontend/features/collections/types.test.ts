import { describe, it, expect } from 'vitest'
import {
  validateCoverImageUrl,
  MAX_COVER_IMAGE_URL_LENGTH,
} from './types'

// PSY-371: client-side validator for the cover image URL field on the
// Edit Collection dialog. Server-side sanitization is the source of truth;
// this is purely UX so curators see the problem before they hit Save.
describe('validateCoverImageUrl', () => {
  it('returns null for empty string (clearing is intentional)', () => {
    expect(validateCoverImageUrl('')).toBeNull()
  })

  it('returns null for whitespace-only string (treated as empty)', () => {
    expect(validateCoverImageUrl('   ')).toBeNull()
  })

  it('returns null for a valid https URL', () => {
    expect(
      validateCoverImageUrl('https://example.com/cover.jpg')
    ).toBeNull()
  })

  it('returns null for a valid http URL', () => {
    expect(validateCoverImageUrl('http://example.com/cover.jpg')).toBeNull()
  })

  it('returns null for surrounding whitespace around a valid URL', () => {
    expect(
      validateCoverImageUrl('  https://example.com/cover.jpg  ')
    ).toBeNull()
  })

  it('rejects strings without a scheme (no http:// or https://)', () => {
    expect(validateCoverImageUrl('example.com/cover.jpg')).toMatch(
      /http/i
    )
    expect(validateCoverImageUrl('not-a-url')).toMatch(/http/i)
  })

  it('rejects javascript: URLs', () => {
    expect(validateCoverImageUrl('javascript:alert(1)')).toMatch(/http/i)
  })

  it('rejects data: URLs', () => {
    expect(
      validateCoverImageUrl('data:image/png;base64,iVBOR')
    ).toMatch(/http/i)
  })

  it('rejects file: URLs', () => {
    expect(validateCoverImageUrl('file:///etc/passwd')).toMatch(/http/i)
  })

  it('rejects URLs longer than the cap', () => {
    const oversized =
      'https://example.com/' + 'a'.repeat(MAX_COVER_IMAGE_URL_LENGTH)
    expect(validateCoverImageUrl(oversized)).toMatch(/long|max/i)
  })

  it('accepts URLs at exactly the cap', () => {
    const prefix = 'https://example.com/'
    const padded = prefix + 'a'.repeat(MAX_COVER_IMAGE_URL_LENGTH - prefix.length)
    expect(padded.length).toBe(MAX_COVER_IMAGE_URL_LENGTH)
    expect(validateCoverImageUrl(padded)).toBeNull()
  })
})
