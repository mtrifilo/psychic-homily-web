import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  NOTIFICATION_ENTITY_COMMENT_REPLY,
  NOTIFICATION_ENTITY_COMMENT_MENTION,
  NOTIFICATION_ENTITY_REQUEST_FULFILLMENT_PROPOSED,
  isCommentNotification,
  isRequestNotification,
  NOTIFY_ENTITY_TYPES,
  formatTimeAgo,
  getFilterSummary,
  toRelativeIfSameOrigin,
  normalizeNotificationDeepLinks,
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
    source: 'user',
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

  it('exposes the PSY-890 request-fulfillment row type', () => {
    expect(NOTIFICATION_ENTITY_REQUEST_FULFILLMENT_PROPOSED).toBe(
      'request_fulfillment_proposed'
    )
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

  it('returns false for a request-fulfillment row', () => {
    expect(
      isCommentNotification(
        logEntry({ entity_type: 'request_fulfillment_proposed' })
      )
    ).toBe(false)
  })
})

describe('isRequestNotification', () => {
  it('returns true for a request_fulfillment_proposed row', () => {
    expect(
      isRequestNotification(
        logEntry({ entity_type: 'request_fulfillment_proposed' })
      )
    ).toBe(true)
  })

  it('returns false for a comment row', () => {
    expect(
      isRequestNotification(logEntry({ entity_type: 'comment_reply' }))
    ).toBe(false)
  })

  it('returns false for a show-filter row', () => {
    expect(isRequestNotification(logEntry({ entity_type: 'show' }))).toBe(false)
  })
})

describe('toRelativeIfSameOrigin', () => {
  it('rewrites same-origin absolute URLs to path + search + hash', () => {
    const href = `${window.location.origin}/shows/the-show?tab=comments#comment-9`
    expect(toRelativeIfSameOrigin(href)).toBe(
      '/shows/the-show?tab=comments#comment-9'
    )
  })

  it('passes through cross-origin, relative, and malformed URLs', () => {
    expect(toRelativeIfSameOrigin('https://example.com/x#y')).toBe(
      'https://example.com/x#y'
    )
    expect(toRelativeIfSameOrigin('/requests')).toBe('/requests')
    expect(toRelativeIfSameOrigin('http://')).toBe('http://')
  })
})

describe('normalizeNotificationDeepLinks', () => {
  it('normalizes comment_url and request_url, returning the same entry when unchanged', () => {
    const sameOrigin = logEntry({
      comment_url: `${window.location.origin}/venues/v#comment-3`,
    })
    expect(normalizeNotificationDeepLinks(sameOrigin).comment_url).toBe(
      '/venues/v#comment-3'
    )
    const untouched = logEntry({ comment_url: 'https://example.com/x' })
    expect(normalizeNotificationDeepLinks(untouched)).toBe(untouched)
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

  // Smoke test — full behavior is covered in lib/formatTimeAgo.test.ts. The
  // notifications module re-exports the shared helper (PSY-780); these cases
  // pin the integration so a future divergence is caught here too.
  it('returns "just now" under a minute', () => {
    expect(ago(30 * SECOND)).toBe('just now')
  })

  it('pluralizes weeks', () => {
    expect(ago(3 * WEEK)).toBe('3 weeks ago')
  })

  it('rolls over to months at 35 days (PSY-780 consolidation)', () => {
    // Previously the notifications copy dropped to an absolute date at 5 weeks;
    // the consolidated helper renders "1 month ago" at 35 days, matching the
    // requests-feature behavior.
    expect(ago(35 * DAY)).toBe('1 month ago')
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
