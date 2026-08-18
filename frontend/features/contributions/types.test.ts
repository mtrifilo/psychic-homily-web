import { describe, it, expect } from 'vitest'
import {
  CATALOG_YEAR_BOUNDS,
  EDITABLE_FIELDS,
  MIN_CATALOG_YEAR,
  VENUE_CAPACITY_BOUNDS,
  fieldChangeValue,
  maxCatalogYear,
  validateFieldValue,
  validateNumberField,
  validateUrlField,
  type EditableField,
} from './types'

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

// PSY-1694: client-side pre-validator for the drawer's `type: 'number'` fields
// (venue capacity is the only one today). Mirrors `validateBoundedInt` in
// `backend/internal/api/handlers/shared/url_validation.go`, which stays the
// source of truth; this exists so a typo surfaces before the 422 roundtrip.
describe('validateNumberField', () => {
  const capacityBounds = VENUE_CAPACITY_BOUNDS

  it('returns null for empty string (clearing is intentional)', () => {
    expect(validateNumberField('', capacityBounds)).toBeNull()
  })

  it('returns null for whitespace-only string (treated as empty)', () => {
    expect(validateNumberField('   ', capacityBounds)).toBeNull()
  })

  it('accepts a whole number inside the range, padding included', () => {
    expect(validateNumberField('550', capacityBounds)).toBeNull()
    expect(validateNumberField('  550  ', capacityBounds)).toBeNull()
  })

  it('accepts both inclusive bounds', () => {
    expect(validateNumberField(String(capacityBounds.min), capacityBounds)).toBeNull()
    expect(validateNumberField(String(capacityBounds.max), capacityBounds)).toBeNull()
  })

  it('reports a digit string too large to represent as out of range', () => {
    // Twenty digits parse to a float that no longer holds what was typed. The
    // useful message is the range, not "that is not a whole number" (it is).
    expect(validateNumberField('99999999999999999999', capacityBounds)).toMatch(/between/i)
  })

  it('rejects zero and negatives as out of range, not as gibberish', () => {
    // NULL already means "we do not know this room's capacity", so a stored 0
    // would be a second way to say the same thing.
    expect(validateNumberField('0', capacityBounds)).toMatch(/between/i)
    expect(validateNumberField('-5', capacityBounds)).toMatch(/between/i)
  })

  it('rejects a value past the ceiling', () => {
    expect(validateNumberField('200001', capacityBounds)).toMatch(/between/i)
  })

  it('rejects fractions', () => {
    expect(validateNumberField('550.5', capacityBounds)).toMatch(/whole number/i)
  })

  it('rejects notations Number() would silently accept', () => {
    // Number('0x10') is 16 and Number('1e3') is 1000; neither is what someone
    // typing a capacity meant, and both would be stored as a number the user
    // never wrote.
    expect(validateNumberField('0x10', capacityBounds)).toMatch(/whole number/i)
    expect(validateNumberField('1e3', capacityBounds)).toMatch(/whole number/i)
    expect(validateNumberField('Infinity', capacityBounds)).toMatch(/whole number/i)
  })

  it('rejects trailing junk parseInt would have swallowed', () => {
    expect(validateNumberField('3600abc', capacityBounds)).toMatch(/whole number/i)
    expect(validateNumberField('550 people', capacityBounds)).toMatch(/whole number/i)
  })

  it('reports a one-sided bound when only one is set', () => {
    expect(validateNumberField('0', { min: 1 })).toMatch(/at least/i)
    expect(validateNumberField('5', { max: 1 })).toMatch(/at most/i)
  })
})

describe('fieldChangeValue', () => {
  const capacity = EDITABLE_FIELDS.venue.find((f) => f.key === 'capacity') as EditableField
  const agePolicy = EDITABLE_FIELDS.venue.find((f) => f.key === 'age_policy') as EditableField

  it('exposes capacity as the venue drawer numeric field', () => {
    expect(capacity).toBeDefined()
    expect(capacity.type).toBe('number')
    expect(capacity.min).toBe(1)
    expect(capacity.max).toBe(200000)
  })

  it('submits a numeric field as a JSON number, not the input string', () => {
    // The column is an integer and the backend rejects a numeric string on
    // purpose, so the coercion here is load-bearing rather than cosmetic.
    expect(fieldChangeValue(capacity, '550')).toBe(550)
    expect(fieldChangeValue(capacity, '  550  ')).toBe(550)
  })

  it('submits null when a numeric field is cleared', () => {
    // Clearing must reach the column as NULL, never as 0.
    expect(fieldChangeValue(capacity, '')).toBeNull()
    expect(fieldChangeValue(capacity, '   ')).toBeNull()
  })

  it('passes an unparseable numeric value through unconverted', () => {
    // Submit is disabled while the field has an error, so this never ships;
    // coercing it would invent a number the user did not type.
    expect(fieldChangeValue(capacity, '550abc')).toBe('550abc')
  })

  it('leaves non-numeric fields exactly as they were', () => {
    expect(fieldChangeValue(agePolicy, '21+')).toBe('21+')
    expect(fieldChangeValue(agePolicy, '')).toBeNull()
    // Whitespace-only stays a string for text fields: the server normalizes it,
    // and changing it here would alter behavior for every existing field.
    expect(fieldChangeValue(agePolicy, '   ')).toBe('   ')
  })
})

describe('validateFieldValue', () => {
  it('dispatches to the validator that matches the field type', () => {
    const capacity = EDITABLE_FIELDS.venue.find((f) => f.key === 'capacity') as EditableField
    const instagram = EDITABLE_FIELDS.venue.find((f) => f.key === 'instagram') as EditableField
    const name = EDITABLE_FIELDS.venue.find((f) => f.key === 'name') as EditableField

    expect(validateFieldValue(capacity, '0')).toMatch(/between/i)
    expect(validateFieldValue(instagram, 'not-a-real-url')).toMatch(/http/i)
    // Plain text carries no client-side constraint.
    expect(validateFieldValue(name, 'anything at all')).toBeNull()
  })
})

// PSY-1703: `labels.founded_year` and `releases.release_year` are integer
// columns whose drawer control submitted TEXT until this change, which the
// backend then coerced silently: `1985.7` landed as 1985 with no error at any
// layer. They are `type: 'number'` fields now, so the drawer sends a JSON
// number and this validator mirrors the server's range.
describe('catalog year fields', () => {
  const foundedYear = EDITABLE_FIELDS.label.find(
    (f) => f.key === 'founded_year'
  ) as EditableField
  const releaseYear = EDITABLE_FIELDS.release.find(
    (f) => f.key === 'release_year'
  ) as EditableField
  const yearFields: ReadonlyArray<[string, EditableField]> = [
    ['founded_year', foundedYear],
    ['release_year', releaseYear],
  ]

  it('resolves the ceiling on every read rather than at module load', () => {
    // The one direction a client-side pre-validator must never fail in is
    // "stricter than the server". A ceiling frozen at module load would drift
    // behind the server's the moment the year turned over.
    expect(maxCatalogYear()).toBe(new Date().getUTCFullYear() + 1)
    expect(CATALOG_YEAR_BOUNDS.max).toBe(maxCatalogYear())
    expect(CATALOG_YEAR_BOUNDS.min).toBe(MIN_CATALOG_YEAR)
  })

  it.each(yearFields)('declares %s as a bounded numeric field', (_key, field) => {
    expect(field).toBeDefined()
    expect(field.type).toBe('number')
    expect(field.min).toBe(MIN_CATALOG_YEAR)
    // Read through the FIELD, not the constant: this is the assertion that
    // would break if the bounds were spread into the definition and froze.
    expect(field.max).toBe(maxCatalogYear())
  })

  it.each(yearFields)('accepts a real year for %s, on both bounds', (_key, field) => {
    expect(validateFieldValue(field, '1985')).toBeNull()
    expect(validateFieldValue(field, String(MIN_CATALOG_YEAR))).toBeNull()
    // Next year: a release can be announced before it is pressed.
    expect(validateFieldValue(field, String(maxCatalogYear()))).toBeNull()
    // Clearing is intentional and reaches the column as NULL.
    expect(validateFieldValue(field, '')).toBeNull()
  })

  it.each(yearFields)('rejects out-of-range values for %s', (_key, field) => {
    expect(validateFieldValue(field, '0')).toMatch(/between/i)
    expect(validateFieldValue(field, '-1985')).toMatch(/between/i)
    expect(validateFieldValue(field, String(MIN_CATALOG_YEAR - 1))).toMatch(/between/i)
    expect(validateFieldValue(field, String(maxCatalogYear() + 1))).toMatch(/between/i)
    // The trailing-digit slip this ceiling mostly exists to catch.
    expect(validateFieldValue(field, '19850')).toMatch(/between/i)
  })

  it.each(yearFields)('rejects values that are not whole numbers for %s', (_key, field) => {
    expect(validateFieldValue(field, '1985.7')).toMatch(/whole number/i)
    expect(validateFieldValue(field, '1985 approx')).toMatch(/whole number/i)
    expect(validateFieldValue(field, '1e3')).toMatch(/whole number/i)
  })

  it.each(yearFields)('submits %s as a JSON number, or null when cleared', (_key, field) => {
    // The column is an integer and the backend rejects a numeric string on
    // purpose, so this coercion is the load-bearing half of the change.
    expect(fieldChangeValue(field, '1985')).toBe(1985)
    expect(fieldChangeValue(field, '  1985  ')).toBe(1985)
    expect(fieldChangeValue(field, '')).toBeNull()
    expect(fieldChangeValue(field, '   ')).toBeNull()
  })

  it('leaves release_date alone: it is free text, not a gated number', () => {
    const releaseDate = EDITABLE_FIELDS.release.find(
      (f) => f.key === 'release_date'
    ) as EditableField
    expect(releaseDate.type).toBe('text')
    expect(validateFieldValue(releaseDate, '1991-09-24')).toBeNull()
    expect(fieldChangeValue(releaseDate, '1991-09-24')).toBe('1991-09-24')
  })
})
