'use client'

import { Fragment, useMemo } from 'react'
import Link from 'next/link'
import { DenseTable, DenseTableGroupHeader } from '@/components/shared'
// Deep imports, not the barrel: `@/features/shows`'s barrel edge drags in
// ShowForm and the whole mutation graph, and pulls an artists -> shows ->
// artists value cycle in behind it (the same reason ShowForm deep-imports
// ArtistInput). See features/artists/components/index.ts.
import { ShowBill } from '@/features/shows/components/ShowBill'
import {
  EN_DASH,
  groupByMonth,
  type MonthGroup,
} from '@/features/shows/showArchive'
import { formatLocation, LOCATION_UNKNOWN } from '@/lib/formatLocation'
import { showPriceLabel } from '@/lib/utils/showPrice'
import {
  formatShowDate,
  formatShowTime,
} from '@/lib/utils/formatters'
import { artistShowZone } from '../showArchive'
import type { ArtistShow } from '../types'

/** Date, Bill, Price, Time. Group headings must span all of them. */
const COLUMN_COUNT = 4

/** Stands in for a value nobody has recorded. */
const ABSENT = EN_DASH

/**
 * Where the show happened, rendered inline after the bill.
 *
 * This is the one deliberate difference from the venue archive's row: a venue's
 * list is already scoped to one place, an artist's is not, so the place is the
 * second most important thing in the row after who played.
 */
function VenueContext({ venue }: { venue: ArtistShow['venue'] }) {
  if (!venue) {
    return <span className="text-muted-foreground">· Venue TBA</span>
  }
  // The locked PSY-558/780 rule, not a hand-rolled `${city}, ${state}` — which
  // is the exact drift PSY-780 consolidated away. It matters most here: an
  // artist's rows span metros, so "Portland" alone does not say which Portland.
  // `LOCATION_UNKNOWN` is a field placeholder and must not print mid-line, so a
  // venue with no placeable location contributes nothing (the helper exports
  // the constant for precisely this composition case).
  const location = formatLocation({ city: venue.city, state: venue.state })
  return (
    <span className="text-muted-foreground">
      {'· '}
      {/* Unlinked without a slug: the backend leaves it empty for venues that
          predate the backfill, and `/venues/` is a 404, not a venue page. */}
      {venue.slug ? (
        <Link
          href={`/venues/${venue.slug}`}
          className="hover:text-foreground hover:underline"
        >
          {venue.name}
        </Link>
      ) : (
        venue.name
      )}
      {location === LOCATION_UNKNOWN ? '' : `, ${location}`}
    </span>
  )
}

/**
 * The row's price: `$35`, `Free`, the split `$35/$40`, or an en dash when
 * nobody recorded one.
 *
 * The pair is derived by {@link showPriceLabel}, shared with every other list
 * on the site and with the show page's own ticket line, so a reader who scans
 * this table and opens a show is never shown two different prices for it.
 *
 * The spelled-out `title` rides along for the split only. It is set as
 * `aria-label` too, because "$35 slash $40" is what a screen reader otherwise
 * announces for a fact about money.
 */
function PriceCell({ show }: { show: { price: number | null; door_price: number | null } }) {
  const price = showPriceLabel(show)
  if (!price) return <>{ABSENT}</>
  return (
    <span title={price.title} aria-label={price.title}>
      {price.text}
    </span>
  )
}

function ShowRow({ show }: { show: ArtistShow }) {
  const zone = artistShowZone(show)

  return (
    <tr>
      <td className="whitespace-nowrap">
        <Link
          href={`/shows/${show.slug || show.id}`}
          className="hover:text-primary hover:underline underline-offset-2"
        >
          {formatShowDate(show.event_date, zone.state, false, zone.timezone)}
        </Link>
      </td>
      <td>
        <ShowBill
          artists={show.artists}
          isCancelled={show.is_cancelled}
          isSoldOut={show.is_sold_out}
          afterBill={<VenueContext venue={show.venue} />}
        />
      </td>
      <td className="whitespace-nowrap text-right font-mono text-xs text-muted-foreground">
        <PriceCell show={show} />
      </td>
      <td className="whitespace-nowrap text-right text-muted-foreground">
        {formatShowTime(show.event_date, zone.state, zone.timezone)}
      </td>
    </tr>
  )
}

export interface ArtistShowsTableProps {
  shows: ArtistShow[]
  /** Accessible name for the table. Unique per page. */
  ariaLabel: string
  /**
   * Break the rows into `SEP 2025` group headings. On for the past archive,
   * where a page can span half a year; off for the upcoming list, which is
   * read as one forward-looking run.
   *
   * Only meaningful while the rows are in date order — the grouping walks runs
   * rather than collecting, so an unsorted list gets repeated headings instead
   * of silently reordered rows.
   *
   * The artist list is sorted, but on a different axis than it is grouped by:
   * the API orders on the absolute instant, while each row is LABELLED in its
   * own venue's zone. Inside the ~1-day band around a month boundary those two
   * disagree, so a page covering it can show a heading twice or out of order.
   * That is the honest rendering of the underlying rows and it is pinned by a
   * test — it is not a sorting bug to go fix.
   */
  groupByMonthHeadings?: boolean
}

/**
 * The artist page's show table, shared by the upcoming and past sections
 * (PSY-1754).
 *
 * Presentational: it renders exactly the rows it is handed, in the order it is
 * handed them. Paging, year filtering and month-range labels all belong to the
 * section around it.
 *
 * The venue twin is `VenueShowsTable`. They are two components rather than one
 * because the rows differ in both directions: an artist row names its venue and
 * resolves its zone per row, a venue row does neither. What they DO share — how
 * a bill reads, how a cancelled or sold-out date is badged — lives in
 * `ShowBill`, so it has one answer on both pages.
 */
export function ArtistShowsTable({
  shows,
  ariaLabel,
  groupByMonthHeadings = false,
}: ArtistShowsTableProps) {
  // `null` is "one ungrouped run", the single representation of not grouping —
  // and the memo matters: grouping resolves a timezone and formats a month per
  // row, and `keepPreviousData` re-renders this table with referentially
  // identical rows while the next page is in flight.
  const groups: MonthGroup<ArtistShow>[] | null = useMemo(
    () => (groupByMonthHeadings ? groupByMonth(shows, artistShowZone) : null),
    [groupByMonthHeadings, shows]
  )

  return (
    <DenseTable variant="alternating" aria-label={ariaLabel}>
      <thead>
        <tr>
          <th>Date</th>
          {/* Not just "Bill", which the venue twin can say: this cell also
              carries the venue and city, and a screen reader announces the
              column header with every cell in it. */}
          <th>Bill · Venue</th>
          <th className="text-right">Price</th>
          <th className="text-right">Time</th>
        </tr>
      </thead>
      <tbody>
        {groups === null
          ? shows.map(show => <ShowRow key={show.id} show={show} />)
          : groups.map((group, index) => (
              // Indexed because a month label is only unique while the rows are
              // in date order, which this component does not get to assume.
              <Fragment key={index}>
                <DenseTableGroupHeader
                  title={group.label}
                  colSpan={COLUMN_COUNT}
                />
                {group.rows.map(show => (
                  <ShowRow key={show.id} show={show} />
                ))}
              </Fragment>
            ))}
      </tbody>
    </DenseTable>
  )
}
