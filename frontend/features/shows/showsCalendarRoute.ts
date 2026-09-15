/**
 * The date-addressed half of the `/shows` URL family: `/shows/{yyyy}/{mm}` and
 * `/shows/{yyyy}/{mm}/{dd}`.
 *
 * Pure: no React, no router, no fetching, so the rules that decide whether a
 * path segment names a month can be tested without a rendered route.
 *
 * THIS IS THE AUTHORITY on that grammar. The proxy keeps its own copy, because
 * it may not import `features/`, and a test asserts the two reach the same
 * verdict on every input; the legacy redirect in `next.config.ts` carries the
 * day shape as an exclusion, and a test asserts it claims a segment if and only
 * if this grammar does not. Widen anything here and both of those go red.
 */

import { formatCalendarMonthParts } from '@/lib/utils/formatters'

/** A month or day window of the upcoming list, as a route addresses it. */
export interface ShowsCalendarWindow {
  year: number
  /** Calendar month, 1-12, matching the wire format and NOT JavaScript's. */
  month: number
  /** Calendar day of month, 1-31. Absent on a month window. */
  day?: number
  /**
   * Days in a RUN starting on `day`, 2 or more, the anchor day included.
   * Absent on every window that is one day, one month, or the whole list.
   *
   * A run is chrome: it is addressed as `?days=` on the day's own path, so its
   * canonical is that path and a reader who bookmarks it keeps the day. One is
   * never stored here, because a run of one IS the day and two spellings of one
   * window is what makes a title and a canonical drift apart.
   */
  days?: number
}

/** The list root every window hangs off. */
export const SHOWS_ROOT = '/shows'

/**
 * Shape of each segment. Fixed width, so a month has exactly ONE spelling:
 * `/shows/2026/09` is the address and `/shows/2026/9` is a not-found, which is
 * what keeps the canonical honest without a redirect table.
 *
 * The YEAR is bounded as well as shaped, which the other two get for free from
 * their ranges, and it does three things at once:
 *
 *   - `0026` and `2026` would otherwise both parse, while the path builders
 *     emit the year unpadded. `/shows/0026/11` would then advertise a canonical
 *     and a pager pointing at `/shows/26/11`, which is not four digits and so
 *     is a not-found.
 *   - `0000` parses to year 0, and `GET /shows/calendar` reads a zero year as
 *     NO WINDOW: the request would answer with the whole upcoming list under a
 *     heading naming one month.
 *   - It is the crawl bound, and the only one that costs no backend call. Four
 *     digits alone is ten thousand years crossed with twelve months, and every
 *     one of those the proxy has to ask the addressable span about.
 *
 * The range is wide enough to hold every show this catalogue can carry and
 * narrow enough that the addressable space is a few thousand URLs rather than
 * a hundred thousand. It matches the range the API accepts for a year a reader
 * may address.
 */
const YEAR_SEGMENT = /^\d{4}$/
export const SHOWS_CALENDAR_MIN_YEAR = 2000
export const SHOWS_CALENDAR_MAX_YEAR = 2100
const MONTH_SEGMENT = /^(0[1-9]|1[0-2])$/
const DAY_SEGMENT = /^(0[1-9]|[12]\d|3[01])$/

/** Whether a year is inside the addressable range. */
export function isAddressableYearNumber(year: number): boolean {
  return year >= SHOWS_CALENDAR_MIN_YEAR && year <= SHOWS_CALENDAR_MAX_YEAR
}

/** Whether a four-digit year segment is inside the addressable range. */
export function isAddressableYear(yearSegment: string): boolean {
  return YEAR_SEGMENT.test(yearSegment) && isAddressableYearNumber(Number(yearSegment))
}

/** Two digits, the spelling every month and day segment is written in. */
function pad2(value: number): string {
  return String(value).padStart(2, '0')
}

/**
 * Whether a (year, month, day) triple names a day the calendar actually has.
 *
 * `2027-02-31` passes the segment shapes above and does not exist. `Date.UTC`
 * normalizes an out-of-range date, so comparing the components back is the
 * whole check: a day's validity is Gregorian arithmetic, which needs no
 * database, no timezone and no round trip.
 */
export function isRealCalendarDay(
  year: number,
  month: number,
  day: number
): boolean {
  const parsed = new Date(Date.UTC(year, month - 1, day))
  return (
    parsed.getUTCFullYear() === year &&
    parsed.getUTCMonth() === month - 1 &&
    parsed.getUTCDate() === day
  )
}

/**
 * The month window a pair of path segments names, or `null` when the pair
 * cannot be one.
 *
 * SHAPE ONLY. Whether that month has any shows is a data question answered
 * where the data is; this decides what could never be a month whatever the
 * database says, which is the half a crawler can walk for free.
 */
export function parseMonthSegments(
  yearSegment: string,
  monthSegment: string
): ShowsCalendarWindow | null {
  if (!isAddressableYear(yearSegment) || !MONTH_SEGMENT.test(monthSegment)) {
    return null
  }
  return { year: Number(yearSegment), month: Number(monthSegment) }
}

/**
 * The day window a triple of path segments names, or `null`.
 *
 * Adds the calendar test the month parse has no use for: a month segment in
 * range always names a real month, while a day segment in range does not
 * always name a real day.
 */
export function parseDaySegments(
  yearSegment: string,
  monthSegment: string,
  daySegment: string
): ShowsCalendarWindow | null {
  const month = parseMonthSegments(yearSegment, monthSegment)
  if (!month || !DAY_SEGMENT.test(daySegment)) return null
  const day = Number(daySegment)
  if (!isRealCalendarDay(month.year, month.month, day)) return null
  return { ...month, day }
}

/**
 * `/shows/2026/11`.
 *
 * Typed as a rooted path rather than a bare string so it satisfies
 * `listRootCanonical` without a cast: a builder that lost its leading slash
 * would fail the build rather than emit a relative canonical.
 */
export function showsMonthPath(year: number, month: number): `/${string}` {
  return `${SHOWS_ROOT}/${year}/${pad2(month)}`
}

/** `/shows/2026/11/14`. */
export function showsDayPath(
  year: number,
  month: number,
  day: number
): `/${string}` {
  return `${showsMonthPath(year, month)}/${pad2(day)}`
}

/**
 * The path a window is addressed at, month or day.
 *
 * A RUN has no path of its own: it hangs off its anchor day's, which is what
 * makes the run's canonical the day root structurally rather than by
 * remembering to strip a parameter.
 */
export function showsWindowPath(window: ShowsCalendarWindow): `/${string}` {
  return window.day === undefined
    ? showsMonthPath(window.year, window.month)
    : showsDayPath(window.year, window.month, window.day)
}

/**
 * The longest run a day window may name, matching the bound the API enforces.
 *
 * A run is a relative window ("this weekend", "the next seven days") resolved to
 * an absolute anchor rather than an identity anyone bookmarks, and it is bounded
 * on both counts: an unbounded run is a full-catalog scan behind a URL that
 * reads like one day, and anchor-crossed-with-length is the addressable space.
 */
export const SHOWS_WINDOW_MAX_DAYS = 14

/**
 * The window a day route's `?days=` names, or `null` when the parameter refuses
 * one.
 *
 * `null` is a NOT-FOUND, not a fallback to the bare day. A run names the span it
 * lists, so a URL asking for a span this route will not serve must not quietly
 * answer with a different one: the address and the rows have to agree, and the
 * reader who hand-edited the number is the one who would never find out.
 *
 * A run of one is normalized AWAY rather than carried, so `?days=1` renders
 * exactly the day it anchors on, under that day's own title and canonical.
 *
 * Applied to a MONTH window the parameter is ignored: a month is not anchored on
 * a date, so there is no run for the value to name and nothing for it to break.
 * A repeated `?days=` (an array here) names two spans, which is no span at all.
 */
export function applyWindowDays(
  window: ShowsCalendarWindow,
  raw: string | string[] | undefined
): ShowsCalendarWindow | null {
  if (window.day === undefined) return window
  const days = parseWindowDays(raw)
  if (days === null) return null
  return days === undefined ? window : { ...window, days }
}

/**
 * The run length a `?days=` value names: `undefined` when it names none (the
 * parameter is absent, or spells the anchor day alone), a number for a run this
 * route serves, and `null` for a value it refuses.
 *
 * Three answers rather than two because they lead three different places: no run
 * renders the day, a run renders the span, and a refusal is a not-found.
 */
export function parseWindowDays(
  raw: string | string[] | undefined
): number | undefined | null {
  // A key with no value names no run, which is the same as not being there.
  // Every query-string builder writes `days=` when it clears the key, and
  // refusing that would 404 a real day over a URL that said nothing.
  if (raw === undefined || raw === '') return undefined
  // A repeated `?days=` names two spans, which is no span at all.
  if (Array.isArray(raw)) return null
  // `Number` on a blank string is 0, and on a padded or signed one it is a
  // number the reader did not write. The shape test is what keeps `?days=+3`,
  // `?days=03` and `?days=3.0` from all addressing one window.
  if (!/^[1-9]\d*$/.test(raw)) return null
  const days = Number(raw)
  if (days > SHOWS_WINDOW_MAX_DAYS) return null
  return days === 1 ? undefined : days
}

/**
 * The window's own address, the run's `?days=` included.
 *
 * Distinct from {@link showsWindowPath}, which is the run's CANONICAL. The two
 * differ for exactly one window shape, and each call site wants one of them: a
 * link to the run wants this, and the canonical and the pager's base path want
 * the path.
 */
export function showsWindowHref(window: ShowsCalendarWindow): string {
  const path = showsWindowPath(window)
  return window.days === undefined ? path : `${path}?days=${window.days}`
}

/** A calendar date as its parts, with the weekday it fell on (0 is Sunday). */
export interface CalendarDayParts {
  year: number
  /** Calendar month, 1-12, matching this grammar and NOT JavaScript's. */
  month: number
  day: number
  weekday: number
}

/**
 * The calendar date `offset` days from the given one.
 *
 * The one calendar-shift in this feature, so a run's end, a chip's anchor and
 * anything later built on either roll over months, years and leap days by the
 * same arithmetic. `Date.UTC` is calendar arithmetic and nothing else here: no
 * instant is converted between zones, so no offset or DST rule applies.
 */
export function shiftCalendarDay(
  year: number,
  month: number,
  day: number,
  offset: number
): CalendarDayParts {
  const shifted = new Date(Date.UTC(year, month - 1, day + offset))
  return {
    year: shifted.getUTCFullYear(),
    month: shifted.getUTCMonth() + 1,
    day: shifted.getUTCDate(),
    weekday: shifted.getUTCDay(),
  }
}

/**
 * The last venue-local date a run covers. Takes the run's own parts, so a month
 * window, which has no end distinct from its start, cannot reach it.
 */
function runEndDay(
  year: number,
  month: number,
  day: number,
  days: number
): CalendarDayParts {
  return shiftCalendarDay(year, month, day, days - 1)
}

/**
 * The day path for a `2026-11-14` venue-local date key, or `null` when the key
 * is not one.
 *
 * Takes the key rather than its parts because that is the spelling the list's
 * day groups already carry, and re-splitting it at every call site is how a
 * heading and its link would come to disagree about which day they name.
 */
export function showsDayPathFromDateKey(dateKey: string): `/${string}` | null {
  const parts = dateKey.split('-')
  if (parts.length !== 3) return null
  const window = parseDaySegments(parts[0], parts[1], parts[2])
  return window ? showsWindowPath(window) : null
}

/**
 * The month name in full, for a month someone has ALREADY placed.
 *
 * The short form lives in `formatCalendarMonthParts`, which the month strip
 * renders through; this is the same construction with `month: 'long'`, and the
 * two are deliberately the only two spellings of a month name on these
 * surfaces. Locale and zone are pinned for the reason that helper pins them:
 * these labels render on the server and again on the client.
 */
const longMonthFormatter = new Intl.DateTimeFormat('en-US', {
  month: 'long',
  timeZone: 'UTC',
})

/**
 * Mid-month, so no offset or rollover rule can move the month being named. The
 * year never reaches a formatter, which keeps the 0-99 remap `Date.UTC` applies
 * out of reach.
 */
function longMonthName(month: number): string {
  return longMonthFormatter.format(new Date(Date.UTC(2000, month - 1, 15)))
}

/** `November 2026`. */
export function calendarMonthLabel(year: number, month: number): string {
  return `${longMonthName(month)} ${year}`
}

/** `November 14, 2026`. */
export function calendarDayLabel(
  year: number,
  month: number,
  day: number
): string {
  return `${longMonthName(month)} ${day}, ${year}`
}

/** `Sep 18`, the compact form a run's two edges are named in. */
function shortCalendarDayLabel(year: number, month: number, day: number): string {
  return `${formatCalendarMonthParts(year, month).month} ${day}`
}

/**
 * `Sep 18 to Oct 1, 2026`, and `Dec 29, 2026 to Jan 4, 2027` across a new year.
 *
 * The year is never elided: this list runs into the following year, and a span
 * whose edges carry no year is a date a reader has to guess at. It is printed
 * ONCE when both edges share it, which is the ordinary case, and on both edges
 * when they do not.
 *
 * Both edges are the days the window actually holds. The end is the run's LAST
 * date rather than the half-open bound the query carries, because a heading that
 * named a date holding none of the rows beneath it would be false.
 */
function calendarRunLabel(
  year: number,
  month: number,
  day: number,
  days: number
): string {
  const end = runEndDay(year, month, day, days)
  const startLabel = shortCalendarDayLabel(year, month, day)
  const endLabel = shortCalendarDayLabel(end.year, end.month, end.day)
  return year === end.year
    ? `${startLabel} to ${endLabel}, ${end.year}`
    : `${startLabel}, ${year} to ${endLabel}, ${end.year}`
}

/** The reader-facing name of a window: a month, a day, or a run of days. */
export function calendarWindowLabel(window: ShowsCalendarWindow): string {
  if (window.day === undefined) return calendarMonthLabel(window.year, window.month)
  if (window.days === undefined) {
    return calendarDayLabel(window.year, window.month, window.day)
  }
  return calendarRunLabel(window.year, window.month, window.day, window.days)
}

/**
 * The document's own name for a window: `Shows in November 2026`, `Shows on
 * November 14, 2026`, `Shows from Sep 18 to Oct 1, 2026`. A month is a period
 * one is IN, a day is one one is ON, and a run is one one goes FROM and TO;
 * that preposition is the only part that differs.
 *
 * One function because the `<h1>` and the `<title>` must say the same thing,
 * and nothing but this would keep them saying it.
 */
export function showsCalendarWindowTitle(window: ShowsCalendarWindow): string {
  const label = calendarWindowLabel(window)
  if (window.day === undefined) return `Shows in ${label}`
  return window.days === undefined ? `Shows on ${label}` : `Shows from ${label}`
}

/** `Nov 2026`, the compact form the adjacent-month links carry. */
export function shortCalendarMonthLabel(year: number, month: number): string {
  const parts = formatCalendarMonthParts(year, month)
  return `${parts.month} ${parts.year}`
}

/**
 * The window half of a `GET /shows/calendar` request.
 *
 * Written once and shared by the client hook and the route's server seed,
 * because the two have to produce the SAME URL for the seed to be a hit: a
 * drifted pair produces no error anywhere, just a month page that quietly
 * stops being server-rendered.
 */
export function appendShowsCalendarWindow(
  params: URLSearchParams,
  window: ShowsCalendarWindow | undefined
): void {
  if (!window) return
  params.set('year', String(window.year))
  params.set('month', String(window.month))
  if (window.day !== undefined) params.set('day', String(window.day))
  if (window.days !== undefined) params.set('days', String(window.days))
}

/**
 * The cache-key half of the same contract.
 *
 * `undefined` for every field on the unwindowed list, which is what keeps the
 * root's key identical to the one it had before windows existed: react-query
 * hashes keys through `JSON.stringify`, which drops undefined members.
 */
export function showsCalendarWindowKey(
  window: ShowsCalendarWindow | undefined
): { year?: number; month?: number; day?: number; days?: number } {
  return {
    year: window?.year,
    month: window?.month,
    day: window?.day,
    days: window?.days,
  }
}

/** A month the histogram carries, in the shape the strip and the links take. */
interface MonthBucket {
  year: number
  month: number
}

/** Ordering key for a calendar month: `202611`, comparable as a number. */
function monthOrdinal(entry: MonthBucket): number {
  return entry.year * 100 + entry.month
}

/**
 * The months either side of `current` IN THE HISTOGRAM, nearest first.
 *
 * Neighbours are taken from the months that HAVE shows rather than from the
 * calendar, so a link is never offered to a month the route would 404. The
 * histogram's own order is not trusted: these are derived by ordinal so a
 * consumer that hands them over unsorted still gets the true neighbours.
 */
export function adjacentMonths(
  months: readonly MonthBucket[],
  current: MonthBucket
): { previous: MonthBucket | null; next: MonthBucket | null } {
  const target = monthOrdinal(current)
  let previous: MonthBucket | null = null
  let next: MonthBucket | null = null

  for (const entry of months) {
    const ordinal = monthOrdinal(entry)
    if (ordinal < target) {
      if (previous === null || ordinal > monthOrdinal(previous)) previous = entry
    } else if (ordinal > target) {
      if (next === null || ordinal < monthOrdinal(next)) next = entry
    }
  }

  return { previous, next }
}
