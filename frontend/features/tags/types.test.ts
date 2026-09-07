import { describe, it, expect } from 'vitest'
import {
  TAG_CATEGORIES,
  TAG_SORT_OPTIONS,
  DEFAULT_TAG_SORT,
  TAG_ENTITY_TYPES,
  LOW_QUALITY_REASON_LABELS,
  LOW_QUALITY_SIGNAL_CHIPS,
  FACET_TAG_CATEGORIES,
  isDescriptiveTagCategory,
  getCategoryChipClasses,
  getCategoryTint,
  getTagChipClasses,
  getCategoryLabel,
  getEntityUrl,
  getEntityTypePluralLabel,
} from './types'

describe('tag constants', () => {
  it('lists the four tag categories the UI knows', () => {
    expect(TAG_CATEGORIES).toEqual(['genre', 'locale', 'other', 'crew'])
  })

  it('keeps crew out of the facet vocabulary and everything else in', () => {
    expect(FACET_TAG_CATEGORIES).toEqual(['genre', 'locale', 'other'])
    expect(FACET_TAG_CATEGORIES).not.toContain('crew')
  })
})

describe('isDescriptiveTagCategory', () => {
  it('admits every category except crew', () => {
    expect(isDescriptiveTagCategory('genre')).toBe(true)
    expect(isDescriptiveTagCategory('locale')).toBe(true)
    expect(isDescriptiveTagCategory('other')).toBe(true)
    expect(isDescriptiveTagCategory('crew')).toBe(false)
  })

  it('admits a category this build does not know', () => {
    // A category the server adds before the frontend ships must render,
    // not vanish from every row that consults this.
    expect(isDescriptiveTagCategory('era')).toBe(true)
    expect(isDescriptiveTagCategory('')).toBe(true)
  })

  it('excludes crew whatever its casing or padding', () => {
    // tags.category is an unconstrained column: a guard that only knows the
    // exact lowercase spelling is one seeder away from leaking.
    for (const variant of ['Crew', 'CREW', ' crew ', 'cReW']) {
      expect(isDescriptiveTagCategory(variant)).toBe(false)
    }
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
    expect(getCategoryChipClasses('crew')).toBe(
      'bg-transparent text-muted-foreground border-border rounded-[2px] font-mono uppercase tracking-[0.04em]'
    )
  })

  it('gives crew no colour fill, so it is not another tint among the tints', () => {
    expect(getCategoryChipClasses('crew')).not.toMatch(/bg-(?:chart|muted|primary)/)
    expect(getCategoryChipClasses('crew')).not.toBe(getCategoryChipClasses('other'))
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

  it('falls back for a category name that collides with an object member', () => {
    // The category is read straight out of the database, so it can be any
    // string; an object-index lookup would answer these from the prototype.
    for (const key of ['toString', 'constructor', 'hasOwnProperty', '__proto__']) {
      expect(getCategoryChipClasses(key)).toBe(getCategoryChipClasses('other'))
    }
  })
})

describe('getCategoryTint', () => {
  it('returns the colour token alone, with no chip shape attached', () => {
    expect(getCategoryTint('genre')).toBe('text-chart-6')
    expect(getCategoryTint('locale')).toBe('text-chart-8')
    expect(getCategoryTint('other')).toBe('text-muted-foreground')
    // Crew's identity is its shape; a text-only surface takes the tint only.
    expect(getCategoryTint('crew')).toBe('text-muted-foreground')
  })

  it('never returns a shape or layout class', () => {
    for (const cat of TAG_CATEGORIES) {
      expect(getCategoryTint(cat)).toMatch(/^text-\S+$/)
    }
  })

  it('falls back to the "other" tint for an unknown category', () => {
    expect(getCategoryTint('mystery')).toBe(getCategoryTint('other'))
    expect(getCategoryTint('toString')).toBe('text-muted-foreground')
  })
})

describe('getTagChipClasses', () => {
  it('swaps a tint-only category for the official accent', () => {
    for (const cat of ['genre', 'locale', 'other']) {
      expect(getTagChipClasses({ category: cat, is_official: true })).toBe(
        'border-primary/40 bg-primary/10 text-foreground'
      )
    }
  })

  it('keeps crew classes on an official crew tag', () => {
    // Crew is admin-minted, so nearly every crew tag is official. The accent
    // is the same pill an official genre tag wears, so taking it would erase
    // the only thing that tells a booker from a sound.
    expect(getTagChipClasses({ category: 'crew', is_official: true })).toBe(
      getCategoryChipClasses('crew')
    )
  })

  it('uses the category classes for any unofficial tag', () => {
    expect(getTagChipClasses({ category: 'genre', is_official: false })).toBe(
      'bg-chart-6/10 text-chart-6 border-chart-6/20'
    )
    for (const cat of TAG_CATEGORIES) {
      expect(getTagChipClasses({ category: cat, is_official: false })).toBe(
        getCategoryChipClasses(cat)
      )
    }
  })

  it('gives an unknown official category the accent, like the "other" it falls back to', () => {
    expect(getTagChipClasses({ category: 'era', is_official: true })).toBe(
      getTagChipClasses({ category: 'other', is_official: true })
    )
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
