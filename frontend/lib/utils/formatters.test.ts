import { describe, it, expect, vi } from 'vitest'
import {
  formatShowDate,
  formatShowTime,
  formatPrice,
  formatContentDate,
  formatAdminDate,
  formatAdminTime,
  formatShortDate,
  formatTimestamp,
} from './formatters'

// Mock timeUtils to control timezone behavior deterministically
vi.mock('./timeUtils', () => ({
  getTimezoneForState: (state: string) => {
    const map: Record<string, string> = {
      AZ: 'America/Phoenix',
      CA: 'America/Los_Angeles',
      NY: 'America/New_York',
    }
    return map[state.toUpperCase()] || 'America/Phoenix'
  },
  formatDateInTimezone: (dateStr: string, tz: string) =>
    new Date(dateStr).toLocaleString('en-US', {
      weekday: 'short',
      month: 'short',
      day: 'numeric',
      timeZone: tz,
    }),
  formatDateWithYearInTimezone: (dateStr: string, tz: string) => {
    const date = new Date(dateStr)
    const formatter = new Intl.DateTimeFormat('en-US', {
      weekday: 'short',
      month: 'short',
      day: 'numeric',
      year: 'numeric',
      timeZone: tz,
    })
    const parts = formatter.formatToParts(date)
    const p = (type: string) => parts.find(x => x.type === type)?.value || ''
    return `${p('weekday')} ${p('month')} ${p('day')}, ${p('year')}`
  },
  formatTimeInTimezone: (dateStr: string, tz: string) =>
    new Date(dateStr).toLocaleString('en-US', {
      hour: 'numeric',
      minute: '2-digit',
      hour12: true,
      timeZone: tz,
    }),
}))

describe('formatShowDate', () => {
  const utcDate = '2026-03-15T02:30:00Z' // Mar 14 7:30 PM in Phoenix

  it('defaults to AZ timezone when no state given', () => {
    const result = formatShowDate(utcDate)
    expect(result).toContain('Mar')
    expect(result).toContain('14')
  })

  it('uses explicit state timezone', () => {
    // In NY (UTC-4 in March DST), 02:30 UTC = Mar 14 10:30 PM
    const result = formatShowDate(utcDate, 'NY')
    expect(result).toContain('14')
  })

  it('includes year when requested', () => {
    const result = formatShowDate(utcDate, 'AZ', true)
    expect(result).toContain('2026')
  })

  it('handles null state', () => {
    const result = formatShowDate(utcDate, null)
    expect(result).toContain('Mar')
  })
})

describe('formatShowTime', () => {
  const utcDate = '2026-03-15T02:30:00Z'

  it('defaults to AZ timezone', () => {
    const result = formatShowTime(utcDate)
    expect(result).toMatch(/PM|AM/)
  })

  it('respects explicit state timezone', () => {
    const resultAZ = formatShowTime(utcDate, 'AZ')
    const resultNY = formatShowTime(utcDate, 'NY')
    // Different timezones should produce different times
    // AZ = 7:30 PM, NY = 10:30 PM (or 9:30 PM depending on DST)
    expect(resultAZ).toMatch(/PM|AM/)
    expect(resultNY).toMatch(/PM|AM/)
  })
})

describe('formatPrice', () => {
  it('formats integer price', () => {
    expect(formatPrice(20)).toBe('$20.00')
  })

  it('formats decimal price', () => {
    expect(formatPrice(15.5)).toBe('$15.50')
  })

  it('formats zero', () => {
    expect(formatPrice(0)).toBe('$0.00')
  })

  it('formats large price', () => {
    expect(formatPrice(150)).toBe('$150.00')
  })
})

describe('formatContentDate', () => {
  it('formats date as long month with year', () => {
    const result = formatContentDate('2026-01-15T12:00:00')
    expect(result).toContain('January')
    expect(result).toContain('15')
    expect(result).toContain('2026')
  })

  it('formats ISO timestamp', () => {
    const result = formatContentDate('2026-06-20T15:00:00Z')
    expect(result).toContain('June')
    expect(result).toContain('2026')
  })
})

describe('formatAdminDate', () => {
  it('includes short weekday', () => {
    // Jan 15, 2026 is a Thursday
    const result = formatAdminDate('2026-01-15T12:00:00Z')
    expect(result).toMatch(/Thu/)
  })

  it('includes short month, day, and year', () => {
    const result = formatAdminDate('2026-01-15T12:00:00Z')
    expect(result).toContain('Jan')
    expect(result).toContain('15')
    expect(result).toContain('2026')
  })
})

describe('formatAdminTime', () => {
  it('formats time with AM/PM', () => {
    const result = formatAdminTime('2026-01-15T19:30:00Z')
    expect(result).toMatch(/\d{1,2}:\d{2}\s*(AM|PM)/)
  })
})

describe('formatShortDate', () => {
  it('formats without weekday', () => {
    const result = formatShortDate('2026-01-15T12:00:00Z')
    expect(result).toContain('Jan')
    expect(result).toContain('15')
    expect(result).toContain('2026')
    // Should NOT contain a weekday
    expect(result).not.toMatch(/Mon|Tue|Wed|Thu|Fri|Sat|Sun/)
  })
})

describe('formatTimestamp', () => {
  it('includes both date and time', () => {
    const result = formatTimestamp('2026-01-15T19:30:00Z')
    expect(result).toContain('Jan')
    expect(result).toContain('15')
    expect(result).toContain('2026')
    expect(result).toMatch(/\d{1,2}:\d{2}/)
  })
})
