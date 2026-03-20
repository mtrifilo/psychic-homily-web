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
import { getTimezoneForState, formatInTimezone } from './timeUtils'

describe('formatShowDateBadge', () => {
  it('returns dayOfWeek and monthDay parts', () => {
    const result = formatShowDateBadge('2025-03-17T19:00:00Z')

    expect(result).toHaveProperty('dayOfWeek')
    expect(result).toHaveProperty('monthDay')
    expect(typeof result.dayOfWeek).toBe('string')
    expect(typeof result.monthDay).toBe('string')
  })

  it('calls getTimezoneForState with the provided state', () => {
    formatShowDateBadge('2025-03-17T19:00:00Z', 'CA')

    expect(getTimezoneForState).toHaveBeenCalledWith('CA')
  })

  it('defaults to AZ when state is not provided', () => {
    formatShowDateBadge('2025-03-17T19:00:00Z')

    expect(getTimezoneForState).toHaveBeenCalledWith('AZ')
  })

  it('defaults to AZ when state is null', () => {
    formatShowDateBadge('2025-03-17T19:00:00Z', null)

    expect(getTimezoneForState).toHaveBeenCalledWith('AZ')
  })

  it('calls formatInTimezone three times (weekday, month, day)', () => {
    vi.mocked(formatInTimezone).mockClear()

    formatShowDateBadge('2025-03-17T19:00:00Z', 'AZ')

    expect(formatInTimezone).toHaveBeenCalledTimes(3)
    expect(formatInTimezone).toHaveBeenCalledWith(
      '2025-03-17T19:00:00Z',
      'America/Phoenix',
      { weekday: 'short' }
    )
    expect(formatInTimezone).toHaveBeenCalledWith(
      '2025-03-17T19:00:00Z',
      'America/Phoenix',
      { month: 'short' }
    )
    expect(formatInTimezone).toHaveBeenCalledWith(
      '2025-03-17T19:00:00Z',
      'America/Phoenix',
      { day: 'numeric' }
    )
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
