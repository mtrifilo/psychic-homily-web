/**
 * The scene window family: the pure half.
 *
 * A "window" is a stretch of a scene's calendar addressed by a PATH SEGMENT —
 * `/tonight`, `/this-weekend`, `/week`, `/next-4-weeks`. Never a query param:
 * the prior art agrees, and the two segments that shipped first already do it.
 *
 * Everything here is a pure function over day rows the backend already resolved
 * in the scene's own timezone, so each window's bounds can be tested against a
 * pinned clock. That is the only way to catch the off-by-one-day faults this
 * module exists to avoid — the same reason `sceneCalendar.ts` is split this way.
 *
 * Why these windows compose WEEK payloads rather than reading
 * `GET /scenes/{slug}/shows`: that endpoint bounds its lower edge at
 * `event_date >= now()` as a UTC INSTANT, and a date-only show is stored at UTC
 * midnight — so tonight's date-only shows are already gone from it by the time
 * anyone is deciding where to go (verified against a live scene, PSY-1849). The
 * week endpoint bounds on scene-local calendar dates instead, which is the only
 * source on this surface that can answer "what is on tonight" honestly.
 */

import {
  formatCalendarMonthDay,
  isCalendarDate,
  parseCalendarDate,
  type SceneWeekDay,
  type SceneWeekResponse,
} from './sceneWeek'
// Type-only, so this module stays a runtime leaf: nothing here pulls the view's
// graph. The data type lives HERE rather than beside the component because the
// page module and the JSON-LD builder both consume it, and a data shape defined
// inside a view makes every non-view consumer depend on the view to describe it.
import type { TrackedRoom } from './components/sceneChrome'

/** The four windows the family locks, keyed by the concept rather than the path. */
export type SceneWindowKey = 'tonight' | 'this-weekend' | 'this-week' | 'next-4-weeks'

/**
 * Key → path segment. `this-week` is the one that differs, and deliberately:
 * the route shipped as `/week` before the family had names, and changing a URL
 * that has been shared costs more than the small asymmetry of mapping it here.
 */
const WINDOW_SEGMENT: Record<SceneWindowKey, string> = {
  tonight: 'tonight',
  'this-weekend': 'this-weekend',
  'this-week': 'week',
  'next-4-weeks': 'next-4-weeks',
}

/** The chip copy. One spelling per window, shared by every page that links one. */
export const SCENE_WINDOW_LABEL: Record<SceneWindowKey, string> = {
  tonight: 'Tonight',
  'this-weekend': 'This weekend',
  'this-week': 'This week',
  'next-4-weeks': 'Next 4 weeks',
}

/** Family order, so every strip lists the windows shortest-to-longest. */
export const SCENE_WINDOW_ORDER: SceneWindowKey[] = [
  'tonight',
  'this-weekend',
  'this-week',
  'next-4-weeks',
]

/** `/scenes/phoenix-az/this-weekend`. Slug encoded — it reaches here from a route param. */
export function sceneWindowHref(slug: string, key: SceneWindowKey): string {
  return `/scenes/${encodeURIComponent(slug)}/${WINDOW_SEGMENT[key]}`
}

/**
 * `Sep 14`, the `{MON D}` stem every dated label in this family is built from.
 *
 * Delegates to `formatCalendarMonthDay`, which carries the shape guard and the
 * component-wise parse this form depends on: `new Date('2026-09-14')` is UTC
 * midnight, which prints as Sep 13 in every negative-offset zone and would name
 * the wrong night in a title. A value that is not a calendar date falls back to
 * itself rather than to a confident wrong date.
 */
export function formatMonthDay(iso: string): string {
  return formatCalendarMonthDay(iso) ?? iso
}

/**
 * `This week in Chicago`, the family's one title rule, `{WINDOW} in {CITY}`.
 *
 * Every route in the family builds its H1 and its `<title>` through one of the
 * three functions below, so the phrase a reader saw in a tab is the phrase at
 * the top of the page. They are the only public spelling of the rule: composing
 * it at a call site is how the tab and the heading drift apart.
 */
function windowTitle(phrase: string, city: string): string {
  return `${phrase} in ${city}`
}

/** `This weekend in Phoenix`, one of the four rolling windows. */
export function sceneWindowTitle(key: SceneWindowKey, city: string): string {
  return windowTitle(SCENE_WINDOW_LABEL[key], city)
}

/**
 * `Tonight in Phoenix` on the rolling route, `Sep 14 in Phoenix` on a dated
 * permalink.
 *
 * The discriminator is the ROUTE, not the payload's `is_tonight`: that flag is
 * also true for the dated permalink naming today, and a permanent URL that
 * calls itself "tonight" is false from the following morning on.
 */
export function sceneDayTitle(date: string, city: string, isRollingRoute: boolean): string {
  return isRollingRoute
    ? sceneWindowTitle('tonight', city)
    : windowTitle(formatMonthDay(date), city)
}

/**
 * `This week in Chicago` on the rolling route, `Week of Sep 7 in Chicago` on a
 * dated permalink.
 *
 * The discriminator is the ROUTE, not the payload's `is_current_week`, and it is the
 * same rule, for the same reason, as the day above. That flag is true for the
 * dated permalink of the week now in progress, and that permalink is permanent:
 * it is its own canonical and the form the sitemap announces, so a title saying
 * "this week" there is false from the following Monday and stays in the index
 * saying it.
 */
export function sceneWeekTitle(
  startDate: string,
  city: string,
  isRollingRoute: boolean
): string {
  return isRollingRoute
    ? sceneWindowTitle('this-week', city)
    : windowTitle(sceneWeekName(startDate), city)
}

/** `Week of Sep 7`, a week named by its own Monday. */
function sceneWeekName(startDate: string): string {
  return `Week of ${formatMonthDay(startDate)}`
}

/**
 * The same week as a CLAUSE: `this week`, or `the week of Sep 7`.
 *
 * The form that follows a preposition or a verb ("we track {clause}", "share
 * {clause}"), so a page's heading, its share control and its quiet copy spell
 * one week one way. Lowercasing the name would not do: it would print "week of
 * sep 7".
 *
 * Keyed on the route for the reason `sceneWeekTitle` gives: only the rolling
 * URL may say "this week", because only it will still mean this week tomorrow.
 */
export function sceneWeekClause(startDate: string, isRollingRoute: boolean): string {
  return isRollingRoute ? 'this week' : `the week of ${formatMonthDay(startDate)}`
}

/**
 * How a neighbouring week reads in the prev/next row.
 *
 * Relative on the ROLLING route, where "last" and "next" are read against a
 * page that is always the current week, and named by its own Monday on a
 * permalink, the same idiom the day row uses, and the same route rule as the
 * title. A neighbour is never called "this week": identifying one as the
 * current week would take a clock this payload does not carry, and a wrong
 * "this week" is a claim about now.
 *
 * The neighbour's date is arithmetic on the week's own start, which is the same
 * arithmetic the key in the href comes from: `prev_week` and `next_week` are
 * `ISOWeekKey(start ± 7 days)` (backend/internal/services/catalog/scene_week.go),
 * so the label and the link name one week.
 */
export function sceneWeekStepLabel(
  startDate: string,
  direction: 'prev' | 'next',
  isRollingRoute: boolean
): string {
  if (isRollingRoute) return direction === 'prev' ? 'Last week' : 'Next week'
  const neighbour = shiftCalendarDate(startDate, direction === 'prev' ? -7 : 7)
  // A start date this page cannot read names no neighbour. The shift runs
  // BEFORE the format, so `formatMonthDay`'s own fallback cannot catch it:
  // `parseCalendarDate('')` is 1 Jan 1900, and seven days either side of that
  // is a perfectly well-formed `Week of Dec 25` nobody can check. The
  // page-relative words are true for the one week this row leads to whatever
  // the payload says, which is the only claim left to make.
  if (neighbour === null) return direction === 'prev' ? 'Previous week' : 'Next week'
  return sceneWeekName(neighbour)
}

/**
 * The calendar date `days` away, as `YYYY-MM-DD`, or null when the input is not
 * a calendar date to begin with.
 */
function shiftCalendarDate(iso: string, days: number): string | null {
  if (!isCalendarDate(iso.trim())) return null
  const date = parseCalendarDate(iso.trim())
  date.setDate(date.getDate() + days)
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${date.getFullYear()}-${month}-${day}`
}

/**
 * Everything a window page renders, resolved before it reaches the view.
 *
 * Lives in this module rather than beside the component so the page module and
 * the JSON-LD builder can describe their input without importing a view.
 */
export interface SceneWindowData {
  window: SceneWindowKey
  slug: string
  sceneName: string
  city: string
  state: string
  /**
   * IANA zone the window's dates were resolved in, for structured data. Null
   * when the payload could not name one, which keeps a guessed zone out of a
   * published `startDate` offset.
   */
  timezone: string | null
  /** Day rows, already sliced to the window and capped. */
  days: SceneWeekDay[]
  /** Shows actually rendered — may be under the window's true total. */
  rendered: number
  /** Whether the row cap cut the list. */
  truncated: boolean
  trackedVenues: TrackedRoom[]
  /**
   * The window one step wider than this one, for the quiet state. Null when
   * nothing in the family is wider.
   */
  widerWindow: SceneWindowKey | null
}

/**
 * A list surface filtered to one scene, e.g. `/shows?cities=Phoenix%2CAZ`.
 *
 * The single home of "scene pair to city-filtered list URL" for this feature,
 * so every destination the scene pages point at spells the param one way.
 *
 * The `?cities=` value is built here rather than through
 * `components/filters/cityParams`, whose module pulls `nuqs` in for a parser
 * these server-rendered pages never use. The format is one pair, `City,ST`.
 *
 * Encoding is transport hygiene only. `parseCitiesParam` reads the value after
 * the URL layer has decoded it, so it splits on a `|` or a `,` that came out of
 * a city name exactly as it splits on the ones written here. One pair in means
 * one pair out only for city names that carry neither character.
 *
 * @param basePath A LITERAL in-app route prefix that parses `?cities=` in this
 *   wire format (`/shows`, `/artists`, `/venues`), with no trailing slash and
 *   no query string of its own. Never a computed or caller-supplied value: it
 *   is interpolated unencoded, so an absolute URL here would build an off-site
 *   link.
 * @param extraParams Further params for the destination, appended in the order
 *   given and encoded. Keys and values are for the caller's own literals, not
 *   for anything a reader supplies.
 */
export function sceneCityListHref(
  basePath: string,
  city: string,
  state: string,
  extraParams?: Record<string, string>
): string {
  const cities = `cities=${encodeURIComponent(`${city},${state}`)}`
  if (!extraParams) return `${basePath}?${cities}`

  const query = [cities]
  for (const [key, value] of Object.entries(extraParams)) {
    query.push(`${encodeURIComponent(key)}=${encodeURIComponent(value)}`)
  }
  return `${basePath}?${query.join('&')}`
}

/**
 * The city's whole upcoming listing, `/shows?cities=City%2CST` — the way out of
 * a window that could not hold everything, and of the widest window when it is
 * empty.
 */
export function allUpcomingHref(city: string, state: string): string {
  return sceneCityListHref('/shows', city, state)
}

/** The rolling window `/next-4-weeks` serves: 28 days from tonight. */
export const NEXT_4_WEEKS_DAYS = 28

/**
 * Week payloads to fetch to guarantee 28 rolling days.
 *
 * FIVE, not four. The window opens on whatever weekday it happens to be, so
 * four Monday-anchored weeks only reach 28 days ahead when today IS Monday; on
 * a Sunday they cover eight. The fifth week is what makes the label true every
 * day of the week, and the surplus is discarded by `rollingDays`.
 */
export const NEXT_4_WEEKS_FETCH_WEEKS = 5

/**
 * How many rows a window page renders.
 *
 * 28 days of a dense scene is several hundred bills, and a reader asked to
 * scroll that is not being served.
 *
 * This used to mirror a cap of the same size on the scene ROOT. That one is
 * gone (PSY-1850): the root shows tonight and the next day and points here, so
 * this is now the only row cap in the family rather than the second copy of one.
 */
export const SCENE_WINDOW_ROW_CAP = 60

/** Go/JS weekday numbers for the nights the locked weekend covers. */
const WEEKEND_WEEKDAYS = new Set([5, 6, 0]) // Fri, Sat, Sun

/**
 * Friday, Saturday and Sunday of a week payload.
 *
 * Selected by WEEKDAY rather than by slicing `days[4..6]`. The backend seeds
 * seven Monday-anchored days so the slice would be correct today, but an index
 * silently mislabels every row the day that seeding changes, whereas a weekday
 * test is wrong only if the date itself is wrong. Dates are parsed
 * component-wise (`parseCalendarDate`) because `new Date('2026-08-21')` is UTC
 * midnight, which reads as the previous day in every negative-offset zone —
 * exactly the fault that would shift the whole weekend by one.
 */
export function weekendDays(days: SceneWeekDay[]): SceneWeekDay[] {
  return days.filter(day => WEEKEND_WEEKDAYS.has(parseCalendarDate(day.date).getDay()))
}

/** Consecutive week payloads flattened into one ordered day list. */
export function flattenWeekDays(weeks: SceneWeekResponse[]): SceneWeekDay[] {
  return weeks.flatMap(week => week.days ?? [])
}

/**
 * The days from `fromDate` forward, at most `count` of them.
 *
 * Drops days already past, which is the whole reason the window can call itself
 * "next 4 weeks" honestly: a window anchored on the week's Monday would spend
 * up to six of its days describing nights that have already happened, and a
 * label naming a forward stretch of time while listing a backward one is the
 * kind of wrong that looks authoritative.
 *
 * ISO dates compare correctly as strings (`2026-08-09` < `2026-08-10`), so no
 * parsing is needed to order them — and none is wanted, since parsing is where
 * the timezone faults come from.
 */
export function rollingDays(
  days: SceneWeekDay[],
  fromDate: string,
  count: number
): SceneWeekDay[] {
  return days.filter(day => day.date >= fromDate).slice(0, count)
}

/** Total shows across a window's day rows. */
export function countWindowShows(days: SceneWeekDay[]): number {
  return days.reduce((n, day) => n + (day.shows?.length ?? 0), 0)
}

/**
 * A window's days truncated to `rowCap` SHOWS, not to `rowCap` days.
 *
 * Truncation is reported rather than inferred. A caller comparing the rendered
 * count to the cap cannot tell a cut list from a window that happens to hold
 * exactly that many, and getting it wrong either invents a disclosure or
 * silently drops a date from a page whose entire job is listing them.
 *
 * A day that would be split by the cap is kept with its rows trimmed, and the
 * caller is told which one — printing that day's partial count in the same
 * register as every verified count would state a per-day figure nobody checked.
 */
export function capWindowRows(
  days: SceneWeekDay[],
  rowCap: number
): { days: SceneWeekDay[]; rendered: number; truncated: boolean } {
  const total = countWindowShows(days)
  if (total <= rowCap) return { days, rendered: total, truncated: false }

  const capped: SceneWeekDay[] = []
  let rendered = 0
  for (const day of days) {
    if (rendered >= rowCap) break
    const shows = day.shows ?? []
    const room = rowCap - rendered
    capped.push(shows.length <= room ? day : { ...day, shows: shows.slice(0, room) })
    rendered += Math.min(shows.length, room)
  }
  return { days: capped, rendered, truncated: true }
}

/**
 * `Fri Aug 21 – Sun Aug 23, 2026` — a window's span, read off the rows it
 * actually renders rather than off the window's nominal bounds.
 *
 * Naming a span the page does not list would be a claim it cannot keep: a
 * weekend viewed on Sunday renders one night, and a header still promising
 * three would be false. Returns null for an empty window, whose header has no
 * span to state.
 */
export function formatWindowRange(days: SceneWeekDay[]): string | null {
  if (days.length === 0) return null
  const first = days[0].date
  const last = days[days.length - 1].date
  const fmt = (iso: string) =>
    parseCalendarDate(iso).toLocaleDateString('en-US', {
      weekday: 'short',
      month: 'short',
      day: 'numeric',
    })
  const year = parseCalendarDate(last).getFullYear()
  if (first === last) return `${fmt(first)}, ${year}`
  return `${fmt(first)} – ${fmt(last)}, ${year}`
}
