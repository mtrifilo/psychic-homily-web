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
  formatInTimezone: vi.fn(
    (dateString: string, _timezone: string, options: Intl.DateTimeFormatOptions) => {
      const date = new Date(dateString)
      if (options.weekday === 'short') return date.toLocaleString('en-US', { weekday: 'short', timeZone: 'America/Phoenix' })
      if (options.month === 'short') return date.toLocaleString('en-US', { month: 'short', timeZone: 'America/Phoenix' })
      if (options.day === 'numeric') return date.toLocaleString('en-US', { day: 'numeric', timeZone: 'America/Phoenix' })
      return ''
    }
  ),
}))

import { formatShowDateBadge } from './showDateBadge'
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

  it('uppercases dayOfWeek and month', () => {
    vi.mocked(formatInTimezone)
      .mockReturnValueOnce('Mon') // weekday
      .mockReturnValueOnce('Mar') // month
      .mockReturnValueOnce('17') // day

    const result = formatShowDateBadge('2025-03-17T19:00:00Z')

    expect(result.dayOfWeek).toBe('MON')
    expect(result.monthDay).toBe('MAR 17')
  })

  it('formats monthDay as "MONTH DAY"', () => {
    vi.mocked(formatInTimezone)
      .mockReturnValueOnce('Fri') // weekday
      .mockReturnValueOnce('Dec') // month
      .mockReturnValueOnce('1') // day

    const result = formatShowDateBadge('2025-12-01T03:00:00Z')

    expect(result.dayOfWeek).toBe('FRI')
    expect(result.monthDay).toBe('DEC 1')
  })
})
