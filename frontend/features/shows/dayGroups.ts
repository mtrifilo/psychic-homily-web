/**
 * Day grouping for the `/shows` list: the pure half of `DayGroupedShowList`.
 *
 * Kept out of the component so the awkward parts — cross-zone runs, the anchor
 * that must stay unique, the tonight test — are exercised against fixtures
 * rather than through a rendered list.
 */

import { formatShowDateBadge } from '@/lib/utils/showDateBadge'
import {
  getShowLifecycleState,
  venueLocalDateKey,
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

/** The venue zone fields of a row, in the spelling the date helpers take. */
function zoneOf(show: ShowResponse) {
  return {
    eventDate: show.event_date,
    state: show.state,
    timezone: show.venues?.[0]?.timezone,
  }
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

  for (const show of shows) {
    const zone = zoneOf(show)
    const dateKey = venueLocalDateKey(zone)
    const current = groups[groups.length - 1]
    if (current && current.dateKey === dateKey) {
      current.rows.push(show)
      continue
    }

    const badge = formatShowDateBadge(
      show.event_date,
      show.state,
      show.venues?.[0]?.timezone
    )
    const anchorId =
      dateKey !== null && !anchored.has(dateKey) ? dayAnchorId(dateKey) : null
    if (anchorId !== null) anchored.add(dateKey as string)

    groups.push({
      dateKey,
      anchorId,
      dayOfWeek: badge.dayOfWeek,
      monthDay: badge.monthDay,
      // Read off the group's FIRST row. A group is one local date, which two
      // venues in different zones can share, so "today" is answered where this
      // group's leading show happens.
      isToday:
        now !== null && getShowLifecycleState(zone, now) === 'today',
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
