import { describe, it, expect } from 'vitest'
import {
  TAG_CATEGORIES,
  TAG_SORT_OPTIONS,
  DEFAULT_TAG_SORT,
  DEFAULT_TAG_VIEW,
  TAG_ENTITY_TYPES,
  LOW_QUALITY_REASON_LABELS,
  LOW_QUALITY_SIGNAL_CHIPS,
  getCategoryColor,
  getCategoryLabel,
  getEntityUrl,
  getEntityTypePluralLabel,
} from './types'

describe('tag constants', () => {
  it('exposes the three tag categories', () => {
    expect(TAG_CATEGORIES).toEqual(['genre', 'locale', 'other'])
  })

  it('maps each sort option to a backend slug', () => {
    expect(TAG_SORT_OPTIONS).toEqual([
      { value: 'popularity', label: 'Popularity', backend: 'usage' },
      { value: 'alphabetical', label: 'Alphabetical', backend: 'name' },
      { value: 'newest', label: 'Newest', backend: 'created' },
    ])
  })

  it('defaults sort to popularity and view to grid', () => {
    expect(DEFAULT_TAG_SORT).toBe('popularity')
    expect(DEFAULT_TAG_VIEW).toBe('grid')
  })

  it('lists the polymorphic tag entity types (collection included)', () => {
    expect(TAG_ENTITY_TYPES).toEqual([
      'artist',
      'release',
      'label',
      'show',
      'venue',
      'festival',
      'collection',
    ])
  })
})

describe('low-quality reason labels', () => {
  it('keeps a label for every low-quality reason', () => {
    const reasons = [
      'orphaned',
      'aging_unused',
      'downvoted',
      'short_name',
      'long_name',
    ] as const
    for (const reason of reasons) {
      expect(LOW_QUALITY_REASON_LABELS[reason]).toBeTruthy()
    }
  })
})

describe('low-quality signal chips', () => {
  it('merges short_name and long_name into one "Unusual length" chip', () => {
    const chip = LOW_QUALITY_SIGNAL_CHIPS.find(c => c.id === 'unusual_length')
    expect(chip?.reasons).toEqual(['short_name', 'long_name'])
  })

  it('covers every reason across the chip set', () => {
    const covered = LOW_QUALITY_SIGNAL_CHIPS.flatMap(c => c.reasons).sort()
    expect(covered).toEqual(
      ['aging_unused', 'downvoted', 'long_name', 'orphaned', 'short_name'].sort()
    )
  })
})

describe('getCategoryColor', () => {
  it('returns a distinct class string per known category', () => {
    expect(getCategoryColor('genre')).toContain('blue')
    expect(getCategoryColor('locale')).toContain('cyan')
    expect(getCategoryColor('other')).toContain('zinc')
  })

  it('falls back to the "other" styling for an unknown category', () => {
    expect(getCategoryColor('mystery')).toBe(getCategoryColor('other'))
  })
})

describe('getCategoryLabel', () => {
  it('capitalizes the first letter', () => {
    expect(getCategoryLabel('genre')).toBe('Genre')
    expect(getCategoryLabel('locale')).toBe('Locale')
  })

  it('returns an empty string for empty input', () => {
    expect(getCategoryLabel('')).toBe('')
  })
})

describe('getEntityUrl', () => {
  it.each([
    ['artist', 'the-band', '/artists/the-band'],
    ['venue', 'the-club', '/venues/the-club'],
    ['show', 'a-show', '/shows/a-show'],
    ['release', 'an-album', '/releases/an-album'],
    ['label', 'a-label', '/labels/a-label'],
    ['festival', 'a-fest', '/festivals/a-fest'],
    ['collection', 'a-list', '/collections/a-list'],
  ])('builds the %s url', (type, slug, expected) => {
    expect(getEntityUrl(type, slug)).toBe(expected)
  })

  it('returns "#" for an unknown entity type', () => {
    expect(getEntityUrl('mixtape', 'whatever')).toBe('#')
  })
})

describe('getEntityTypePluralLabel', () => {
  it.each([
    ['artist', 'Artists'],
    ['venue', 'Venues'],
    ['show', 'Shows'],
    ['release', 'Releases'],
    ['label', 'Labels'],
    ['festival', 'Festivals'],
    ['collection', 'Collections'],
  ])('pluralizes %s', (type, expected) => {
    expect(getEntityTypePluralLabel(type)).toBe(expected)
  })

  it('returns the raw value for an unknown entity type', () => {
    expect(getEntityTypePluralLabel('mixtape')).toBe('mixtape')
  })
})
