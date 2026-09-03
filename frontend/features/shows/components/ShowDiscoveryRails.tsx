'use client'

import { useId } from 'react'
import Link from 'next/link'
import { BracketLink, SectionHeader } from '@/components/shared'
import { useVenueShows } from '@/features/venues/hooks/useVenues'
import { useShowAlsoTonight } from '../hooks'
import {
  alsoTonightDrawnIds,
  buildAlsoTonightRail,
  buildMoreAtVenueRail,
  VENUE_RAIL_FETCH_LIMIT,
  type RailRowData,
  type ShowAlsoTonightResponse,
  type ShowRail,
} from '../showRails'
import type { ShowLifecycleState } from '@/lib/utils/showTiming'
import type { VenueShowsResponse } from '@/features/venues/types'
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
 * The PENDING case is not reached on a first paint: the route fetches both
 * rails on the server and seeds them here, so the rows are in the served HTML
 * and nothing below is pushed down when they arrive. That matters beyond
 * tidiness — the provenance byline, `RevisionHistory` and `CommentThread` all
 * sit under these rails, and `/shows/{slug}#comment-123` scrolls to a comment
 * once its own query resolves, so a rail inserting rows after that scroll would
 * take the targeted comment out from under the reader.
 *
 * A seed can still be absent (the server fetch failed, or was skipped), and
 * then these rails behave exactly as they did before it existed: nothing until
 * the client query resolves.
 */
export function ShowDiscoveryRails({
  show,
  lifecycle,
  now,
  initialAlsoTonight,
  initialVenueShows,
}: {
  show: ShowResponse
  lifecycle: ShowLifecycleState
  /**
   * The instant this page is being rendered at, read ONCE on the server (see
   * the show route) and threaded here for the same reason `lifecycle` is: this
   * component renders on the server and again on the hydrating client, and a
   * clock read on each side would let the two disagree about which of the
   * night's shows have started — reordering the rows under the reader and
   * failing hydration for the whole page.
   */
  now: Date
  /**
   * Rows the SERVER already fetched for exactly these two requests.
   *
   * Passed to the hooks as `initialData` rather than seeded into a query key on
   * the route, following the venue archive's precedent: the key is then built
   * in this component from this component's own arguments, so the URL and the
   * key cannot describe different requests. Seeding a key from the route would
   * mean restating `VENUE_RAIL_FETCH_LIMIT` there, and a stale copy of it would
   * register an entry the hook never reads — a silent miss that leaves the
   * layout shift in place with every test still green.
   */
  initialAlsoTonight?: ShowAlsoTonightResponse
  initialVenueShows?: VenueShowsResponse
}) {
  const venue = show.venues[0]

  // The two rails answer different questions, and only one survives the show.
  //
  // ALSO-TONIGHT is scoped to THIS show's night: on a past page it would offer
  // a reader other shows they equally cannot attend, under a heading naming a
  // date that has gone by. It is withheld, and not fetched.
  //
  // MORE-AT-VENUE queries the venue's UPCOMING window, so it is forward-looking
  // whatever the subject show's date: from an archive page it is the live "this
  // room is still putting on shows" thread, and it stays.
  const showsAlsoTonight = lifecycle !== 'past'

  // Both hooks run unconditionally. `useVenueShows` is DISABLED rather than
  // skipped for a venue-less show, because a conditional hook would change the
  // hook order between renders. `useShowAlsoTonight` is disabled the same way
  // on a past show.
  const { data: alsoTonightPayload } = useShowAlsoTonight(
    show.slug || show.id,
    showsAlsoTonight,
    // Withheld when this rail is not drawn at all: `initialData` on a DISABLED
    // query still populates its cache entry, which would leave a past show's
    // page holding a rail it refuses to render.
    showsAlsoTonight ? initialAlsoTonight : undefined
  )
  const { data: venueShows } = useVenueShows({
    venueId: venue?.id ?? 0,
    timeFilter: 'upcoming',
    limit: VENUE_RAIL_FETCH_LIMIT,
    enabled: Boolean(venue),
    initialData: venue ? initialVenueShows : undefined,
  })

  // ORDER MATTERS: the also-tonight rail is built first and its rows are handed
  // to the venue rail as an exclusion set. The two overlap on one population —
  // another show at this room on this night — and without this the same bill
  // renders in both columns at once. The venue rail yields because its heading
  // already names the room.
  const alsoTonight = showsAlsoTonight
    ? buildAlsoTonightRail(alsoTonightPayload, show.id, now)
    : null
  const moreAtVenue = buildMoreAtVenueRail(
    venue,
    venueShows?.shows,
    venueShows?.total,
    show.id,
    // Nothing to yield to when the other rail is not drawn: an exclusion set
    // built from a rail the reader cannot see would silently drop bills from
    // the only rail they can.
    alsoTonight
      ? alsoTonightDrawnIds(alsoTonightPayload, show.id, now)
      : undefined
  )

  if (!alsoTonight && !moreAtVenue) return null

  return (
    <div
      data-testid="show-discovery-rails"
      className="mb-8 grid grid-cols-1 gap-x-10 gap-y-6 lg:grid-cols-2"
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
  // reachable only by heading.
  //
  // The repo labels regions both ways: ~20 `<section aria-label>` (see
  // `GraphPanelShell`, whose landmark is a tested contract) and a handful of
  // `<section aria-labelledby>`. The closest precedent is `app/library/page.tsx`
  // — a `section` labelled by a per-instance heading id — and `aria-labelledby`
  // is the right half of that pair HERE because `showRails.ts` already composes
  // the heading text; an `aria-label` would restate it and the two would drift.
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
          <RailRow
            key={row.href}
            row={row}
            hasRoomColumn={rail.hasRoomColumn}
            hasAgeColumn={rail.hasAgeColumn}
            leadKind={rail.leadKind}
          />
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
  hasAgeColumn,
  leadKind,
}: {
  row: RailRowData
  hasRoomColumn: boolean
  hasAgeColumn: boolean
  leadKind: ShowRail['leadKind']
}) {
  // A clock time in the compact register is at most `10:30PM`; a date is at
  // most `SEP 04 '27`, which is three characters longer and needs the wider
  // reservation. Both are `text-xs` mono, so the difference is real width, not
  // a rounding.
  const leadWidth = leadKind === 'time' ? 'sm:w-14' : 'sm:w-20'

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
            `tabular-nums` worth anything here.

            The widths have to FIT the narrowest column the layout can hand
            them, which is not the 672px the mock is drawn at. The rails row
            goes two-up at `lg`, and `EntityDetailLayout` may also be carrying
            a 320px chart-rank sidebar, so the narrowest real rail column is
            ~300px (lg + sidebar). Reserving every cell at the mock's
            proportions overflows that outright and leaves the BILL — the one
            cell a reader is scanning for — as a bare ellipsis. So the two
            least load-bearing columns, room and age, hide in the horizontal
            band, and the bill keeps `flex-1` everywhere.

            Budget with the arithmetic, not by eye. The container caps at
            `max-w-6xl` (1152), so the narrowest real rail column is ~300px at
            `lg` with the sidebar, ~364px at `xl` with it, and ~540px at `xl`
            without. Gaps are 12px each. At `lg`, room and age are hidden:
            lead 56 + figure 64 + gaps 24 = 144 of 300, bill 156. At `xl` all
            five columns are up, and the room takes a SIXTH of the column
            rather than a fixed width so it scales with the space instead of
            eating a fixed bite out of the narrowest case — 56 + 61 + 40 + 64
            + 48 = 269 of 364, bill 95; and at 540, 56 + 90 + 40 + 64 + 48 =
            298, bill 242. The 95px case is the tightest the layout produces
            (a charted show, whose sidebar is present); the bill truncates
            there rather than pushing a column off the row, and it is the cell
            every other width here is budgeted around. */}
        <span
          className={`shrink-0 whitespace-nowrap font-mono text-xs uppercase tabular-nums text-muted-foreground ${leadWidth}`}
        >
          {row.lead ?? ''}
        </span>
        <span
          className={`min-w-0 flex-1 text-sm group-hover:underline sm:truncate ${
            row.isCancelled ? 'text-muted-foreground line-through' : ''
          }`}
        >
          {row.title}
        </span>
        {/* Visible when STACKED (below `sm`, where every cell is its own
            full-width line and there is no competition at all) and again from
            `xl`. Hidden only in the horizontal band between them, where the
            columns genuinely fight — hiding it on mobile too would leave the
            metro rail unable to say WHERE, which is most of its answer, on the
            majority of this site's traffic.

            Not `shrink-0`: under pressure this cell yields to the bill rather
            than pushing it out. */}
        {hasRoomColumn && (
          <span className="block min-w-0 truncate font-mono text-xs text-muted-foreground sm:hidden xl:block xl:w-1/6">
            {row.room ?? ''}
          </span>
        )}
        {/* The age column, on the same visibility rule as the room and for the
            same reason: below `sm` every cell is its own full-width line and
            there is no competition, and from `xl` there is room for all five.
            In the horizontal band between them the columns genuinely fight,
            and a door policy is the cell a reader can most afford to open the
            show page for.

            Fixed-width and truncating rather than sized to the value: this is
            contributor-written free text with no vocabulary enforced, so a
            column sized to fit whatever arrived would move under the row
            below it. The width fits the short forms this column is mostly
            made of (`21+`, `18+`); a longer one truncates and the `title`
            carries the whole of it. */}
        {hasAgeColumn && (
          <span
            className="block min-w-0 truncate font-mono text-xs uppercase text-muted-foreground sm:hidden xl:block xl:w-10"
            title={row.age ?? undefined}
          >
            {row.age ?? ''}
          </span>
        )}
        {/* Uppercased here rather than in the policy, so `Free` reaches the
            mock's `FREE` and `Sold out` reaches `SOLD OUT` without forking
            `formatPrice`, which serves the whole site.

            `figureLabel` is set only for a SPLIT price, where the visible
            `$35/$40` would otherwise be read out as punctuation. Same mechanism
            as the `ShowPrice` component — hide the glyphs, offer the spelling —
            and for the same reason it cannot be `aria-label`: a bare span is
            role `generic`, which prohibits an author name. This column cannot
            use that component itself because it also holds status tokens, which
            are not prices. */}
        <span
          className="shrink-0 font-mono text-xs uppercase tabular-nums text-muted-foreground sm:w-16"
          title={row.figureLabel ?? undefined}
        >
          {row.figureLabel ? (
            <>
              <span aria-hidden="true">{row.figure}</span>
              <span className="sr-only">{row.figureLabel}</span>
            </>
          ) : (
            (row.figure ?? '')
          )}
        </span>
      </Link>
    </li>
  )
}
