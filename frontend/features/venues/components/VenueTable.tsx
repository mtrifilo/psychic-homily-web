'use client'

import { Fragment } from 'react'
import Link from 'next/link'
import { BadgeCheck } from 'lucide-react'
import { DenseTable } from '@/components/shared'
import { formatCount } from '@/components/shared/paginationChrome'
import { formatShowDate, formatShowTime } from '@/lib/utils/formatters'
import { socialLinkHref } from '@/lib/socialLinks'
import { cn } from '@/lib/utils'
import type { VenueWithShowCount } from '../types'
import type { VenueSort } from '../venuesListNavigation'

/**
 * Which way each order runs, so a sorted column can say so.
 *
 * Fixed per key rather than toggleable: the API offers one direction per sort
 * value, and a header that flipped on a second press would address an order the
 * backend cannot serve.
 */
const SORT_DIRECTION: Record<VenueSort, 'ascending' | 'descending'> = {
  upcoming: 'descending',
  name: 'ascending',
  next: 'ascending',
}

/** Columns the table draws, for the group header's span. */
const COLUMN_COUNT = 5

/** The two-line mobile grid: the name/count line, then the place/date line. */
const rowGridClass =
  'grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-3 sm:table-row'
const leadCellClass = 'block pb-0 sm:table-cell sm:pb-1.5'
const trailCellClass = 'block pt-0 sm:table-cell sm:pt-1.5'

export interface VenueTableProps {
  /** The page's rows, in the order the API returned them. */
  venues: VenueWithShowCount[]
  /** The order in force, which is what the headers mark. */
  sort: VenueSort
  /** Applies a new order. The consumer owns the URL write and the page reset. */
  onSortChange: (sort: VenueSort) => void
  /**
   * Whether a row has to say which city it is in.
   *
   * True whenever the list is NOT scoped to exactly one city, where the heading
   * already names it and every row would repeat it. Without this an all-cities
   * or multi-city list is rows of names with no way to tell them apart.
   */
  showCity: boolean
}

/**
 * One room's row.
 *
 * `isQuiet` is derived from the count rather than passed as a group flag, so a
 * row cannot be drawn under one heading and dated under the other: a quiet room
 * prints its LAST show where an active one prints its next, and the two fields
 * partition the room's shows on one boundary (see `VenueListShowRef`).
 */
function VenueRow({
  venue,
  showCity,
}: {
  venue: VenueWithShowCount
  showCity: boolean
}) {
  const isQuiet = venue.upcoming_show_count === 0
  const show = isQuiet ? venue.last_show : venue.next_show

  // The venue's own calendar, never the reader's. `formatShowTime` returns null
  // when this room's zone is not known, and the contract for that is to drop the
  // hour and its separator rather than print a guessed clock.
  // The quiet block's dates carry their YEAR: a room with nothing booked has
  // often been dark for years, and "last: Jun 14" cannot say which.
  const date = show
    ? formatShowDate(show.event_date, venue.state, isQuiet, venue.timezone)
    : null
  const time = show
    ? formatShowTime(show.event_date, venue.state, venue.timezone)
    : null
  const when = date
    ? `${isQuiet ? 'last: ' : ''}${date}${!isQuiet && time ? ` ${time}` : ''}`
    : null

  // The place line: the street when the scope already names the city, and the
  // city AHEAD of the street when it does not. Leading with it is what keeps it
  // out of the clip: it is the only thing telling two rows apart there.
  const place = showCity
    ? [`${venue.city}, ${venue.state}`, venue.address].filter(Boolean).join(' · ')
    : (venue.address ?? '')

  // Through the shared gate, never raw: the column is user-editable free text,
  // so a stored `javascript:` value would otherwise become a live link, and a
  // scheme-less one a relative href into /venues.
  const website = socialLinkHref('website', venue.social?.website)

  return (
    <tr role="row" className={cn(rowGridClass, isQuiet && 'text-muted-foreground')}>
      <td role="cell" className={cn(leadCellClass, 'col-start-1 row-start-1')}>
        {venue.slug ? (
          <Link
            href={`/venues/${venue.slug}`}
            className="font-medium text-foreground hover:text-primary hover:underline underline-offset-4"
          >
            {venue.name}
          </Link>
        ) : (
          <span className="font-medium">{venue.name}</span>
        )}
        {venue.verified && (
          <>
            {' '}
            <BadgeCheck
              className="inline h-3.5 w-3.5 shrink-0 text-primary align-[-2px]"
              aria-hidden="true"
            />
            <span className="sr-only">Verified room</span>
          </>
        )}
      </td>

      <td
        role="cell"
        className={cn(
          trailCellClass,
          'col-start-1 row-start-2 text-muted-foreground sm:max-w-[14rem]'
        )}
      >
        {/* The clip lives on a block child: `text-overflow` does nothing on an
            auto-layout table cell, which sizes to its content instead. */}
        <span className="block truncate">{place}</span>
      </td>

      <td
        role="cell"
        className={cn(
          trailCellClass,
          'col-start-2 row-start-2 whitespace-nowrap text-right font-mono text-xs sm:text-left sm:text-sm'
        )}
      >
        {when ?? ''}
      </td>

      <td
        role="cell"
        className={cn(
          leadCellClass,
          'col-start-2 row-start-1 whitespace-nowrap text-right'
        )}
      >
        {formatCount(venue.upcoming_show_count)}
        {/* The column header carries the unit at table widths; with no header
            row on mobile the cell has to carry it itself. */}
        <span className="sm:hidden"> upcoming</span>
      </td>

      <td role="cell" className="hidden whitespace-nowrap sm:table-cell">
        {website ? (
          <a
            href={website}
            target="_blank"
            rel="noopener noreferrer"
            data-testid={`venue-site-link-${venue.id}`}
            className="text-primary hover:underline underline-offset-4"
          >
            <span className="sr-only">Website for {venue.name}</span>
            <span aria-hidden="true">site &#8599;</span>
          </a>
        ) : (
          ''
        )}
      </td>
    </tr>
  )
}

/**
 * A column header that applies an order.
 *
 * `aria-sort` is on the header cell rather than the button because the cell is
 * what the column is, and it is only ever set on the ONE column in force:
 * marking every sortable column would tell a reader three orders are applied.
 */
function SortableHeader({
  sortKey,
  label,
  current,
  onSortChange,
  align = 'left',
}: {
  sortKey: VenueSort
  label: string
  current: VenueSort
  onSortChange: (sort: VenueSort) => void
  align?: 'left' | 'right'
}) {
  const isActive = current === sortKey
  return (
    <th
      role="columnheader"
      scope="col"
      aria-sort={isActive ? SORT_DIRECTION[sortKey] : 'none'}
    >
      {/* A flex wrapper rather than `text-right` on the cell: DenseTable sets
          the column alignment through a descendant selector, which outranks a
          utility class on the cell itself. */}
      <span className={cn('flex', align === 'right' && 'justify-end')}>
        <button
          type="button"
          onClick={() => onSortChange(sortKey)}
          data-testid={`venue-sort-header-${sortKey}`}
          className={cn(
            'inline-flex items-center gap-1 uppercase tracking-wider hover:text-foreground',
            'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
            isActive && 'text-primary'
          )}
        >
          {label}
          {isActive && <span aria-hidden="true">&#9662;</span>}
        </button>
      </span>
    </th>
  )
}

/**
 * The quiet-rooms divider.
 *
 * A `<th scope="rowgroup">` rather than the shared `DenseTableGroupHeader`: the
 * frame's register here is a label and a rule, not the filled band that groups
 * a discography, and the label has to say what the column under it now means
 * (the date is the LAST show, not the next).
 */
function QuietRoomsHeader() {
  return (
    <tr role="row" className="block sm:table-row">
      <th
        role="rowheader"
        scope="rowgroup"
        colSpan={COLUMN_COUNT}
        className="block pt-4 sm:table-cell"
        data-testid="venues-quiet-group-header"
      >
        <span className="flex items-center gap-3 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          <span className="whitespace-nowrap">
            Quiet rooms
            <span className="hidden font-normal normal-case tracking-normal sm:inline">
              {' '}
              &middot; no upcoming shows &middot; last show shown instead
            </span>
          </span>
          <span className="h-px min-w-0 flex-1 bg-border" aria-hidden="true" />
        </span>
      </th>
    </tr>
  )
}

/**
 * The directory's rooms.
 *
 * ONE element tree across the breakpoint, reflowed by CSS rather than swapped:
 * a second rendering for narrow widths would put both in the document and make
 * every row's text appear twice to assistive tech and to the tests. At table
 * widths it is a five-column table; below `sm` the header row is dropped and
 * each row becomes a two-line grid, name and count over place and date.
 *
 * Quiet rooms are not filtered or re-sorted here. The API already orders them
 * after every room that has something booked, under every sort value, so the
 * divider is inserted where the first zero-count row falls and the rows keep
 * the order they arrived in. A page that begins inside the quiet block still
 * gets the divider: it is what says why these rows read `last:`.
 */
export function VenueTable({
  venues,
  sort,
  onSortChange,
  showCity,
}: VenueTableProps) {
  const firstQuietIndex = venues.findIndex(v => v.upcoming_show_count === 0)

  return (
    // Explicit roles: below `sm` the display of the table, its groups, rows and
    // cells is overridden, which strips the implicit table semantics with it.
    <DenseTable role="table" variant="alternating" className="block sm:table">
      <thead role="rowgroup" className="hidden sm:table-header-group">
        <tr role="row">
          <SortableHeader
            sortKey="name"
            label="Room"
            current={sort}
            onSortChange={onSortChange}
          />
          <th role="columnheader" scope="col">
            Neighbourhood
          </th>
          <SortableHeader
            sortKey="next"
            label="Next show"
            current={sort}
            onSortChange={onSortChange}
          />
          <SortableHeader
            sortKey="upcoming"
            label="Upcoming shows"
            current={sort}
            onSortChange={onSortChange}
            align="right"
          />
          <th role="columnheader" scope="col">
            Links
          </th>
        </tr>
      </thead>
      <tbody role="rowgroup" className="block sm:table-row-group">
        {venues.map((venue, index) => (
          <Fragment key={venue.id}>
            {index === firstQuietIndex && <QuietRoomsHeader />}
            <VenueRow venue={venue} showCity={showCity} />
          </Fragment>
        ))}
      </tbody>
    </DenseTable>
  )
}
