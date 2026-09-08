import { describe, expect, it } from 'vitest'
import { addressesAnEntity, looksLikeSlug } from './entity-slug'

describe('addressesAnEntity', () => {
  it('accepts an ordinary slug', () => {
    expect(addressesAnEntity('gatecreeper')).toBe(true)
  })

  // A NULLABLE slug column and a `GenerateSlug` that can return "" both produce
  // these, and `/artists/` resolves to the INDEX rather than 404ing.
  it.each(['', '   ', '\t'])('refuses %j, which addresses the index', value => {
    expect(addressesAnEntity(value)).toBe(false)
  })

  // `encodeURIComponent` leaves both untouched, so `/collections/..` walks back
  // up to `/collections`.
  it.each(['.', '..', ' .. '])('refuses %j, which walks up to the index', value => {
    expect(addressesAnEntity(value)).toBe(false)
  })
})

describe('looksLikeSlug', () => {
  // Every shape the backend's `lower(replace(city,' ','-')) || '-' || lower(state)`
  // rule can produce. Punctuation and accents survive it, so a rule that
  // rejected them would take these pages down.
  it.each(['phoenix-az', 'st.-louis-mo', "coeur-d'alene-id", 'española-nm', 'r.e.m'])(
    'accepts %j',
    slug => {
      expect(looksLikeSlug(slug)).toBe(true)
    }
  )

  // The characters that take a value out of the segment it was written into,
  // plus the surrounding space that makes it a different address.
  it.each([
    ['../..', 'a path walk'],
    ['a/b', 'a second segment'],
    ['a\\b', 'a backslash the browser normalizes to a slash'],
    ['phoenix-az?x', 'a query'],
    ['phoenix-az#x', 'a fragment'],
    ['%2e%2e', 'an encoded path walk waiting for a decoder'],
    [' phoenix-az', 'a leading space'],
    ['phoenix-az ', 'a trailing space'],
  ])('refuses %j: %s', slug => {
    expect(looksLikeSlug(slug)).toBe(false)
  })

  // Space INSIDE a slug does not take it out of its segment, and the backend's
  // slug rule replaces only U+0020, so a city carrying any other space
  // character produces a slug that still addresses a page this site serves.
  // Refusing it would 404 that scene rather than stop an attack.
  it.each([
    ['new york-ny', 'a plain space'],
    ['new\u00a0york-ny', 'a non-breaking space'],
  ])('accepts %j, which carries %s inside', slug => {
    expect(looksLikeSlug(slug)).toBe(true)
  })

  // Inherited from the index rule rather than restated: `.` breaks nothing in a
  // URL, it just names somewhere else.
  it.each(['', '.', '..'])('refuses %j, which addresses the index', slug => {
    expect(looksLikeSlug(slug)).toBe(false)
  })
})
