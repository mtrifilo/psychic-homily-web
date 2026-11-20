import { formatLocation, LOCATION_UNKNOWN } from '@/lib/formatLocation'
import {
  hasReadableStartDate,
  isAnnouncedTimeOfStart,
  type ShowTimingInput,
} from '@/lib/utils/showTiming'
import {
  showPageDateLong,
  showPageMonthDay,
} from '@/features/shows/showPageDate'
import { splitBill } from '@/features/shows/utils'
import { formatShowTime } from '@/lib/utils/formatters'
import { formatCount } from '@/components/shared/paginationChrome'
import type { LabelStatus } from '@/features/labels/types'
import type { FestivalStatus } from '@/features/festivals/types'

/**
 * Search-snippet strings (`<title>` and meta description) for the show, artist,
 * venue and release detail pages, the label and festival descriptions, and
 * the show page's generated `MusicEvent.description`.
 *
 * Every builder here is pure: the page resolves the entity, hands over plain
 * values, and gets back the two strings it puts in `generateMetadata`. The
 * budgets and the drop order live here so they are testable without a route.
 */

/**
 * What the root layout's title template (`%s | Psychic Homily`) appends to
 * every page title. It counts toward {@link TITLE_BUDGET}.
 */
export const SITE_TITLE_SUFFIX = ' | Psychic Homily'

/** Characters in the full rendered `<title>`, site suffix included. */
export const TITLE_BUDGET = 60

/** Characters in the meta description, ellipsis included. */
export const DESCRIPTION_BUDGET = 155

const ELLIPSIS = '...'

export interface EntitySnippet {
  /** The page's own title segment. The layout template appends the suffix. */
  title: string
  description: string
}

/** Length in code points, so an emoji or accented name counts as one each. */
function charCount(text: string): number {
  return Array.from(text).length
}

function nonEmpty(value: string | null | undefined): string | null {
  const trimmed = value?.trim()
  return trimmed ? trimmed : null
}

/**
 * `City, ST`, or whichever half is present, or null when neither is.
 *
 * Country is not part of the snippet templates, so it is never passed on.
 */
export function snippetPlace(
  city: string | null | undefined,
  state: string | null | undefined
): string | null {
  const place = formatLocation({ city, state })
  return place === LOCATION_UNKNOWN ? null : place
}

/**
 * The longest title that fits {@link TITLE_BUDGET} once the site suffix is
 * appended.
 *
 * `optionalSegments` are appended to `base` in the order given and dropped from
 * the END first, so the caller lists them most important first. The base is
 * never dropped or cut: a title whose base alone is over budget is returned as
 * the bare base.
 */
export function fitTitle(
  base: string,
  optionalSegments: ReadonlyArray<string | null>
): string {
  const segments = optionalSegments.filter(
    (segment): segment is string => segment !== null
  )
  for (let kept = segments.length; kept > 0; kept--) {
    const candidate = base + segments.slice(0, kept).join('')
    if (charCount(candidate + SITE_TITLE_SUFFIX) <= TITLE_BUDGET) {
      return candidate
    }
  }
  return base
}

function collapseWhitespace(text: string): string {
  return text.replace(/\s+/g, ' ').trim()
}

/**
 * Collapse whitespace, then cut to {@link DESCRIPTION_BUDGET} with the ellipsis
 * counted inside the budget. Text already within budget is returned whole.
 * Whitespace and punctuation at the cut are dropped so the ellipsis never
 * follows a full stop or a comma.
 */
export function fitDescription(text: string): string {
  const normalized = collapseWhitespace(text)
  const chars = Array.from(normalized)
  if (chars.length <= DESCRIPTION_BUDGET) return normalized
  const kept = chars
    .slice(0, DESCRIPTION_BUDGET - ELLIPSIS.length)
    .join('')
    .replace(/[\s.,;:!?]+$/u, '')
  return kept + ELLIPSIS
}

/**
 * The longest description that fits {@link DESCRIPTION_BUDGET} without cutting
 * an optional sentence.
 *
 * `optionalSentences` follow `lead` in the order given and are dropped from the
 * END first, whole, so the caller lists them most important first. The lead is
 * never dropped: when it alone is over budget it is cut by
 * {@link fitDescription}.
 */
function fitDescriptionSentences(
  lead: string,
  optionalSentences: ReadonlyArray<string | null>
): string {
  const sentences = optionalSentences.filter(
    (text): text is string => text !== null
  )
  for (let kept = sentences.length; kept > 0; kept--) {
    const candidate = collapseWhitespace(
      [lead, ...sentences.slice(0, kept)].join(' ')
    )
    if (charCount(candidate) <= DESCRIPTION_BUDGET) return candidate
  }
  return fitDescription(lead)
}

/** Ends a generated sentence with one full stop, never two. */
function sentence(text: string): string {
  return /[.!?]$/.test(text) ? text : `${text}.`
}

/** A count as a positive whole number, or null for zero, negative or unreadable. */
function positiveCount(value: number | null | undefined): number | null {
  return typeof value === 'number' && Number.isInteger(value) && value > 0
    ? value
    : null
}

/** `1 release`, `2 releases`: the page's count grouping, singular at exactly one. */
function countedNoun(value: number, singular: string, plural: string): string {
  return `${formatCount(value)} ${value === 1 ? singular : plural}`
}

/** The fields of an upcoming show row the upcoming-count rule reads. */
interface CancellableRow {
  is_cancelled: boolean
}

/**
 * The upcoming count a description may state: the payload's `total` (the
 * number the page's Upcoming shows header prints), but only when at least one
 * listed row is not cancelled, so a calendar of called-off shows is never
 * described as shows to come. Null when there is nothing to state.
 */
export function statedUpcomingCount(
  payload: { total: number; shows: ReadonlyArray<CancellableRow> } | null | undefined
): number | null {
  if (!payload || !payload.shows.some(row => !row.is_cancelled)) return null
  return positiveCount(payload.total)
}

/**
 * `Sep 19` on the show's own calendar, marked with the page's guessed-day
 * marker when the zone is a fallback. Null when the instant is unreadable.
 */
export function snippetMonthDay(timing: ShowTimingInput): string | null {
  if (!timing.eventDate || !hasReadableStartDate(timing.eventDate)) return null
  return showPageMonthDay(timing.eventDate, timing.state, timing.timezone)
}

function snippetLongDate(timing: ShowTimingInput): string | null {
  if (!timing.eventDate || !hasReadableStartDate(timing.eventDate)) return null
  return showPageDateLong(timing.eventDate, timing.state, timing.timezone)
}

export interface ShowSnippetInput {
  headliner: string
  venue: string
  city?: string | null
  state?: string | null
  timing: ShowTimingInput
  /** The show's own prose. Follows the generated sentence, never replaces it. */
  authoredDescription?: string | null
}

/**
 * Title `{headliner} at {venue}, {city} · {Mon D}`, dropping the date and then
 * the city to fit the budget.
 *
 * Description `{headliner} live at {venue} in {city}, {ST} on {weekday, Month
 * D, YYYY}.`, followed by any authored description, cut to the budget. A
 * missing place or date drops its clause. Support acts are not named.
 */
export function showSnippet(input: ShowSnippetInput): EntitySnippet {
  const city = nonEmpty(input.city)
  const place = snippetPlace(input.city, input.state)
  const monthDay = snippetMonthDay(input.timing)
  const longDate = snippetLongDate(input.timing)

  const title = fitTitle(`${input.headliner} at ${input.venue}`, [
    city ? `, ${city}` : null,
    monthDay ? ` · ${monthDay}` : null,
  ])

  const generated = sentence(
    `${input.headliner} live at ${input.venue}` +
      (place ? ` in ${place}` : '') +
      (longDate ? ` on ${longDate}` : '')
  )
  const authored = nonEmpty(input.authoredDescription)
  const description = fitDescription(
    authored ? `${generated} ${authored}` : generated
  )

  return { title, description }
}

export interface ShowEventDescriptionInput {
  /**
   * The bill sorted as the show page's header sorts it (`byBillPosition`).
   * `set_type` and `is_headliner` then decide who leads, through the same
   * `splitBill` the header calls.
   */
  artists?: ReadonlyArray<{
    name: string
    set_type?: string | null
    is_headliner?: boolean | null
  }> | null
  venue?: string | null
  city?: string | null
  /** The show's start instant, the one the page prints as its start time. */
  eventDate: string
  doorsAt?: string | null
  musicAt?: string | null
  /** The venue's zone inputs. A clock is stated only when they resolve one. */
  state?: string | null
  timezone?: string | null
}

/** `7:00 PM` in the zone, or null when the zone is a guess or the instant is unreadable. */
function eventClock(
  instant: string | null | undefined,
  input: Pick<ShowEventDescriptionInput, 'state' | 'timezone'>
): string | null {
  if (!instant || !Number.isFinite(Date.parse(instant))) return null
  return formatShowTime(instant, input.state, input.timezone)
}

/**
 * The bill's names split as the header splits them, blanks dropped and each
 * name stated once. The split runs on the whole bill, so an act the header
 * lists as support stays support even when the act it lists as lead has a
 * blank name. A name both leading and in support keeps its lead slot; a name
 * repeated in support keeps its first support slot.
 */
function billNames(
  artists: ShowEventDescriptionInput['artists']
): { leads: string[]; support: string[] } {
  const { headliners, support } = splitBill([...(artists ?? [])])
  const seen = new Set<string>()
  const unseenNames = (acts: ReadonlyArray<{ name: string }>) =>
    acts.flatMap(act => {
      const name = nonEmpty(act.name)
      if (!name || seen.has(name.toLowerCase())) return []
      seen.add(name.toLowerCase())
      return [name]
    })
  // Leads first, so a name in both keeps its lead slot.
  return { leads: unseenNames(headliners), support: unseenNames(support) }
}

/**
 * A `MusicEvent.description` for a show with no prose of its own, built only
 * from facts the show page renders:
 * `{headliner} with {support, comma separated} at {venue}, {city}. Doors {h:mm
 * AM/PM}, music {h:mm AM/PM}.` Null when the bill names nobody.
 *
 * Each clause is dropped when its fact is missing. The city qualifies the
 * venue, so it is stated only with one. Co-headliners share the lead, comma
 * separated, as the page's heading lists them.
 *
 * Clocks follow the page's own refusals. None is stated when the venue's zone
 * is a guess. Doors and music come only from the announced times, never from
 * the start instant alone, because a date-only show stores a placeholder
 * evening start that nobody announced. The page prints a music time beside a
 * door time; with no door time, the only clock it prints is the start time, so
 * a lone music time is stated only when it is that same clock. Those page
 * rules are `doorsMusicFactSegment` and `startTimeFactSegment` in
 * `features/shows/components/showStatusStripeCopy`; this module's tests run
 * one fixture table through them and through this function to hold the two in
 * step.
 *
 * A door or music time stored for another night than the start is not stated
 * (see `isAnnouncedTimeOfStart`), so the description can say less than the
 * page, never more.
 */
export function showEventDescription(
  input: ShowEventDescriptionInput
): string | null {
  const { leads, support } = billNames(input.artists)
  if (leads.length === 0) return null

  const venue = nonEmpty(input.venue)
  const city = venue ? nonEmpty(input.city) : null
  const lead = sentence(
    leads.join(', ') +
      (support.length > 0 ? ` with ${support.join(', ')}` : '') +
      (venue ? ` at ${venue}` : '') +
      (city ? `, ${city}` : '')
  )

  const doors = isAnnouncedTimeOfStart(input.doorsAt, input.eventDate, 'doors')
    ? eventClock(input.doorsAt, input)
    : null
  const announcedMusic = isAnnouncedTimeOfStart(input.musicAt, input.eventDate, 'music')
    ? eventClock(input.musicAt, input)
    : null
  const music =
    announcedMusic &&
    (doors || announcedMusic === eventClock(input.eventDate, input))
      ? announcedMusic
      : null
  const times = timesSentence(doors, music)

  // Also turns the narrow no-break space some ICU builds put before AM/PM
  // into a plain one.
  return collapseWhitespace(times ? `${lead} ${times}` : lead)
}

function timesSentence(doors: string | null, music: string | null): string | null {
  if (doors && music) return `Doors ${doors}, music ${music}.`
  if (doors) return `Doors ${doors}.`
  if (music) return `Music ${music}.`
  return null
}

export interface ArtistNextShow {
  venue: string
  /** The venue's city, which qualifies the venue name. */
  venueCity?: string | null
  timing: ShowTimingInput
}

/** The fields of an artist's upcoming show row the `Next show` sentence reads. */
export interface ArtistUpcomingRow extends CancellableRow {
  event_date: string
  venue: {
    name: string
    city?: string | null
    state?: string | null
    timezone?: string | null
  } | null
}

/**
 * The artist's next show in a soonest-first upcoming list: the first row that
 * is not cancelled and has a readable date, read on its venue's calendar. Null
 * when that row has no named venue, because naming a later show would call it
 * the next one when it is not.
 */
export function artistNextShowFrom(
  rows: ReadonlyArray<ArtistUpcomingRow>
): ArtistNextShow | null {
  const row = rows.find(
    candidate => !candidate.is_cancelled && hasReadableStartDate(candidate.event_date)
  )
  const venue = nonEmpty(row?.venue?.name)
  if (!row || !venue) return null
  return {
    venue,
    venueCity: row.venue?.city,
    timing: {
      eventDate: row.event_date,
      state: row.venue?.state,
      timezone: row.venue?.timezone,
    },
  }
}

export interface ArtistSnippetInput {
  name: string
  city?: string | null
  state?: string | null
  nextShow?: ArtistNextShow | null
  /** From {@link statedUpcomingCount}. */
  upcomingCount?: number | null
  releaseCount?: number | null
  similarArtistCount?: number | null
}

/**
 * Title `{name} · {city}, {ST}`, or `{name}` when there is no location or it
 * does not fit.
 *
 * Description `{name} from {city}, {ST}.`, then `Next show: {venue}, {venue
 * city}, {Mon D}.`, `Upcoming shows: {N}.`, `Releases: {R}.` and `Similar
 * artists: {S}.`. Each sentence appears only when its fact exists and is not
 * zero; no location drops `from`. To fit the budget the sentences go from the
 * end, whole; the lead is never dropped.
 */
export function artistSnippet(input: ArtistSnippetInput): EntitySnippet {
  const place = snippetPlace(input.city, input.state)
  const next = input.nextShow
  const nextVenue = nonEmpty(next?.venue)
  const nextCity = nonEmpty(next?.venueCity)
  const nextDay = next ? snippetMonthDay(next.timing) : null
  const upcoming = positiveCount(input.upcomingCount)
  const releases = positiveCount(input.releaseCount)
  const similar = positiveCount(input.similarArtistCount)

  return {
    title: fitTitle(input.name, [place ? ` · ${place}` : null]),
    description: fitDescriptionSentences(
      sentence(`${input.name}${place ? ` from ${place}` : ''}`),
      [
        nextVenue && nextDay
          ? `Next show: ${nextVenue}${nextCity ? `, ${nextCity}` : ''}, ${nextDay}.`
          : null,
        upcoming ? `Upcoming shows: ${formatCount(upcoming)}.` : null,
        releases ? `Releases: ${formatCount(releases)}.` : null,
        similar ? `Similar artists: ${formatCount(similar)}.` : null,
      ]
    ),
  }
}

export interface VenueNextShow {
  headliner: string
  timing: ShowTimingInput
}

/** The fields of an upcoming venue show row the `Next` clause reads. */
export interface VenueUpcomingRow {
  event_date: string
  is_cancelled: boolean
  /** The show's own state, which outranks the venue's for its calendar day. */
  state?: string | null
  title?: string | null
  artists?: ReadonlyArray<{ name: string; is_headliner?: boolean | null }>
}

/**
 * The first show in a soonest-first upcoming list that can be named as next:
 * not cancelled, with a parseable date and a headliner (or, failing that, a
 * title). Its day is read on the venue's calendar.
 */
export function venueNextShowFrom(
  rows: ReadonlyArray<VenueUpcomingRow>,
  venue: { state?: string | null; timezone?: string | null }
): VenueNextShow | null {
  for (const row of rows) {
    if (row.is_cancelled || !hasReadableStartDate(row.event_date)) continue
    const headliner = nonEmpty(
      row.artists?.find(artist => artist.is_headliner)?.name ||
        row.artists?.[0]?.name ||
        row.title
    )
    if (!headliner) continue
    return {
      headliner,
      timing: {
        eventDate: row.event_date,
        state: row.state ?? venue.state,
        timezone: venue.timezone,
      },
    }
  }
  return null
}

/** A venue's past-show year histogram as the archive reads it. */
export interface VenuePastRecord {
  years: ReadonlyArray<{ year: number; count: number }>
}

/**
 * The past shows on record and the first year they start in, from the
 * histogram's well-formed rows (a whole year with a positive count). Null when
 * no row qualifies.
 */
export function venuePastSummary(
  record: VenuePastRecord | null | undefined
): { count: number; firstYear: number } | null {
  const rows = (record?.years ?? []).filter(
    entry => Number.isInteger(entry.year) && positiveCount(entry.count) !== null
  )
  if (rows.length === 0) return null
  return {
    count: rows.reduce((sum, entry) => sum + entry.count, 0),
    firstYear: Math.min(...rows.map(entry => entry.year)),
  }
}

export interface VenueSnippetInput {
  name: string
  city?: string | null
  state?: string | null
  address?: string | null
  nextShow?: VenueNextShow | null
  /** From {@link statedUpcomingCount}. */
  upcomingCount?: number | null
  pastRecord?: VenuePastRecord | null
  /** The current calendar year on the venue's clock. */
  currentYear: number
}

/**
 * Title `{name} shows · {city}, {ST}`: the place drops first, then `shows`, to
 * fit the budget; the name is never cut.
 *
 * Description, by what the room has on record:
 * - shows coming: `{N} upcoming shows at {name} in {city}, {ST}.`, then `Next:
 *   {headliner}, {Mon D}.`, `{P} past shows on record since {Y}.` and
 *   `Address: {address}.`;
 * - only past shows: `{name} in {city}, {ST}: {P} past shows on record since
 *   {Y}.`, then the address;
 * - nothing: `{name} is a venue in {city}, {ST}.`, then the address.
 *
 * `since {Y}` names the first year on record only when it is before the
 * current year. Nothing is said about upcoming shows for a room with none to
 * state. Optional sentences drop from the end, whole; the lead is never
 * dropped.
 */
export function venueSnippet(input: VenueSnippetInput): EntitySnippet {
  const place = snippetPlace(input.city, input.state)
  const inPlace = place ? ` in ${place}` : ''
  const nextHeadliner = nonEmpty(input.nextShow?.headliner)
  const nextDay = input.nextShow ? snippetMonthDay(input.nextShow.timing) : null
  const upcoming = positiveCount(input.upcomingCount)
  const past = venuePastSummary(input.pastRecord)
  const pastOnRecord = past
    ? countedNoun(past.count, 'past show', 'past shows') +
      ' on record' +
      (past.firstYear < input.currentYear ? ` since ${past.firstYear}` : '')
    : null
  const address = nonEmpty(input.address)
  const addressSentence = address ? sentence(`Address: ${address}`) : null

  const [lead, optional]: [string, ReadonlyArray<string | null>] = upcoming
    ? [
        `${countedNoun(upcoming, 'upcoming show', 'upcoming shows')} at ${input.name}${inPlace}`,
        [
          nextHeadliner && nextDay ? `Next: ${nextHeadliner}, ${nextDay}.` : null,
          pastOnRecord ? sentence(pastOnRecord) : null,
          addressSentence,
        ],
      ]
    : pastOnRecord
      ? [`${input.name}${inPlace}: ${pastOnRecord}`, [addressSentence]]
      : [`${input.name} is a venue${inPlace}`, [addressSentence]]

  return {
    title: fitTitle(input.name, [' shows', place ? ` · ${place}` : null]),
    description: fitDescriptionSentences(sentence(lead), optional),
  }
}

/**
 * How a release's artists are named: `A`, `A and B`, or `A and others` for
 * three or more. Null when no artist has a name.
 */
export function releaseArtistPhrase(
  names: ReadonlyArray<string> | null | undefined
): string | null {
  const named = (names ?? [])
    .map(name => nonEmpty(name))
    .filter((name): name is string => name !== null)
  if (named.length === 0) return null
  if (named.length === 1) return named[0]
  if (named.length === 2) return `${named[0]} and ${named[1]}`
  return `${named[0]} and others`
}

export interface ReleaseSnippetInput {
  title: string
  /** The credited artists' names, in credit order. */
  artists?: ReadonlyArray<string> | null
  /** The type label the page shows. */
  typeLabel?: string | null
  year?: number | null
  /** The label to name: the first one the page lists with a name. */
  label?: string | null
  /** Services the release can be played or bought on, as the page names them. */
  listenServices?: ReadonlyArray<string> | null
}

/**
 * Title `{title} by {artist} ({year})`, dropping the year to fit the budget.
 * The artist is never dropped, so a title over budget without the year is kept
 * whole.
 *
 * Description `{title}, {type} by {artist} ({year}).`, then `Label: {label}.`
 * and `Listen: {services}.`. A missing artist, type, year, label or service
 * drops its clause. To fit the budget the Listen sentence goes first, then the
 * Label sentence, each whole; a first sentence over budget alone is cut.
 */
export function releaseSnippet(input: ReleaseSnippetInput): EntitySnippet {
  const artist = releaseArtistPhrase(input.artists)
  const by = artist ? ` by ${artist}` : ''
  const typeLabel = nonEmpty(input.typeLabel)
  const year = input.year != null ? ` (${input.year})` : null
  const label = nonEmpty(input.label)
  const services = (input.listenServices ?? [])
    .map(service => nonEmpty(service))
    .filter((service): service is string => service !== null)

  return {
    title: fitTitle(`${input.title}${by}`, [year]),
    description: fitDescriptionSentences(
      sentence(
        `${input.title}${typeLabel ? `, ${typeLabel}` : ''}${by}${year ?? ''}`
      ),
      [
        label ? sentence(`Label: ${label}`) : null,
        services.length > 0 ? sentence(`Listen: ${services.join(', ')}`) : null,
      ]
    ),
  }
}

export interface LabelSnippetInput {
  name: string
  city?: string | null
  state?: string | null
  foundedYear?: number | null
  /** A `LabelStatus`; anything else reads as active. */
  status?: string | null
  artistCount?: number | null
  releaseCount?: number | null
}

/** `a label`, or the status that the page's badge shows for one not active. */
function labelKind(status: string | null | undefined): string {
  switch (status?.trim().toLowerCase() as LabelStatus | undefined) {
    case 'inactive':
      return 'an inactive label'
    case 'defunct':
      return 'a defunct label'
    default:
      return 'a label'
  }
}

/**
 * Description `{name} is a label from {city}, {ST}, founded in {Y}.`, then
 * `{A} artists and {R} releases on record.`. No location drops `from`, no
 * founding year drops `founded`, a label that is not active reads `is an
 * inactive label` or `is a defunct label`, and a zero count drops its half
 * (both zero drop the sentence). The counts sentence drops first to fit the
 * budget.
 */
export function labelDescription(input: LabelSnippetInput): string {
  const place = snippetPlace(input.city, input.state)
  const founded = positiveCount(input.foundedYear)
  const artists = positiveCount(input.artistCount)
  const releases = positiveCount(input.releaseCount)
  const countPhrases = [
    artists ? countedNoun(artists, 'artist', 'artists') : null,
    releases ? countedNoun(releases, 'release', 'releases') : null,
  ].filter((phrase): phrase is string => phrase !== null)

  return fitDescriptionSentences(
    sentence(
      `${input.name} is ${labelKind(input.status)}${place ? ` from ${place}` : ''}${founded ? `, founded in ${founded}` : ''}`
    ),
    [countPhrases.length > 0 ? `${countPhrases.join(' and ')} on record.` : null]
  )
}

const MONTH_ABBREVIATIONS = [
  'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
  'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
] as const

interface CalendarDate {
  year: number
  month: number
  day: number
}

/** A `YYYY-MM-DD` festival date as calendar parts, or null when it is not one. */
function parseFestivalDate(value: string | null | undefined): CalendarDate | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value?.trim() ?? '')
  if (!match) return null
  const [year, month, day] = match.slice(1).map(Number)
  // Round-trips through UTC so a day the month does not have is refused.
  const probe = new Date(Date.UTC(year, month - 1, day))
  if (probe.getUTCMonth() !== month - 1 || probe.getUTCDate() !== day) return null
  return { year, month, day }
}

/**
 * `May 30, 2026`, `Sep 18 to 20, 2026`, `Sep 30 to Oct 2, 2026`, or
 * `Dec 30, 2026 to Jan 2, 2027`: the snippet's own form, joined with "to"
 * where the page's `formatFestivalDateRange` uses a dash. The dates are
 * calendar dates, so no zone is applied. An unreadable end date gives the start date alone; an unreadable
 * start date gives null.
 */
export function festivalDateRange(
  startDate: string | null | undefined,
  endDate: string | null | undefined
): string | null {
  const start = parseFestivalDate(startDate)
  if (!start) return null
  const startMonth = MONTH_ABBREVIATIONS[start.month - 1]
  const end = parseFestivalDate(endDate)
  const sameDay =
    !end || (end.year === start.year && end.month === start.month && end.day === start.day)
  if (sameDay) return `${startMonth} ${start.day}, ${start.year}`
  const endMonth = MONTH_ABBREVIATIONS[end.month - 1]
  if (end.year !== start.year) {
    return `${startMonth} ${start.day}, ${start.year} to ${endMonth} ${end.day}, ${end.year}`
  }
  if (end.month !== start.month) {
    return `${startMonth} ${start.day} to ${endMonth} ${end.day}, ${start.year}`
  }
  return `${startMonth} ${start.day} to ${end.day}, ${start.year}`
}

export interface FestivalSnippetInput {
  name: string
  startDate?: string | null
  endDate?: string | null
  locationName?: string | null
  city?: string | null
  state?: string | null
  status?: string | null
  artistCount?: number | null
  venueCount?: number | null
}

/**
 * Description `{name}: {dates}, {location name}, {city}, {ST}.`, then
 * `Lineup: {A} artists.` and `Venues: {V}.`. A missing date, location name or
 * place drops its part; a cancelled festival reads `{name} (cancelled): ...`.
 * The lineup sentence needs at least one artist and the venues sentence more
 * than one venue. To fit the budget Venues goes first, then Lineup.
 */
export function festivalDescription(input: FestivalSnippetInput): string {
  const parts = [
    festivalDateRange(input.startDate, input.endDate),
    nonEmpty(input.locationName),
    snippetPlace(input.city, input.state),
  ].filter((part): part is string => part !== null)
  const cancelled =
    (input.status?.trim().toLowerCase() as FestivalStatus | undefined) === 'cancelled'
  const artists = positiveCount(input.artistCount)
  const venues = positiveCount(input.venueCount)

  return fitDescriptionSentences(
    sentence(
      `${input.name}${cancelled ? ' (cancelled)' : ''}${parts.length > 0 ? `: ${parts.join(', ')}` : ''}`
    ),
    [
      artists ? `Lineup: ${countedNoun(artists, 'artist', 'artists')}.` : null,
      venues && venues > 1 ? `Venues: ${formatCount(venues)}.` : null,
    ]
  )
}
