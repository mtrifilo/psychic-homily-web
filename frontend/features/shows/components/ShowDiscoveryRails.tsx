'use client'

import Link from 'next/link'
import { BracketLink, SectionHeader } from '@/components/shared'
// Deep imports, not the `features/scenes/components` barrel: that barrel is
// root-reachable, and merely LISTING an export there puts it in the one client
// chunk every route loads eagerly (PSY-1772, guarded by
// features/sharedChunkBarrelGuard.test.ts). Importing it is the same hazard in
// reverse — it would pull the whole scenes component graph into the show route
// for one small badge.
import { ShowStatusBadge } from '@/features/scenes/components/sceneChrome'
import {
  formatShowPrice,
  formatShowStartTime,
} from '@/features/scenes/sceneDay'
import { showDisplayTitle, showHref } from '@/features/scenes/sceneWeek'
import { useVenueShows } from '@/features/venues/hooks/useVenues'
import type { VenueShow } from '@/features/venues/types'
import { formatPrice, formatShowDayMonth } from '@/lib/utils/formatters'
import { useShowAlsoTonight } from '../hooks/useShows'
import {
  alsoTonightHasMore,
  alsoTonightRailRows,
  alsoTonightRailTitle,
  alsoTonightSeeAllHref,
  moreAtVenueRailRows,
  VENUE_RAIL_FETCH_LIMIT,
  type AlsoTonightShow,
  type ShowAlsoTonightResponse,
} from '../showRails'
import type { ShowResponse, VenueResponse } from '../types'

/**
 * The show page's discovery rails: `ALSO / TONIGHT · CHICAGO` beside
 * `MORE AT / SALT SHED`, the locked mock's two-column row above the footer.
 *
 * Both rails are GRAPH-DERIVED and say so in their headings — "what else is on
 * in this metro that night" and "what else this room has booked" are questions
 * with answers, not a personalization surface. Neither is ever labelled "you
 * may also like", and neither ranks: the left rail is the night in clock order,
 * the right rail is the room's calendar in date order.
 *
 * Both requests and both row policies are resolved HERE, and the frame below is
 * only reached once each rail knows whether it has rows. That ordering is the
 * whole point of the split: a rail deciding its own emptiness inside the frame
 * still returns a truthy element, so the row would keep its bottom margin over
 * two invisible columns — the same slot-reservation trap `ShowHeader`'s actions
 * gate documents.
 *
 * Pending and errored requests draw nothing, deliberately. The rails are
 * supplementary and sit at the foot of the page, where a late arrival costs
 * nothing, and a spinner would advertise a wait for something the reader did
 * not ask for.
 */
export function ShowDiscoveryRails({ show }: { show: ShowResponse }) {
  const venue = show.venues[0]

  // Both hooks run unconditionally; `useVenueShows` is disabled rather than
  // skipped, because a venue-less show must not change the hook order.
  const { data: alsoTonight } = useShowAlsoTonight(show.slug || show.id)
  const { data: venueShows } = useVenueShows({
    venueId: venue?.id ?? 0,
    timeFilter: 'upcoming',
    limit: VENUE_RAIL_FETCH_LIMIT,
    enabled: Boolean(venue),
  })

  const alsoTonightRows = alsoTonightRailRows(alsoTonight, show.id)
  const venueRows = moreAtVenueRailRows(venueShows?.shows, show.id)

  if (alsoTonightRows.length === 0 && venueRows.length === 0) return null

  return (
    <div
      data-testid="show-discovery-rails"
      className="mb-8 grid grid-cols-1 gap-x-10 gap-y-6 md:grid-cols-2"
    >
      {/* Each column renders or does not; the surviving rail keeps its own
          measure either way rather than stretching across the row, so a page
          with one rail and a page with two set the same bills to the same
          width. */}
      {alsoTonight && alsoTonightRows.length > 0 && (
        <AlsoTonightRail
          rail={alsoTonight}
          rows={alsoTonightRows}
          currentShowId={show.id}
        />
      )}
      {venue && venueRows.length > 0 && (
        <MoreAtVenueRail
          venue={venue}
          rows={venueRows}
          upcomingTotal={venueShows?.total ?? venueRows.length}
        />
      )}
    </div>
  )
}

/** Other shows in this show's metro on this show's own night. */
function AlsoTonightRail({
  rail,
  rows,
  currentShowId,
}: {
  rail: ShowAlsoTonightResponse
  rows: AlsoTonightShow[]
  currentShowId: number
}) {
  const seeAllHref = alsoTonightSeeAllHref(rail)
  const hasMore = alsoTonightHasMore(rail, rows.length, currentShowId)

  return (
    <section data-testid="also-tonight-rail">
      <SectionHeader
        title={alsoTonightRailTitle(rail)}
        as="h2"
        size="md"
        // Bare bracket on the section header, never another verb in the entity
        // action row (pattern_syndication_affordance_placement). It appears
        // ONLY when the night actually holds more than the rows drawn — a
        // "see all" over a complete list promises a page with nothing new on
        // it. The guard is hoisted to this call site so `SectionHeader` never
        // receives an element that renders null and reserves its slot.
        action={
          seeAllHref && hasMore ? (
            <BracketLink
              label="See all"
              href={seeAllHref}
              // "[See all]" means nothing read out of its heading, and a
              // screen reader reaching the bracket has usually left the
              // heading behind.
              ariaLabel={`See every show in ${rail.city || 'this scene'} on ${rail.date}`}
            />
          ) : undefined
        }
      />
      <ul>
        {rows.map(row => (
          <AlsoTonightRow key={row.id} show={row} railTimezone={rail.timezone} />
        ))}
      </ul>
    </section>
  )
}

/**
 * One also-tonight row: time, bill, room, price.
 *
 * Time leads because a single night is read as a schedule — the reader has
 * already chosen the date and is choosing an hour. Same argument, and the same
 * column order, as the scene day view's row.
 *
 * No age column, which the mock draws: `SceneShowSummary` carries no age
 * requirement, and this rail is not worth a second request per row to invent
 * one. The venue module above states the age rule for the show being read,
 * which is the one a reader on this page is deciding about.
 */
function AlsoTonightRow({
  show,
  railTimezone,
}: {
  show: AlsoTonightShow
  railTimezone: string
}) {
  // Venue-local, never the reader's. `formatShowStartTime` prefers the row's
  // own `venue_timezone` and falls back to the zone the BACKEND computed this
  // night's window in — the scene's modal clock, the same one that decided this
  // show belongs to this night. Falling back to the viewer's device instead is
  // how a listed time comes to disagree with the heading above it: a reader in
  // Berlin would see a Chicago night set in CEST.
  const time = formatShowStartTime(show, railTimezone)

  return (
    <RailRow
      href={showHref(show)}
      lead={time}
      title={showDisplayTitle(show)}
      isCancelled={show.is_cancelled}
      isSoldOut={show.is_sold_out}
      facts={[show.venue_name, formatShowPrice(show)]}
    />
  )
}

/** The venue's next shows, the one being read excluded. */
function MoreAtVenueRail({
  venue,
  rows,
  upcomingTotal,
}: {
  venue: VenueResponse
  rows: VenueShow[]
  /** Every upcoming show at this room, INCLUDING the one being viewed. */
  upcomingTotal: number
}) {
  // `total` counts the show being read too, so it has to go back on the drawn
  // side before asking whether anything is hidden. Without that the rail
  // offers "see all" to a venue page holding exactly the rows already on
  // screen. The `+ 1` is unconditional because the rows were filtered by id
  // and this venue is the show's own — its upcoming list contains the subject
  // show whenever the subject show is upcoming, and when it is not, `total`
  // simply exceeds the cap by more and the comparison still holds.
  const hasMore = upcomingTotal > rows.length + 1

  return (
    <section data-testid="more-at-venue-rail">
      <SectionHeader
        title={`More at / ${venue.name}`}
        as="h2"
        size="md"
        // Guarded on the slug as well as on truncation: entity slugs are
        // nullable in this schema and an empty one resolves `/venues/` to the
        // INDEX rather than 404ing, so an unguarded href silently lands a
        // reader on the wrong page (PSY-1754).
        action={
          venue.slug && hasMore ? (
            <BracketLink
              label="See all"
              href={`/venues/${venue.slug}`}
              ariaLabel={`See every upcoming show at ${venue.name}`}
            />
          ) : undefined
        }
      />
      <ul>
        {rows.map(row => (
          <MoreAtVenueRow key={row.id} show={row} venue={venue} />
        ))}
      </ul>
    </section>
  )
}

/**
 * One more-at-venue row: date, bill, price.
 *
 * Date leads, not time: every row is at the same room, so the question this
 * rail answers is "when next". The room's name is likewise absent — it is in
 * the heading.
 */
function MoreAtVenueRow({
  show,
  venue,
}: {
  show: VenueShow
  venue: VenueResponse
}) {
  // A show's own state wins over the venue's, and the venue's resolved IANA
  // zone wins over the state map (PSY-986) — the same resolution order the
  // venue page's own table uses, so the two surfaces cannot print different
  // dates for one show.
  const state = show.state ?? venue.state

  return (
    <RailRow
      href={`/shows/${show.slug || show.id}`}
      lead={formatShowDayMonth(show.event_date, state, venue.timezone)}
      title={venueShowBill(show)}
      isCancelled={show.is_cancelled}
      isSoldOut={show.is_sold_out}
      facts={[typeof show.price === 'number' ? formatPrice(show.price) : null]}
    />
  )
}

/**
 * The bill as one line of text.
 *
 * Deliberately not `ShowBill`, which links each act separately: a rail row is
 * ONE link to the show, and nested interactive elements inside it would be
 * invalid markup and an unusable tab order. Mirrors the scene views'
 * `showDisplayTitle`, on the venue payload's shape.
 */
function venueShowBill(show: VenueShow): string {
  const names = show.artists.map(artist => artist.name).filter(Boolean)
  if (names.length > 0) return names.join(', ')
  return show.title || 'Live music'
}

/**
 * The ledger row both rails draw: a lead column, the bill, right-aligned mono
 * facts, hairline beneath.
 *
 * One renderer rather than two, so the hairline, the hover target, the
 * cancelled treatment and the column rhythm cannot drift apart between rails
 * that sit side by side, where any drift is visible at a glance.
 */
function RailRow({
  href,
  lead,
  title,
  isCancelled,
  isSoldOut,
  facts,
}: {
  href: string
  /** Time or date. Null when the payload carries no usable instant. */
  lead: string | null
  title: string
  isCancelled: boolean
  isSoldOut: boolean
  /** Right-hand facts in order; absent ones are dropped here, not by callers. */
  facts: Array<string | null | undefined>
}) {
  const shownFacts = facts.filter((fact): fact is string => Boolean(fact))

  return (
    <li className="border-b border-border/40 last:border-0">
      <Link
        href={href}
        className="group flex flex-col gap-0.5 py-1.5 transition-colors hover:bg-muted/40 sm:flex-row sm:items-baseline sm:gap-3"
      >
        {/* Reserved even when the instant is unusable, so one undated row does
            not pull every bill beneath it out of line. */}
        <span className="shrink-0 font-mono text-xs uppercase tabular-nums text-muted-foreground sm:w-16">
          {lead ?? ''}
        </span>
        <span className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5">
          <span
            className={`text-sm group-hover:underline ${
              isCancelled ? 'text-muted-foreground line-through' : ''
            }`}
          >
            {title}
          </span>
          {isCancelled && <ShowStatusBadge label="CANCELLED" />}
          {!isCancelled && isSoldOut && <ShowStatusBadge label="SOLD OUT" />}
        </span>
        <span className="hidden flex-1 sm:block" aria-hidden="true" />
        {/* Index-keyed: these are stateless strings whose order and count are
            fixed per rail, so a positional remount repaints identically (the
            same rule `MiddotSegments` states for its fallback). */}
        {shownFacts.map((fact, index) => (
          <span
            key={index}
            className="shrink-0 font-mono text-xs tabular-nums text-muted-foreground"
          >
            {fact}
          </span>
        ))}
      </Link>
    </li>
  )
}
