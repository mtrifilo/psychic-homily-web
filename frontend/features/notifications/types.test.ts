import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  NOTIFICATION_ENTITY_COMMENT_REPLY,
  NOTIFICATION_ENTITY_COMMENT_MENTION,
  isCommentNotification,
  NOTIFY_ENTITY_TYPES,
  formatTimeAgo,
  getFilterSummary,
  type NotificationLogEntry,
  type NotificationFilter,
} from './types'

function logEntry(overrides: Partial<NotificationLogEntry> = {}): NotificationLogEntry {
  return {
    id: 1,
    entity_type: 'show',
    entity_id: 10,
    channel: 'email',
    sent_at: '2026-05-19T12:00:00Z',
    ...overrides,
  }
}

function filter(overrides: Partial<NotificationFilter> = {}): NotificationFilter {
  return {
    id: 1,
    name: 'My filter',
    is_active: true,
    notify_email: true,
    notify_in_app: false,
    notify_push: false,
    match_count: 0,
    created_at: '2026-05-19T12:00:00Z',
    updated_at: '2026-05-19T12:00:00Z',
    ...overrides,
  }
}

describe('notification entity constants', () => {
  it('exposes the two PSY-595 comment row types', () => {
    expect(NOTIFICATION_ENTITY_COMMENT_REPLY).toBe('comment_reply')
    expect(NOTIFICATION_ENTITY_COMMENT_MENTION).toBe('comment_mention')
  })

  it('lists the quick-create entity types', () => {
    expect(NOTIFY_ENTITY_TYPES).toEqual(['artist', 'venue', 'label', 'tag'])
  })
})

describe('isCommentNotification', () => {
  it('returns true for a comment_reply row', () => {
    expect(
      isCommentNotification(logEntry({ entity_type: 'comment_reply' }))
    ).toBe(true)
  })

  it('returns true for a comment_mention row', () => {
    expect(
      isCommentNotification(logEntry({ entity_type: 'comment_mention' }))
    ).toBe(true)
  })

  it('returns false for a show-filter row', () => {
    expect(isCommentNotification(logEntry({ entity_type: 'show' }))).toBe(false)
  })

  it('returns false for an unrelated entity type', () => {
    expect(isCommentNotification(logEntry({ entity_type: 'venue' }))).toBe(
      false
    )
  })
})

describe('formatTimeAgo', () => {
  // Compares against Date.now(); pin the clock for deterministic strings.
  const NOW = new Date('2026-05-19T12:00:00Z')

  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(NOW)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  const SECOND = 1000
  const MINUTE = 60 * SECOND
  const HOUR = 60 * MINUTE
  const DAY = 24 * HOUR
  const WEEK = 7 * DAY

  function ago(ms: number): string {
    return formatTimeAgo(new Date(NOW.getTime() - ms).toISOString())
  }

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

  it('falls back to an absolute date at five weeks (no month branch)', () => {
    // This formatter has no "months ago" branch — once diffWeeks reaches 5 it
    // drops straight to a localized absolute date. Compare against the same
    // toLocaleDateString call so the assertion is timezone-independent.
    const old = new Date(NOW.getTime() - 5 * WEEK)
    expect(ago(5 * WEEK)).toBe(
      old.toLocaleDateString('en-US', {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
      })
    )
  })
})

describe('getFilterSummary', () => {
  it('returns "No criteria set" for an empty filter', () => {
    expect(getFilterSummary(filter())).toBe('No criteria set')
  })

  it('singularizes a single artist', () => {
    expect(getFilterSummary(filter({ artist_ids: [1] }))).toBe('1 artist')
  })

  it('pluralizes multiple of each entity', () => {
    expect(
      getFilterSummary(
        filter({ artist_ids: [1, 2], venue_ids: [3, 4, 5], label_ids: [6, 7] })
      )
    ).toBe('2 artists / 3 venues / 2 labels')
  })

  it('describes included and excluded tags', () => {
    expect(
      getFilterSummary(filter({ tag_ids: [1], exclude_tag_ids: [2, 3] }))
    ).toBe('1 tag / excluding 2 tags')
  })

  it('joins multiple cities with semicolons', () => {
    expect(
      getFilterSummary(
        filter({
          cities: [
            { city: 'Phoenix', state: 'AZ' },
            { city: 'Austin', state: 'TX' },
          ],
        })
      )
    ).toBe('Phoenix, AZ; Austin, TX')
  })

  it('renders "free only" when the price cap is zero', () => {
    expect(getFilterSummary(filter({ price_max_cents: 0 }))).toBe('free only')
  })

  it('formats a non-zero price cap as whole dollars', () => {
    expect(getFilterSummary(filter({ price_max_cents: 2500 }))).toBe('max $25')
  })

  it('treats null/empty arrays as unset criteria', () => {
    expect(
      getFilterSummary(
        filter({ artist_ids: [], venue_ids: null, price_max_cents: null })
      )
    ).toBe('No criteria set')
  })
})
