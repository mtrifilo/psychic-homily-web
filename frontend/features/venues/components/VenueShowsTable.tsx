'use client'

import { Fragment } from 'react'
import Link from 'next/link'
import { DenseTable, DenseTableGroupHeader } from '@/components/shared'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import {
  formatPrice,
  formatShowDate,
  formatShowTime,
} from '@/lib/utils/formatters'
import { groupByMonth, type ArchiveZone } from '../showArchive'
import type { VenueShow } from '../types'

/** Date, Bill, Price, Time. Group headings must span all of them. */
const COLUMN_COUNT = 4

/** Stands in for a price nobody has recorded. An en dash, never an em dash. */
const ABSENT = '–'

/**
 * The act at the top of the bill, and everyone else in listed order.
 *
 * `is_headliner` is the only bill role the frontend surfaces (`set_type` also
 * carries 'performer', which means "on the bill, slot unknown" and must not be
 * rendered as a role). When no act claims the slot the first listed one leads,
 * matching how every other surface in the app reads a bill.
 */
function splitBill(artists: VenueShow['artists']) {
  if (artists.length === 0) return { headliner: null, support: [] }
  const headliner = artists.find(artist => artist.is_headliner) ?? artists[0]
  return {
    headliner,
    support: artists.filter(artist => artist !== headliner),
  }
}

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
  zone: ArchiveZone
}) {
  // A show's own state wins over the venue's for date/time formatting; the
  // venue's resolved timezone (when known) wins over the state map (PSY-986).
  const state = show.state ?? zone.venueState
  const { headliner, support } = splitBill(show.artists)

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
        {headliner ? (
          <span className="flex flex-wrap items-baseline gap-x-1.5 gap-y-1">
            <ArtistLink
              artist={headliner}
              className={cn(
                'font-medium text-foreground hover:text-primary hover:underline',
                show.is_cancelled && 'line-through'
              )}
            />
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
  zone: ArchiveZone
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
  className?: string
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
  className,
}: VenueShowsTableProps) {
  const groups = groupByMonthHeadings
    ? groupByMonth(shows, zone)
    : [{ label: '', rows: shows }]

  return (
    <DenseTable
      variant="alternating"
      aria-label={ariaLabel}
      className={className}
    >
      <thead>
        <tr>
          <th>Date</th>
          <th>Bill</th>
          <th className="text-right">Price</th>
          <th className="text-right">Time</th>
        </tr>
      </thead>
      <tbody>
        {groups.map((group, index) => (
          // Indexed because a month label is only unique while the rows are in
          // date order, and this component does not get to assume they are.
          <Fragment key={`${group.label}-${index}`}>
            {groupByMonthHeadings && (
              <DenseTableGroupHeader
                title={group.label}
                colSpan={COLUMN_COUNT}
              />
            )}
            {group.rows.map(show => (
              <ShowRow key={show.id} show={show} zone={zone} />
            ))}
          </Fragment>
        ))}
      </tbody>
    </DenseTable>
  )
}
