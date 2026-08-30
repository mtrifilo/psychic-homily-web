import type { components } from '@/types/api'
import {
  looksLikeCalendarDate,
  formatDayChip,
  formatShowStartTime,
} from '@/features/scenes/sceneDay'
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
 * `total` counts every upcoming show at this room, INCLUDING the one being
 * read, so it has to go back on the drawn side before asking whether anything
 * is hidden. Without that the rail offers "see all" to a venue page holding
 * exactly the rows already on screen. The `+ 1` is unconditional: when the
 * subject show is upcoming it really is in `total`, and when it is not,
 * `total` exceeds the cap by more and the comparison still holds.
 */
export function buildMoreAtVenueRail(
  venue: VenueResponse | undefined,
  shows: VenueShow[] | undefined,
  total: number | undefined,
  currentShowId: number
): ShowRail | null {
  if (!venue) return null

  const drawn = (shows ?? [])
    .filter(show => show.id !== currentShowId)
    .slice(0, SHOW_RAIL_ROW_CAP)
  if (drawn.length === 0) return null

  const hasMore = (total ?? drawn.length) > drawn.length + 1

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
 */
function alsoTonightQualifier(rail: ShowAlsoTonightResponse): string {
  return rail.is_tonight ? 'Tonight' : formatDayChip(rail.date)
}

/**
 * The full also-tonight heading, in the page's `SECTION / QUALIFIER` register.
 *
 * The city is the metro's PRINCIPAL city as the backend resolved it, so an
 * Evanston room reads "Chicago" — the scope the rows were actually selected
 * by. It is omitted rather than guessed when the payload has no scene, which
 * is also the case where there are no rows to head.
 */
export function alsoTonightRailTitle(rail: ShowAlsoTonightResponse): string {
  const qualifier = alsoTonightQualifier(rail)
  return rail.city ? `Also / ${qualifier} · ${rail.city}` : `Also / ${qualifier}`
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
 * No age column, which the mock draws: `SceneShowSummary` carries no age
 * requirement, and the rail is not worth a request per row to invent one. The
 * venue module above states the age rule for the show being read, which is the
 * one a reader on this page is deciding about.
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
