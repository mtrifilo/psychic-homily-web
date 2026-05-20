import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  REQUEST_ENTITY_TYPES,
  REQUEST_STATUSES,
  REQUEST_SORT_OPTIONS,
  getEntityTypeLabel,
  getStatusLabel,
  getEntityTypeColor,
  getStatusColor,
  getEntityUrl,
  formatTimeAgo,
  formatDate,
} from './types'

describe('request type constants', () => {
  it('exposes the six supported entity types', () => {
    expect(REQUEST_ENTITY_TYPES).toEqual([
      'artist',
      'release',
      'label',
      'show',
      'venue',
      'festival',
    ])
  })

  it('exposes the five lifecycle statuses', () => {
    expect(REQUEST_STATUSES).toEqual([
      'pending',
      'in_progress',
      'fulfilled',
      'rejected',
      'cancelled',
    ])
  })

  it('exposes the three sort options', () => {
    expect(REQUEST_SORT_OPTIONS).toEqual(['votes', 'newest', 'oldest'])
  })
})

describe('getEntityTypeLabel', () => {
  it.each([
    ['artist', 'Artist'],
    ['venue', 'Venue'],
    ['show', 'Show'],
    ['release', 'Release'],
    ['label', 'Label'],
    ['festival', 'Festival'],
  ])('maps %s to %s', (input, expected) => {
    expect(getEntityTypeLabel(input)).toBe(expected)
  })

  it('returns the raw value for an unknown type', () => {
    expect(getEntityTypeLabel('mixtape')).toBe('mixtape')
  })
})

describe('getStatusLabel', () => {
  it.each([
    ['pending', 'Pending'],
    ['in_progress', 'In Progress'],
    ['fulfilled', 'Fulfilled'],
    ['rejected', 'Rejected'],
    ['cancelled', 'Cancelled'],
  ])('maps %s to %s', (input, expected) => {
    expect(getStatusLabel(input)).toBe(expected)
  })

  it('returns the raw value for an unknown status', () => {
    expect(getStatusLabel('archived')).toBe('archived')
  })
})

describe('getEntityTypeColor', () => {
  it('returns a distinct class for each known entity type', () => {
    const colors = REQUEST_ENTITY_TYPES.map(getEntityTypeColor)
    // Every known type maps to a unique, non-muted class.
    expect(new Set(colors).size).toBe(REQUEST_ENTITY_TYPES.length)
    colors.forEach(c => expect(c).not.toContain('text-muted-foreground'))
  })

  it('falls back to muted styling for an unknown type', () => {
    expect(getEntityTypeColor('unknown')).toBe(
      'bg-muted text-muted-foreground'
    )
  })
})

describe('getStatusColor', () => {
  it('returns a yellow class for pending', () => {
    expect(getStatusColor('pending')).toContain('yellow')
  })

  it('returns a green class for fulfilled', () => {
    expect(getStatusColor('fulfilled')).toContain('green')
  })

  it('returns a red class for rejected', () => {
    expect(getStatusColor('rejected')).toContain('red')
  })

  it('falls back to muted styling for cancelled and unknown', () => {
    expect(getStatusColor('cancelled')).toBe('bg-muted text-muted-foreground')
    expect(getStatusColor('whatever')).toBe('bg-muted text-muted-foreground')
  })
})

describe('getEntityUrl', () => {
  it.each([
    ['artist', 7, '/artists/7'],
    ['venue', 7, '/venues/7'],
    ['show', 7, '/shows/7'],
    ['release', 7, '/releases/7'],
    ['label', 7, '/labels/7'],
    ['festival', 7, '/festivals/7'],
  ])('builds %s url', (type, id, expected) => {
    expect(getEntityUrl(type, id)).toBe(expected)
  })

  it('returns "#" for an unknown entity type', () => {
    expect(getEntityUrl('mixtape', 7)).toBe('#')
  })
})

describe('formatDate', () => {
  it('formats an ISO date as "Mon D, YYYY"', () => {
    // Use midday UTC so the date does not roll backward in negative-offset
    // test environments.
    expect(formatDate('2026-01-05T12:00:00Z')).toBe('Jan 5, 2026')
  })
})

describe('formatTimeAgo', () => {
  // formatTimeAgo compares against Date.now(); pin the clock so the relative
  // strings are deterministic regardless of when the suite runs.
  const NOW = new Date('2026-05-19T12:00:00Z')

  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(NOW)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  function ago(ms: number): string {
    return formatTimeAgo(new Date(NOW.getTime() - ms).toISOString())
  }

  const SECOND = 1000
  const MINUTE = 60 * SECOND
  const HOUR = 60 * MINUTE
  const DAY = 24 * HOUR
  const WEEK = 7 * DAY

  it('returns "just now" under a minute', () => {
    expect(ago(30 * SECOND)).toBe('just now')
  })

  it('singularizes one minute', () => {
    expect(ago(MINUTE)).toBe('1 minute ago')
  })

  it('pluralizes minutes', () => {
    expect(ago(5 * MINUTE)).toBe('5 minutes ago')
  })

  it('singularizes one hour', () => {
    expect(ago(HOUR)).toBe('1 hour ago')
  })

  it('pluralizes hours', () => {
    expect(ago(3 * HOUR)).toBe('3 hours ago')
  })

  it('singularizes one day', () => {
    expect(ago(DAY)).toBe('1 day ago')
  })

  it('pluralizes days', () => {
    expect(ago(3 * DAY)).toBe('3 days ago')
  })

  it('singularizes one week', () => {
    expect(ago(WEEK)).toBe('1 week ago')
  })

  it('pluralizes weeks', () => {
    expect(ago(3 * WEEK)).toBe('3 weeks ago')
  })

  it('singularizes one month', () => {
    // The month branch is only reachable once the week count reaches 5
    // (diffWeeks < 5 short-circuits first), so 35 days is the floor for
    // "1 month ago" rather than 31.
    expect(ago(35 * DAY)).toBe('1 month ago')
  })

  it('pluralizes months', () => {
    expect(ago(90 * DAY)).toBe('3 months ago')
  })

  it('falls back to an absolute date past a year', () => {
    // ~13 months ago — beyond the relative window, so it returns formatDate().
    expect(ago(400 * DAY)).toBe(formatDate(new Date(NOW.getTime() - 400 * DAY).toISOString()))
  })
})
