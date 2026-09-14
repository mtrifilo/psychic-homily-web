/**
 * The date-addressed half of the `/shows` URL family: `/shows/{yyyy}/{mm}` and
 * `/shows/{yyyy}/{mm}/{dd}`.
 *
 * Pure: no React, no router, no fetching, so the rules that decide whether a
 * path segment names a month can be tested without a rendered route. The proxy
 * keeps its own copy of the SHAPE rules (it must not import `features/`, the
 * same constraint the scenes and venue-year branches work under); the two are
 * pinned against each other by `proxy.shows-calendar.test.ts`.
 */

/** A month or day window of the upcoming list, as a route addresses it. */
export interface ShowsCalendarWindow {
  year: number
  /** Calendar month, 1-12, matching the wire format and NOT JavaScript's. */
  month: number
  /** Calendar day of month, 1-31. Absent on a month window. */
  day?: number
}

/** The list root every window hangs off. */
export const SHOWS_ROOT = '/shows'

/**
 * Shape of each segment. Fixed width, so `/shows/2026/9` and `/shows/2026/09`
 * cannot both address November's neighbour: one spelling per month is what
 * keeps the canonical honest without a redirect table.
 */
const YEAR_SEGMENT = /^\d{4}$/
const MONTH_SEGMENT = /^(0[1-9]|1[0-2])$/
const DAY_SEGMENT = /^(0[1-9]|[12]\d|3[01])$/

/** Two digits, the spelling every month and day segment is written in. */
function pad2(value: number): string {
  return String(value).padStart(2, '0')
}

/**
 * Whether a (year, month, day) triple names a day the calendar actually has.
 *
 * `2027-02-31` passes the segment shapes above and does not exist. `Date.UTC`
 * normalizes an out-of-range date, so comparing the components back is the
 * whole check — the same derivation `proxy.ts` uses for scene day permalinks,
 * for the same reason: Gregorian arithmetic needs no database and no timezone.
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
  if (!YEAR_SEGMENT.test(yearSegment) || !MONTH_SEGMENT.test(monthSegment)) {
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

/** The path a window is addressed at, month or day. */
export function showsWindowPath(window: ShowsCalendarWindow): `/${string}` {
  return window.day === undefined
    ? showsMonthPath(window.year, window.month)
    : showsDayPath(window.year, window.month, window.day)
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
 * Locale and zone are pinned for the reason every other formatter in this repo
 * pins them: these labels render on the server and again on the client, and a
 * runtime that named months differently would hydrate into a mismatch. UTC
 * because the caller is naming a month that is ALREADY placed, not reading an
 * instant on some calendar.
 */
const monthNameFormatter = new Intl.DateTimeFormat('en-US', {
  month: 'long',
  timeZone: 'UTC',
})

/** `November 2026`. */
export function calendarMonthLabel(year: number, month: number): string {
  // Mid-month, so no offset or rollover rule can move the month being named.
  // The year is returned verbatim rather than formatted, which keeps the 0-99
  // remap `Date.UTC` applies out of reach.
  return `${monthNameFormatter.format(new Date(Date.UTC(2000, month - 1, 15)))} ${year}`
}

/** `November 14, 2026`. */
export function calendarDayLabel(
  year: number,
  month: number,
  day: number
): string {
  return `${monthNameFormatter.format(new Date(Date.UTC(2000, month - 1, 15)))} ${day}, ${year}`
}

/** The reader-facing name of a window, month or day. */
export function calendarWindowLabel(window: ShowsCalendarWindow): string {
  return window.day === undefined
    ? calendarMonthLabel(window.year, window.month)
    : calendarDayLabel(window.year, window.month, window.day)
}

/** `Nov 2026`, the compact form the adjacent-month links carry. */
const shortMonthFormatter = new Intl.DateTimeFormat('en-US', {
  month: 'short',
  timeZone: 'UTC',
})

export function shortCalendarMonthLabel(year: number, month: number): string {
  return `${shortMonthFormatter.format(new Date(Date.UTC(2000, month - 1, 15)))} ${year}`
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
): { year?: number; month?: number; day?: number } {
  return {
    year: window?.year,
    month: window?.month,
    day: window?.day,
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
