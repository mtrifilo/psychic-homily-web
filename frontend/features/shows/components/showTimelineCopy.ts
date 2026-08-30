import { formatInTimezone } from '@/lib/utils/timeUtils'

/**
 * The copy rules for the gig-timeline spine and the last-played line.
 *
 * Separated from the components because these are the assertions the modules
 * make in words, and they are worth testing without a DOM.
 *
 * EVERY date here is read on ITS OWN stop's zone. The stops name rooms in
 * different zones, and a spine that dated all three on the subject's clock
 * would print the wrong day for a neighbour across a date line, which is the
 * one thing a date-ordered module must not do.
 *
 * The parameters are structural rather than `ShowTimelineEntry`, so the show
 * being read is formatted by the same rules as its neighbours without being
 * dressed up as an archive row it is not.
 */

/** A dated stop: when it happened, and on whose clock that date is read. */
export interface TimelineStopDate {
  /** ISO instant. */
  event_date: string
  /** Resolved IANA zone. Never the venue's raw nullable column. */
  timezone: string
}

/** Where a stop happened, as the spine names it. */
export interface TimelineStopPlace {
  venue_name?: string | null
  city?: string | null
  state?: string | null
}

/** Venue-local calendar year, for deciding whether a date needs one. */
export function timelineYear(stop: TimelineStopDate): string {
  return formatInTimezone(stop.event_date, stop.timezone, { year: 'numeric' })
}

/**
 * `AUG 9`, or `AUG 9 2025` when the stop falls in a different venue-local year
 * than the show being read.
 *
 * The spine is stamped, uppercase and tracked, so the month arrives already
 * capitalised rather than being cased in CSS: `text-transform` is a paint, and
 * a copy helper that returns the string the reader sees is testable.
 *
 * The year is conditional because on a tour leg it would be three redundant
 * repetitions of the year in the heading. An archive spine reaching back across
 * a New Year is the case that needs it, and there the year is the whole point.
 */
export function timelineDateLabel(
  stop: TimelineStopDate,
  subjectYear: string,
): string {
  const date = formatInTimezone(stop.event_date, stop.timezone, {
    month: 'short',
    day: 'numeric',
  }).toUpperCase()
  const year = timelineYear(stop)
  return year === subjectYear ? date : `${date} ${year}`
}

/**
 * Where a stop happened: `METRO, CHICAGO`, or `CHICAGO, IL` for a room with no
 * name on record.
 *
 * The room first and the city after it, because the room is the specific fact
 * and the city is the one a reader scanning a tour route groups by. A stop with
 * neither returns "", and the caller renders the date alone rather than a
 * dangling separator.
 */
export function timelinePlaceLabel(stop: TimelineStopPlace): string {
  const parts = stop.venue_name?.trim()
    ? [stop.venue_name, stop.city]
    : [stop.city, stop.state]
  return parts
    .map(part => part?.trim())
    .filter(Boolean)
    .join(', ')
    .toUpperCase()
}

/**
 * `Nov 2023, Aragon Ballroom`: when and where an act last played here.
 *
 * Month resolution, not a full date: the line is a recurrence fact ("they come
 * through about yearly"), and a day number invites a reader to check a calendar
 * the line is not offering.
 *
 * Sentence case, unlike the spine: this line sits directly under the bill in
 * the same register as the band names above it.
 */
export function lastPlayedLabel(
  stop: TimelineStopDate & Pick<TimelineStopPlace, 'venue_name'>,
): string {
  const month = formatInTimezone(stop.event_date, stop.timezone, {
    month: 'short',
    year: 'numeric',
  })
  const venue = stop.venue_name?.trim()
  return venue ? `${month}, ${venue}` : month
}
