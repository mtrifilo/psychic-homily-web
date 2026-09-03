import { describe, it, expect } from 'vitest'
import {
  markGuessedShowDay,
  showPageDate,
  showPageDateLong,
} from './showPageDate'

/**
 * Real `resolveShowTimezone` and a real tz database, deliberately: the whole
 * question here is whether a zone is one the row supplied or the fallback, and
 * a mocked map would let the suite keep passing after the real predicate moved.
 */

// 03:00 UTC is the previous evening in the fallback zone (UTC-7), so a
// formatter that quietly read this in UTC would name the wrong day as well as
// skip the mark.
const UTC_DATE = '2026-11-14T03:00:00Z'

describe('markGuessedShowDay', () => {
  it('leaves a date alone when the row supplies its own zone', () => {
    expect(markGuessedShowDay('Fri, Nov 13', null, 'Europe/Berlin')).toBe(
      'Fri, Nov 13'
    )
  })

  it('leaves a date alone when the state map knows the state', () => {
    expect(markGuessedShowDay('Fri, Nov 13', 'AZ')).toBe('Fri, Nov 13')
  })

  it('prefixes the tilde when the day was read on the fallback', () => {
    expect(markGuessedShowDay('Fri, Nov 13', '')).toBe('~Fri, Nov 13')
    // Naming a region is not naming a zone: 'England' resolves to the same
    // fallback as a blank state.
    expect(markGuessedShowDay('Fri, Nov 13', 'England')).toBe('~Fri, Nov 13')
  })

  it('marks once, whatever the register', () => {
    expect(markGuessedShowDay('WED, AUG 12 2026', '')).toBe('~WED, AUG 12 2026')
    expect(markGuessedShowDay('SAT', '')).toBe('~SAT')
  })
})

describe('showPageDate', () => {
  it('reads the day on the venue zone', () => {
    expect(showPageDate(UTC_DATE, 'AZ')).toBe('Fri, Nov 13')
    expect(showPageDate(UTC_DATE, 'AZ', 'Europe/Berlin')).toBe('Sat, Nov 14')
  })

  it('marks the day when the zone is a guess', () => {
    expect(showPageDate(UTC_DATE, '')).toBe('~Fri, Nov 13')
    expect(showPageDate(UTC_DATE, 'England')).toBe('~Fri, Nov 13')
  })
})

describe('showPageDateLong', () => {
  it('spells the long form in the venue zone', () => {
    expect(showPageDateLong(UTC_DATE, 'AZ')).toBe('Friday, November 13, 2026')
  })

  it('abbreviates on request without changing the day', () => {
    expect(showPageDateLong(UTC_DATE, 'AZ', null, true)).toBe(
      'Fri, Nov 13, 2026'
    )
  })

  it('marks the day when the zone is a guess, in both widths', () => {
    expect(showPageDateLong(UTC_DATE, '')).toBe('~Friday, November 13, 2026')
    expect(showPageDateLong(UTC_DATE, 'England', null, true)).toBe(
      '~Fri, Nov 13, 2026'
    )
  })

  it('prefers the venue timezone over the state', () => {
    expect(showPageDateLong(UTC_DATE, 'AZ', 'Europe/Berlin')).toBe(
      'Saturday, November 14, 2026'
    )
  })
})
