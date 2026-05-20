import { describe, it, expect } from 'vitest'
import {
  getReleaseTypeLabel,
  RELEASE_TYPE_LABELS,
  RELEASE_TYPES,
  RELEASE_SORT_OPTIONS,
} from './types'
import type { ReleaseType, ReleaseSortOption } from './types'

describe('getReleaseTypeLabel', () => {
  it('maps every known release type to its display label', () => {
    const expected: Record<ReleaseType, string> = {
      lp: 'LP',
      ep: 'EP',
      single: 'Single',
      compilation: 'Compilation',
      live: 'Live',
      remix: 'Remix',
      demo: 'Demo',
    }
    for (const [type, label] of Object.entries(expected)) {
      expect(getReleaseTypeLabel(type)).toBe(label)
    }
  })

  it('falls back to the uppercased value for an unknown type', () => {
    expect(getReleaseTypeLabel('mixtape')).toBe('MIXTAPE')
  })

  it('uppercases an unknown lowercase string', () => {
    expect(getReleaseTypeLabel('split')).toBe('SPLIT')
  })

  it('returns an empty string unchanged (uppercase of empty is empty)', () => {
    expect(getReleaseTypeLabel('')).toBe('')
  })
})

describe('RELEASE_TYPES / RELEASE_TYPE_LABELS', () => {
  it('lists every type that has a label, with no extras', () => {
    expect([...RELEASE_TYPES].sort()).toEqual(
      Object.keys(RELEASE_TYPE_LABELS).sort()
    )
  })

  it('has a non-empty label for every type in the list', () => {
    for (const type of RELEASE_TYPES) {
      expect(RELEASE_TYPE_LABELS[type]).toBeTruthy()
    }
  })
})

describe('RELEASE_SORT_OPTIONS', () => {
  it('exposes the five browse sort options with stable values', () => {
    const values = RELEASE_SORT_OPTIONS.map(o => o.value)
    const expected: ReleaseSortOption[] = [
      'newest',
      'oldest',
      'title_asc',
      'title_desc',
      'recently_added',
    ]
    expect(values).toEqual(expected)
  })

  it('pairs every value with a human label', () => {
    for (const option of RELEASE_SORT_OPTIONS) {
      expect(option.label.length).toBeGreaterThan(0)
    }
  })
})
