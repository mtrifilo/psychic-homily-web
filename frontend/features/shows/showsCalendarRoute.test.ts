import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import {
  adjacentMonths,
  applyWindowDays,
  appendShowsCalendarWindow,
  calendarDayLabel,
  calendarMonthLabel,
  calendarWindowLabel,
  isRealCalendarDay,
  parseDaySegments,
  parseMonthSegments,
  parseWindowDays,
  SHOWS_CALENDAR_MAX_YEAR,
  SHOWS_CALENDAR_MIN_YEAR,
  SHOWS_WINDOW_MAX_DAYS,
  shortCalendarMonthLabel,
  showsCalendarWindowKey,
  showsCalendarWindowTitle,
  showsDayPath,
  showsDayPathFromDateKey,
  showsMonthPath,
  showsWindowHref,
  showsWindowPath,
  windowMonths,
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

  it('sends the run length for a run', () => {
    const run = new URLSearchParams()
    appendShowsCalendarWindow(run, { year: 2026, month: 11, day: 4, days: 3 })
    expect(run.toString()).toBe('year=2026&month=11&day=4&days=3')
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
      days: undefined,
    })
  })

  it('carries the window and leaves day undefined for a month', () => {
    expect(showsCalendarWindowKey({ year: 2026, month: 11 })).toEqual({
      year: 2026,
      month: 11,
      day: undefined,
      days: undefined,
    })
  })

  it('distinguishes a day from its month', () => {
    expect(
      JSON.stringify(showsCalendarWindowKey({ year: 2026, month: 11, day: 4 }))
    ).not.toBe(JSON.stringify(showsCalendarWindowKey({ year: 2026, month: 11 })))
  })

  // A run and its anchor day are different sets of rows, so they must be
  // different cache entries: sharing one would serve three days' rows to the day
  // page that asked for one.
  it('distinguishes a run from the day it anchors on', () => {
    expect(
      JSON.stringify(
        showsCalendarWindowKey({ year: 2026, month: 11, day: 4, days: 3 })
      )
    ).not.toBe(
      JSON.stringify(showsCalendarWindowKey({ year: 2026, month: 11, day: 4 }))
    )
  })
})

/**
 * The bound this module enforces and the bound the API enforces are ONE bound,
 * spelled in two languages: raise the Go constant alone and these routes 404
 * runs the API would serve, raise this one alone and every chip past the old
 * bound links to a 422.
 *
 * Read off the GENERATED contract rather than off a second literal here, so the
 * check is against what the backend actually publishes. The description is the
 * only place the OpenAPI document carries the range (`openapi-typescript` drops
 * numeric bounds), which is why the Go side pins that string to its own
 * constant in `show_list_window_test.go`.
 */
describe('SHOWS_WINDOW_MAX_DAYS', () => {
  it('matches the range the generated API contract states', () => {
    const contract = readFileSync(
      join(import.meta.dirname, '../../types/api.d.ts'),
      'utf8'
    )
    const documented = contract.match(
      /Length in venue-local days of a run beginning on the requested day, 1-(\d+)\./
    )

    expect(documented).not.toBeNull()
    expect(Number(documented?.[1])).toBe(SHOWS_WINDOW_MAX_DAYS)
  })
})

describe('parseWindowDays', () => {
  it('names no run when the parameter is absent', () => {
    expect(parseWindowDays(undefined)).toBeUndefined()
  })

  // One day is the day itself, which is already an address. Carrying the run
  // would give that one window two spellings.
  it('reads 1 as no run', () => {
    expect(parseWindowDays('1')).toBeUndefined()
  })

  it.each([2, 3, 7, SHOWS_WINDOW_MAX_DAYS])('reads %s as a run', days => {
    expect(parseWindowDays(String(days))).toBe(days)
  })

  it.each([
    String(SHOWS_WINDOW_MAX_DAYS + 1),
    '99',
    '0',
    '-3',
    '3.5',
    '+3',
    '03',
    ' 3',
    'three',
  ])('refuses %s', raw => {
    expect(parseWindowDays(raw)).toBeNull()
  })

  // A key with no value names no run, which is the same as not being there.
  // Query-string builders write `days=` when they clear the key, and refusing
  // that would 404 a real day over a URL that said nothing.
  it('reads a valueless parameter as no run', () => {
    expect(parseWindowDays('')).toBeUndefined()
  })

  it('refuses a repeated parameter, which names two runs', () => {
    expect(parseWindowDays(['3', '7'])).toBeNull()
  })
})

describe('applyWindowDays', () => {
  const day = { year: 2026, month: 11, day: 14 }

  it('leaves a month window alone whatever the parameter says', () => {
    const month = { year: 2026, month: 11 }
    expect(applyWindowDays(month, '3')).toEqual(month)
    expect(applyWindowDays(month, '99')).toEqual(month)
  })

  it('carries a run onto a day window', () => {
    expect(applyWindowDays(day, '3')).toEqual({ ...day, days: 3 })
  })

  it('leaves the day itself for no parameter and for the one-day spelling', () => {
    expect(applyWindowDays(day, undefined)).toEqual(day)
    expect(applyWindowDays(day, '1')).toEqual(day)
  })

  it('refuses a run it will not serve rather than serving a nearer one', () => {
    expect(applyWindowDays(day, '15')).toBeNull()
    expect(applyWindowDays(day, 'three')).toBeNull()
  })
})

describe('showsWindowHref', () => {
  // The href is the window's OWN address and the path is its canonical. They
  // differ for exactly one shape, and that difference is what keeps a run
  // canonicalizing to its anchor day.
  it('carries the run, where the path does not', () => {
    const run = { year: 2026, month: 11, day: 4, days: 3 }
    expect(showsWindowHref(run)).toBe('/shows/2026/11/04?days=3')
    expect(showsWindowPath(run)).toBe('/shows/2026/11/04')
  })

  it('is the path itself for a month and for a day', () => {
    expect(showsWindowHref({ year: 2026, month: 11 })).toBe('/shows/2026/11')
    expect(showsWindowHref({ year: 2026, month: 11, day: 4 })).toBe(
      '/shows/2026/11/04'
    )
  })
})

describe('calendarWindowLabel and showsCalendarWindowTitle, for a run', () => {
  // The end named is the run's LAST date, not the half-open bound the query
  // carries: a heading naming a date that holds none of the rows beneath it
  // would be false.
  it('names both edges and prints the year once', () => {
    expect(
      calendarWindowLabel({ year: 2026, month: 9, day: 18, days: 3 })
    ).toBe('Sep 18 to Sep 20, 2026')
    expect(
      showsCalendarWindowTitle({ year: 2026, month: 9, day: 18, days: 3 })
    ).toBe('Shows from Sep 18 to Sep 20, 2026')
  })

  it.each([
    [{ year: 2026, month: 11, day: 29, days: 7 }, 'Nov 29 to Dec 5, 2026'],
    [{ year: 2028, month: 2, day: 27, days: 3 }, 'Feb 27 to Feb 29, 2028'],
    [{ year: 2027, month: 2, day: 27, days: 3 }, 'Feb 27 to Mar 1, 2027'],
  ])('rolls over the calendar: %o', (window, want) => {
    expect(calendarWindowLabel(window)).toBe(want)
  })

  // A span whose edges carry no year is a date a reader has to guess at, and
  // this list runs into the following year.
  it('names both years when the run crosses one', () => {
    expect(
      calendarWindowLabel({ year: 2026, month: 12, day: 29, days: 7 })
    ).toBe('Dec 29, 2026 to Jan 4, 2027')
  })
})

describe('windowMonths', () => {
  it('is the window itself for a month, a day, and a run inside one month', () => {
    expect(windowMonths({ year: 2026, month: 11 })).toEqual([
      { year: 2026, month: 11 },
    ])
    expect(windowMonths({ year: 2026, month: 11, day: 14 })).toEqual([
      { year: 2026, month: 11 },
    ])
    expect(windowMonths({ year: 2026, month: 11, day: 14, days: 3 })).toEqual([
      { year: 2026, month: 11 },
    ])
  })

  // What the histogram gate asks about. A run opening in a quiet month and
  // closing in a busy one is a real page, and asking about the anchor month
  // alone would 404 it.
  it('names both months a run spans, across a year boundary too', () => {
    expect(windowMonths({ year: 2026, month: 11, day: 29, days: 7 })).toEqual([
      { year: 2026, month: 11 },
      { year: 2026, month: 12 },
    ])
    expect(windowMonths({ year: 2026, month: 12, day: 29, days: 7 })).toEqual([
      { year: 2026, month: 12 },
      { year: 2027, month: 1 },
    ])
  })

  // Walked rather than read off the edges, so a run longer than a month would
  // still name the months in the middle. No such run is addressable today; this
  // is what makes raising the bound safe rather than silently lossy.
  it('names every month a run passes through, not only its edges', () => {
    expect(windowMonths({ year: 2026, month: 11, day: 29, days: 40 })).toEqual([
      { year: 2026, month: 11 },
      { year: 2026, month: 12 },
      { year: 2027, month: 1 },
    ])
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
