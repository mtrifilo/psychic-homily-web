import { describe, it, expect } from 'vitest'
import {
  civilDateInZone,
  isQuickWindowCurrent,
  NEXT_7_DAYS,
  QUICK_WINDOW_LABEL,
  QUICK_WINDOW_ORDER,
  quickWindowHref,
  quickWindowTargets,
  type CalendarDayParts,
  type QuickWindowKey,
} from './quickWindows'
import { SHOWS_WINDOW_MAX_DAYS } from './showsCalendarRoute'

/** The chip for one key, by key, so a table reads as the rule it pins. */
function target(today: CalendarDayParts, key: QuickWindowKey) {
  const found = quickWindowTargets(today).find(chip => chip.key === key)
  if (!found) throw new Error(`no ${key} chip`)
  return found
}

/**
 * A week of real dates with their real weekdays. September 2026 opens on a
 * Tuesday, so 14-20 September is a whole Monday-to-Sunday week, and every
 * weekday below is the weekday that date actually fell on.
 */
const WEEK_2026_09: Array<CalendarDayParts & { name: string }> = [
  { name: 'Monday', year: 2026, month: 9, day: 14, weekday: 1 },
  { name: 'Tuesday', year: 2026, month: 9, day: 15, weekday: 2 },
  { name: 'Wednesday', year: 2026, month: 9, day: 16, weekday: 3 },
  { name: 'Thursday', year: 2026, month: 9, day: 17, weekday: 4 },
  { name: 'Friday', year: 2026, month: 9, day: 18, weekday: 5 },
  { name: 'Saturday', year: 2026, month: 9, day: 19, weekday: 6 },
  { name: 'Sunday', year: 2026, month: 9, day: 20, weekday: 0 },
]

describe('the weekday table the real calendar gives', () => {
  // The fixtures above are the whole premise of the weekend table below. A
  // typo in one weekday would make every assertion in this file agree with a
  // rule the calendar does not have.
  it.each(WEEK_2026_09)('$name $year-$month-$day', date => {
    expect(new Date(Date.UTC(date.year, date.month - 1, date.day)).getUTCDay()).toBe(
      date.weekday
    )
  })
})

describe('quickWindowTargets: this weekend', () => {
  /**
   * The locked rule: Friday night through Sunday, and from Friday until Sunday
   * ends it is the weekend IN PROGRESS.
   *
   * So Monday to Thursday reach forward to the coming Friday and ask for three
   * days; Friday asks for three from itself; Saturday and Sunday anchor on
   * themselves with the run shortened so it still ends on Sunday. The anchor
   * never names a night already past: the list behind it holds only upcoming
   * shows, so a Friday anchor read on Saturday would promise three nights and
   * show two.
   */
  it.each([
    ['Monday', WEEK_2026_09[0], '/shows/2026/09/18', 3],
    ['Tuesday', WEEK_2026_09[1], '/shows/2026/09/18', 3],
    ['Wednesday', WEEK_2026_09[2], '/shows/2026/09/18', 3],
    ['Thursday', WEEK_2026_09[3], '/shows/2026/09/18', 3],
    ['Friday', WEEK_2026_09[4], '/shows/2026/09/18', 3],
    ['Saturday', WEEK_2026_09[5], '/shows/2026/09/19', 2],
    ['Sunday', WEEK_2026_09[6], '/shows/2026/09/20', undefined],
  ] as const)('on %s anchors %s for %s days', (_name, today, path, days) => {
    const chip = target(today, 'this-weekend')
    expect(chip.path).toBe(path)
    expect(chip.days).toBe(days)
  })

  // Every weekend window ends on the Sunday of the weekend in progress or next
  // up. Asserted as arithmetic rather than by eye, so a future edit to the
  // anchor rule cannot pass by moving both halves.
  it.each(WEEK_2026_09)('ends on Sunday when read on $name', today => {
    const chip = target(today, 'this-weekend')
    const [, , anchorDay] = chip.path.split('/').slice(2)
    const end = new Date(
      Date.UTC(today.year, today.month - 1, Number(anchorDay) + (chip.days ?? 1) - 1)
    )
    expect(end.getUTCDay()).toBe(0)
  })

  // A weekend crossing a month, a year or a leap day is arithmetic on the
  // calendar rather than on the month's length.
  it.each([
    // Wednesday 30 September 2026 reaches Friday 2 October.
    [{ year: 2026, month: 9, day: 30, weekday: 3 }, '/shows/2026/10/02', 3],
    // Wednesday 30 December 2026 reaches Friday 1 January 2027.
    [{ year: 2026, month: 12, day: 30, weekday: 3 }, '/shows/2027/01/01', 3],
    // Thursday 27 February 2031 reaches Friday the 28th; 2031 is not a leap year.
    [{ year: 2031, month: 2, day: 27, weekday: 4 }, '/shows/2031/02/28', 3],
    // Wednesday 27 February 2036 reaches Friday the 29th, which that leap year has.
    [{ year: 2036, month: 2, day: 27, weekday: 3 }, '/shows/2036/02/29', 3],
  ])('rolls over the calendar from %o', (today, path, days) => {
    const chip = target(today, 'this-weekend')
    expect(chip.path).toBe(path)
    expect(chip.days).toBe(days)
  })
})

describe('quickWindowTargets: the other three', () => {
  const friday = WEEK_2026_09[4]

  it('points tonight at today, with no run', () => {
    const chip = target(friday, 'tonight')
    expect(chip.path).toBe('/shows/2026/09/18')
    expect(chip.days).toBeUndefined()
  })

  it('points the week at today for seven days', () => {
    const chip = target(friday, 'next-7-days')
    expect(chip.path).toBe('/shows/2026/09/18')
    expect(chip.days).toBe(NEXT_7_DAYS)
  })

  it('points the month at the month root', () => {
    const chip = target(friday, 'this-month')
    expect(chip.path).toBe('/shows/2026/09')
    expect(chip.days).toBeUndefined()
  })

  it('is the four windows in row order, labelled once each', () => {
    const chips = quickWindowTargets(friday)
    expect(chips.map(chip => chip.key)).toEqual(QUICK_WINDOW_ORDER)
    expect(chips.map(chip => chip.label)).toEqual(
      QUICK_WINDOW_ORDER.map(key => QUICK_WINDOW_LABEL[key])
    )
  })

  // Every run a chip asks for has to be one the route will serve, or the chip
  // is a link to a not-found.
  it.each(WEEK_2026_09)('asks for no run the route refuses, on $name', today => {
    for (const chip of quickWindowTargets(today)) {
      if (chip.days === undefined) continue
      expect(chip.days).toBeGreaterThanOrEqual(2)
      expect(chip.days).toBeLessThanOrEqual(SHOWS_WINDOW_MAX_DAYS)
    }
  })
})

describe('civilDateInZone', () => {
  /**
   * The zone decides the date, which is the whole reason the chips take one.
   * 06:00 UTC on 19 September is 23:00 on the 18th in Phoenix and 01:00 on the
   * 19th in Chicago, and those are two different "tonight" URLs.
   */
  const at = new Date('2026-09-19T06:00:00Z')

  it('reads the date the zone was on, not the one UTC was', () => {
    expect(civilDateInZone(at, 'America/Phoenix')).toEqual({
      year: 2026,
      month: 9,
      day: 18,
      weekday: 5,
    })
    expect(civilDateInZone(at, 'America/Chicago')).toEqual({
      year: 2026,
      month: 9,
      day: 19,
      weekday: 6,
    })
  })

  // A metro whose zone is a day ahead of the viewer's gets that metro's chips:
  // the shows are there, and the night they are on is the one that zone is
  // having.
  it('gives a metro and its viewer different weekends across the date line', () => {
    const tokyo = civilDateInZone(at, 'Asia/Tokyo')
    const phoenix = civilDateInZone(at, 'America/Phoenix')
    expect(tokyo).not.toBeNull()
    expect(phoenix).not.toBeNull()
    // Saturday in Tokyo, still Friday in Phoenix: a two-day weekend there and a
    // three-day one here.
    expect(target(tokyo as CalendarDayParts, 'this-weekend').days).toBe(2)
    expect(target(phoenix as CalendarDayParts, 'this-weekend').days).toBe(3)
  })

  /**
   * DST is a property of the INSTANT, not of the calendar arithmetic: the date
   * is read off the zone, and every window built from it is whole days. A spring
   * forward at 02:00 local therefore moves no window's edge.
   */
  it('reads the date across a DST transition', () => {
    // 08:30 UTC on 8 March 2026 is 02:30 CST, the hour America/Chicago skips.
    expect(civilDateInZone(new Date('2026-03-08T08:30:00Z'), 'America/Chicago')).toEqual({
      year: 2026,
      month: 3,
      day: 8,
      weekday: 0,
    })
    // 07:30 UTC on 1 November 2026 is 02:30 CDT, the hour it repeats.
    expect(civilDateInZone(new Date('2026-11-01T07:30:00Z'), 'America/Chicago')).toEqual({
      year: 2026,
      month: 11,
      day: 1,
      weekday: 0,
    })
    // Arizona keeps one offset all year and lands on the previous day for both.
    expect(civilDateInZone(new Date('2026-03-08T06:30:00Z'), 'America/Phoenix')?.day).toBe(7)
  })

  // A zone the runtime will not resolve names no day, and a window anchored on
  // the wrong day is wrong in a way nothing on the page reveals.
  it('names no date for a zone it cannot resolve', () => {
    expect(civilDateInZone(at, 'Not/AZone')).toBeNull()
  })
})

describe('quickWindowHref', () => {
  const friday = WEEK_2026_09[4]

  // The URL-state rule: a chip owns `days` and `page`, and carries everything
  // else through. A chip that minted a bare path would drop an explicit All
  // Cities back to the viewer's derived default on every click.
  it('carries the filters already on screen', () => {
    const params = new URLSearchParams({
      cities: 'Phoenix,AZ',
      tags: 'post-punk',
      tag_match: 'any',
      utm_source: 'newsletter',
    })
    const href = quickWindowHref(params, target(friday, 'this-weekend'))

    expect(href).toContain('cities=Phoenix%2CAZ')
    expect(href).toContain('tags=post-punk')
    expect(href).toContain('tag_match=any')
    expect(href).toContain('utm_source=newsletter')
    expect(href.startsWith('/shows/2026/09/18?')).toBe(true)
  })

  it('sets its own run and clears one it does not want', () => {
    const params = new URLSearchParams({ days: '7' })
    expect(quickWindowHref(params, target(friday, 'this-weekend'))).toBe(
      '/shows/2026/09/18?days=3'
    )
    expect(quickWindowHref(params, target(friday, 'tonight'))).toBe(
      '/shows/2026/09/18'
    )
    expect(quickWindowHref(params, target(friday, 'this-month'))).toBe(
      '/shows/2026/09'
    )
  })

  // A different window is a different question, answered from its first page.
  it('drops the page number', () => {
    const params = new URLSearchParams({ page: '4', cities: 'all' })
    expect(quickWindowHref(params, target(friday, 'next-7-days'))).toBe(
      '/shows/2026/09/18?cities=all&days=7'
    )
  })
})

describe('isQuickWindowCurrent', () => {
  const friday = WEEK_2026_09[4]
  const tonight = target(friday, 'tonight')
  const weekend = target(friday, 'this-weekend')
  const week = target(friday, 'next-7-days')

  it('marks the chip whose path and run the reader is on', () => {
    expect(isQuickWindowCurrent(tonight, '/shows/2026/09/18', undefined)).toBe(true)
    expect(isQuickWindowCurrent(weekend, '/shows/2026/09/18', 3)).toBe(true)
    expect(isQuickWindowCurrent(week, '/shows/2026/09/18', NEXT_7_DAYS)).toBe(true)
  })

  // Same path, different run: three chips share a path on a Friday, and only
  // the one naming the run in the URL is current.
  it('tells runs on one path apart', () => {
    expect(isQuickWindowCurrent(tonight, '/shows/2026/09/18', 3)).toBe(false)
    expect(isQuickWindowCurrent(weekend, '/shows/2026/09/18', undefined)).toBe(false)
    expect(isQuickWindowCurrent(week, '/shows/2026/09/18', 3)).toBe(false)
  })

  it('is not current on another day, or on the root', () => {
    expect(isQuickWindowCurrent(tonight, '/shows/2026/09/19', undefined)).toBe(false)
    expect(isQuickWindowCurrent(tonight, '/shows', undefined)).toBe(false)
  })

  /**
   * On a Sunday the weekend still running is one night, which is also tonight,
   * so two chips name ONE window and both are true of it. The row is a set of
   * true descriptions rather than a partition; marking exactly one of them is
   * the component's job, pinned in `QuickWindowChips.test.tsx`.
   */
  it('is true of both chips that name the same window on a Sunday', () => {
    const sunday = WEEK_2026_09[6]
    const sundayTonight = target(sunday, 'tonight')
    const sundayWeekend = target(sunday, 'this-weekend')

    expect(sundayWeekend.path).toBe(sundayTonight.path)
    expect(sundayWeekend.days).toBe(sundayTonight.days)
    expect(isQuickWindowCurrent(sundayTonight, '/shows/2026/09/20', undefined)).toBe(true)
    expect(isQuickWindowCurrent(sundayWeekend, '/shows/2026/09/20', undefined)).toBe(true)
  })
})

/**
 * A run is whole venue-local DAYS from an anchor date, so a clock change inside
 * it moves nothing: the span is calendar arithmetic and the membership test the
 * backend runs is on dates. Pinned for the two US transitions, where an
 * hour-based span would be a day out at one edge.
 */
describe('spans across a DST transition', () => {
  it.each([
    // Sunday 8 March 2026 springs forward; the week from the Wednesday before
    // still ends on the following Tuesday.
    [{ year: 2026, month: 3, day: 4, weekday: 3 }, '/shows/2026/03/04', '2026-03-10'],
    // Sunday 1 November 2026 falls back.
    [{ year: 2026, month: 10, day: 28, weekday: 3 }, '/shows/2026/10/28', '2026-11-03'],
  ])('next 7 days from %o covers seven dates', (today, path, lastDate) => {
    const chip = target(today, 'next-7-days')
    expect(chip.path).toBe(path)
    expect(chip.days).toBe(NEXT_7_DAYS)

    const end = new Date(
      Date.UTC(today.year, today.month - 1, today.day + NEXT_7_DAYS - 1)
    )
    expect(end.toISOString().slice(0, 10)).toBe(lastDate)
  })

  // The weekend that straddles a transition is still Friday through Sunday.
  it('the weekend containing a spring-forward is three days', () => {
    // Thursday 5 March 2026 reaches Friday the 6th; the 8th springs forward.
    const chip = target({ year: 2026, month: 3, day: 5, weekday: 4 }, 'this-weekend')
    expect(chip.path).toBe('/shows/2026/03/06')
    expect(chip.days).toBe(3)
  })
})
