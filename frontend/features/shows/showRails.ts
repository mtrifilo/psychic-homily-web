import type { components } from '@/types/api'
import {
  looksLikeCalendarDate,
  formatDayChip,
  formatShowStartTime,
} from '@/features/scenes/sceneDay'
import { isCalendarDate, parseCalendarDate } from '@/features/scenes/sceneWeek'
import type { SceneShowSummary } from '@/features/scenes/types'
import { formatPrice } from '@/lib/utils/formatters'
import { formatShowMonthDay } from '@/lib/utils/showDateBadge'
import type { VenueShow } from '@/features/venues/types'
import type { VenueResponse } from './types'

/**
 * The show page's two discovery rails: what else is on in this metro that
 * night, and what else this room has coming.
 *
 * This module is the rails' POLICY — which rows survive, what the headings
 * say, whether a "see all" is offered and where it points — kept apart from
 * the markup in `ShowDiscoveryRails.tsx` so each decision has one testable
 * home and neither rail's rule can be changed without the other's being read.
 * Nothing here renders.
 */

/** The also-tonight payload, derived from the generated schema (never hand-written). */
export type ShowAlsoTonightResponse =
  components['schemas']['ShowAlsoTonightResponse']

/**
 * One row of the also-tonight rail.
 *
 * Aliases the scenes feature's own alias rather than re-reaching into
 * `components`, matching how `sceneDay.ts` names its reading of the same
 * schema type: a rename then has one site to find, not two.
 */
export type AlsoTonightShow = SceneShowSummary

/**
 * Rows drawn per rail.
 *
 * Three, the number the locked mock draws. A rail is a glance, not a listing:
 * the full answer lives one click away behind each rail's "see all", and the
 * year-anchored pagination precedent the venue and artist show lists set
 * (PSY-1750..1756) is deliberately NOT imported here — it governs ORDERED
 * ARCHIVES a reader navigates, not a fixed-length teaser.
 */
export const SHOW_RAIL_ROW_CAP = 3

/**
 * Rows to REQUEST for the venue rail: one more than the cap.
 *
 * The venue's upcoming list contains the show being viewed whenever that show
 * is itself upcoming, and dropping it is what stops a page recommending
 * itself. Asking for exactly the cap would then leave the rail one row short
 * on every upcoming show — the common case. One spare absorbs that removal.
 *
 * The also-tonight endpoint excludes the subject show server-side, so its rail
 * needs no equivalent (`buildAlsoTonightRail` still re-checks; see there).
 */
export const VENUE_RAIL_FETCH_LIMIT = SHOW_RAIL_ROW_CAP + 1

/**
 * Everything a rail needs in order to render, and nothing about how.
 *
 * `seeAllHref` folds the destination and the decision to offer it into one
 * field on purpose: "is there a link" and "is anything actually hidden" are
 * never separately interesting to a caller, and keeping them apart is what let
 * the two rails answer the truncation question in two different places.
 */
export interface ShowRail {
  /** The `SECTION / QUALIFIER` heading, composed. */
  title: string
  /** Non-empty, and already rendered to primitives. */
  rows: RailRow[]
  /** Where "see all" goes, or null when it must not be offered. */
  seeAllHref: string | null
}

/**
 * The also-tonight rail, or null when there is nothing to head.
 *
 * The subject-show exclusion is belt-and-braces: `GET /shows/{id}/also-tonight`
 * already documents that it excludes the subject show, and this is the
 * boundary where that promise arrives from another process. A show listed in
 * its own "also tonight" rail is the single most visible way this feature can
 * be wrong, and the check costs one comparison per row.
 *
 * That filter runs ONCE, and the truncation question is answered from its
 * result rather than by re-deriving it, so the rows drawn and the claim that
 * more exist can never disagree.
 */
export function buildAlsoTonightRail(
  rail: ShowAlsoTonightResponse | undefined,
  currentShowId: number
): ShowRail | null {
  if (!rail) return null

  // `shows` is typed nullable by the generator even though the API always
  // emits an array — the same accommodation `dayShows` makes.
  const listable = (rail.shows ?? []).filter(show => show.id !== currentShowId)
  const drawn = listable.slice(0, SHOW_RAIL_ROW_CAP)
  if (drawn.length === 0) return null

  // Two independent sources of truncation: the backend's own cap (`has_more`,
  // which compares against the whole night rather than against this rail), and
  // this rail's cap of three. Either one hiding a show is a reason to offer
  // the full night.
  const hasMore = rail.has_more || listable.length > drawn.length

  return {
    title: alsoTonightRailTitle(rail),
    rows: drawn.map(show => alsoTonightRow(show, rail.timezone)),
    seeAllHref: hasMore ? alsoTonightSeeAllHref(rail) : null,
  }
}

/**
 * The more-at-venue rail, or null when the room has no other dates.
 *
 * The truncation question is "does the venue page hold a show this rail did
 * not draw", and answering it means reconciling two different populations.
 * `total` counts the venue's APPROVED UPCOMING shows; `drawn` is this rail's
 * three, already minus the show being read. Those differ by the subject show
 * only when the subject is itself approved and upcoming — a PAST show, and the
 * majority of show pages become past ones, is absent from `total` entirely.
 *
 * So the adjustment is conditional on the subject actually having been
 * removed, not unconditional. Adding 1 either way withheld "see all" from
 * exactly the case it exists for: a past show at a room with four upcoming
 * dates draws three, hides one, and `4 > 4` is false.
 *
 * `subjectWasRemoved` is read off the fetched page rather than inferred, and
 * it is trustworthy precisely where it matters. It can only be wrong if the
 * subject is upcoming but sorted beyond the fetched page — which requires
 * `total` to exceed the page size, and in that case `hasMore` is already true
 * by a wide margin and the adjustment cannot change the answer.
 *
 * This rail's `[See all]` and `ShowVenueModule`'s `More at {venue} →` point at
 * the same venue page, which the mock draws BOTH of: the module's link is part
 * of the venue's own verb cluster beside [Directions] and [Follow venue], and
 * this one is the rail's own overflow. They are kept because the mock keeps
 * them, not by oversight — if one is ever cut, cut the module's, since a
 * bracket that overflows a visible list is the one carrying new information.
 */
export function buildMoreAtVenueRail(
  venue: VenueResponse | undefined,
  shows: VenueShow[] | undefined,
  total: number | undefined,
  currentShowId: number
): ShowRail | null {
  if (!venue) return null

  const fetched = shows ?? []
  const drawn = fetched
    .filter(show => show.id !== currentShowId)
    .slice(0, SHOW_RAIL_ROW_CAP)
  if (drawn.length === 0) return null

  const subjectWasRemoved = fetched.some(show => show.id === currentShowId)
  const hasMore =
    (total ?? drawn.length) > drawn.length + (subjectWasRemoved ? 1 : 0)

  return {
    title: `More at / ${venue.name}`,
    rows: drawn.map(show => moreAtVenueRow(show, venue)),
    // Guarded on the slug as well as on truncation: entity slugs are nullable
    // in this schema and an empty one resolves `/venues/` to the INDEX rather
    // than 404ing, so an unguarded href silently lands a reader on the wrong
    // page (PSY-1754).
    seeAllHref: hasMore && venue.slug ? `/venues/${venue.slug}` : null,
  }
}

/**
 * The also-tonight heading's qualifier: `Tonight`, or the night's own date.
 *
 * "Tonight" is the mock's register and the rail's name, but it is only TRUE on
 * the night itself, and a show page is read months early and years late. The
 * payload settles it: `is_tonight` is computed on the SCENE's clock with the
 * 6am night boundary applied (until 06:00 local, "tonight" still names
 * yesterday's date), because a client computing it from the viewer's device
 * would give a reader in Berlin a different answer than a reader in Chicago
 * for the same Chicago night. Read the flag; never re-derive it.
 *
 * The date is SHAPE-CHECKED first, which `parseCalendarDate` requires of any
 * caller whose field might be absent: it builds its Date component-wise, so
 * `''` yields a confident `Mon Jan 1 1900` and an absent field throws out of
 * `split`, which from here would take the whole show page to its error
 * boundary rather than just dropping a rail. The type says the field is
 * required, but a type is not a runtime guarantee across two independently
 * deployed services — the same standard `startInstant` applies to `starts_at`.
 * `isCalendarDate`, deliberately, not `looksLikeCalendarDate`: the latter also
 * bounds the year to cap URL cache keys, which applied to a payload field
 * would blank a legitimately dated old show.
 *
 * The year joins the label whenever it is not the current one. A rail headed
 * `Also / Thu Aug 15` on a 2019 archive page reads as this August to every
 * reader, and unlike the scene day view there is no full date elsewhere in the
 * row to correct the impression — the same reasoning, and the same remedy, as
 * `formatPointerDay`. Comparing against the viewer's clock is safe here
 * because these rails are client-only and render nothing until their query
 * resolves, so there is no server pass to disagree with.
 */
function alsoTonightQualifier(rail: ShowAlsoTonightResponse): string | null {
  if (rail.is_tonight) return 'Tonight'
  if (!isCalendarDate(rail.date ?? '')) return null

  const chip = formatDayChip(rail.date)
  const year = parseCalendarDate(rail.date).getFullYear()
  return year === new Date().getFullYear() ? chip : `${chip}, ${year}`
}

/**
 * The full also-tonight heading, in the page's `SECTION / QUALIFIER` register.
 *
 * The city is the metro's PRINCIPAL city as the backend resolved it, so an
 * Evanston room reads "Chicago" — the scope the rows were actually selected
 * by. It is omitted rather than guessed when the payload has no scene, which
 * is also the case where there are no rows to head.
 *
 * Each half degrades independently, so an unusable date still leaves a heading
 * that names its scope (`Also / Chicago`) rather than a dangling separator.
 */
export function alsoTonightRailTitle(rail: ShowAlsoTonightResponse): string {
  const parts = [alsoTonightQualifier(rail), rail.city].filter(
    (part): part is string => Boolean(part)
  )
  return parts.length > 0 ? `Also / ${parts.join(' · ')}` : 'Also'
}

/**
 * Where the also-tonight rail's "see all" goes: the scene's own page for that
 * night, `/scenes/{slug}/{YYYY-MM-DD}`.
 *
 * Null unless BOTH halves are honest. `scene_slug` is withheld by the backend
 * whenever following it would land somewhere that does not list the show it
 * came from (an archive date outside the scene-day window, or a room the metro
 * backfill never reached), so its presence is the server's own permission to
 * link. The date is re-checked against the same shape the `/scenes/{slug}/
 * {period}` route uses to route a segment, so a malformed date can never be
 * turned into a link to a page that will 404.
 */
export function alsoTonightSeeAllHref(
  rail: ShowAlsoTonightResponse
): string | null {
  if (!rail.scene_slug || !looksLikeCalendarDate(rail.date)) return null
  return `/scenes/${rail.scene_slug}/${rail.date}`
}

/**
 * A rail row's `/shows/...` target, from either rail's payload shape.
 *
 * Both rails address a show the same way and must keep doing so: an empty slug
 * is a modeled case here, and `/shows/` resolves to the INDEX rather than
 * 404ing (PSY-1754), so the id fallback is load-bearing, not merely defensive.
 */
function railShowHref(show: { slug?: string | null; id: number }): string {
  return show.slug ? `/shows/${show.slug}` : `/shows/${show.id}`
}

/** A rail row's price, or null when the show has none recorded. */
function railPrice(price: number | null | undefined): string | null {
  return typeof price === 'number' ? formatPrice(price) : null
}

/**
 * A rail row's bill as ONE line of text.
 *
 * Bill-first, matching the scene views rather than
 * `lib/utils/showDisplayTitle` (which is title-first): these rails list who is
 * playing, and a promoter's event title is the fallback, not the headline.
 * Trimmed, because a whitespace-only title is truthy and would otherwise
 * render an invisible, unclickable label.
 */
function railBillLine(
  names: Array<string | null | undefined>,
  title?: string | null
): string {
  const billed = names
    .map(name => name?.trim())
    .filter((name): name is string => Boolean(name))
  if (billed.length > 0) return billed.join(', ')
  return title?.trim() || 'Live music'
}

/**
 * One rendered ledger row: primitives only, no payload shape.
 *
 * The two rails carry two different wire types, and this is the seam where
 * that stops mattering. Keeping it primitives-only is what lets ONE row
 * renderer serve both without a mode flag — the alternative, a row component
 * that understands both shapes, is the multi-mode design `CompactShowRow`
 * already demonstrates the cost of.
 */
export interface RailRow {
  href: string
  /** Time or date. Null when the payload carries no usable instant. */
  lead: string | null
  title: string
  isCancelled: boolean
  isSoldOut: boolean
  /** Right-hand facts, already filtered — the renderer drops nothing. */
  facts: string[]
}

/**
 * One also-tonight row: time, bill, room, price.
 *
 * The time is VENUE-LOCAL, never the reader's. `formatShowStartTime` prefers
 * the row's own `venue_timezone` and falls back to the zone the BACKEND
 * computed this night's window in — the scene's modal clock, the same one that
 * decided this show belongs to this night. Falling back to the viewer's device
 * instead is how a listed time comes to disagree with the heading above it: a
 * reader in Berlin would see a Chicago night set in CEST.
 *
 * Three deliberate deviations from the locked mock, all of them here:
 *
 *  - No age column, which the mock draws (`21+`). `SceneShowSummary` carries no
 *    age requirement, and the rail is not worth a request per row to invent
 *    one. The venue module above states the age rule for the show being read,
 *    which is the one a reader on this page is deciding about.
 *  - `8:00 PM`, not the mock's `8PM`. `formatShowTime` is the site's one
 *    show-time format; the mock's sample times all happen to fall on the hour,
 *    and a 7:30 door cannot be said as "7PM".
 *  - `$15.00`, not `$15`. Same argument: `formatPrice` is the one money format,
 *    and forking it here would put two dollar renderings on one page.
 */
export function alsoTonightRow(
  show: AlsoTonightShow,
  railTimezone: string
): RailRow {
  return {
    href: railShowHref(show),
    lead: formatShowStartTime(show, railTimezone),
    title: railBillLine(show.artist_names ?? [], show.title),
    isCancelled: show.is_cancelled,
    isSoldOut: show.is_sold_out,
    facts: [show.venue_name, railPrice(show.price)].filter(
      (fact): fact is string => Boolean(fact)
    ),
  }
}

/**
 * One more-at-venue row: date, bill, price.
 *
 * Date leads, not time: every row is at the same room, so the question this
 * rail answers is "when next". The room's name is likewise absent — it is in
 * the heading.
 *
 * A show's own state wins over the venue's, and the venue's resolved IANA zone
 * wins over the state map (PSY-986) — the same resolution order the venue
 * page's own table uses, so the two surfaces cannot print different dates for
 * one show.
 */
export function moreAtVenueRow(show: VenueShow, venue: VenueResponse): RailRow {
  return {
    href: railShowHref(show),
    lead: formatShowMonthDay(
      show.event_date,
      show.state ?? venue.state,
      venue.timezone
    ),
    title: railBillLine(show.artists.map(artist => artist.name), show.title),
    isCancelled: show.is_cancelled,
    isSoldOut: show.is_sold_out,
    facts: [railPrice(show.price)].filter((fact): fact is string =>
      Boolean(fact)
    ),
  }
}
