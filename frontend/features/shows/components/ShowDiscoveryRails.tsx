'use client'

import { useId } from 'react'
import Link from 'next/link'
import { BracketLink, SectionHeader } from '@/components/shared'
import { useVenueShows } from '@/features/venues/hooks/useVenues'
import { useShowAlsoTonight } from '../hooks/useShows'
import {
  alsoTonightDrawnIds,
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
 * Pending and errored requests draw nothing. That is deliberate for the error
 * case — an empty night is a 200 with an empty list, so a failed request has
 * nothing honest to say and a spinner would advertise a wait for something the
 * reader did not ask for.
 *
 * It is an ACCEPTED COST for the pending case, not a free one, and the cost is
 * layout shift: these rails are not the last thing on the page. The provenance
 * byline sits below them, and `RevisionHistory` + `CommentThread` sit below
 * that, so rows arriving late push all three down — worst on a phone, where the
 * columns stack. The page already inserts four client-fetched blocks above the
 * comment thread (collections, field notes, tags, the byline's revision
 * count), so this joins an existing pattern rather than starting one; it does
 * make it materially bigger. Removing the class properly means server-fetching
 * both rails into the route's prefetch so they are in the HTML, which is
 * PSY-1967 and is deliberately not attempted here.
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

  // ORDER MATTERS: the also-tonight rail is built first and its rows are handed
  // to the venue rail as an exclusion set. The two overlap on one population —
  // another show at this room on this night — and without this the same bill
  // renders in both columns at once. The venue rail yields because its heading
  // already names the room.
  const alsoTonight = buildAlsoTonightRail(alsoTonightPayload, show.id)
  const moreAtVenue = buildMoreAtVenueRail(
    venue,
    venueShows?.shows,
    venueShows?.total,
    show.id,
    alsoTonightDrawnIds(alsoTonightPayload, show.id)
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
  // A `section` with no accessible name is a generic element, not a `region`,
  // so landmark navigation could not tell the two rails apart — they would be
  // reachable only by heading. `ShowSubmissionsConsole` names its regions the
  // same way, from their own heading.
  //
  // `useId`, NOT the testId: a `data-testid` is a testing affordance, and
  // hanging an accessibility contract off it means a test refactor that renames
  // or strips testids silently breaks the landmark with nothing to catch it.
  const headingId = useId()

  return (
    <section data-testid={testId} aria-labelledby={headingId}>
      <SectionHeader
        title={rail.title}
        as="h2"
        size="md"
        headingProps={{ id: headingId }}
        // Bare bracket on the section header, never another verb in the entity
        // action row (pattern_syndication_affordance_placement). `seeAllHref`
        // is already null unless something is genuinely hidden AND the
        // destination is honest, so the guard here is just presence.
        action={
          rail.seeAllHref ? (
            <BracketLink
              label="See all"
              href={rail.seeAllHref}
              ariaLabel={rail.seeAllLabel}
            />
          ) : undefined
        }
      />
      <ul>
        {rail.rows.map(row => (
          <RailRow key={row.href} row={row} hasRoomColumn={rail.hasRoomColumn} />
        ))}
      </ul>
    </section>
  )
}

/**
 * The ledger row both rails draw: lead, bill, room, figure — fixed columns,
 * hairline beneath.
 *
 * It takes primitives, not a payload, which is what lets one renderer serve
 * two different wire types without a mode flag — and what keeps the hairline,
 * the hover target, the cancelled treatment and the column rhythm from
 * drifting apart between two rails sitting side by side, where any drift is
 * visible at a glance.
 *
 * Deliberately not `ShowBill`, which links each act separately: a rail row is
 * ONE link to the show, so nested anchors would be invalid markup and an
 * unusable tab order. That means the cancelled treatment has a second home —
 * if `ShowBill`'s strike-through or its status copy changes, change it here
 * too.
 */
function RailRow({
  row,
  hasRoomColumn,
}: {
  row: RailRowData
  hasRoomColumn: boolean
}) {
  return (
    <li className="border-b border-border/40 last:border-0">
      <Link
        href={row.href}
        className="group flex flex-col gap-0.5 py-1.5 transition-colors hover:bg-muted/40 sm:flex-row sm:items-baseline sm:gap-3"
      >
        {/* Every cell below is a fixed-width COLUMN, and an absent value leaves
            its column reserved rather than collapsing it. That is the whole
            difference between the mock's ledger and a list of facts pushed to
            the right margin: the figures only read as a column when they start
            at the same x on every row, which is also the only thing that makes
            `tabular-nums` worth anything here. Widths are `sm:` so the columns
            dissolve into a stack on a phone, where they cannot fit. */}
        <span className="shrink-0 font-mono text-xs uppercase tabular-nums text-muted-foreground sm:w-16">
          {row.lead ?? ''}
        </span>
        <span
          className={`min-w-0 flex-1 truncate text-sm group-hover:underline ${
            row.isCancelled ? 'text-muted-foreground line-through' : ''
          }`}
        >
          {row.title}
        </span>
        {hasRoomColumn && (
          <span className="shrink-0 truncate font-mono text-xs text-muted-foreground sm:w-40">
            {row.room ?? ''}
          </span>
        )}
        {/* Uppercased here rather than in the policy, so `Free` reaches the
            mock's `FREE` and `Sold out` reaches `SOLD OUT` without forking
            `formatPrice`, which serves the whole site. */}
        <span className="shrink-0 font-mono text-xs uppercase tabular-nums text-muted-foreground sm:w-20">
          {row.figure ?? ''}
        </span>
      </Link>
    </li>
  )
}
