import { describe, it, expect } from 'vitest'
import {
  getTimezoneForState,
  combineDateTimeToUTC,
  formatInTimezone,
  formatDateInTimezone,
  formatTimeInTimezone,
  parseISOToDateAndTime,
} from './timeUtils'

describe('getTimezoneForState', () => {
  it('returns correct timezone for known states', () => {
    expect(getTimezoneForState('AZ')).toBe('America/Phoenix')
    expect(getTimezoneForState('CA')).toBe('America/Los_Angeles')
    expect(getTimezoneForState('NV')).toBe('America/Los_Angeles')
    expect(getTimezoneForState('CO')).toBe('America/Denver')
    expect(getTimezoneForState('NM')).toBe('America/Denver')
    expect(getTimezoneForState('TX')).toBe('America/Chicago')
    expect(getTimezoneForState('NY')).toBe('America/New_York')
  })

  it('handles lowercase state codes', () => {
    expect(getTimezoneForState('az')).toBe('America/Phoenix')
    expect(getTimezoneForState('ca')).toBe('America/Los_Angeles')
  })

  it('handles mixed case state codes', () => {
    expect(getTimezoneForState('Az')).toBe('America/Phoenix')
    expect(getTimezoneForState('cA')).toBe('America/Los_Angeles')
  })

  it('defaults to America/Phoenix for unknown states', () => {
    expect(getTimezoneForState('XX')).toBe('America/Phoenix')
    expect(getTimezoneForState('ZZ')).toBe('America/Phoenix')
    expect(getTimezoneForState('')).toBe('America/Phoenix')
  })
})

describe('combineDateTimeToUTC', () => {
  it('combines date and time into UTC ISO string', () => {
    const result = combineDateTimeToUTC('2024-12-15', '19:30')
    // Result should be a valid ISO string ending with Z (UTC)
    expect(result).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/)
  })

  it('produces Go-compatible format without milliseconds', () => {
    const result = combineDateTimeToUTC('2024-06-01', '14:00')
    // Should NOT have milliseconds (.000)
    expect(result).not.toContain('.')
    expect(result).toMatch(/Z$/)
  })

  it('handles midnight correctly', () => {
    const result = combineDateTimeToUTC('2024-01-01', '00:00')
    expect(result).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/)
  })

  it('handles noon correctly', () => {
    const result = combineDateTimeToUTC('2024-07-04', '12:00')
    expect(result).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/)
  })

  it('handles end of day correctly', () => {
    const result = combineDateTimeToUTC('2024-12-31', '23:59')
    expect(result).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/)
  })

  // Note: combineDateTimeToUTC uses browser local timezone, not the timezone parameter
  // This behavior is documented - the timezone param is currently unused
  it('always uses browser local time (timezone param is unused)', () => {
    const result1 = combineDateTimeToUTC('2024-06-15', '10:00', 'America/Phoenix')
    const result2 = combineDateTimeToUTC('2024-06-15', '10:00', 'America/New_York')
    // Both should produce the same result since timezone is unused
    expect(result1).toBe(result2)
  })
})

describe('formatInTimezone', () => {
  it('formats UTC date in specified timezone', () => {
    const utcDate = '2024-12-15T02:30:00Z' // 2:30 AM UTC
    const result = formatInTimezone(utcDate, 'America/Phoenix', {
      hour: 'numeric',
      minute: '2-digit',
      hour12: true,
    })
    // Phoenix is UTC-7, so 2:30 AM UTC = 7:30 PM previous day
    expect(result).toBe('7:30 PM')
  })

  it('handles Arizona (no DST) correctly', () => {
    // In summer, Phoenix is UTC-7 (same as PDT)
    const summerDate = '2024-07-15T19:00:00Z' // 7 PM UTC
    const summerResult = formatInTimezone(summerDate, 'America/Phoenix', {
      hour: 'numeric',
      hour12: true,
    })
    expect(summerResult).toBe('12 PM') // 7 PM - 7 hours = 12 PM

    // In winter, Phoenix is still UTC-7 (while PST is UTC-8)
    const winterDate = '2024-01-15T19:00:00Z' // 7 PM UTC
    const winterResult = formatInTimezone(winterDate, 'America/Phoenix', {
      hour: 'numeric',
      hour12: true,
    })
    expect(winterResult).toBe('12 PM') // Same offset year-round
  })

  it('handles DST transitions for other states', () => {
    // California observes DST
    // Summer: PDT (UTC-7), Winter: PST (UTC-8)
    const summerDate = '2024-07-15T19:00:00Z' // During PDT
    const winterDate = '2024-01-15T19:00:00Z' // During PST

    const summerResult = formatInTimezone(summerDate, 'America/Los_Angeles', {
      hour: 'numeric',
      hour12: true,
    })
    const winterResult = formatInTimezone(winterDate, 'America/Los_Angeles', {
      hour: 'numeric',
      hour12: true,
    })

    // Summer: 7 PM UTC - 7 hours = 12 PM
    expect(summerResult).toBe('12 PM')
    // Winter: 7 PM UTC - 8 hours = 11 AM
    expect(winterResult).toBe('11 AM')
  })

  it('formats with various options', () => {
    const date = '2024-12-25T18:00:00Z'
    const result = formatInTimezone(date, 'America/New_York', {
      weekday: 'long',
      month: 'long',
      day: 'numeric',
      year: 'numeric',
    })
    expect(result).toContain('December')
    expect(result).toContain('25')
    expect(result).toContain('2024')
  })
})

describe('formatDateInTimezone', () => {
  it('formats date as "Mon, Dec 1" style', () => {
    const date = '2024-12-15T12:00:00Z'
    const result = formatDateInTimezone(date, 'America/Phoenix')
    // Should include weekday, month, and day
    expect(result).toMatch(/^[A-Z][a-z]{2}, [A-Z][a-z]{2} \d{1,2}$/)
  })

  it('displays correct date for timezone', () => {
    // 3 AM UTC on Dec 15 = 8 PM Dec 14 in Phoenix (UTC-7)
    const date = '2024-12-15T03:00:00Z'
    const result = formatDateInTimezone(date, 'America/Phoenix')
    expect(result).toContain('14') // Should show Dec 14, not Dec 15
    expect(result).toContain('Dec')
  })

  it('handles different timezones correctly', () => {
    const date = '2024-07-04T04:00:00Z' // 4 AM UTC on July 4

    // In New York (UTC-4 during DST): midnight July 4
    const nyResult = formatDateInTimezone(date, 'America/New_York')
    expect(nyResult).toContain('4')
    expect(nyResult).toContain('Jul')

    // In Phoenix (UTC-7): 9 PM July 3
    const phxResult = formatDateInTimezone(date, 'America/Phoenix')
    expect(phxResult).toContain('3')
    expect(phxResult).toContain('Jul')
  })
})

describe('formatTimeInTimezone', () => {
  it('formats time as "7:30 PM" style', () => {
    const date = '2024-12-15T02:30:00Z' // 2:30 AM UTC
    const result = formatTimeInTimezone(date, 'America/Phoenix')
    // Phoenix is UTC-7, so 2:30 AM UTC = 7:30 PM previous day
    expect(result).toBe('7:30 PM')
  })

  it('uses 12-hour format with AM/PM', () => {
    const morningDate = '2024-12-15T15:00:00Z' // 3 PM UTC = 8 AM Phoenix
    const morningResult = formatTimeInTimezone(morningDate, 'America/Phoenix')
    expect(morningResult).toBe('8:00 AM')

    const eveningDate = '2024-12-15T02:00:00Z' // 2 AM UTC = 7 PM prev day Phoenix
    const eveningResult = formatTimeInTimezone(eveningDate, 'America/Phoenix')
    expect(eveningResult).toBe('7:00 PM')
  })

  it('handles midnight and noon', () => {
    // Noon in Phoenix = 7 PM UTC (Phoenix is UTC-7)
    const noonUtc = '2024-06-15T19:00:00Z'
    const noonResult = formatTimeInTimezone(noonUtc, 'America/Phoenix')
    expect(noonResult).toBe('12:00 PM')

    // Midnight in Phoenix = 7 AM UTC
    const midnightUtc = '2024-06-15T07:00:00Z'
    const midnightResult = formatTimeInTimezone(midnightUtc, 'America/Phoenix')
    expect(midnightResult).toBe('12:00 AM')
  })

  it('pads minutes correctly', () => {
    const date = '2024-12-15T02:05:00Z' // 2:05 AM UTC = 7:05 PM Phoenix
    const result = formatTimeInTimezone(date, 'America/Phoenix')
    expect(result).toBe('7:05 PM')
  })
})

describe('parseISOToDateAndTime', () => {
  // Note: parseISOToDateAndTime uses local time, so tests may vary by environment
  // These tests use toISOString() to create consistent UTC strings

  it('returns date in YYYY-MM-DD format', () => {
    const isoDate = new Date(2024, 11, 15, 19, 30).toISOString() // Dec 15, 2024 7:30 PM local
    const result = parseISOToDateAndTime(isoDate)
    expect(result.date).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })

  it('returns time in HH:MM format', () => {
    const isoDate = new Date(2024, 11, 15, 19, 30).toISOString()
    const result = parseISOToDateAndTime(isoDate)
    expect(result.time).toMatch(/^\d{2}:\d{2}$/)
  })

  it('pads single-digit months correctly', () => {
    const isoDate = new Date(2024, 0, 5, 10, 0).toISOString() // Jan 5
    const result = parseISOToDateAndTime(isoDate)
    // Month should be padded to 01
    expect(result.date).toMatch(/-0[1-9]-/)
  })

  it('pads single-digit days correctly', () => {
    const isoDate = new Date(2024, 11, 5, 10, 0).toISOString() // Dec 5
    const result = parseISOToDateAndTime(isoDate)
    expect(result.date).toMatch(/-05$/)
  })

  it('pads single-digit hours correctly', () => {
    const isoDate = new Date(2024, 11, 15, 9, 30).toISOString() // 9:30 AM
    const result = parseISOToDateAndTime(isoDate)
    expect(result.time).toMatch(/^0\d:/)
  })

  it('pads single-digit minutes correctly', () => {
    const isoDate = new Date(2024, 11, 15, 19, 5).toISOString() // 7:05 PM
    const result = parseISOToDateAndTime(isoDate)
    expect(result.time).toMatch(/:05$/)
  })

  it('handles midnight', () => {
    const isoDate = new Date(2024, 11, 15, 0, 0).toISOString()
    const result = parseISOToDateAndTime(isoDate)
    expect(result.time).toBe('00:00')
  })

  it('handles end of day', () => {
    const isoDate = new Date(2024, 11, 15, 23, 59).toISOString()
    const result = parseISOToDateAndTime(isoDate)
    expect(result.time).toBe('23:59')
  })
})

describe('round-trip conversions', () => {
  it('parseISOToDateAndTime output can be used with combineDateTimeToUTC', () => {
    const originalDate = new Date(2024, 11, 15, 19, 30) // Local time
    const isoString = originalDate.toISOString()

    const { date, time } = parseISOToDateAndTime(isoString)
    const roundTripped = combineDateTimeToUTC(date, time)

    // The round-tripped result should produce the same local time
    const roundTrippedDate = new Date(roundTripped)
    expect(roundTrippedDate.getFullYear()).toBe(originalDate.getFullYear())
    expect(roundTrippedDate.getMonth()).toBe(originalDate.getMonth())
    expect(roundTrippedDate.getDate()).toBe(originalDate.getDate())
    expect(roundTrippedDate.getHours()).toBe(originalDate.getHours())
    expect(roundTrippedDate.getMinutes()).toBe(originalDate.getMinutes())
  })
})

describe('error handling', () => {
  it('formatInTimezone handles invalid date gracefully', () => {
    const result = formatInTimezone('invalid-date', 'America/Phoenix', {
      hour: 'numeric',
    })
    // Invalid Date produces "Invalid Date" string in toLocaleString
    expect(result).toBe('Invalid Date')
  })

  it('parseISOToDateAndTime handles invalid date', () => {
    const result = parseISOToDateAndTime('invalid-date')
    // NaN values will be returned for invalid dates
    expect(result.date).toContain('NaN')
    expect(result.time).toContain('NaN')
  })
})
