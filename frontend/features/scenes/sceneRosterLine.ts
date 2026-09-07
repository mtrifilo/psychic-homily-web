/**
 * Wording for the expanded roster's per-band line.
 *
 * Same rule as `sceneNewBands`: every clause states a fact the payload carries,
 * and no clause states a quantity the payload does not. The line reads
 * `N upcoming · next {date}, {venue}`, and each half drops independently
 * because each can be missing on a real row.
 */

import { entityHref } from './components/sceneChrome'
import { formatCalendarMonthDay, showHref } from './sceneWeek'
import type { SceneArtist } from './types'

/**
 * The line's parts, resolved. `null` means the band has nothing to say here and
 * renders as a bare name.
 *
 * Parts rather than a string because two of them are LINKS — the date points at
 * the show, the room at its own page — and a component cannot link inside a
 * string it was handed.
 */
export type RosterUpcomingLine = {
  /** `2 upcoming`. Always present; it is the reason the line exists. */
  countText: string
  /** `Sep 9`, or null when the payload's date is not a calendar date. */
  day: string | null
  /** Canonical `/shows/...` target for the day, null when there is no show. */
  dayHref: string | null
  /** The room's name, or null when the show has no venue yet. */
  venueName: string | null
  /** `/venues/{slug}`, null when the room has no usable slug. */
  venueHref: string | null
}

/**
 * The band's upcoming clause, or null when there is nothing to state.
 *
 * A count of zero is null rather than `0 upcoming`: the locked shape for a band
 * with nothing booked is the name alone, and a zero printed under every quiet
 * band in a roster of forty is noise the module is built to avoid.
 *
 * An ABSENT count is also null, and that is a different case from zero. The
 * field is required on the wire, but a cached body fetched before the backend
 * widened does not carry it, and inventing "0 upcoming" for a band that may
 * have three would be the module stating something false rather than nothing.
 *
 * The next-show half drops on its own when the payload cannot support it — a
 * count with no show attached still tells the reader how much is booked. Within
 * it, an undateable show can still name its room and a roomless show can still
 * carry its date, mirroring `formatNewBandShow`; only when both are missing does
 * the clause disappear entirely.
 */
export function rosterUpcomingLine(artist: SceneArtist): RosterUpcomingLine | null {
  const count = artist.upcoming_show_count
  if (typeof count !== 'number' || count <= 0) return null

  const show = artist.next_show
  const day = show ? formatCalendarMonthDay(show.event_date ?? '') : null
  const venueName = show?.venue_name?.trim() || null

  return {
    countText: `${count} upcoming`,
    day,
    dayHref: show && day ? showHref({ id: show.id, slug: show.slug }) : null,
    venueName,
    venueHref: venueName ? entityHref('/venues', show?.venue_slug) : null,
  }
}
