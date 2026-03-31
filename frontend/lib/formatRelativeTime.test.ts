import { describe, it, expect, vi, afterEach } from 'vitest'
import { formatRelativeTime } from './formatRelativeTime'

describe('formatRelativeTime', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('returns "just now" for timestamps less than 60 seconds ago', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-30T12:00:30Z'))

    expect(formatRelativeTime('2026-03-30T12:00:00Z')).toBe('just now')
  })

  it('returns minutes ago for timestamps within the last hour', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-30T12:05:00Z'))

    expect(formatRelativeTime('2026-03-30T12:00:00Z')).toBe('5 minutes ago')
  })

  it('returns singular minute', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-30T12:01:30Z'))

    expect(formatRelativeTime('2026-03-30T12:00:00Z')).toBe('1 minute ago')
  })

  it('returns hours ago for timestamps within the last day', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-30T15:00:00Z'))

    expect(formatRelativeTime('2026-03-30T12:00:00Z')).toBe('3 hours ago')
  })

  it('returns singular hour', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-30T13:00:00Z'))

    expect(formatRelativeTime('2026-03-30T12:00:00Z')).toBe('1 hour ago')
  })

  it('returns days ago for timestamps within the last month', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-04-02T12:00:00Z'))

    expect(formatRelativeTime('2026-03-30T12:00:00Z')).toBe('3 days ago')
  })

  it('returns a formatted date for timestamps older than 30 days', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-05-15T12:00:00Z'))

    const result = formatRelativeTime('2026-03-30T12:00:00Z')
    expect(result).toContain('Mar')
    expect(result).toContain('30')
    expect(result).toContain('2026')
  })

  // The core bug fix: timestamps without timezone suffix must be treated as UTC
  it('treats timestamps without Z suffix as UTC (the core timezone fix)', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-30T12:00:30Z'))

    // Without the fix, this would be parsed as local time, causing a
    // multi-hour offset for users in non-UTC timezones
    expect(formatRelativeTime('2026-03-30T12:00:00')).toBe('just now')
  })

  it('handles timestamps with +00:00 offset', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-30T12:00:30Z'))

    expect(formatRelativeTime('2026-03-30T12:00:00+00:00')).toBe('just now')
  })

  // Short format tests
  describe('short format', () => {
    it('returns "just now" for very recent timestamps', () => {
      vi.useFakeTimers()
      vi.setSystemTime(new Date('2026-03-30T12:00:30Z'))

      expect(formatRelativeTime('2026-03-30T12:00:00Z', { short: true })).toBe('just now')
    })

    it('returns abbreviated minutes', () => {
      vi.useFakeTimers()
      vi.setSystemTime(new Date('2026-03-30T12:05:00Z'))

      expect(formatRelativeTime('2026-03-30T12:00:00Z', { short: true })).toBe('5m ago')
    })

    it('returns abbreviated hours', () => {
      vi.useFakeTimers()
      vi.setSystemTime(new Date('2026-03-30T15:00:00Z'))

      expect(formatRelativeTime('2026-03-30T12:00:00Z', { short: true })).toBe('3h ago')
    })

    it('returns abbreviated days', () => {
      vi.useFakeTimers()
      vi.setSystemTime(new Date('2026-04-02T12:00:00Z'))

      expect(formatRelativeTime('2026-03-30T12:00:00Z', { short: true })).toBe('3d ago')
    })

    it('returns abbreviated weeks', () => {
      vi.useFakeTimers()
      vi.setSystemTime(new Date('2026-04-13T12:00:00Z'))

      expect(formatRelativeTime('2026-03-30T12:00:00Z', { short: true })).toBe('2w ago')
    })

    it('treats timestamps without Z suffix as UTC in short format', () => {
      vi.useFakeTimers()
      vi.setSystemTime(new Date('2026-03-30T12:00:30Z'))

      expect(formatRelativeTime('2026-03-30T12:00:00', { short: true })).toBe('just now')
    })
  })
})
