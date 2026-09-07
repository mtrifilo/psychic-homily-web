/**
 * Wording for the expanded roster's per-band line.
 *
 * Same rule as `sceneNewBands`: every clause states a fact the payload carries,
 * and no clause states a quantity the payload does not.
 */

import { formatCalendarMonthDay, showHref } from './sceneWeek'
import type { SceneArtist } from './types'

/**
 * The line's parts, resolved. `null` means the band has nothing to say here and
 * renders as a bare name.
 *
 * Parts rather than a string because two of them are LINKS — the date points at
 * the show, the room at its own page — and a component cannot link inside a
 * string it was handed. The venue is passed on as a name and a slug rather than
 * an href so the component can hand it to `EntityNameLink`, which owns the
 * nullable-slug guard for the whole feature.
 */
export type RosterUpcomingLine = {
  /** `2 upcoming`. Always present; it is the reason the line exists. */
  countText: string
  /** `Sep 9`, or null when the payload's date is not a calendar date. */
  day: string | null
  /** Canonical `/shows/...` target for the day, null when there is no day. */
  dayHref: string | null
  /** The room, or null when the show has no venue yet. */
  venue: { name: string; slug?: string } | null
}

/**
 * The band's upcoming clause, or null when there is nothing to state.
 *
 * Absent, zero and negative all answer null: the locked shape for a band with
 * nothing booked is the name alone, and a cached body fetched before the
 * backend widened carries no count to print.
 *
 * The next-show half drops on its own when the payload cannot support it — a
 * count with no show attached still tells the reader how much is booked. Within
 * it, an undateable show can still name its room and a roomless show can still
 * carry its date, mirroring `formatNewBandShow`; only when both are missing does
 * the clause disappear entirely.
 */
export function rosterUpcomingLine(artist: SceneArtist): RosterUpcomingLine | null {
  const count = artist.upcoming_show_count
  if (!(typeof count === 'number' && count > 0)) return null

  const show = artist.next_show
  const day = show ? formatCalendarMonthDay(show.event_date ?? '') : null
  const venueName = show?.venue_name?.trim()

  return {
    countText: `${count} upcoming`,
    day,
    dayHref: show && day ? showHref(show) : null,
    venue: venueName ? { name: venueName, slug: show?.venue_slug } : null,
  }
}
