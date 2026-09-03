import type { components } from '@/types/api'
import { formatShowTime, formatShowTimeCompact } from '@/lib/utils/formatters'
import { hasShowStarted } from '@/lib/utils/showTiming'
import {
  parseCalendarDate,
  startInstant,
  type SceneWeekShow,
} from './sceneWeek'

/**
 * Scene-day types, DERIVED from the generated OpenAPI schema for the same
 * reason the scene-week types are: a hand-written type can claim a field the
 * API never sends, and the mismatch is invisible to CI while being fatal in
 * production.
 */
export type SceneDayResponse = components['schemas']['SceneDayResponse']
export type SceneTrackedVenue = components['schemas']['SceneTrackedVenue']
/** Same row shape the week view renders; aliased so day code reads as day code. */
export type SceneDayShow = SceneWeekShow

/** ISO calendar date, e.g. `2026-07-31`. */
const CALENDAR_DATE_KEY = /^\d{4}-\d{2}-\d{2}$/

/**
 * Earliest year worth serving, and how far ahead. Identical bounds to the week
 * key's, and for the identical reason: the date segment is dynamic, so an
 * unbounded key space is an unbounded set of distinct URLs, each one a cache
 * miss. The backend remains the authority on whether a date is real.
 */
const FIRST_TRACKED_YEAR = 2015

/**
 * Whether a URL segment even looks like a calendar date.
 *
 * A cheap shape check only — it deliberately does NOT decide whether the date
 * exists. `2026-02-30` is well-formed and impossible, and only the backend,
 * which owns the calendar maths and the scene's timezone, can say so. This
 * exists to avoid a pointless round-trip for obvious junk.
 *
 * Matches the RAW segment, deliberately: trimming here would accept spellings
 * the proxy's copy of this rule rejects, so a page and its existence check
 * would disagree about which URLs are legal. One spelling per day.
 */
export function looksLikeCalendarDate(segment: string): boolean {
  if (!CALENDAR_DATE_KEY.test(segment)) return false
  const year = Number(segment.slice(0, 4))
  return year >= FIRST_TRACKED_YEAR && year <= new Date().getUTCFullYear() + 1
}

/** `Friday, July 31, 2026` — the header's date line and the page title's date. */
export function formatDayFull(iso: string): string {
  return parseCalendarDate(iso).toLocaleDateString('en-US', {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  })
}

/**
 * `Thu Jul 30` — the adjacent-day nav chips.
 *
 * Composed rather than taken from one `toLocaleDateString` call, which punctuates
 * it as a sentence fragment ("Thu, Jul 30"). A chip is a label, and the comma is
 * noise inside a bordered box already flanked by an arrow.
 */
export function formatDayChip(iso: string): string {
  const date = parseCalendarDate(iso)
  const weekday = date.toLocaleDateString('en-US', { weekday: 'short' })
  const month = date.toLocaleDateString('en-US', { month: 'short' })
  return `${weekday} ${month} ${date.getDate()}`
}

/**
 * How far ahead a day can be named by its WEEKDAY alone and still be
 * unambiguous. Past that, "Friday" could be any of several Fridays.
 */
const WEEKDAY_ONLY_HORIZON_DAYS = 6

/**
 * The day label in a quiet night's "next on our calendar" pointer.
 *
 * A bare weekday is how anyone would say a near date out loud, and it is only
 * legible from the night you are actually standing in — "Friday" means *this*
 * Friday to a reader, whatever date the page is about. So the shorthand is
 * reserved for the live night; a dated permalink always spells the date out,
 * or a visitor arriving from search at a page about January 2020 would read
 * "Friday" as this week.
 *
 * Even on the live night the shorthand runs out: a show five weeks ahead names
 * a Friday the reader has no way to identify, so the date joins it.
 */
export function formatPointerDay(
  fromISO: string,
  targetISO: string,
  isLiveNight: boolean
): string {
  const from = parseCalendarDate(fromISO)
  const target = parseCalendarDate(targetISO)
  const days = Math.round((target.getTime() - from.getTime()) / 86_400_000)
  if (isLiveNight && days > 0 && days <= WEEKDAY_ONLY_HORIZON_DAYS) {
    return target.toLocaleDateString('en-US', { weekday: 'long' })
  }
  return target.toLocaleDateString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    // The YEAR whenever it differs from the page's own. A page headed
    // "January 15, 2020" saying "Next on our calendar: Sat, Aug 8" reads as
    // August 2020 to every reader — dropping the weekday shorthand is not
    // enough on its own, because a bare month and day is just as relative.
    ...(target.getFullYear() !== from.getFullYear() ? { year: 'numeric' as const } : {}),
  })
}

/**
 * The row's start time, in the VENUE's zone — `8:00 PM`.
 *
 * Guards the instant through the SAME check the structured data uses, so a show
 * the JSON-LD refuses to describe is exactly the show whose row shows no time.
 *
 * `starts_at` is an absolute instant, so it has to be rendered against a zone
 * or it reads as the viewer's, which is the one zone that is never right here.
 * The scene's own zone is the fallback rather than the state map, matching the
 * JSON-LD: the backend bucketed this show into this day using the scene's modal
 * venue zone, and rendering it against a different one is how a listed time
 * ends up disagreeing with the date heading above it.
 *
 * Returns null when the payload cannot supply a usable instant — a row with no
 * time still lists its bill and venue, which is most of its value.
 */
export function formatShowStartTime(
  show: SceneDayShow,
  sceneTimezone?: string | null
): string | null {
  const raw = startInstant(show)
  if (raw === null) return null
  return formatShowTime(raw, show.venue_state, show.venue_timezone || sceneTimezone)
}

/**
 * The same start time in the COMPACT ledger register — `8PM`, `7:30PM`.
 *
 * For dense fixed-width lead columns; see {@link formatShowTimeCompact} for why
 * two registers are allowed to share a page. Identical zone resolution and
 * identical instant guard to `formatShowStartTime` above, so a surface can move
 * between the two registers without moving a clock.
 */
export function formatShowStartTimeCompact(
  show: SceneDayShow,
  sceneTimezone?: string | null
): string | null {
  const raw = startInstant(show)
  if (raw === null) return null
  return formatShowTimeCompact(
    raw,
    show.venue_state,
    show.venue_timezone || sceneTimezone
  )
}

/**
 * The order a night's rows are READ in, which is not always the order they
 * happened in (user decision, PSY-1969).
 *
 * On the LIVE night, a show that has already started sorts after every show
 * that has not. A reader opening a night's page at 22:00 is deciding where to
 * go next, and the 19:00 doors at the top of a clock-ordered list are the rows
 * that answer that question least. Nothing is dropped: an early show may well
 * still be going, and the reader — not this function — is entitled to decide
 * whether it is worth walking to.
 *
 * Only on the live night, and `isTonight` is the BACKEND's answer, computed on
 * the scene's clock against its 6am night boundary. An archive or future night
 * has no started rows to sink and must stay in clock order, which is the order
 * a schedule is read in.
 *
 * Stable within each half, so the input's clock order survives into both: the
 * upcoming rows stay earliest-first and the started rows stay in the order they
 * began.
 *
 * `hasShowStarted` counts an unreadable instant as started, so a row whose date
 * cannot be parsed sinks with them rather than heading the list.
 */
export function orderNightShows<T extends SceneDayShow>(
  shows: T[],
  isTonight: boolean,
  now: Date = new Date()
): T[] {
  if (!isTonight) return shows
  const upcoming: T[] = []
  const started: T[] = []
  for (const show of shows) {
    if (hasShowStarted(show.starts_at, now)) started.push(show)
    else upcoming.push(show)
  }
  // The identity is returned when nothing sank, so a caller comparing
  // references (a memo, a test) sees no change on the overwhelming majority of
  // nights.
  if (started.length === 0 || upcoming.length === 0) return shows
  return [...upcoming, ...started]
}

/**
 * Shows for the day, and the ONLY count of them.
 *
 * `shows` is typed nullable by the generator even though the API always emits
 * an array. Deliberately no `show_count` reader alongside this: the payload
 * carries one, but it is `len(shows)` computed by the same handler that
 * serialized the slice, so a header sourced from it could only ever agree with
 * the list — or, if it somehow did not, would state a number the page cannot
 * show. The rows are the truth; the header counts them.
 */
export function dayShows(day: SceneDayResponse): SceneDayShow[] {
  return day.shows ?? []
}

/** The rooms this scene draws from. Nullable by the generator, never in practice. */
export function dayTrackedVenues(day: SceneDayResponse): SceneTrackedVenue[] {
  return day.tracked_venues ?? []
}

/**
 * The meta line's count phrase — `4 shows`, or `0 shows listed`.
 *
 * The empty case says "listed" and the populated case does not, deliberately.
 * We only ever know our own calendar: claiming "0 shows" would assert that
 * nothing is happening in the city, which is a claim this site is not entitled
 * to make about rooms it tracks a slice of.
 */
export function formatDayCountLine(total: number): string {
  if (total === 0) return '0 shows listed'
  return `${total} ${total === 1 ? 'show' : 'shows'}`
}
