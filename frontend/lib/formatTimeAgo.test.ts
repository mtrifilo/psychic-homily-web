import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { formatTimeAgo } from './formatTimeAgo'

describe('formatTimeAgo (canonical PSY-780)', () => {
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

  it('singularizes one month at 35 days', () => {
    // Week branch caps at < 5 weeks (35 days), so the months branch kicks in
    // at exactly 5 weeks rather than at 30 days.
    expect(ago(35 * DAY)).toBe('1 month ago')
  })

  it('pluralizes months', () => {
    expect(ago(90 * DAY)).toBe('3 months ago')
  })

  it('falls back to a localized absolute date past 12 months', () => {
    const old = new Date(NOW.getTime() - 400 * DAY)
    expect(ago(400 * DAY)).toBe(
      old.toLocaleDateString('en-US', {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
      })
    )
  })

  // PSY-780: the previous duplicates parsed `new Date(dateString)` directly,
  // which treats a timezone-less timestamp as LOCAL time. The consolidated
  // helper routes through `ensureUTC`, matching the PSY-255 fix in
  // `formatRelativeTime`.
  it('treats timestamps without Z suffix as UTC (PSY-255 parity)', () => {
    expect(formatTimeAgo('2026-05-19T11:59:45')).toBe('just now')
  })

  it('handles timestamps with +00:00 offset', () => {
    expect(formatTimeAgo('2026-05-19T11:59:45+00:00')).toBe('just now')
  })
})
