import { formatShowMonth, resolveShowTimezone } from '@/lib/utils/formatters'
import { formatInTimezone } from '@/lib/utils/timeUtils'
import { markGuessedShowDay } from '../showPageDate'
import type { ShowTimelineRecurrence } from '../types'

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
  /**
   * The IANA zone the stop's room is on, or null when the site cannot name one.
   *
   * NULL is a refusal, not a gap: the only zone left is the fallback, and its
   * reading of the day is what `state` below is for. A stop that arrives null
   * has its date marked as a guess.
   */
  timezone: string | null
  venue_name?: string | null
  city?: string | null
  /**
   * The room's state, read alongside `timezone` by the same
   * `resolveShowTimezone` precedence every other surface dates a show on, and by
   * the marking rule. Also the place label's second half for a room with no
   * name on record.
   */
  state?: string | null
}

/**
 * ONE stop type, but each helper takes only the fields it reads: the place
 * label has no business demanding a date, and a caller with only a place should
 * not have to invent one to ask for its label.
 */
type StopDate = Pick<TimelineStop, 'event_date' | 'timezone' | 'state'>
type StopPlace = Pick<TimelineStop, 'venue_name' | 'city' | 'state'>

/** The clock one stop's date is read on: its own zone, then the state map. */
function stopZone(stop: StopDate): string {
  return resolveShowTimezone(stop.state, stop.timezone)
}

/** Venue-local calendar year, for deciding whether a date needs one. */
export function timelineYear(stop: StopDate): string {
  return formatInTimezone(stop.event_date, stopZone(stop), { year: 'numeric' })
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
 *
 * MARKED `~AUG 9` when the stop's zone is the fallback rather than one its room
 * supplies, in the same register the header a few lines above uses. One mark per
 * date, ahead of the whole label, so a stop carrying a year is marked once.
 */
export function timelineDateLabel(
  stop: StopDate,
  subjectYear: string,
): string {
  const date = formatInTimezone(stop.event_date, stopZone(stop), {
    month: 'short',
    day: 'numeric',
  }).toUpperCase()
  const year = timelineYear(stop)
  const label = year === subjectYear ? date : `${date} ${year}`
  return markGuessedShowDay(label, stop.state, stop.timezone)
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
 * heading a reader lands on after following the link are one string.
 *
 * UNMARKED, unlike the spine. A month is a weaker claim than a day: the
 * fallback's reading of a date can name the wrong day, but only a set within
 * the last hours of a month can land it in the wrong month, and this line reads
 * as prose rather than as a dateline. The header's decided marking list does
 * not reach it.
 *
 * Sentence case, unlike the spine: this line sits directly under the bill in
 * the same register as the band names above it.
 */
export function lastPlayedLabel(
  stop: StopDate & Pick<TimelineStop, 'venue_name'>,
): string {
  const month = formatShowMonth(stop.event_date, stop.state, stop.timezone)
  const venue = stop.venue_name?.trim()
  return venue ? `${month}, ${venue}` : month
}

/** The bill fields the last-played line needs: a name to say, keyed by id. */
export interface RecurrenceBillArtist {
  id: number
  name: string
}

/** One act's clause on the last-played line, keyed for a stable render list. */
export interface RecurrenceSegment {
  id: number
  text: string
}

/**
 * The last-played line's clauses, in bill order, or NONE when the line has no
 * recurrence story to tell.
 *
 * Every act on the line is one of two kinds. A PRIOR-DATE clause says when and
 * where an act last played this place. A HOMETOWN clause says the act lives
 * here, and WINS over a prior date: "Califone last played Chicago" is true of
 * every Chicago band and says nothing, so living here is the fact stated.
 *
 * ALL-LOCAL BILLS RENDER NOTHING. A line whose every clause is a hometown
 * clause repeats one phrase per act and adds nothing to the bill above it,
 * which already carries a hometown label on each of those acts. A prior date is
 * the recurrence story this module exists to tell, so ONE prior-date clause is
 * what turns the line on, and the hometown clauses then ride along beside it.
 *
 * The test runs over the clauses that would RENDER, not over the bill. An act
 * that contributes no clause is neither local nor touring here: an act with no
 * hometown claim and no prior date on record, and an entry naming an act the
 * bill cannot name, are both invisible to this line and so cannot turn it on.
 *
 * The city named is the one on the ENTRY, never the show's own. The backend
 * matches prior dates across a whole metro, so an entry can name a neighbouring
 * city, and naming the show's city would claim a date in a city the act did not
 * play.
 *
 * Names come from the BILL, so this line and the heading above it cannot
 * disagree about what an act is called.
 */
export function billRecurrenceSegments(
  recurrence: ShowTimelineRecurrence[],
  artists: RecurrenceBillArtist[],
): RecurrenceSegment[] {
  const nameById = new Map(artists.map(artist => [artist.id, artist.name]))

  const clauses = recurrence.flatMap(entry => {
    const name = nameById.get(entry.artist_id)
    if (!name) return []
    if (entry.is_hometown) {
      return [
        { id: entry.artist_id, text: `${name}: hometown show`, hometown: true },
      ]
    }
    if (!entry.last_played) return []
    // A room with no city on record leaves the sentence no place to name, so it
    // states what it still has: when, and which room. The colon goes with the
    // city, because a colon introduces the place clause and reads as a
    // rendering fault standing on its own.
    const city = entry.last_played.city?.trim()
    const when = lastPlayedLabel(entry.last_played)
    return [
      {
        id: entry.artist_id,
        text: city
          ? `${name} last played ${city}: ${when}`
          : `${name} last played ${when}`,
        hometown: false,
      },
    ]
  })

  if (!clauses.some(clause => !clause.hometown)) return []
  return clauses.map(({ id, text }) => ({ id, text }))
}
