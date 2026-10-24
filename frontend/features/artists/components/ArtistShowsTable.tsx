'use client'

import { Fragment, memo, useMemo } from 'react'
import Link from 'next/link'
import { DenseTable, DenseTableGroupHeader, ShowPrice } from '@/components/shared'
import {
  batchedSaveFor,
  type BatchedSaveData,
} from '@/components/shared/batchedSaveData'
import {
  STACKED_OFF_LINE,
  stackedTableLayout,
} from '@/components/shared/stackedTable'
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
import {
  ShowSaveCell,
  ShowSaveHeaderCell,
  type ShowSaveColumnData,
} from '@/features/shows/components/ShowSaveColumn'
import { cn } from '@/lib/utils'
import { formatLocation, LOCATION_UNKNOWN } from '@/lib/formatLocation'
import {
  formatShowDate,
  formatShowTime,
} from '@/lib/utils/formatters'
import { artistShowZone } from '../showArchive'
import type { ArtistShow } from '../types'

/** Date, Bill, Price, Time. Group headings must span all of them. */
const COLUMN_COUNT = 4

/**
 * A table with a Save column stacks below `sm`: the date and time, then the
 * Save control at the end of the line; the bill; the place. The price has no
 * line there and stays in the row for assistive tech.
 */
const STACKED_LAYOUT = stackedTableLayout(
  'max-sm:grid-cols-[auto_minmax(0,1fr)_auto]'
)
const TABLE_LAYOUT = stackedTableLayout()

/** Stands in for a value nobody has recorded. */
const ABSENT = EN_DASH

/**
 * Where the show happened, rendered inline after the bill.
 *
 * This is the one deliberate difference from the venue archive's row: a venue's
 * list is already scoped to one place, an artist's is not, so the place is the
 * second most important thing in the row after who played.
 */
function VenueContext({
  venue,
  stacked,
}: {
  venue: ArtistShow['venue']
  stacked: boolean
}) {
  // Stacked, the place is the row's last line rather than a run-on of the
  // bill: a full-width flex item after the status badges, with no separator.
  const className = cn(
    'text-muted-foreground',
    stacked && 'max-sm:order-last max-sm:basis-full'
  )
  const separator = stacked ? (
    <span className="max-sm:hidden">{'· '}</span>
  ) : (
    '· '
  )
  if (!venue) {
    return (
      <span className={className}>
        {separator}
        Venue TBA
      </span>
    )
  }
  // The locked PSY-558/780 rule, not a hand-rolled `${city}, ${state}` — which
  // is the exact drift PSY-780 consolidated away. It matters most here: an
  // artist's rows span metros, so "Portland" alone does not say which Portland.
  // `LOCATION_UNKNOWN` is a field placeholder and must not print mid-line, so a
  // venue with no placeable location contributes nothing (the helper exports
  // the constant for precisely this composition case).
  const location = formatLocation({ city: venue.city, state: venue.state })
  return (
    <span className={className}>
      {separator}
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
 * Memoized: a save toggles one entry of the batch map, and every other row's
 * `saveData` keeps its identity, so only the toggled row re-renders.
 */
const ShowRow = memo(function ShowRow({
  show,
  stacked,
  saveData,
}: {
  show: ArtistShow
  /** The row carries the Save cell and stacks below `sm`. */
  stacked: boolean
  saveData?: BatchedSaveData
}) {
  const zone = artistShowZone(show)
  const layout = stacked ? STACKED_LAYOUT : TABLE_LAYOUT

  return (
    <tr {...layout.row}>
      <td
        {...layout.cell}
        className={cn(
          'whitespace-nowrap',
          stacked && 'max-sm:col-start-1 max-sm:row-start-1'
        )}
      >
        <Link
          href={`/shows/${show.slug || show.id}`}
          className="hover:text-primary hover:underline underline-offset-2"
        >
          {formatShowDate(show.event_date, zone.state, false, zone.timezone)}
        </Link>
      </td>
      <td
        {...layout.cell}
        className={
          stacked ? 'max-sm:col-span-full max-sm:row-start-2' : undefined
        }
      >
        <ShowBill
          artists={show.artists}
          isCancelled={show.is_cancelled}
          isSoldOut={show.is_sold_out}
          afterBill={<VenueContext venue={show.venue} stacked={stacked} />}
          linkTreatment="restrained"
        />
      </td>
      <td
        {...layout.cell}
        className={cn(
          'whitespace-nowrap text-right font-mono text-xs text-muted-foreground',
          stacked && STACKED_OFF_LINE
        )}
      >
        <ShowPrice show={show} fallback={ABSENT} />
      </td>
      <td
        {...layout.cell}
        className={cn(
          'whitespace-nowrap text-right text-muted-foreground',
          stacked && 'max-sm:col-start-2 max-sm:row-start-1 max-sm:text-left'
        )}
      >
        {/* Empty, not `ABSENT`, on a guessed zone. The en dash says "nobody
            recorded this"; a withheld clock is a time the row HAS and declines
            to name on a zone it cannot trust. The cell stays for the column
            count. */}
        {formatShowTime(show.event_date, zone.state, zone.timezone)}
      </td>
      {stacked && (
        <ShowSaveCell
          {...layout.cell}
          showId={show.id}
          saveData={saveData}
          className="max-sm:col-start-3 max-sm:row-start-1"
        />
      )}
    </tr>
  )
})

interface ArtistShowsTableBaseProps {
  shows: ArtistShow[]
  /** Accessible name for the table. Unique per page. */
  ariaLabel: string
}

interface GroupedArtistShowsTableProps extends ArtistShowsTableBaseProps {
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
  groupByMonthHeadings: true
  /** Grouped rows never carry Save: the group headings do not stack. */
  saveColumn?: never
}

interface RunArtistShowsTableProps extends ArtistShowsTableBaseProps {
  groupByMonthHeadings?: false
  /**
   * Adds the trailing Save column, from one batched save-count read
   * (`useShowSaveColumn`), and stacks the rows below `sm`. Upcoming rows only:
   * past rows are a record, not a plan, so the past archive renders without it.
   */
  saveColumn?: ShowSaveColumnData
}

export type ArtistShowsTableProps =
  | GroupedArtistShowsTableProps
  | RunArtistShowsTableProps

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
  saveColumn,
}: ArtistShowsTableProps) {
  // `null` is "one ungrouped run", the single representation of not grouping —
  // and the memo matters: grouping resolves a timezone and formats a month per
  // row, and `keepPreviousData` re-renders this table with referentially
  // identical rows while the next page is in flight.
  const groups: MonthGroup<ArtistShow>[] | null = useMemo(
    () => (groupByMonthHeadings ? groupByMonth(shows, artistShowZone) : null),
    [groupByMonthHeadings, shows]
  )

  const stacked = saveColumn !== undefined
  const layout = stacked ? STACKED_LAYOUT : TABLE_LAYOUT

  return (
    <DenseTable variant="alternating" aria-label={ariaLabel} {...layout.table}>
      <thead {...layout.head}>
        <tr {...layout.headRow}>
          <th {...layout.columnHeader}>Date</th>
          {/* Not just "Bill", which the venue twin can say: this cell also
              carries the venue and city, and a screen reader announces the
              column header with every cell in it. */}
          <th {...layout.columnHeader}>Bill · Venue</th>
          <th {...layout.columnHeader} className="text-right">
            Price
          </th>
          <th {...layout.columnHeader} className="text-right">
            Time
          </th>
          {stacked && <ShowSaveHeaderCell {...layout.columnHeader} />}
        </tr>
      </thead>
      <tbody {...layout.body}>
        {groups === null
          ? shows.map(show => (
              <ShowRow
                key={show.id}
                show={show}
                stacked={stacked}
                saveData={
                  saveColumn ? batchedSaveFor(saveColumn.counts, show.id) : undefined
                }
              />
            ))
          : groups.map((group, index) => (
              // Indexed because a month label is only unique while the rows are
              // in date order, which this component does not get to assume.
              <Fragment key={index}>
                <DenseTableGroupHeader
                  title={group.label}
                  colSpan={COLUMN_COUNT}
                />
                {group.rows.map(show => (
                  <ShowRow key={show.id} show={show} stacked={false} />
                ))}
              </Fragment>
            ))}
      </tbody>
    </DenseTable>
  )
}
