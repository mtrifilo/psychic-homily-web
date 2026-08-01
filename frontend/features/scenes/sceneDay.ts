import type { components } from '@/types/api'
import { formatPrice, formatShowTime } from '@/lib/utils/formatters'
import { parseCalendarDate, type SceneWeekShow } from './sceneWeek'

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
 * The day label in a quiet night's "next on our calendar" pointer, relative to
 * the day being viewed.
 *
 * Near dates read as a bare weekday, which is how anyone would say it out loud.
 * A show five weeks out cannot: "Friday" would name a Friday the reader has no
 * way to identify, so the date joins it.
 */
export function formatPointerDay(fromISO: string, targetISO: string): string {
  const from = parseCalendarDate(fromISO)
  const target = parseCalendarDate(targetISO)
  const days = Math.round((target.getTime() - from.getTime()) / 86_400_000)
  if (days > 0 && days <= WEEKDAY_ONLY_HORIZON_DAYS) {
    return target.toLocaleDateString('en-US', { weekday: 'long' })
  }
  return target.toLocaleDateString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
  })
}

/**
 * The row's start time, in the VENUE's zone — `8:00 PM`.
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
  const raw = show.starts_at
  if (typeof raw !== 'string' || !Number.isFinite(Date.parse(raw))) return null
  return formatShowTime(raw, show.venue_state, show.venue_timezone || sceneTimezone)
}

/** The row's price, or null when the show has none recorded. */
export function formatShowPrice(show: SceneDayShow): string | null {
  return typeof show.price === 'number' ? formatPrice(show.price) : null
}

/** Shows for the day. `shows` is typed nullable by the generator. */
export function dayShows(day: SceneDayResponse): SceneDayShow[] {
  return day.shows ?? []
}

/** The rooms this scene draws from. Nullable by the generator, never in practice. */
export function dayTrackedVenues(day: SceneDayResponse): SceneTrackedVenue[] {
  return day.tracked_venues ?? []
}

/**
 * Total shows listed.
 *
 * Prefers the server's `show_count` but falls back to counting, because a
 * header that disagrees with the list below it is worse than a recount.
 */
export function countDayShows(day: SceneDayResponse): number {
  if (typeof day.show_count === 'number') return day.show_count
  return dayShows(day).length
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

/**
 * A venue website safe to put in an `href`, or null.
 *
 * Venue websites are operator-supplied data reaching an anchor tag, so the
 * scheme is checked rather than assumed: a stored `javascript:` value would
 * otherwise be a stored-XSS vector on a page that renders one link per room.
 * Anything that is not an absolute http(s) URL is treated as "no website on
 * file", which lands the room on its internal page — a worse link, never an
 * unsafe one.
 */
export function venueWebsiteHref(raw: string | undefined): string | null {
  if (!raw) return null
  try {
    const url = new URL(raw)
    return url.protocol === 'http:' || url.protocol === 'https:' ? url.toString() : null
  } catch {
    return null
  }
}
