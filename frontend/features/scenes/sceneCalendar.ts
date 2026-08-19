/**
 * The scene page's calendar spine: the pure half.
 *
 * The detail page is a calendar with identity wrapped around it, so the day
 * bucketing, the "which night is tonight" question and the header's counts all
 * have to be decidable without a DOM. They live here so they can be tested
 * against a pinned clock and a pinned zone, which is the only way to catch the
 * off-by-one-day faults this module exists to avoid.
 */

import {
  isShowTimezoneResolved,
  isValidTimeZone,
  resolveShowTimezone,
} from '@/lib/utils/formatters'
import { parseCalendarDate } from './sceneWeek'
import type { SceneShowSummary } from './types'

/**
 * The root's 28-day window and its 60-row cap are GONE (PSY-1850).
 *
 * The root is the front page of the scene now, not a calendar window: it renders
 * tonight and the next full day from the day endpoint (`sceneSlice.ts`) and
 * points at `/week` and `/next-4-weeks` for anything longer. The four-week
 * window lives on `/next-4-weeks`, which carries its own `NEXT_4_WEEKS_DAYS` and
 * `SCENE_WINDOW_ROW_CAP` in `sceneWindow.ts`.
 *
 * The client-side date BUCKETING that window needed went with it, for the same
 * reason: the day endpoint returns rows already bucketed into whole scene-local
 * dates, so nothing on the root re-derives a date from an instant any more.
 * `calendarDateInZone` and `sceneTonightDate` survive because the window pages
 * still need them (`sceneWindowPage.tsx`).
 */

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
 * Returns `null` when the zone is unknown or the instant unusable, so a caller
 * never tags a group TONIGHT on the strength of a guess. The `!timeZone` guard
 * is load-bearing and NOT defensive noise: `Intl.DateTimeFormat` treats an
 * undefined `timeZone` as the RUNTIME's zone, so without it a reader in Tokyo
 * and a reader in Los Angeles would each be told a different night was tonight
 * in Phoenix, and one of them would be shown an empty bucket asserting a zero
 * the page never checked.
 */
export function sceneTonightDate(now: Date, timeZone?: string): string | null {
  if (!timeZone) return null
  if (!Number.isFinite(now.getTime())) return null
  const { year, month, day, hour } = zonedParts(now, timeZone)
  if (!Number.isFinite(year) || !Number.isFinite(hour)) return null
  const date = new Date(year, month - 1, day)
  if (hour < NIGHT_START_HOUR) date.setDate(date.getDate() - 1)
  return toCalendarDate(date)
}

/**
 * The zone ONE row should be read in, or undefined when nothing knows.
 *
 * Two sources, in order. The venue's own `timezone` when it is a real IANA
 * name. Otherwise the US state map, but ONLY when it actually holds the state:
 * `resolveShowTimezone` silently answers America/Phoenix for anything it does
 * not recognise, so it has to be gated on `isShowTimezoneResolved`, which is
 * the predicate that helper's own docstring nominates for same-day claims.
 *
 * Exported so the ROW can print its clock time in the same zone its heading
 * was bucketed in. `formatShowStartTime` alone is not enough: it ends at
 * `resolveShowTimezone`, which silently answers America/Phoenix for anything
 * outside the US state map, so a zone-less London row would file under a
 * UTC-derived date and print an Arizona time under it. A heading and a start
 * time disagreeing about which day a show is on is the worst possible output,
 * because both look authoritative and only one can be right. When this returns
 * undefined and the scene has no zone either, the row prints no time at all.
 */
export function rowTimeZone(show: SceneShowSummary): string | undefined {
  const zone = show.venue_timezone
  if (zone && isValidTimeZone(zone)) return zone
  if (isShowTimezoneResolved(show.venue_state, zone)) {
    return resolveShowTimezone(show.venue_state, zone)
  }
  return undefined
}

/**
 * `SUNDAY, AUGUST 17` — the slice's date-group heading.
 *
 * Spelled OUT, and the locked mock is why: the root now shows two dates instead
 * of a month of them, so each one is read as a statement about a night rather
 * than scanned as a column key, and `SUN · AUG 17` is a column key. The window
 * pages keep their own compact headings, which is correct — they list seven or
 * twenty-eight of these.
 *
 * NO year, also per the mock. Both dates the root can render are within a day of
 * now, so a year would be noise on the one surface where it can never
 * disambiguate anything. `formatDayFull` (which does carry the year) stays the
 * spelling for the DATED permalinks, where a reader really can arrive at a page
 * about January 2020.
 */
export function formatSliceDateHeading(iso: string): string {
  const date = parseCalendarDate(iso)
  const weekday = date.toLocaleDateString('en-US', { weekday: 'long' }).toUpperCase()
  const month = date.toLocaleDateString('en-US', { month: 'long' }).toUpperCase()
  return `${weekday}, ${month} ${date.getDate()}`
}

/**
 * A date group's count is `formatDayCountLine` from `sceneDay.ts`, imported at
 * the call site rather than re-exported here.
 *
 * Deliberately NOT a second phrasing. A reader clicking TONIGHT lands on
 * `/scenes/{slug}/tonight` looking at the same night, and the two pages
 * disagreeing about how to say `0` would be the first thing they noticed. That
 * helper also already carries the rule: the empty case reads "0 shows listed",
 * because a bare "0 shows" would assert nothing is happening in the city, which
 * is a claim this site is not entitled to make about rooms it tracks a slice of.
 */

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
 * `12 venues`, `1 venue`. Naive -s pluralization, which is all this feature
 * needs — every noun it counts (venue, artist, show, room, band) is regular.
 *
 * Exported so the scene page's modules stop each keeping a copy. It is NOT a
 * general-purpose pluralizer and should not become one here; a real one belongs
 * in `lib/utils` alongside the thirty-odd inline copies elsewhere in the app.
 */
export function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? '' : 's'}`
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
  return [
    plural(stats.venue_count, 'venue'),
    `${plural(stats.artist_count, 'artist')} based here`,
    plural(stats.upcoming_show_count, 'upcoming show'),
  ]
}
