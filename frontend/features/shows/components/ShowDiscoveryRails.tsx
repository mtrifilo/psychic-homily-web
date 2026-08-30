'use client'

import Link from 'next/link'
import { BracketLink, SectionHeader } from '@/components/shared'
// Deep import, not the `features/scenes/components` barrel: that barrel is
// root-reachable, and merely LISTING an export there puts it in the one client
// chunk every route loads eagerly (PSY-1772, guarded by
// features/sharedChunkBarrelGuard.test.ts). Importing it is the same hazard in
// reverse — it would pull the whole scenes component graph into the show route
// for one small badge.
import { ShowStatusBadge } from '@/features/scenes/components/sceneChrome'
import { useVenueShows } from '@/features/venues/hooks/useVenues'
import { useShowAlsoTonight } from '../hooks/useShows'
import {
  buildAlsoTonightRail,
  buildMoreAtVenueRail,
  VENUE_RAIL_FETCH_LIMIT,
  type RailRow as RailRowData,
  type ShowRail,
} from '../showRails'
import type { ShowResponse } from '../types'

/**
 * The show page's discovery rails: `ALSO / TONIGHT · CHICAGO` beside
 * `MORE AT / SALT SHED`, the locked mock's two-column row above the footer.
 *
 * Both rails are GRAPH-DERIVED and say so in their headings — "what else is on
 * in this metro that night" and "what else this room has booked" are questions
 * with answers, not a personalization surface. Neither is ever labelled "you
 * may also like", and neither ranks: the left rail is the night in clock
 * order, the right rail is the room's calendar in date order.
 *
 * This file is markup and hook wiring only. Which rows survive, what the
 * headings say, and whether a "see all" is offered all live in `showRails.ts`,
 * so both rails answer those questions in one place and under one test file —
 * the rules are close cousins and the subtle ones (the venue rail's
 * off-by-one, the tonight-vs-date register) are exactly the ones that rot when
 * they are only reachable through a rendered component.
 *
 * Each rail resolves to null when it has nothing to say, and the row itself
 * disappears when both do. That decision has to be made HERE, before the grid
 * renders: a rail returning null from inside the grid is still a truthy
 * element, so the row would keep its bottom margin over two invisible columns
 * — the same slot-reservation trap `ShowHeader`'s actions gate documents.
 *
 * Pending and errored requests draw nothing, deliberately. The rails are
 * supplementary and sit at the foot of the page, where a late arrival costs
 * nothing, and a spinner would advertise a wait for something the reader did
 * not ask for.
 */
export function ShowDiscoveryRails({ show }: { show: ShowResponse }) {
  const venue = show.venues[0]

  // Both hooks run unconditionally. `useVenueShows` is DISABLED rather than
  // skipped for a venue-less show, because a conditional hook would change the
  // hook order between renders.
  const { data: alsoTonightPayload } = useShowAlsoTonight(show.slug || show.id)
  const { data: venueShows } = useVenueShows({
    venueId: venue?.id ?? 0,
    timeFilter: 'upcoming',
    limit: VENUE_RAIL_FETCH_LIMIT,
    enabled: Boolean(venue),
  })

  const alsoTonight = buildAlsoTonightRail(alsoTonightPayload, show.id)
  const moreAtVenue = buildMoreAtVenueRail(
    venue,
    venueShows?.shows,
    venueShows?.total,
    show.id
  )

  if (!alsoTonight && !moreAtVenue) return null

  return (
    <div
      data-testid="show-discovery-rails"
      className="mb-8 grid grid-cols-1 gap-x-10 gap-y-6 md:grid-cols-2"
    >
      {/* Each column renders or does not; the surviving rail keeps its own
          measure rather than stretching across the row, so a page with one
          rail and a page with two set the same bills to the same width. */}
      {alsoTonight && (
        <Rail rail={alsoTonight} testId="also-tonight-rail" />
      )}
      {moreAtVenue && (
        <Rail rail={moreAtVenue} testId="more-at-venue-rail" />
      )}
    </div>
  )
}

/**
 * One rail: a stacked `SECTION / QUALIFIER` heading, an optional bare bracket,
 * and the ledger.
 *
 * Both rails render through this — they differ only in the data `showRails.ts`
 * hands over, which is what keeps their type and rhythm identical while they
 * sit side by side.
 */
function Rail({ rail, testId }: { rail: ShowRail; testId: string }) {
  return (
    <section data-testid={testId}>
      <SectionHeader
        title={rail.title}
        as="h2"
        size="md"
        // Bare bracket on the section header, never another verb in the entity
        // action row (pattern_syndication_affordance_placement). `seeAllHref`
        // is already null unless something is genuinely hidden AND the
        // destination is honest, so the guard here is just presence.
        action={
          rail.seeAllHref ? (
            <BracketLink
              label="See all"
              href={rail.seeAllHref}
              // "[See all]" means nothing read out of its heading, and a
              // screen reader reaching the bracket has usually left the
              // heading behind.
              ariaLabel={`See all: ${rail.title.replace(' / ', ', ')}`}
            />
          ) : undefined
        }
      />
      <ul>
        {rail.rows.map(row => (
          <RailRow key={row.href} row={row} />
        ))}
      </ul>
    </section>
  )
}

/**
 * The ledger row both rails draw: a lead column, the bill, right-aligned mono
 * facts, hairline beneath.
 *
 * It takes primitives, not a payload, which is what lets one renderer serve
 * two different wire types without a mode flag — and what keeps the hairline,
 * the hover target, the cancelled treatment and the column rhythm from
 * drifting apart between two rails sitting side by side, where any drift is
 * visible at a glance.
 */
function RailRow({ row }: { row: RailRowData }) {
  return (
    <li className="border-b border-border/40 last:border-0">
      <Link
        href={row.href}
        className="group flex flex-col gap-0.5 py-1.5 transition-colors hover:bg-muted/40 sm:flex-row sm:items-baseline sm:gap-3"
      >
        {/* Reserved even when the instant is unusable, so one undated row does
            not pull every bill beneath it out of line. */}
        <span className="shrink-0 font-mono text-xs uppercase tabular-nums text-muted-foreground sm:w-16">
          {row.lead ?? ''}
        </span>
        <span className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5">
          <span
            className={`text-sm group-hover:underline ${
              row.isCancelled ? 'text-muted-foreground line-through' : ''
            }`}
          >
            {row.title}
          </span>
          {row.isCancelled && <ShowStatusBadge label="CANCELLED" />}
          {!row.isCancelled && row.isSoldOut && (
            <ShowStatusBadge label="SOLD OUT" />
          )}
        </span>
        <span className="hidden flex-1 sm:block" aria-hidden="true" />
        {/* Index-keyed: these are stateless strings whose order and count are
            fixed per rail, so a positional remount repaints identically (the
            same rule `MiddotSegments` states for its fallback). */}
        {row.facts.map((fact, index) => (
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
