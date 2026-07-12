import { describe, it, expect, vi } from 'vitest'

// Mock the timeUtils module
vi.mock('./timeUtils', () => ({
  getTimezoneForState: vi.fn((state: string) => {
    const map: Record<string, string> = {
      AZ: 'America/Phoenix',
      CA: 'America/Los_Angeles',
      NY: 'America/New_York',
    }
    return map[state.toUpperCase()] || 'America/Phoenix'
  }),
  // Honor the passed timezone (the real formatInTimezone does) so tests can
  // verify venue-tz threading, not just the hardcoded-Phoenix path.
  formatInTimezone: vi.fn(
    (dateString: string, timezone: string, options: Intl.DateTimeFormatOptions) => {
      return new Date(dateString).toLocaleString('en-US', {
        ...options,
        timeZone: timezone,
      })
    }
  ),
}))

import { formatShowDateBadge, formatShowMonthDay } from './showDateBadge'
import { formatInTimezone } from './timeUtils'

describe('formatShowDateBadge', () => {
  it('returns dayOfWeek and monthDay parts', () => {
    const result = formatShowDateBadge('2025-03-17T19:00:00Z')

    expect(result).toHaveProperty('dayOfWeek')
    expect(result).toHaveProperty('monthDay')
    expect(typeof result.dayOfWeek).toBe('string')
    expect(typeof result.monthDay).toBe('string')
  })

  it('defaults to AZ timezone when state is not provided or null', () => {
    const result1 = formatShowDateBadge('2025-03-17T19:00:00Z')
    const result2 = formatShowDateBadge('2025-03-17T19:00:00Z', null)

    // Both should produce the same result (AZ timezone)
    expect(result1).toEqual(result2)
  })

  it('prefers the venue timezone over state (PSY-986)', () => {
    // 05:30 UTC: Mar 14 in Phoenix vs Mar 15 in New York. Venue tz must win.
    const result = formatShowDateBadge('2026-03-15T05:30:00Z', 'NY', 'America/Phoenix')
    expect(result.monthDay).toBe('MAR 14')
    // State-only (no venue tz) renders the New York date instead.
    expect(formatShowDateBadge('2026-03-15T05:30:00Z', 'NY').monthDay).toBe('MAR 15')
  })

  it('uppercases dayOfWeek and month', () => {
    vi.mocked(formatInTimezone)
      .mockReturnValueOnce('Mon') // weekday
      .mockReturnValueOnce('Mar 17') // month/day

    const result = formatShowDateBadge('2025-03-17T19:00:00Z')

    expect(result.dayOfWeek).toBe('MON')
    expect(result.monthDay).toBe('MAR 17')
  })

  it('formats monthDay as "MONTH DAY"', () => {
    vi.mocked(formatInTimezone)
      .mockReturnValueOnce('Fri') // weekday
      .mockReturnValueOnce('Dec 1') // month/day

    const result = formatShowDateBadge('2025-12-01T03:00:00Z')

    expect(result.dayOfWeek).toBe('FRI')
    expect(result.monthDay).toBe('DEC 1')
  })
})

describe('formatShowMonthDay', () => {
  it('formats the compact label with one timezone-aware Intl call', () => {
    vi.mocked(formatInTimezone).mockReturnValueOnce('Jul 12')

    expect(
      formatShowMonthDay('2026-07-12T19:30:00Z', 'AZ', 'America/Phoenix')
    ).toBe('JUL 12')
    expect(formatInTimezone).toHaveBeenCalledTimes(1)
    expect(formatInTimezone).toHaveBeenCalledWith(
      '2026-07-12T19:30:00Z',
      'America/Phoenix',
      { month: 'short', day: 'numeric' }
    )
  })
})
