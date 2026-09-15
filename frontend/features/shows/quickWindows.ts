/**
 * The quick windows above the `/shows` list: Tonight, This weekend, Next 7 days,
 * This month.
 *
 * Pure: no React, no router, no clock of its own. Every function here takes the
 * civil date it should reason from, which is the only way the weekend rule can
 * be pinned for all seven weekdays without waiting a week to see it.
 *
 * THE WORDS COME FROM THE SCENE WINDOWS and the routes deliberately do not. A
 * scene addresses its windows by path segment (`/scenes/phoenix-az/this-weekend`)
 * because there the window IS the page. Here a window is chrome over a dated
 * list: each chip resolves, at render, to an absolute dated URL that means the
 * same thing tomorrow as it does tonight, which is what a shared link has to do.
 */

import { isValidTimeZone } from '@/lib/utils/formatters'
import {
  shiftCalendarDay,
  showsDayPath,
  showsMonthPath,
  type CalendarDayParts,
} from './showsCalendarRoute'
import { showsPageHref } from './showsListNavigation'

/** The four windows the row offers, keyed by the concept rather than the URL. */
export type QuickWindowKey =
  | 'tonight'
  | 'this-weekend'
  | 'next-7-days'
  | 'this-month'

/** The chip copy. One spelling per window, shared by every surface that links one. */
export const QUICK_WINDOW_LABEL: Record<QuickWindowKey, string> = {
  tonight: 'Tonight',
  'this-weekend': 'This weekend',
  'next-7-days': 'Next 7 days',
  'this-month': 'This month',
}

/** Row order, shortest window first. */
export const QUICK_WINDOW_ORDER: QuickWindowKey[] = [
  'tonight',
  'this-weekend',
  'next-7-days',
  'this-month',
]

/** Days in the "next 7 days" window, the label's own number. */
export const NEXT_7_DAYS = 7

/**
 * A calendar date read on one clock in one zone, in the shape the route grammar
 * names dates in. The weekday travels WITH the date rather than being derived
 * later: deriving it means rebuilding a `Date` from the parts, and a `Date`
 * built from parts carries the runtime's own zone, which is the fault this type
 * exists to keep out.
 */
export type { CalendarDayParts }

/** Weekday numbers, named, so the weekend rule below reads as the rule it is. */
const SUNDAY = 0
const FRIDAY = 5

/** The short weekday names `en-US` prints, in `getDay` order. */
const WEEKDAY_INDEX = new Map<string, number>([
  ['Sun', 0],
  ['Mon', 1],
  ['Tue', 2],
  ['Wed', 3],
  ['Thu', 4],
  ['Fri', 5],
  ['Sat', 6],
])

const dayPartsFormatters = new Map<string, Intl.DateTimeFormat>()

/**
 * The civil date `instant` fell on in `timeZone`, or `null` when the zone is one
 * the runtime cannot resolve.
 *
 * `null` rather than a fallback zone: every date this module produces goes into
 * a URL a reader will share, and a window anchored on the wrong day is wrong in
 * a way nothing on the page reveals. A row of chips that cannot be computed is
 * simply not rendered.
 *
 * The formatter is memoized by zone for the reason the row of shows memoizes its
 * own: constructing an `Intl.DateTimeFormat` is the expensive half.
 */
export function civilDateInZone(
  instant: Date,
  timeZone: string
): CalendarDayParts | null {
  let formatter = dayPartsFormatters.get(timeZone)
  if (!formatter) {
    // The validity question is asked through the memoized probe the date
    // helpers already share, rather than by catching the constructor here,
    // which would be a second cache of one answer.
    if (!isValidTimeZone(timeZone)) return null
    formatter = new Intl.DateTimeFormat('en-US', {
      timeZone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      weekday: 'short',
    })
    dayPartsFormatters.set(timeZone, formatter)
  }

  const parts = formatter.formatToParts(instant)
  const value = (type: string) => parts.find(part => part.type === type)?.value
  const year = Number(value('year'))
  const month = Number(value('month'))
  const day = Number(value('day'))
  const weekday = WEEKDAY_INDEX.get(value('weekday') ?? '')
  if (
    !Number.isFinite(year) ||
    !Number.isFinite(month) ||
    !Number.isFinite(day) ||
    weekday === undefined
  ) {
    return null
  }
  return { year, month, day, weekday }
}

/** One chip: where it points, and how long a run it asks for. */
export interface QuickWindowTarget {
  key: QuickWindowKey
  label: string
  /** The dated path, always a month root or a day root. */
  path: `/${string}`
  /** Days in the run, or `undefined` when the path alone is the window. */
  days?: number
}

/**
 * The weekend in progress or next up, as an anchor date and a length.
 *
 * The weekend is Friday night through Sunday, and it is the CURRENT one from
 * Friday until Sunday ends. So the anchor is the later of this weekend's Friday
 * and today, and the length is however many days are left from the anchor
 * through Sunday: three on Monday through Friday, two on Saturday, one on
 * Sunday.
 *
 * Anchoring on today rather than on Friday from Saturday on is what keeps the
 * chip honest in both directions. A Friday anchor would name a night that has
 * already happened, and the list it reaches is the UPCOMING partition, which no
 * longer holds it: the URL would promise three nights and the page would show
 * two, with no way for a reader to tell which.
 */
function weekendWindow(today: CalendarDayParts): { anchor: CalendarDayParts; days: number } {
  // Sunday is `0`, so it is the one weekday that is not "days until Friday"
  // arithmetic: the weekend it belongs to began two days BEFORE it.
  if (today.weekday === SUNDAY) return { anchor: today, days: 1 }
  const untilFriday = FRIDAY - today.weekday
  if (untilFriday > 0) {
    return {
      anchor: shiftCalendarDay(today.year, today.month, today.day, untilFriday),
      days: 3,
    }
  }
  // Friday itself (0) and Saturday (-1): the anchor is today and the window ends
  // on the Sunday that is `2 + untilFriday` days out.
  return { anchor: today, days: 3 + untilFriday }
}

/**
 * The four chips as of `today`, in row order.
 *
 * Every target is ABSOLUTE. The relative word is the label; the href names the
 * dates, so a link shared on Friday still opens that weekend on Monday.
 *
 * A run of one day is emitted as the bare day path rather than as `?days=1`,
 * the same normalization the route applies when it reads the parameter back, so
 * no chip mints a second spelling of a window that already has one.
 *
 * Two chips can name ONE window: on a Sunday the weekend still running is one
 * night, which is also tonight. The row is a set of true descriptions, not a
 * partition, and the current-chip rule is what keeps one of them marked.
 */
export function quickWindowTargets(today: CalendarDayParts): QuickWindowTarget[] {
  const weekend = weekendWindow(today)
  const byKey: Record<QuickWindowKey, QuickWindowTarget> = {
    tonight: {
      key: 'tonight',
      label: QUICK_WINDOW_LABEL.tonight,
      path: showsDayPath(today.year, today.month, today.day),
    },
    'this-weekend': {
      key: 'this-weekend',
      label: QUICK_WINDOW_LABEL['this-weekend'],
      path: showsDayPath(weekend.anchor.year, weekend.anchor.month, weekend.anchor.day),
      days: weekend.days > 1 ? weekend.days : undefined,
    },
    'next-7-days': {
      key: 'next-7-days',
      label: QUICK_WINDOW_LABEL['next-7-days'],
      path: showsDayPath(today.year, today.month, today.day),
      days: NEXT_7_DAYS,
    },
    'this-month': {
      key: 'this-month',
      label: QUICK_WINDOW_LABEL['this-month'],
      path: showsMonthPath(today.year, today.month),
    },
  }
  return QUICK_WINDOW_ORDER.map(key => byKey[key])
}

/**
 * A chip's href, built FROM the params already on screen.
 *
 * Every key but `days` and `page` is carried through, so the city filter, the
 * tag filter and whatever a campaign link brought along survive the jump. A chip
 * that minted a bare path would silently drop an explicit All Cities back to the
 * viewer's derived default.
 *
 * `page` goes because a different window is a different question, answered from
 * its first page, and `days` is set or cleared because the chip owns that key.
 */
export function quickWindowHref(
  params: URLSearchParams | { toString: () => string },
  target: QuickWindowTarget
): string {
  const next = new URLSearchParams(params.toString())
  if (target.days === undefined) next.delete('days')
  else next.set('days', String(target.days))
  return showsPageHref(next, 1, target.path)
}

/**
 * Whether a chip names the window the reader is already looking at.
 *
 * Compared on the PATH and the run, never on the whole URL: a filter or a page
 * number is a different slice of the same window. `currentDays` is the run the
 * route resolved, so `?days=1` and no parameter are one window here too.
 */
export function isQuickWindowCurrent(
  target: QuickWindowTarget,
  pathname: string,
  currentDays: number | undefined
): boolean {
  return pathname === target.path && currentDays === target.days
}
