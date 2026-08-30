import { formatShowMonth } from '@/lib/utils/formatters'
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
 */

/**
 * One dated stop on a timeline: when it happened, on whose clock that date is
 * read, and where.
 *
 * Structural rather than `ShowTimelineEntry` so the show BEING READ is
 * formatted by the same rules as its neighbours without being dressed up as an
 * archive row it is not. `ShowTimelineEntry` satisfies it as-is.
 */
export interface TimelineStop {
  /** ISO instant. */
  event_date: string
  /** Resolved IANA zone. Never the venue's raw nullable column. */
  timezone: string
  venue_name?: string | null
  city?: string | null
  state?: string | null
}

/**
 * ONE stop type, but each helper takes only the fields it reads: the place
 * label has no business demanding a date, and a caller with only a place should
 * not have to invent one to ask for its label.
 */
type StopDate = Pick<TimelineStop, 'event_date' | 'timezone'>
type StopPlace = Pick<TimelineStop, 'venue_name' | 'city' | 'state'>

/** Venue-local calendar year, for deciding whether a date needs one. */
export function timelineYear(stop: StopDate): string {
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
  stop: StopDate,
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
 *
 * Hand-joined rather than delegating to `formatLocation`: that helper's rule
 * turns on the COUNTRY, which a timeline stop does not carry, and its
 * "Location Unknown" placeholder is designed to stand alone in a location field
 * rather than inside a dateline.
 */
export function timelinePlaceLabel(stop: StopPlace): string {
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
 * The same label for the stop the reader is ALREADY on: `SALT SHED`, the room
 * and nothing else.
 *
 * No city, unlike the neighbours. The city is what distinguishes one stop on a
 * route from another, and for this stop the reader has the venue module a few
 * lines below carrying the full address. The locked mock states it this way.
 *
 * A room with no name on record still needs to say where it is, so it falls
 * through to the neighbour rule.
 */
export function timelineCurrentPlaceLabel(stop: StopPlace): string {
  const venue = stop.venue_name?.trim()
  return venue ? venue.toUpperCase() : timelinePlaceLabel(stop)
}

/**
 * `Nov 2023, Aragon Ballroom`: when and where an act last played here.
 *
 * Month resolution through the same `formatShowMonth` the month-grouped
 * archives head their sections with, so "Nov 2023" here and the "Nov 2023"
 * heading a reader lands on after following the link are one string. It is
 * passed a null state because the stop's zone is already resolved.
 *
 * Sentence case, unlike the spine: this line sits directly under the bill in
 * the same register as the band names above it.
 */
export function lastPlayedLabel(
  stop: StopDate & Pick<TimelineStop, 'venue_name'>,
): string {
  const month = formatShowMonth(stop.event_date, null, stop.timezone)
  const venue = stop.venue_name?.trim()
  return venue ? `${month}, ${venue}` : month
}
