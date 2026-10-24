'use client'

import { Fragment, useMemo } from 'react'
import Link from 'next/link'
import { DenseTable, DenseTableGroupHeader, ShowPrice } from '@/components/shared'
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
  STACKED_TABLE,
  ShowSaveCell,
  ShowSaveHeaderCell,
  type ShowSaveColumnData,
} from '@/features/shows/components/ShowSaveColumn'
import { cn } from '@/lib/utils'
import { groupByMonth } from '../showArchive'
import type { VenueShow, VenueShowZone } from '../types'

/**
 * Date, Bill, Price, Time, and Save when the table has a save column. Group
 * headings must span all of them.
 */
const BASE_COLUMN_COUNT = 4

/** Stands in for a price nobody has recorded. */
const ABSENT = EN_DASH

function ShowRow({
  show,
  zone,
  saveColumn,
}: {
  show: VenueShow
  zone: VenueShowZone
  saveColumn?: ShowSaveColumnData
}) {
  // A show's own state wins over the venue's for date/time formatting; the
  // venue's resolved timezone (when known) wins over the state map (PSY-986).
  const state = show.state ?? zone.venueState
  const time = formatShowTime(show.event_date, state, zone.venueTimezone)
  const stacked = saveColumn !== undefined
  const cell = stacked ? 'cell' : undefined

  // Stacked below `sm` (see STACKED_TABLE): the date, then price and time and
  // the Save control at the end of the line; the bill.
  return (
    <tr
      role={stacked ? 'row' : undefined}
      className={
        stacked
          ? cn(
              STACKED_TABLE.row,
              'max-sm:grid-cols-[minmax(0,1fr)_auto_auto_auto]'
            )
          : undefined
      }
    >
      <td
        role={cell}
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
        role={cell}
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
        role={cell}
        className={cn(
          'whitespace-nowrap text-right font-mono text-xs text-muted-foreground',
          stacked && 'max-sm:col-start-2 max-sm:row-start-1'
        )}
      >
        <ShowPrice show={show} fallback={ABSENT} />
      </td>
      <td
        role={cell}
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
      {saveColumn && (
        <ShowSaveCell
          showId={show.id}
          column={saveColumn}
          className="max-sm:col-start-4 max-sm:row-start-1"
        />
      )}
    </tr>
  )
}

export interface VenueShowsTableProps {
  shows: VenueShow[]
  zone: VenueShowZone
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
   */
  groupByMonthHeadings?: boolean
  /**
   * Adds the trailing Save column, from one batched save-count read
   * (`useShowSaveColumn`). Upcoming rows only: past rows are a record, not a
   * plan, so the past archive renders without it.
   */
  saveColumn?: ShowSaveColumnData
}

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
  const header = stacked ? 'columnheader' : undefined

  return (
    <DenseTable
      variant="alternating"
      aria-label={ariaLabel}
      role={stacked ? 'table' : undefined}
      className={stacked ? STACKED_TABLE.table : undefined}
    >
      <thead
        role={stacked ? 'rowgroup' : undefined}
        className={stacked ? STACKED_TABLE.head : undefined}
      >
        <tr role={stacked ? 'row' : undefined}>
          <th role={header}>Date</th>
          <th role={header}>Bill</th>
          <th role={header} className="text-right">
            Price
          </th>
          <th role={header} className="text-right">
            Time
          </th>
          {saveColumn && <ShowSaveHeaderCell />}
        </tr>
      </thead>
      <tbody
        role={stacked ? 'rowgroup' : undefined}
        className={stacked ? STACKED_TABLE.body : undefined}
      >
        {groups === null
          ? shows.map(show => (
              <ShowRow
                key={show.id}
                show={show}
                zone={zone}
                saveColumn={saveColumn}
              />
            ))
          : groups.map((group, index) => (
              // Indexed because a month label is only unique while the rows are
              // in date order, which this component does not get to assume.
              <Fragment key={index}>
                <DenseTableGroupHeader
                  title={group.label}
                  colSpan={BASE_COLUMN_COUNT + (saveColumn ? 1 : 0)}
                />
                {group.rows.map(show => (
                  <ShowRow
                    key={show.id}
                    show={show}
                    zone={zone}
                    saveColumn={saveColumn}
                  />
                ))}
              </Fragment>
            ))}
      </tbody>
    </DenseTable>
  )
}
