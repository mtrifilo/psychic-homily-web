import { formatLocation, LOCATION_UNKNOWN } from '@/lib/formatLocation'
import {
  hasReadableStartDate,
  type ShowTimingInput,
} from '@/lib/utils/showTiming'
import {
  showPageDateLong,
  showPageMonthDay,
} from '@/features/shows/showPageDate'

/**
 * Search-snippet strings (`<title>` and meta description) for the show, artist,
 * venue and release detail pages.
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

export interface ArtistSnippetInput {
  name: string
  city?: string | null
  state?: string | null
}

/**
 * Title `{name} · {city}, {ST}`, or `{name}` when there is no location or it
 * does not fit. Description `{name} from {city}, {ST}: shows, similar artists
 * and connections on Psychic Homily`, without the `from` clause when there is
 * no location.
 */
export function artistSnippet(input: ArtistSnippetInput): EntitySnippet {
  const place = snippetPlace(input.city, input.state)
  return {
    title: fitTitle(input.name, [place ? ` · ${place}` : null]),
    description: fitDescription(
      `${input.name}${place ? ` from ${place}` : ''}: shows, similar artists and connections on Psychic Homily`
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

export interface VenueSnippetInput {
  name: string
  city?: string | null
  state?: string | null
  nextShow?: VenueNextShow | null
}

/**
 * Title `{name} · {city}, {ST}`, or `{name}` when there is no location or it
 * does not fit. Description `Upcoming shows at {name} in {city}, {ST}. Next:
 * {headliner}, {Mon D}.`, without the `Next` clause when there is no next show
 * or its date is unknown.
 */
export function venueSnippet(input: VenueSnippetInput): EntitySnippet {
  const place = snippetPlace(input.city, input.state)
  const nextHeadliner = nonEmpty(input.nextShow?.headliner)
  const nextDay = input.nextShow ? snippetMonthDay(input.nextShow.timing) : null
  const lead = sentence(
    `Upcoming shows at ${input.name}${place ? ` in ${place}` : ''}`
  )
  return {
    title: fitTitle(input.name, [place ? ` · ${place}` : null]),
    description: fitDescription(
      nextHeadliner && nextDay
        ? `${lead} Next: ${nextHeadliner}, ${nextDay}.`
        : lead
    ),
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
