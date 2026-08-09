'use client'

import { Fragment, useMemo } from 'react'
import Link from 'next/link'
import { DenseTable, DenseTableGroupHeader } from '@/components/shared'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
// Deep import, not the barrel: `@/features/shows`'s barrel edge drags in
// ShowForm and the whole mutation graph for one pure function, and pulls a
// venues -> shows -> venues value cycle in behind it (the same reason ShowForm
// deep-imports VenueInput). See features/venues/components/index.ts.
import { splitBill } from '@/features/shows/utils'
import {
  formatPrice,
  formatShowDate,
  formatShowTime,
} from '@/lib/utils/formatters'
import { groupByMonth, type MonthGroup } from '../showArchive'
import type { VenueShow, VenueShowZone } from '../types'

/** Date, Bill, Price, Time. Group headings must span all of them. */
const COLUMN_COUNT = 4

/** Stands in for a price nobody has recorded. An en dash, never an em dash. */
const ABSENT = '–'

function ArtistLink({
  artist,
  className,
}: {
  artist: VenueShow['artists'][number]
  className?: string
}) {
  return (
    <Link href={`/artists/${artist.slug}`} className={className}>
      {artist.name}
    </Link>
  )
}

function ShowRow({
  show,
  zone,
}: {
  show: VenueShow
  zone: VenueShowZone
}) {
  // A show's own state wins over the venue's for date/time formatting; the
  // venue's resolved timezone (when known) wins over the state map (PSY-986).
  const state = show.state ?? zone.venueState
  const { headliners, support } = splitBill(show.artists)

  return (
    <tr>
      <td className="whitespace-nowrap">
        <Link
          href={`/shows/${show.slug || show.id}`}
          className="hover:text-primary hover:underline underline-offset-2"
        >
          {formatShowDate(show.event_date, state, false, zone.venueTimezone)}
        </Link>
      </td>
      <td>
        {headliners.length > 0 ? (
          <span className="flex flex-wrap items-baseline gap-x-1.5 gap-y-1">
            <span
              className={cn(
                'font-medium text-foreground',
                show.is_cancelled && 'line-through'
              )}
            >
              {headliners.map((artist, index) => (
                <span key={artist.id}>
                  {index > 0 && ', '}
                  <ArtistLink
                    artist={artist}
                    className="hover:text-primary hover:underline"
                  />
                </span>
              ))}
            </span>
            {support.length > 0 && (
              <span className="text-muted-foreground">
                w/{' '}
                {support.map((artist, index) => (
                  <span key={artist.id}>
                    {index > 0 && ', '}
                    <ArtistLink
                      artist={artist}
                      className="hover:text-foreground hover:underline"
                    />
                  </span>
                ))}
              </span>
            )}
            {show.is_cancelled && (
              <Badge variant="destructive" className="text-[10px]">
                CANCELLED
              </Badge>
            )}
            {/* A cancelled show's ticket status is moot, and two badges on one
                row read as two separate facts about a show that is not
                happening. */}
            {!show.is_cancelled && show.is_sold_out && (
              <Badge variant="outline" className="text-[10px]">
                SOLD OUT
              </Badge>
            )}
          </span>
        ) : (
          <span className="text-muted-foreground">{ABSENT}</span>
        )}
      </td>
      <td className="whitespace-nowrap text-right font-mono text-xs text-muted-foreground">
        {typeof show.price === 'number' ? formatPrice(show.price) : ABSENT}
      </td>
      <td className="whitespace-nowrap text-right text-muted-foreground">
        {formatShowTime(show.event_date, state, zone.venueTimezone)}
      </td>
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
}: VenueShowsTableProps) {
  // `null` is "one ungrouped run", the single representation of not grouping —
  // and the memo matters: grouping resolves a timezone and formats a month per
  // row, and `keepPreviousData` re-renders this table with referentially
  // identical rows while the next page is in flight.
  const groups: MonthGroup<VenueShow>[] | null = useMemo(
    () => (groupByMonthHeadings ? groupByMonth(shows, zone) : null),
    [groupByMonthHeadings, shows, zone]
  )

  return (
    <DenseTable variant="alternating" aria-label={ariaLabel}>
      <thead>
        <tr>
          <th>Date</th>
          <th>Bill</th>
          <th className="text-right">Price</th>
          <th className="text-right">Time</th>
        </tr>
      </thead>
      <tbody>
        {groups === null
          ? shows.map(show => (
              <ShowRow key={show.id} show={show} zone={zone} />
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
                  <ShowRow key={show.id} show={show} zone={zone} />
                ))}
              </Fragment>
            ))}
      </tbody>
    </DenseTable>
  )
}
