import { describe, it, expect, vi } from 'vitest'
import {
  formatShowDate,
  formatShowDayMonth,
  formatShowTime,
  formatPrice,
  formatContentDate,
  formatAdminDate,
  formatAdminTime,
  formatShortDate,
  formatTimestamp,
  isShowTimezoneResolved,
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
  hasTimezoneForState: (state?: string | null) =>
    !!state && ['AZ', 'CA', 'NY'].includes(state.toUpperCase()),
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
  formatInTimezone: (
    dateStr: string,
    tz: string,
    options: Intl.DateTimeFormatOptions,
  ) => new Intl.DateTimeFormat('en-US', { ...options, timeZone: tz }).format(new Date(dateStr)),
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

describe('formatShowDayMonth', () => {
  const utcDate = '2026-03-15T02:30:00Z' // Mar 14 7:30 PM in Phoenix

  it('drops the weekday formatShowDate carries', () => {
    expect(formatShowDayMonth(utcDate, 'AZ')).toBe('Mar 14')
  })

  it('reads the date on the venue’s clock, not the runtime’s', () => {
    // The same instant is already Mar 15 in London. A rail beside a venue's
    // name must print the venue's date or it contradicts the row it labels.
    expect(formatShowDayMonth(utcDate, null, 'Europe/London')).toBe('Mar 15')
  })

  it('falls back to the state map when the venue has no resolved zone', () => {
    // 02:30 UTC is Mar 14 10:30 PM in New York.
    expect(formatShowDayMonth(utcDate, 'NY')).toBe('Mar 14')
  })

  it('ignores a malformed zone rather than throwing mid-render', () => {
    expect(formatShowDayMonth(utcDate, 'AZ', 'Not/AZone')).toBe('Mar 14')
  })
})

describe('formatShowTime', () => {
  const utcDate = '2026-03-15T02:30:00Z'

  it('defaults to AZ timezone', () => {
    // 02:30 UTC = 7:30 PM in America/Phoenix (UTC-7, no DST)
    const result = formatShowTime(utcDate)
    expect(result).toBe('7:30 PM')
  })

  it('respects explicit state timezone', () => {
    const resultAZ = formatShowTime(utcDate, 'AZ')
    const resultNY = formatShowTime(utcDate, 'NY')
    // 02:30 UTC on Mar 15 = 7:30 PM in Phoenix (UTC-7), 10:30 PM in New York (UTC-4 DST)
    expect(resultAZ).toBe('7:30 PM')
    expect(resultNY).toBe('10:30 PM')
    expect(resultAZ).not.toBe(resultNY)
  })
})

describe('venue timezone preference (PSY-986)', () => {
  // 05:30 UTC crosses the calendar boundary: Mar 14 10:30 PM in Phoenix (UTC-7)
  // vs Mar 15 1:30 AM in New York (UTC-4, DST active by Mar 15) — so the
  // assertions actually discriminate venue-tz from the state fallback.
  const boundaryUtc = '2026-03-15T05:30:00Z'

  it('formatShowTime prefers the explicit venue timezone over state', () => {
    expect(formatShowTime(boundaryUtc, 'NY', 'America/Phoenix')).toBe('10:30 PM')
    expect(formatShowTime(boundaryUtc, 'NY')).toBe('1:30 AM') // state fallback differs
  })

  it('formatShowDate prefers the venue timezone over state across a date boundary', () => {
    // Phoenix → Mar 14; New York → Mar 15. The venue tz (Phoenix) must win.
    expect(formatShowDate(boundaryUtc, 'NY', false, 'America/Phoenix')).toContain('14')
    expect(formatShowDate(boundaryUtc, 'NY')).toContain('15')
  })

  it('falls back to the state map when no venue timezone is given', () => {
    expect(formatShowTime(boundaryUtc, 'AZ')).toBe('10:30 PM')
  })

  it('falls back to the state map when the venue timezone is malformed (no crash)', () => {
    expect(formatShowTime(boundaryUtc, 'AZ', 'Not/AZone')).toBe('10:30 PM')
  })
})

describe('isShowTimezoneResolved', () => {
  // The point of the predicate: tell a zone we know from the Arizona default
  // that `resolveShowTimezone` hands back for everything it does not.
  it('is true for a venue carrying its own valid timezone', () => {
    expect(isShowTimezoneResolved(null, 'Asia/Tokyo')).toBe(true)
  })

  it('is true for a US state the map knows, with no venue timezone', () => {
    expect(isShowTimezoneResolved('NY', null)).toBe(true)
  })

  it('is false for a region the map does not know', () => {
    expect(isShowTimezoneResolved('Tokyo', null)).toBe(false)
    expect(isShowTimezoneResolved(null, null)).toBe(false)
  })

  // A malformed zone is not knowledge, and `resolveShowTimezone` discards it
  // too. Both must agree, or a caller would print a time from the fallback
  // believing it had a real one.
  it('is false for a malformed timezone with no usable state', () => {
    expect(isShowTimezoneResolved('Tokyo', 'Not/AZone')).toBe(false)
  })

  it('accepts a known state even when the timezone is malformed', () => {
    expect(isShowTimezoneResolved('AZ', 'Not/AZone')).toBe(true)
  })
})

describe('formatPrice', () => {
  it('formats integer price', () => {
    expect(formatPrice(20)).toBe('$20.00')
  })

  it('formats decimal price', () => {
    expect(formatPrice(15.5)).toBe('$15.50')
  })

  it('formats zero as Free', () => {
    expect(formatPrice(0)).toBe('Free')
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
  it('formats time with AM/PM matching local timezone', () => {
    const input = '2026-01-15T19:30:00Z'
    const result = formatAdminTime(input)
    // Verify minutes are preserved (invariant across timezones)
    expect(result).toContain(':30')
    // Verify it matches the expected local-timezone conversion
    const expected = new Date(input).toLocaleTimeString('en-US', {
      hour: 'numeric',
      minute: '2-digit',
      hour12: true,
    })
    expect(result).toBe(expected)
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
