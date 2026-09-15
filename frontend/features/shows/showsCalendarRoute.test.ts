import { describe, it, expect } from 'vitest'
import {
  adjacentMonths,
  appendShowsCalendarWindow,
  calendarDayLabel,
  calendarMonthLabel,
  calendarWindowLabel,
  isRealCalendarDay,
  parseDaySegments,
  parseMonthSegments,
  SHOWS_CALENDAR_MAX_YEAR,
  SHOWS_CALENDAR_MIN_YEAR,
  shortCalendarMonthLabel,
  showsCalendarWindowKey,
  showsDayPath,
  showsDayPathFromDateKey,
  showsMonthPath,
  showsWindowPath,
} from './showsCalendarRoute'

describe('parseMonthSegments', () => {
  it('accepts a four-digit year and a zero-padded month', () => {
    expect(parseMonthSegments('2026', '11')).toEqual({ year: 2026, month: 11 })
    expect(parseMonthSegments('2026', '01')).toEqual({ year: 2026, month: 1 })
    expect(parseMonthSegments('2026', '12')).toEqual({ year: 2026, month: 12 })
  })

  /**
   * The year bound is not decoration. `0026` parses to 26, which every path
   * builder emits unpadded, so the page would advertise a canonical and a pager
   * pointing at `/shows/26/11` - not four digits, and a not-found. `0000` is
   * worse: the backend reads a zero year as NO WINDOW and would answer with the
   * whole upcoming list under a heading naming one month.
   */
  it.each([
    ['2000', '01'],
    ['2100', '12'],
    ['2026', '11'],
  ])('accepts the addressable year %s', (year, month) => {
    expect(parseMonthSegments(year, month)).not.toBeNull()
  })

  it.each([
    ['1999', '11'],
    ['2101', '11'],
    ['0026', '11'],
    ['0000', '01'],
    ['9999', '11'],
  ])('refuses the out-of-range year %s', (year, month) => {
    expect(parseMonthSegments(year, month)).toBeNull()
  })

  // Every path a page mints must parse back to the window it came from, or the
  // canonical points somewhere the router will not serve.
  it('round-trips every addressable year through its own path builder', () => {
    for (const year of [SHOWS_CALENDAR_MIN_YEAR, 2026, SHOWS_CALENDAR_MAX_YEAR]) {
      const path = showsMonthPath(year, 1)
      const [, , yearSegment, monthSegment] = path.split('/')
      expect(parseMonthSegments(yearSegment, monthSegment)).toEqual({
        year,
        month: 1,
      })
    }
  })

  // One spelling per month is what keeps the canonical honest without a
  // redirect table: an unpadded month must not be a second address for one page.
  it.each([
    ['2026', '9'],
    ['2026', '009'],
    ['2026', '00'],
    ['2026', '13'],
    ['2026', 'ab'],
    ['20266', '11'],
    [' 2026 ', '11'],
    ['2026', ' 11'],
    ['', ''],
  ])('refuses /shows/%s/%s', (year, month) => {
    expect(parseMonthSegments(year, month)).toBeNull()
  })
})

describe('parseDaySegments', () => {
  it('accepts a real calendar day', () => {
    expect(parseDaySegments('2026', '11', '14')).toEqual({
      year: 2026,
      month: 11,
      day: 14,
    })
  })

  // The leap day exists in 2028 and not in 2027, and only the calendar test
  // can tell the two apart: both pass the segment shape.
  it('accepts a leap day in a leap year', () => {
    expect(parseDaySegments('2028', '02', '29')).toEqual({
      year: 2028,
      month: 2,
      day: 29,
    })
  })

  it('refuses a leap day in a common year', () => {
    expect(parseDaySegments('2027', '02', '29')).toBeNull()
  })

  it.each([
    ['2026', '11', '31'],
    ['2027', '02', '30'],
    ['2026', '11', '00'],
    ['2026', '11', '32'],
    ['2026', '11', '4'],
    ['2026', '13', '14'],
    ['2026', '11', 'xx'],
  ])('refuses /shows/%s/%s/%s', (year, month, day) => {
    expect(parseDaySegments(year, month, day)).toBeNull()
  })
})

describe('isRealCalendarDay', () => {
  it('knows how long each month is', () => {
    expect(isRealCalendarDay(2026, 4, 30)).toBe(true)
    expect(isRealCalendarDay(2026, 4, 31)).toBe(false)
    expect(isRealCalendarDay(2026, 12, 31)).toBe(true)
  })
})

describe('path builders', () => {
  it('zero-pads the month and the day', () => {
    expect(showsMonthPath(2026, 9)).toBe('/shows/2026/09')
    expect(showsDayPath(2026, 9, 4)).toBe('/shows/2026/09/04')
  })

  it('round-trips a built path back through the parser', () => {
    const path = showsDayPath(2026, 11, 14)
    const [, , year, month, day] = path.split('/')
    expect(parseDaySegments(year, month, day)).toEqual({
      year: 2026,
      month: 11,
      day: 14,
    })
  })

  it('addresses a month window at its month path and a day at its day path', () => {
    expect(showsWindowPath({ year: 2026, month: 11 })).toBe('/shows/2026/11')
    expect(showsWindowPath({ year: 2026, month: 11, day: 1 })).toBe(
      '/shows/2026/11/01'
    )
  })
})

describe('showsDayPathFromDateKey', () => {
  it('turns a venue-local date key into a day path', () => {
    expect(showsDayPathFromDateKey('2026-11-14')).toBe('/shows/2026/11/14')
  })

  it.each(['2026-11', '2026-11-14-extra', 'not-a-date', '2027-02-29', ''])(
    'declines %s',
    key => {
      expect(showsDayPathFromDateKey(key)).toBeNull()
    }
  )
})

describe('labels', () => {
  it('names a month in full and a day with its ordinal', () => {
    expect(calendarMonthLabel(2026, 11)).toBe('November 2026')
    expect(calendarDayLabel(2026, 11, 14)).toBe('November 14, 2026')
    expect(shortCalendarMonthLabel(2026, 10)).toBe('Oct 2026')
  })

  it('labels a window by what it addresses', () => {
    expect(calendarWindowLabel({ year: 2026, month: 1 })).toBe('January 2026')
    expect(calendarWindowLabel({ year: 2026, month: 1, day: 3 })).toBe(
      'January 3, 2026'
    )
  })

  // The year is returned verbatim rather than formatted, so the 0-99 remap
  // `Date.UTC` applies to two-digit years cannot reach it.
  it('does not let the reference instant move the month or the year', () => {
    expect(calendarMonthLabel(2026, 12)).toBe('December 2026')
    expect(calendarMonthLabel(99, 1)).toBe('January 99')
  })
})

describe('appendShowsCalendarWindow', () => {
  it('sends nothing for the unwindowed list', () => {
    const params = new URLSearchParams()
    appendShowsCalendarWindow(params, undefined)
    expect(params.toString()).toBe('')
  })

  it('sends year and month for a month window, and adds day for a day', () => {
    const month = new URLSearchParams()
    appendShowsCalendarWindow(month, { year: 2026, month: 11 })
    expect(month.toString()).toBe('year=2026&month=11')

    const day = new URLSearchParams()
    appendShowsCalendarWindow(day, { year: 2026, month: 11, day: 4 })
    expect(day.toString()).toBe('year=2026&month=11&day=4')
  })
})

describe('showsCalendarWindowKey', () => {
  // Undefined members are dropped by react-query's JSON hashing, so the
  // unwindowed key is unchanged by windows existing at all.
  it('is all-undefined for the unwindowed list', () => {
    expect(showsCalendarWindowKey(undefined)).toEqual({
      year: undefined,
      month: undefined,
      day: undefined,
    })
  })

  it('carries the window and leaves day undefined for a month', () => {
    expect(showsCalendarWindowKey({ year: 2026, month: 11 })).toEqual({
      year: 2026,
      month: 11,
      day: undefined,
    })
  })

  it('distinguishes a day from its month', () => {
    expect(
      JSON.stringify(showsCalendarWindowKey({ year: 2026, month: 11, day: 4 }))
    ).not.toBe(JSON.stringify(showsCalendarWindowKey({ year: 2026, month: 11 })))
  })
})

describe('adjacentMonths', () => {
  const histogram = [
    { year: 2026, month: 9 },
    { year: 2026, month: 10 },
    { year: 2026, month: 12 },
    { year: 2027, month: 1 },
  ]

  it('returns the nearest month either side that HAS shows', () => {
    // November is not in the histogram, so a reader on it steps to October and
    // December rather than to two months that 404.
    expect(adjacentMonths(histogram, { year: 2026, month: 11 })).toEqual({
      previous: { year: 2026, month: 10 },
      next: { year: 2026, month: 12 },
    })
  })

  it('crosses the year boundary', () => {
    expect(adjacentMonths(histogram, { year: 2026, month: 12 })).toEqual({
      previous: { year: 2026, month: 10 },
      next: { year: 2027, month: 1 },
    })
  })

  it('has no previous at the start and no next at the end', () => {
    expect(adjacentMonths(histogram, { year: 2026, month: 9 }).previous).toBeNull()
    expect(adjacentMonths(histogram, { year: 2027, month: 1 }).next).toBeNull()
  })

  it('does not trust the input order', () => {
    const shuffled = [...histogram].reverse()
    expect(adjacentMonths(shuffled, { year: 2026, month: 12 })).toEqual({
      previous: { year: 2026, month: 10 },
      next: { year: 2027, month: 1 },
    })
  })

  it('returns nothing for an empty histogram', () => {
    expect(adjacentMonths([], { year: 2026, month: 11 })).toEqual({
      previous: null,
      next: null,
    })
  })
})
