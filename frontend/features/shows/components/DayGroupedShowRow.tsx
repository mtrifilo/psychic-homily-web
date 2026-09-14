'use client'

import Link from 'next/link'
import { useMemo } from 'react'
import { ExternalLink } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { Density } from '@/lib/hooks/common/useDensity'
// Imported by path, not through the `components/shared` barrel: that barrel is
// ~30 client components and any route reaching it pulls the lot into its module
// graph (the reason `SceneWeekView` imports `ShareButton` the same way).
import { SaveButton } from '@/components/shared/SaveButton'
import { ShowPrice } from '@/components/shared/ShowPrice'
import type { BatchedSaveData } from '@/components/shared/batchedSaveData'
import { formatShowTime } from '@/lib/utils/formatters'
import { SHOW_LIST_FEATURE_POLICY } from './showListFeaturePolicy'
import { ShowStatusBadge } from './ShowStatusBadge'
import { splitBill } from '../utils'
import type { ArtistResponse, ShowResponse } from '../types'

/**
 * The en dash that holds a fixed-width column open when a value is unknown.
 * A blank cell reads as "nobody filled this in"; the dash reads as "there is
 * no value", which is what the row actually knows.
 */
const UNKNOWN = '–'

export interface DayGroupedShowRowProps {
  show: ShowResponse
  density: Density
  isAdmin: boolean
  userId?: string
  /** Forwarded to SaveButton; `'pending'` while the list's batch is in flight. */
  saveData?: BatchedSaveData
  /**
   * Row position inside its day group, for the alternating fill. Parity is per
   * GROUP rather than per page, so each day's block stripes from its own first
   * row the way a table does.
   */
  index: number
  /**
   * Whether the list currently spans more than one city. The venue column
   * appends the city only then: on a single-metro list every row would repeat
   * the same one, which is the column's whole width spent on no information.
   */
  showCity: boolean
}

/**
 * Column widths, read off the approved frame at 1152px of content.
 *
 * Spelled as `sm:` utilities because the row is ONE tree that reflows: below
 * `sm` these cells are a wrapped two-line block and the fixed widths must not
 * apply. The header row and the cells read the same constants, so a width moves
 * in one place.
 */
const COLUMN = {
  time: 'sm:w-[90px]',
  venue: 'sm:w-[260px]',
  price: 'sm:w-[80px]',
  age: 'sm:w-[60px]',
  actions: 'sm:w-[100px]',
} as const

/** Vertical padding per density. The columns themselves do not move. */
const ROW_PADDING: Record<Density, string> = {
  compact: 'py-1 px-2',
  comfortable: 'py-1.5 px-2',
  expanded: 'py-2.5 px-2',
}

/** `w/ Support, Support` on the bill line. */
function SupportText({
  artists,
  className,
}: {
  artists: ArtistResponse[]
  className?: string
}) {
  if (artists.length === 0) return null
  return (
    <span className={cn('text-muted-foreground', className)}>
      w/ {artists.map(artist => artist.name).join(', ')}
    </span>
  )
}

/**
 * One show on the `/shows` list: a dense table row under its day heading.
 *
 * SEPARATE from `ShowCard`, which still serves the home rail and every
 * context list. The two answer different questions. A card repeats the date
 * because it can appear anywhere; these rows sit under a heading that already
 * states the day, so the date column would print the same value on every row
 * of a group. Sharing one component would mean a prop that turns half of it
 * off.
 *
 * The `<article aria-label>` is load-bearing and not decoration: it is how the
 * save and list-action E2E specs address a specific seeded show
 * (`getByRole('article', { name })`).
 */
export function DayGroupedShowRow({
  show,
  density,
  isAdmin,
  userId,
  saveData,
  index,
  showCity,
}: DayGroupedShowRowProps) {
  const { headliners, support } = useMemo(
    () => splitBill(show.artists ?? []),
    [show.artists]
  )

  const startTime = useMemo(
    () =>
      formatShowTime(show.event_date, show.state, show.venues?.[0]?.timezone),
    [show.event_date, show.state, show.venues]
  )

  const venue = show.venues?.[0]
  const detailsHref = `/shows/${show.slug || show.id}`
  const headlinerText =
    headliners.length > 0
      ? headliners.map(artist => artist.name).join(' / ')
      : 'TBA'

  // Expanded gives support its own line under the headliner; the other two
  // keep it inline. Compact drops it entirely, with age, because those are the
  // two the frame collapses first.
  const supportOnOwnLine = density === 'expanded'
  const showSupport = density !== 'compact' && support.length > 0
  const showAge = density !== 'compact'

  const stripe = index % 2 === 0 ? 'bg-muted/20' : undefined

  const metaClass =
    'font-mono text-[11.5px] text-muted-foreground tabular-nums sm:text-[12.5px]'

  return (
    <article
      aria-label={show.title}
      className={cn(
        ROW_PADDING[density],
        stripe,
        'transition-colors hover:bg-muted/40',
        show.is_cancelled && 'opacity-60'
      )}
    >
      {/*
        ONE tree that reflows, not a mobile copy beside a desktop copy. Two
        breakpoint branches would put every row's content in the DOM twice: a
        screen reader reads both, every link counts twice for a crawler, and a
        50-row page carries double the nodes. `sm:contents` is what lets the
        mobile metadata run become three separate columns on desktop without a
        second copy of it.

        Mobile is two lines: the bill takes the first whole (`w-full` forces the
        wrap), and the venue, the metadata and the actions share the second.
        Desktop is the frame's single column row, ordered by `sm:order-*`.
      */}
      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 sm:flex-nowrap">
        <span className="order-1 flex w-full min-w-0 flex-col gap-0.5 sm:order-2 sm:w-auto sm:flex-1">
          <span className="flex flex-wrap items-baseline gap-x-1.5">
            <Link
              href={detailsHref}
              className="text-sm font-medium transition-colors hover:text-primary sm:text-[13.5px]"
            >
              {headlinerText}
            </Link>
            {showSupport && !supportOnOwnLine && (
              <SupportText artists={support} className="text-[12.5px] sm:text-[13px]" />
            )}
            <ShowStatusBadge show={show} className="inline-flex gap-1" />
          </span>
          {showSupport && supportOnOwnLine && (
            <SupportText artists={support} className="text-[12.5px] sm:text-[13px]" />
          )}
        </span>

        <span
          className={cn(
            'order-2 min-w-0 flex-1 truncate text-[12.5px] sm:order-3 sm:flex-none sm:text-[13px]',
            COLUMN.venue
          )}
        >
          {venue?.slug ? (
            <Link
              href={`/venues/${venue.slug}`}
              className="text-primary hover:underline"
            >
              {venue.name}
            </Link>
          ) : (
            <span className="text-muted-foreground">{venue?.name}</span>
          )}
          {showCity && show.city && (
            <span className="text-muted-foreground">
              {' \u00b7 '}
              {show.city}
            </span>
          )}
        </span>

        {/* One run on mobile, three columns on desktop. */}
        <span className="order-3 flex shrink-0 items-baseline gap-1 sm:contents">
          <span className={cn(metaClass, 'shrink-0 sm:order-1', COLUMN.time)}>
            {startTime ?? UNKNOWN}
          </span>
          <span aria-hidden="true" className={cn(metaClass, 'sm:hidden')}>
            {'\u00b7'}
          </span>
          {/* The cell is OURS, not `ShowPrice`'s. With no price to show that
              component returns a bare fragment, so a className handed to it is
              dropped along with the column's width and its `order` — which put
              the fallback dash at the head of the row instead of in the price
              column. */}
          <span className={cn(metaClass, 'shrink-0 sm:order-4', COLUMN.price)}>
            <ShowPrice show={show} fallback={UNKNOWN} />
          </span>
          {showAge && show.age_requirement && (
            <span aria-hidden="true" className={cn(metaClass, 'sm:hidden')}>
              {'\u00b7'}
            </span>
          )}
          {/* `truncate` rather than wrap: an age that outgrows its column
              ("All ages") would otherwise take a second line and make that one
              row taller than every other, breaking the table's rhythm. */}
          <span
            className={cn(
              metaClass,
              'shrink-0 truncate sm:order-5',
              COLUMN.age
            )}
          >
            {showAge && show.age_requirement ? show.age_requirement : null}
          </span>
        </span>

        <span
          className={cn(
            'order-4 flex shrink-0 items-center justify-end gap-1 sm:order-6',
            COLUMN.actions
          )}
        >
          {SHOW_LIST_FEATURE_POLICY.discovery.showSaveButton && (
            <SaveButton
              showId={show.id}
              variant="ghost"
              size="sm"
              saveData={saveData}
            />
          )}
          {SHOW_LIST_FEATURE_POLICY.discovery.showDetailsLink && (
            <Link
              href={detailsHref}
              className="inline-flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-primary"
              aria-label="View show details"
            >
              <ExternalLink className="h-3.5 w-3.5" />
            </Link>
          )}
        </span>
      </div>
    </article>
  )
}

/**
 * The column header row, once above the first day group.
 *
 * `aria-hidden` because these rows are `<article>`s, not a `<table>`: there is
 * no header-to-cell association for a screen reader to follow, so the labels
 * would arrive as five orphan words before the list. Each row carries its own
 * accessible name, and every cell that could be ambiguous out of context (the
 * price pair) states itself.
 */
export function DayGroupedShowListHeader() {
  return (
    <div
      className="hidden items-baseline gap-2 border-b border-border px-2 pb-1 font-mono text-[10px] font-bold uppercase tracking-[0.8px] text-muted-foreground sm:flex"
      aria-hidden="true"
    >
      <span className={cn(COLUMN.time, 'shrink-0')}>Time</span>
      <span className="min-w-0 flex-1">Bill</span>
      <span className={cn(COLUMN.venue, 'shrink-0')}>Venue</span>
      <span className={cn(COLUMN.price, 'shrink-0')}>Price</span>
      <span className={cn(COLUMN.age, 'shrink-0')}>Age</span>
      <span className={cn(COLUMN.actions, 'shrink-0')} />
    </div>
  )
}
