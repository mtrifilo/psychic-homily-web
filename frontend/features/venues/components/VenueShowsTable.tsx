'use client'

import { Fragment, memo, useMemo } from 'react'
import Link from 'next/link'
import { DenseTable, DenseTableGroupHeader, ShowPrice } from '@/components/shared'
import {
  batchedSaveFor,
  type BatchedSaveData,
} from '@/components/shared/batchedSaveData'
import { stackedTableLayout } from '@/components/shared/stackedTable'
// Deep import, not the barrel: `@/features/shows`'s barrel edge drags in
// ShowForm and the whole mutation graph for one component, and pulls a
// venues -> shows -> venues value cycle in behind it (the same reason ShowForm
// deep-imports VenueInput). See features/venues/components/index.ts.
import { ShowBill } from '@/features/shows/components/ShowBill'
import {
  formatShowDate,
  formatShowTime,
} from '@/lib/utils/formatters'
import { EN_DASH, type MonthGroup } from '@/features/shows/showArchive'
import {
  ShowSaveCell,
  ShowSaveHeaderCell,
  type ShowSaveColumnData,
} from '@/features/shows/components/ShowSaveColumn'
import { cn } from '@/lib/utils'
import { groupByMonth } from '../showArchive'
import type { VenueShow, VenueShowZone } from '../types'

/** Date, Bill, Price, Time. Group headings must span all of them. */
const COLUMN_COUNT = 4

/**
 * A table with a Save column stacks below `sm`: the date, then price and time
 * and the Save control at the end of the line; the bill.
 */
const STACKED_LAYOUT = stackedTableLayout(
  'max-sm:grid-cols-[minmax(0,1fr)_auto_auto_auto]'
)
const TABLE_LAYOUT = stackedTableLayout()

/** Stands in for a price nobody has recorded. */
const ABSENT = EN_DASH

/**
 * Memoized: a save toggles one entry of the batch map, and every other row's
 * `saveData` keeps its identity, so only the toggled row re-renders.
 */
const ShowRow = memo(function ShowRow({
  show,
  zone,
  stacked,
  saveData,
}: {
  show: VenueShow
  zone: VenueShowZone
  /** The row carries the Save cell and stacks below `sm`. */
  stacked: boolean
  saveData?: BatchedSaveData
}) {
  // A show's own state wins over the venue's for date/time formatting; the
  // venue's resolved timezone (when known) wins over the state map (PSY-986).
  const state = show.state ?? zone.venueState
  const time = formatShowTime(show.event_date, state, zone.venueTimezone)
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
          {formatShowDate(show.event_date, state, false, zone.venueTimezone)}
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
        />
      </td>
      <td
        {...layout.cell}
        className={cn(
          'whitespace-nowrap text-right font-mono text-xs text-muted-foreground',
          stacked && 'max-sm:col-start-2 max-sm:row-start-1'
        )}
      >
        <ShowPrice show={show} fallback={ABSENT} />
      </td>
      <td
        {...layout.cell}
        className={cn(
          'whitespace-nowrap text-right text-muted-foreground',
          stacked && 'max-sm:col-start-3 max-sm:row-start-1'
        )}
      >
        {/* The header names the column at table widths; stacked, the price
            and the time share a line and the dot separates them. */}
        {stacked && time && (
          <span aria-hidden="true" className="sm:hidden">
            {'· '}
          </span>
        )}
        {/* Empty, not `ABSENT`, on a guessed zone. The en dash next door says
            "nobody recorded a price"; a withheld clock is a time the row HAS and
            declines to name on a zone it cannot trust. The cell stays for the
            column count, and the column stays because `show.state` overrides the
            venue's per row, so emptiness here is never uniform. */}
        {time}
      </td>
      {stacked && (
        <ShowSaveCell
          {...layout.cell}
          showId={show.id}
          saveData={saveData}
          className="max-sm:col-start-4 max-sm:row-start-1"
        />
      )}
    </tr>
  )
})

interface VenueShowsTableBaseProps {
  shows: VenueShow[]
  zone: VenueShowZone
  /** Accessible name for the table. Unique per page. */
  ariaLabel: string
}

interface GroupedVenueShowsTableProps extends VenueShowsTableBaseProps {
  /**
   * Break the rows into `SEP 2025` group headings. On for the past archive,
   * where a page can span half a year; off for the upcoming list, which is
   * read as one forward-looking run.
   *
   * Only meaningful while the rows are in date order — the grouping walks runs
   * rather than collecting, so an unsorted list gets repeated headings instead
   * of silently reordered rows.
   */
  groupByMonthHeadings: true
  /** Grouped rows never carry Save: the group headings do not stack. */
  saveColumn?: never
}

interface RunVenueShowsTableProps extends VenueShowsTableBaseProps {
  groupByMonthHeadings?: false
  /**
   * Adds the trailing Save column, from one batched save-count read
   * (`useShowSaveColumn`), and stacks the rows below `sm`. Upcoming rows only:
   * past rows are a record, not a plan, so the past archive renders without it.
   */
  saveColumn?: ShowSaveColumnData
}

export type VenueShowsTableProps =
  | GroupedVenueShowsTableProps
  | RunVenueShowsTableProps

/**
 * The venue page's show table, shared by the upcoming and past sections
 * (PSY-1753).
 *
 * Presentational: it renders exactly the rows it is handed, in the order it is
 * handed them. Paging, year filtering and month-range labels all belong to the
 * section around it.
 */
export function VenueShowsTable({
  shows,
  zone,
  ariaLabel,
  groupByMonthHeadings = false,
  saveColumn,
}: VenueShowsTableProps) {
  // `null` is "one ungrouped run", the single representation of not grouping —
  // and the memo matters: grouping resolves a timezone and formats a month per
  // row, and `keepPreviousData` re-renders this table with referentially
  // identical rows while the next page is in flight.
  const groups: MonthGroup<VenueShow>[] | null = useMemo(
    () => (groupByMonthHeadings ? groupByMonth(shows, zone) : null),
    [groupByMonthHeadings, shows, zone]
  )

  const stacked = saveColumn !== undefined
  const layout = stacked ? STACKED_LAYOUT : TABLE_LAYOUT

  return (
    <DenseTable variant="alternating" aria-label={ariaLabel} {...layout.table}>
      <thead {...layout.head}>
        <tr {...layout.headRow}>
          <th {...layout.columnHeader}>Date</th>
          <th {...layout.columnHeader}>Bill</th>
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
                zone={zone}
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
                  <ShowRow
                    key={show.id}
                    show={show}
                    zone={zone}
                    stacked={false}
                  />
                ))}
              </Fragment>
            ))}
      </tbody>
    </DenseTable>
  )
}
