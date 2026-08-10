/**
 * The scene page's calendar spine: the pure half.
 *
 * The detail page is a calendar with identity wrapped around it, so the day
 * bucketing, the "which night is tonight" question and the header's counts all
 * have to be decidable without a DOM. They live here so they can be tested
 * against a pinned clock and a pinned zone, which is the only way to catch the
 * off-by-one-day faults this module exists to avoid.
 */

import { isValidTimeZone } from '@/lib/utils/formatters'
import { parseCalendarDate } from './sceneWeek'
import type { SceneShowSummary } from './types'

/**
 * The detail page's window: four weeks ahead.
 *
 * The detail page POINTS at `/scenes/{slug}/week` rather than absorbing it
 * (brief decision 2), and the two only genuinely differ if this page looks
 * further than seven days. Four weeks also sits inside the endpoint's own
 * `maximum: 30` on `days`, so it is servable without touching the handler.
 */
export const SCENE_CALENDAR_WINDOW_DAYS = 28

/**
 * How many rows the window may return.
 *
 * The endpoint caps `limit` at 20 (`GetSceneShowsRequest`), so this is not a
 * design choice we are free to raise from the frontend. A denser scene is
 * truncated here and the footer says so rather than pretending 20 is the total.
 */
export const SCENE_CALENDAR_ROW_CAP = 20

/**
 * The scene-local hour at which a new night begins.
 *
 * A mirror of the backend's `nightStartHour` (scene_day.go), and it must stay a
 * mirror: at 01:00 on Saturday "tonight" is still Friday night, and the TONIGHT
 * tag on this page has to point at the same date `/scenes/{slug}/tonight` would
 * serve. As on the backend, this decides only WHICH date is tonight; it does
 * not widen a group, whose contents stay a strict calendar day.
 */
const NIGHT_START_HOUR = 6

/** One date's worth of rows, in the order the endpoint returned them. */
export interface SceneShowGroup {
  /** `YYYY-MM-DD`, resolved in the scene's own zone. */
  date: string
  shows: SceneShowSummary[]
}

interface ZonedParts {
  year: number
  month: number
  day: number
  hour: number
}

/**
 * The wall-clock fields a zone is currently on.
 *
 * `formatToParts` reads them without date maths and without depending on how a
 * locale happens to order a short date, which is the same route
 * `currentWeekBounds` takes to the same question. `hourCycle: 'h23'` rather than
 * `hour12: false` because the latter can yield `24` for midnight in some
 * engines, which would silently defeat the night-boundary comparison below.
 */
function zonedParts(instant: Date, timeZone?: string): ZonedParts {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(instant)
  const field = (type: string) => Number(parts.find(p => p.type === type)?.value)
  return {
    year: field('year'),
    month: field('month'),
    day: field('day'),
    hour: field('hour'),
  }
}

/** `2026-08-09` from a `Date` read in its own local fields. */
function toCalendarDate(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

/** The calendar date `instant` falls on in `timeZone`. */
export function calendarDateInZone(instant: Date, timeZone?: string): string {
  const { year, month, day } = zonedParts(instant, timeZone)
  return toCalendarDate(new Date(year, month - 1, day))
}

/**
 * The date "tonight" names in `timeZone`, honouring the 6am night boundary.
 *
 * Returns `null` when the instant is unusable, so a caller never tags a group
 * TONIGHT on the strength of an invalid clock read.
 */
export function sceneTonightDate(now: Date, timeZone?: string): string | null {
  if (!Number.isFinite(now.getTime())) return null
  const { year, month, day, hour } = zonedParts(now, timeZone)
  if (!Number.isFinite(year) || !Number.isFinite(hour)) return null
  const date = new Date(year, month - 1, day)
  if (hour < NIGHT_START_HOUR) date.setDate(date.getDate() - 1)
  return toCalendarDate(date)
}

/**
 * The zone the scene's rows should be read in.
 *
 * The rows themselves are the only source: `GET /scenes/{slug}` carries no
 * timezone, and `GET /scenes/{slug}/shows` renders `event_date` in **UTC**
 * (`GetSceneUpcomingShows` passes `time.UTC` as its location), so an evening
 * show west of Greenwich arrives labelled with the FOLLOWING day. Every date on
 * this surface is therefore derived from `starts_at` in the venue's own zone,
 * never from the payload's `event_date`.
 *
 * Rows whose zone is not genuinely known are skipped rather than defaulted.
 * The question asked of each row is `isValidTimeZone(venue_timezone)`, NOT
 * `isShowTimezoneResolved(venue_state, venue_timezone)`: the latter answers yes
 * for any US state on the strength of the state map, so an unset or garbage
 * venue zone would pass and then be handed to `Intl` as a real zone. A guessed
 * zone that is a calendar day wrong reads as fact once it is printed under a
 * TONIGHT tag.
 *
 * The most common zone wins, not the first: a metro scene can include a member
 * city across a zone line, and the majority room is the one the page speaks for.
 */
export function resolveSceneTimeZone(shows: SceneShowSummary[]): string | undefined {
  const counts = new Map<string, number>()
  for (const show of shows) {
    const zone = show.venue_timezone
    if (!zone || !isValidTimeZone(zone)) continue
    counts.set(zone, (counts.get(zone) ?? 0) + 1)
  }
  let best: string | undefined
  let bestCount = 0
  for (const [zone, count] of counts) {
    if (count > bestCount) {
      best = zone
      bestCount = count
    }
  }
  return best
}

/**
 * The calendar date one row belongs to, in the scene's zone.
 *
 * Prefers the row's OWN venue zone so a member city across a zone line files
 * under the date its patrons experienced. Falls back to the payload's
 * `event_date` only when there is no usable instant at all. A UTC-rendered
 * date is still better than dropping the row.
 */
export function showCalendarDate(
  show: SceneShowSummary,
  sceneTimeZone?: string
): string {
  const startedAt = Date.parse(show.starts_at)
  if (!Number.isFinite(startedAt)) return show.event_date
  const zone =
    show.venue_timezone && isValidTimeZone(show.venue_timezone)
      ? show.venue_timezone
      : sceneTimeZone
  if (!zone) return show.event_date
  return calendarDateInZone(new Date(startedAt), zone)
}

/**
 * Rows bucketed by date, earliest first, order preserved inside each bucket.
 *
 * The endpoint already sorts by event date then id, so this only has to keep
 * what it was given. Re-sorting inside a bucket would put this page's bills in
 * a different order from the nightly page's.
 */
export function groupShowsByDate(
  shows: SceneShowSummary[],
  sceneTimeZone?: string
): SceneShowGroup[] {
  const groups: SceneShowGroup[] = []
  const byDate = new Map<string, SceneShowGroup>()
  for (const show of shows) {
    const date = showCalendarDate(show, sceneTimeZone)
    const existing = byDate.get(date)
    if (existing) {
      existing.shows.push(show)
      continue
    }
    const group: SceneShowGroup = { date, shows: [show] }
    byDate.set(date, group)
    groups.push(group)
  }
  return groups.sort((a, b) => a.date.localeCompare(b.date))
}

/** `SAT · AUG 8`, the date-group heading. */
export function formatCalendarDateHeading(iso: string): string {
  const date = parseCalendarDate(iso)
  const weekday = date.toLocaleDateString('en-US', { weekday: 'short' }).toUpperCase()
  const month = date.toLocaleDateString('en-US', { month: 'short' }).toUpperCase()
  return `${weekday} · ${month} ${date.getDate()}`
}

/**
 * A date group's count, to the right of its heading.
 *
 * The empty case is only ever reached by the tonight bucket, which renders even
 * when nothing is on it, so a bare `0 shows` is the honest answer and the copy
 * beneath it says whose calendar that zero is about.
 */
export function formatGroupCount(total: number): string {
  return `${total} ${total === 1 ? 'show' : 'shows'}`
}

/**
 * `MST`, the zone every time on this page is printed in.
 *
 * Returns `null` for an unresolved zone rather than naming the reader's own:
 * the claim is about the scene, and a browser-local abbreviation on a Phoenix
 * page would be a confident wrong answer.
 */
export function formatTimeZoneLabel(now: Date, timeZone?: string): string | null {
  if (!timeZone) return null
  const part = new Intl.DateTimeFormat('en-US', {
    timeZone,
    timeZoneName: 'short',
  })
    .formatToParts(now)
    .find(p => p.type === 'timeZoneName')
  return part?.value ?? null
}

/** `(Mesa)`, the sub-locality that lets the scene read as a region. */
export function venueSubLocality(show: SceneShowSummary): string | null {
  const city = show.venue_city?.trim()
  return city ? `(${city})` : null
}

/**
 * The header's middot stat line, with ZERO-VALUED PARTS KEPT.
 *
 * The bug this replaces dropped any part that was zero, so London rendered
 * `2 venues · 197 upcoming shows` as though artists were never a category on
 * this page. A category that disappears when its count is 0 teaches the reader
 * that the site does not track it; `0 artists based here` teaches them it does
 * and that we have none, which is the true and more useful statement.
 */
export function sceneStatParts(stats: {
  venue_count: number
  artist_count: number
  upcoming_show_count: number
}): string[] {
  const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`
  return [
    plural(stats.venue_count, 'venue'),
    `${plural(stats.artist_count, 'artist')} based here`,
    plural(stats.upcoming_show_count, 'upcoming show'),
  ]
}
