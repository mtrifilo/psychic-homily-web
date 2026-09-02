import type { components } from '@/types/api'
import {
  looksLikeCalendarDate,
  formatDayChip,
  formatShowStartTime,
} from '@/features/scenes/sceneDay'
import { isCalendarDate, parseCalendarDate } from '@/features/scenes/sceneWeek'
import type { SceneShowSummary } from '@/features/scenes/types'
import { showPriceLabel } from '@/lib/utils/showPrice'
import {
  formatShowMonthDayPadded,
  showYearInZone,
} from '@/lib/utils/showDateBadge'
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
 * Rows to REQUEST for the venue rail, budgeted for everything it can discard.
 *
 * The rail filters the fetched page TWICE, and the request has to survive both
 * or it draws fewer rows than the mock while the room has a full calendar:
 *
 *  - the subject show, whenever it is itself approved and upcoming, because a
 *    page must not recommend itself; and
 *  - up to `SHOW_RAIL_ROW_CAP` more that the also-tonight rail already drew —
 *    another bill at THIS room on THIS night, which both queries return.
 *
 * Hence cap + 1 + cap. Asking for cap + 1 (which is all the subject show
 * needed, before the cross-rail exclusion existed) leaves a room with an early
 * and a late set drawing two rows, and a room running three bills on one night
 * drawing none at all — the rail vanishing from a venue that is not empty.
 *
 * The also-tonight endpoint excludes the subject show server-side and has no
 * second filter, so its rail needs no equivalent budget.
 */
export const VENUE_RAIL_FETCH_LIMIT = SHOW_RAIL_ROW_CAP * 2 + 1

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
  /**
   * The see-all bracket's accessible name.
   *
   * Composed HERE rather than derived from `title` at the call site: "[See
   * all]" means nothing read out of its heading, and a screen reader reaching
   * the bracket has usually left the heading behind — but reconstructing the
   * name by string-surgery on `title` would make the heading's separator a
   * format two modules have to agree on silently. Changing `SECTION / QUALIFIER`
   * to any other separator would then quietly corrupt every rail's accessible
   * name with nothing to catch it.
   */
  seeAllLabel: string
  /**
   * Whether this rail reserves a room column at all.
   *
   * A per-RAIL question, not a per-row one: the also-tonight rail names a room
   * on every row and must keep the column even for a row missing its venue
   * name, while the venue rail has no room column because its room is the
   * heading. Deriving it from the rows would collapse the column on a rail
   * whose every row happened to lack a name.
   */
  hasRoomColumn: boolean
  /** Non-empty, and already rendered to primitives. */
  rows: RailRowData[]
  /** Where "see all" goes, or null when it must not be offered. */
  seeAllHref: string | null
}

/** The separator between a rail heading's SECTION and its QUALIFIER. */
const RAIL_TITLE_SEPARATOR = ' / '

/**
 * The rows the also-tonight rail will draw — the ONE definition of that set.
 *
 * Everything that needs to know what the left rail shows goes through here:
 * the rail itself, and the id set the venue rail excludes. That is the real
 * invariant, and it is not "the two filters look alike" — it is "the ids the
 * venue rail suppresses are exactly the rows the reader can see in the left
 * column." Two look-alike `filter().slice()` chains cannot hold that: the
 * moment either grows a rule the other does not — PSY-1969 is filed to DECIDE
 * whether to drop shows that already started, and dropping them is one of its
 * options — the venue rail starts hiding a bill that appears nowhere on the
 * page, and silently.
 *
 * The subject-show exclusion here is belt-and-braces: the endpoint already
 * documents that it excludes the subject show, and this is the boundary where
 * that promise arrives from another process. A show listed in its own "also
 * tonight" rail is the most visible way this feature can be wrong.
 */
function listableAlsoTonight(
  rail: ShowAlsoTonightResponse | undefined,
  currentShowId: number
): { drawn: AlsoTonightShow[]; listableCount: number } {
  // `shows` is typed nullable by the generator even though the API always
  // emits an array — the same accommodation `dayShows` makes.
  const listable = (rail?.shows ?? []).filter(
    show => show.id !== currentShowId
  )
  return { drawn: listable.slice(0, SHOW_RAIL_ROW_CAP), listableCount: listable.length }
}

/**
 * The scope the also-tonight rail is about, as parts: the night, then the city.
 *
 * Composed once and joined two ways — `·` for the heading, `,` for the
 * bracket's accessible name. Keeping the PARTS shared rather than the string
 * is what stops the two drifting: a scope that later needs the state to
 * disambiguate Portland OR from Portland ME is added here, and both consumers
 * get it.
 */
function alsoTonightScopeParts(rail: ShowAlsoTonightResponse): string[] {
  return [alsoTonightQualifier(rail), rail.city].filter(
    (part): part is string => Boolean(part)
  )
}

/**
 * The also-tonight rail, or null when there is nothing to head.
 *
 * The truncation question is answered from the same pass that produced the
 * rows, so the rows drawn and the claim that more exist can never disagree.
 */
export function buildAlsoTonightRail(
  rail: ShowAlsoTonightResponse | undefined,
  currentShowId: number
): ShowRail | null {
  if (!rail) return null

  const { drawn, listableCount } = listableAlsoTonight(rail, currentShowId)
  if (drawn.length === 0) return null

  // Two independent sources of truncation: the backend's own cap (`has_more`,
  // which compares against the whole night rather than against this rail), and
  // this rail's cap of three. Either one hiding a show is a reason to offer
  // the full night.
  const hasMore = rail.has_more || listableCount > drawn.length
  const scope = alsoTonightScopeParts(rail)

  return {
    title: alsoTonightRailTitle(rail),
    seeAllLabel:
      scope.length > 0
        ? `See every show ${scope.join(', ')}`
        : 'See every show that night',
    hasRoomColumn: true,
    rows: drawn.map(show => alsoTonightRow(show, rail.timezone)),
    seeAllHref: hasMore ? alsoTonightSeeAllHref(rail) : null,
  }
}

/**
 * The show ids the also-tonight rail draws, for the venue rail to exclude.
 *
 * Reads the same `listableAlsoTonight` the rail itself is built from, so the
 * two cannot describe different sets. Not read back off the built rail because
 * `ShowRail.rows` are primitives with no ids on them by design, and giving
 * them ids purely so a sibling could read them would put payload back into a
 * presentational type.
 */
export function alsoTonightDrawnIds(
  rail: ShowAlsoTonightResponse | undefined,
  currentShowId: number
): ReadonlySet<number> {
  return new Set(
    listableAlsoTonight(rail, currentShowId).drawn.map(show => show.id)
  )
}

/**
 * The more-at-venue rail, or null when the room has no other dates.
 *
 * The truncation question is "does the venue page hold a show this rail did
 * not draw", and answering it means reconciling two different populations.
 * `total` counts the venue's APPROVED UPCOMING shows. `drawn` is this rail's
 * three, minus everything it deliberately withheld — which is TWO things, not
 * one: the show being read, and any row the also-tonight rail already drew.
 *
 * So `withheld` counts what was actually dropped FROM THE FETCHED PAGE, rather
 * than assuming a fixed adjustment. Both populations matter and neither is
 * constant:
 *
 *  - The subject show is in `total` only when it is itself approved and
 *    upcoming. A PAST show — and most show pages become past ones — is absent
 *    from `total` entirely. An unconditional +1 therefore withheld "see all"
 *    from exactly the case it exists for: a past show at a room with four
 *    upcoming dates draws three, hides one, and `4 > 4` is false.
 *  - De-duplicated rows ARE in `total` but are already on screen in the other
 *    column, so counting them as hidden would offer a see-all that reveals
 *    nothing new.
 *
 * `withheld` can never over-count: `fetched` and `total` come from the same
 * query under the same filter, so anything counted here is by construction in
 * `total`. It can under-count only if the subject is upcoming yet sorted
 * beyond the fetched page — which requires `total` to exceed the page size,
 * where `hasMore` is already true by a wide margin.
 *
 * Note this rail's `[See all]` and `ShowVenueModule`'s `More at {venue} →`
 * point at the same venue page and sit about a screen apart. The mock draws
 * the module's link but NO bracket on either rail (node `1241:7` holds only a
 * heading and rows), so this one is an addition the ticket permits rather than
 * something the mock requires — it exists because a rail that truncates owes
 * the reader a way to the rest. If the duplication is ever cut, that is a
 * design call, not a cleanup.
 */
export function buildMoreAtVenueRail(
  venue: VenueResponse | undefined,
  shows: VenueShow[] | undefined,
  total: number | undefined,
  currentShowId: number,
  /**
   * Show ids the also-tonight rail has already drawn.
   *
   * The two rails overlap on exactly one population: another show at THIS room
   * on THIS night — an early/late set, or a second stage. Both queries return
   * it (the also-tonight endpoint excludes only the subject show, and the
   * venue's "upcoming" window includes tonight), and neither rail can see the
   * other's output. Printing the same bill twice side by side is the most
   * visible way a discovery row can look broken, so the venue rail — the one
   * whose heading already names the room — yields.
   */
  alreadyDrawn: ReadonlySet<number> = new Set()
): ShowRail | null {
  if (!venue) return null

  const fetched = shows ?? []
  const drawn = fetched
    .filter(show => show.id !== currentShowId && !alreadyDrawn.has(show.id))
    .slice(0, SHOW_RAIL_ROW_CAP)
  if (drawn.length === 0) return null

  // Everything this rail deliberately withheld has to come off `total` before
  // asking whether the venue page still holds something new: the subject show
  // (when it was in the page at all) and any row the other rail already drew.
  // Counting them as "hidden" would offer a see-all whose destination shows
  // the reader nothing they have not already seen on this screen.
  const withheld = fetched.filter(
    show => show.id === currentShowId || alreadyDrawn.has(show.id)
  ).length
  const hasMore = (total ?? drawn.length) > drawn.length + withheld

  return {
    title: `More at${RAIL_TITLE_SEPARATOR}${venue.name}`,
    seeAllLabel: `See every upcoming show at ${venue.name}`,
    hasRoomColumn: false,
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
  const parts = alsoTonightScopeParts(rail)
  // Both halves absent means the payload had neither a readable date nor a
  // scene — which is also the case where it carries no rows, so this heading
  // is unreachable in practice. It degrades to the bare section name rather
  // than to a dangling separator, and `buildAlsoTonightRail` is what actually
  // prevents an empty rail from being headed at all.
  if (parts.length === 0) return 'Also'
  return `Also${RAIL_TITLE_SEPARATOR}${parts.join(' · ')}`
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
 * This is `sceneWeek.showHref` widened by one degree: that one is typed to
 * `SceneWeekShow`, which requires `starts_at`, and `VenueShow` does not carry
 * it — so the venue rail cannot call it. Rather than let the two rails address
 * shows through two different rules, both go through this structural version.
 * The rule itself must stay identical to its cousin's: an empty slug is a
 * modeled case here, and `/shows/` resolves to the INDEX rather than 404ing
 * (PSY-1754), so the id fallback is load-bearing, not merely defensive.
 */
function railShowHref(show: { slug?: string | null; id: number }): string {
  return show.slug ? `/shows/${show.slug}` : `/shows/${show.id}`
}

/**
 * The trailing figure column: a status token when the show has one, else the
 * price, else nothing.
 *
 * Status SUPERSEDES price, which is how the mock sets it (`SOLD OUT` where its
 * neighbours carry `$45` / `$32`): a ticket that cannot be bought has no useful
 * price, and a cancelled one has none at all. Cancelled outranks sold out —
 * a called-off show's ticket status stopped mattering.
 *
 * Uppercased by the renderer's column, so `Free` reaches the mock's `FREE`
 * without forking `formatPrice`, which serves the whole site.
 */
function railFigure(show: {
  is_cancelled: boolean
  is_sold_out: boolean
  price?: number | null
  door_price?: number | null
}): { figure: string | null; figureLabel: string | null } {
  if (show.is_cancelled) return { figure: 'Cancelled', figureLabel: null }
  if (show.is_sold_out) return { figure: 'Sold out', figureLabel: null }
  const price = showPriceLabel(show)
  return { figure: price?.text ?? null, figureLabel: price?.title ?? null }
}

/**
 * A rail row's bill as ONE line of text.
 *
 * Bill-first, like `sceneWeek.showDisplayTitle` and unlike
 * `lib/utils/showDisplayTitle` (which is title-first): these rails list who is
 * playing, and a promoter's event title is the fallback, not the headline.
 *
 * `sceneWeek.showDisplayTitle` is the direct cousin and is deliberately NOT
 * called, for two reasons that compound. It takes a `SceneWeekShow`, and
 * `VenueShow` carries `artists` rather than `artist_names`, so it could serve
 * only one of the two rails. And it joins with `', '` where the LOCKED MOCK
 * bills these rows with `' + '` (`Dehd + Lifeguard`, `Waxahatchee + Tim
 * Heidecker`) — a real register difference for this surface, not an accidental
 * copy. A rail matching the scene views exactly while its neighbour diverged
 * would be the worse outcome.
 *
 * Also trimmed, which the cousin is not: a whitespace-only name or title is
 * truthy and would render an invisible, unclickable label.
 */
const RAIL_BILL_SEPARATOR = ' + '

function railBillLine(
  names: Array<string | null | undefined>,
  title?: string | null
): string {
  const billed = names
    .map(name => name?.trim())
    .filter((name): name is string => Boolean(name))
  if (billed.length > 0) return billed.join(RAIL_BILL_SEPARATOR)
  return title?.trim() || 'Live music'
}

/**
 * A rail row's lead column when it is a DATE rather than a time.
 *
 * Shape-checked for the same reason `startInstant` guards `starts_at`: a type
 * is not a runtime guarantee across two independently deployed services, and
 * `formatInTimezone` routes through `toLocaleString`, which does not throw on a
 * bad instant — it returns the literal string `Invalid Date`, which the lead
 * column would then uppercase and print. Null keeps the `RailRow.lead` contract
 * honest and leaves the column reserved but blank.
 */
function railShowDate(
  eventDate: string | null | undefined,
  state: string | null | undefined,
  timezone: string | null | undefined
): string | null {
  if (typeof eventDate !== 'string' || !Number.isFinite(Date.parse(eventDate))) {
    return null
  }

  const label = formatShowMonthDayPadded(eventDate, state, timezone)

  // The year joins the cell when it is not the current one, reaching the same
  // outcome as the also-tonight heading by a DIFFERENT mechanism — worth
  // stating, because the obvious shortcut here is wrong. The heading reads its
  // year off `rail.date`, a bare `YYYY-MM-DD` calendar string, so its year and
  // its chip cannot disagree. This cell has an INSTANT, and the month/day is
  // resolved on the venue's clock, so the year must be too: `new Date(x)
  // .getFullYear()` reads the RUNTIME's zone, and a Chicago show at 20:00 on
  // Dec 31 would print `DEC 31 27` to a reader in Berlin — a date that does not
  // exist, on one of the year's most heavily booked nights.
  //
  // Why the year is here at all: on an archive show page the two rails sit in
  // different years. The left one is headed `Also / Thu Aug 15, 2019` while
  // this one lists the room's UPCOMING dates, so a bare `AUG 15` beside that
  // heading reads as 2019 — inverting the one fact the row exists to convey.
  const year = showYearInZone(eventDate, state, timezone)
  const currentYear = showYearInZone(new Date().toISOString(), state, timezone)
  if (year === currentYear) return label
  // Apostrophe, not a bare `27`: `SEP 04 27` reads as a date range in an
  // uppercase mono column.
  return `${label} '${String(year).slice(-2)}`
}

/**
 * One rendered ledger row: primitives only, no payload shape.
 *
 * The two rails carry two different wire types, and this is the seam where
 * that stops mattering. Keeping it primitives-only is what lets ONE row
 * renderer serve both without a mode flag. The alternative — a row component
 * that understands both payload shapes — is the shape `CompactShowRow` has
 * grown into: six optional props (`isPastShow`, `showDetailsLink`,
 * `showVenueLine`, `venue`, `primaryLine`, `secondaryArtists`) whose valid
 * combinations are not stated anywhere.
 */
export interface RailRowData {
  href: string
  /** Time or date. Null when the payload carries no usable instant. */
  lead: string | null
  title: string
  /** Struck through, and the figure column says CANCELLED. */
  isCancelled: boolean
  /**
   * The room. Null on the venue rail, whose room is named in the heading, and
   * on an also-tonight row whose payload has no venue name.
   *
   * Null means RESERVED-BUT-EMPTY, not dropped: these are ledger columns, and a
   * row that omits a cell shifts every cell after it, which is exactly what
   * stops a column of figures from being readable as a column.
   */
  room: string | null
  /**
   * The trailing figure: a price, or the status token that SUPERSEDES it.
   *
   * One column, not two. The mock gives a sold-out row `SOLD OUT` where a
   * priced row has `$45` — a ticket you cannot buy has no useful price, and
   * printing both is how the two facts start arguing.
   */
  figure: string | null
  /**
   * The spelled-out reading of `figure` when it is a SPLIT PRICE, for a screen
   * reader; null otherwise.
   *
   * `$35/$40` is announced as "thirty five slash forty" — punctuation, for a
   * fact about money — so the pair carries the same description the
   * `ShowPrice` component attaches on every other list surface. The rails
   * cannot use that component (this column also holds `SOLD OUT` and
   * `CANCELLED`, which are not prices), so the label rides on the row instead.
   *
   * Null for a lone price, which has nothing to be told apart from, and for a
   * status token, whose text already reads correctly.
   */
  figureLabel: string | null
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
 *  - `8:00 PM`, not the mock's `8PM`. `formatShowTime` is the site's single
 *    time format and forking it here would put two renderings on one page; a
 *    7:30 door also cannot be said as "7PM". NOT settled — a divergence from a
 *    locked mock with a real width cost in these columns, so PSY-1970 holds the
 *    design call rather than leaving it decided by a comment.
 *
 *    The PRICE half of that ticket is settled and no longer diverges: the whole
 *    site moved to the mock's compact form (PSY-1962), so this column now reads
 *    `$15` and `FREE` through the shared `formatPrice`, not through a fork.
 *    What is left for PSY-1970 is the time register alone.
 */
export function alsoTonightRow(
  show: AlsoTonightShow,
  railTimezone: string
): RailRowData {
  return {
    href: railShowHref(show),
    lead: formatShowStartTime(show, railTimezone),
    title: railBillLine(show.artist_names ?? [], show.title),
    isCancelled: show.is_cancelled,
    room: show.venue_name?.trim() || null,
    ...railFigure(show),
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
export function moreAtVenueRow(show: VenueShow, venue: VenueResponse): RailRowData {
  return {
    href: railShowHref(show),
    lead: railShowDate(show.event_date, show.state ?? venue.state, venue.timezone),
    title: railBillLine(show.artists.map(artist => artist.name), show.title),
    isCancelled: show.is_cancelled,
    // No room column: every row on this rail is at the room in the heading.
    room: null,
    ...railFigure(show),
  }
}
