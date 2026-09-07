import { describe, it, expect } from 'vitest'
import {
  TAG_CATEGORIES,
  TAG_SORT_OPTIONS,
  DEFAULT_TAG_SORT,
  TAG_ENTITY_TYPES,
  LOW_QUALITY_REASON_LABELS,
  LOW_QUALITY_SIGNAL_CHIPS,
  FACET_TAG_CATEGORIES,
  getCategoryChipClasses,
  getCategoryLabel,
  getEntityUrl,
  getEntityTypePluralLabel,
} from './types'

describe('tag constants', () => {
  it('exposes the tag categories the backend allowlist accepts', () => {
    expect(TAG_CATEGORIES).toEqual(['genre', 'locale', 'other', 'crew'])
  })

  it('keeps crew out of the facet vocabulary and everything else in', () => {
    expect(FACET_TAG_CATEGORIES).toEqual(['genre', 'locale', 'other'])
    expect(FACET_TAG_CATEGORIES).not.toContain('crew')
  })

  it('maps each sort option to a backend slug', () => {
    expect(TAG_SORT_OPTIONS).toEqual([
      { value: 'popularity', label: 'Popularity', backend: 'usage' },
      { value: 'alphabetical', label: 'Alphabetical', backend: 'name' },
      { value: 'newest', label: 'Newest', backend: 'created' },
    ])
  })

  it('defaults sort to popularity', () => {
    expect(DEFAULT_TAG_SORT).toBe('popularity')
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

describe('getCategoryChipClasses', () => {
  it('binds each known category to a distinct DS chart token (PSY-943)', () => {
    expect(getCategoryChipClasses('genre')).toContain('text-chart-6')
    expect(getCategoryChipClasses('locale')).toContain('text-chart-8')
    expect(getCategoryChipClasses('other')).toContain('text-muted-foreground')
  })

  it('uses no raw off-palette Tailwind hue', () => {
    for (const cat of TAG_CATEGORIES) {
      expect(getCategoryChipClasses(cat)).not.toMatch(
        /(?:bg|text|border)-(?:blue|cyan|zinc)-\d/
      )
    }
  })

  it('gives crew an unfilled hairline square in mono uppercase (Figma 1402:789)', () => {
    const crew = getCategoryChipClasses('crew')
    expect(crew).toContain('bg-transparent')
    expect(crew).toContain('border-border')
    expect(crew).toContain('rounded-[2px]')
    expect(crew).toContain('font-mono')
    expect(crew).toContain('uppercase')
    expect(crew).toContain('tracking-[0.04em]')
    expect(crew).toContain('text-muted-foreground')
  })

  it('gives crew no colour fill, unlike every other category', () => {
    expect(getCategoryChipClasses('crew')).not.toMatch(/bg-(?:chart|muted|primary)/)
    expect(getCategoryChipClasses('crew')).not.toBe(getCategoryChipClasses('other'))
  })

  it('leads every category with its colour token so text-only callers can read it', () => {
    // TagBrowse.categoryTextTint takes the FIRST `text-*` class as the tint.
    for (const cat of TAG_CATEGORIES) {
      const first = getCategoryChipClasses(cat)
        .split(' ')
        .find(c => c.startsWith('text-'))
      expect(first).toMatch(/^text-(?:chart-\d|muted-foreground|foreground)$/)
    }
  })

  it('sets no font size, leaving density to the calling surface', () => {
    for (const cat of TAG_CATEGORIES) {
      expect(getCategoryChipClasses(cat)).not.toMatch(
        /\btext-(?:xs|sm|base|lg|\[\d)/
      )
    }
  })

  it('falls back to the "other" styling for an unknown category', () => {
    expect(getCategoryChipClasses('mystery')).toBe(getCategoryChipClasses('other'))
  })
})

describe('getCategoryLabel', () => {
  it('capitalizes the first letter', () => {
    expect(getCategoryLabel('genre')).toBe('Genre')
    expect(getCategoryLabel('locale')).toBe('Locale')
    expect(getCategoryLabel('crew')).toBe('Crew')
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
