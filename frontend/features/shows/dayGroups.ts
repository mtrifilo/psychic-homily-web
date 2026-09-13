/**
 * Day grouping for the `/shows` list: the pure half of `DayGroupedShowList`.
 *
 * Kept out of the component so the awkward parts — cross-zone runs, the anchor
 * that must stay unique, the tonight test — are exercised against fixtures
 * rather than through a rendered list.
 */

import { formatShowDateBadge } from '@/lib/utils/showDateBadge'
import {
  venueLocalDateKey,
  type ShowTimingInput,
} from '@/lib/utils/showTiming'
import type { ShowResponse } from './types'

/** One day's worth of consecutive rows, with everything the heading needs. */
export interface ShowDayGroup {
  /**
   * The venue-local calendar date, `2026-09-12`, or `null` when the rows'
   * dates cannot be read.
   */
  dateKey: string | null
  /**
   * DOM id for this group's heading, or `null` when the group carries no
   * anchor. Only the FIRST group with a given date gets one — see
   * {@link groupShowsByVenueLocalDay} for why a page can hold two.
   */
  anchorId: string | null
  /** Short weekday, uppercase: `FRI`. */
  dayOfWeek: string
  /** Short month and day, uppercase: `SEP 11`. */
  monthDay: string
  /** Whether this day is today where its shows happen. */
  isToday: boolean
  rows: ShowResponse[]
}

/** The DOM id of a day heading, `d-2026-09-12`. */
export function dayAnchorId(dateKey: string): string {
  return `d-${dateKey}`
}

/**
 * The venue zone fields of a row, in the spelling the date helpers take.
 *
 * `show.state` rather than the venue's, which is what `showTimingInput` in
 * `./utils` would give. The day HEADING has to land on the same day as the date
 * tile inside every row beneath it, and `ShowCard` resolves that tile from
 * `show.state`; for a zone-less venue whose state disagrees with its show row,
 * the two spellings pick different days and a row would sit under a heading
 * contradicting its own badge. Converging the two is the follow-up
 * `showTimingInput` already names, and it has to move both at once.
 */
function zoneOf(show: ShowResponse): ShowTimingInput {
  return {
    eventDate: show.event_date,
    state: show.state,
    timezone: show.venues?.[0]?.timezone,
  }
}

/** The cache key for a zone, before it is resolved to an IANA name. */
function zoneCacheKey(zone: ShowTimingInput): string {
  return `${zone.state ?? ''}|${zone.timezone ?? ''}`
}

/**
 * Split an already-ordered list into consecutive runs of rows that share a
 * venue-local calendar date.
 *
 * RUNS, not buckets: the caller's ordering is the list's ordering, and
 * collecting by date would silently reorder rows under merged headings. The
 * cost is that one date can produce more than one group on a page, and it is
 * not hypothetical — the list sorts on the absolute instant while a date is
 * venue-local, so inside the roughly one-day cross-zone band two rows on the
 * same local date can arrive either side of a row on a different one.
 *
 * That is why the ANCHOR is assigned to the first group of a date and withheld
 * from any later one: an id has to be unique in a document, and the first
 * occurrence is the one a `#d-2026-09-12` link should land on.
 *
 * `now` is the clock the tonight test reads, and `null` means "do not ask".
 * Passing `null` is what the server render does: reading a clock in a
 * prerenderable scope would make this route dynamic, and a heading that gains
 * its TONIGHT prefix one commit after hydration costs a reader nothing.
 */
export function groupShowsByVenueLocalDay(
  shows: readonly ShowResponse[],
  now: Date | null
): ShowDayGroup[] {
  const groups: ShowDayGroup[] = []
  const anchored = new Set<string>()
  // Today's date in each zone the page touches. A list page spans a handful of
  // zones and usually one, so this answers the tonight question once per zone
  // rather than once per group.
  const todayByZone = new Map<string, string | null>()

  const todayKeyFor = (zone: ShowTimingInput): string | null => {
    if (now === null) return null
    const cacheKey = zoneCacheKey(zone)
    let today = todayByZone.get(cacheKey)
    if (today === undefined) {
      today = venueLocalDateKey({ ...zone, eventDate: now.toISOString() })
      todayByZone.set(cacheKey, today)
    }
    return today
  }

  for (const show of shows) {
    const zone = zoneOf(show)
    const dateKey = venueLocalDateKey(zone)
    const current = groups[groups.length - 1]
    if (current && current.dateKey === dateKey) {
      current.rows.push(show)
      continue
    }

    // The zone fields come from `zone`, so the heading is formatted on the same
    // two values the date key above was read from. `event_date` is passed
    // straight through because it is non-null on the row and optional on
    // `ShowTimingInput`.
    const badge = formatShowDateBadge(show.event_date, zone.state, zone.timezone)

    let anchorId: string | null = null
    if (dateKey !== null && !anchored.has(dateKey)) {
      anchorId = dayAnchorId(dateKey)
      anchored.add(dateKey)
    }

    groups.push({
      dateKey,
      anchorId,
      dayOfWeek: badge.dayOfWeek,
      monthDay: badge.monthDay,
      // Both sides of the comparison come from `venueLocalDateKey`, so the day
      // this group IS and the day that is today are read on one boundary by
      // construction rather than by two helpers agreeing.
      //
      // Read off the group's FIRST row. A group is one local date, which two
      // venues in different zones can share, so "today" is answered where this
      // group's leading show happens.
      isToday: dateKey !== null && dateKey === todayKeyFor(zone),
      rows: [show],
    })
  }

  return groups
}

/**
 * The heading a group prints: `FRI · SEP 11`, and `TONIGHT · SAT SEP 12` on
 * the day that is today where its shows are.
 */
export function dayGroupHeading(group: ShowDayGroup): string {
  return group.isToday
    ? `TONIGHT · ${group.dayOfWeek} ${group.monthDay}`
    : `${group.dayOfWeek} · ${group.monthDay}`
}
