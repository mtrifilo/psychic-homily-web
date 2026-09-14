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
import { formatShowTimeCompact } from '@/lib/utils/formatters'
import { SHOW_LIST_FEATURE_POLICY } from './showListFeaturePolicy'
import { ShowStatusBadge } from './ShowStatusBadge'
import { splitBill } from '../utils'
import type { ArtistResponse, ShowResponse } from '../types'

/**
 * Holds a fixed-width column open when a value is unknown.
 *
 * `aria-hidden`, so it is a column that looks continuous to a sighted reader
 * and is simply absent to a screen reader. An unlabelled "en dash" announced
 * between the bill and the price is noise, and the row it sits in already says
 * everything that IS known.
 */
function UnknownCell() {
  return <span aria-hidden="true">–</span>
}

export interface DayGroupedShowRowProps {
  show: ShowResponse
  density: Density
  /** Forwarded to SaveButton; `'pending'` while the list's batch is in flight. */
  saveData?: BatchedSaveData
  /**
   * Row position inside its day group, for the alternating fill. Parity is per
   * GROUP rather than per page, so each day's block stripes from its own first
   * row the way a table does.
   */
  index: number
  /**
   * Whether the venue column should name the city.
   *
   * Decided by the FILTER, not by the rows on screen: a page of an All Cities
   * list can happen to hold one metro, and deriving from the rows would make
   * the column appear on page 1 and vanish on page 2 of one filter.
   */
  showCity: boolean
}

/**
 * Column widths, read off the approved frame at 1152px of content.
 *
 * Spelled as `lg:` utilities, which is where the frame's desktop layout starts.
 * Below that the row is a stacked two-line block and the fixed widths must not
 * apply: they total 590px before the bill column gets a pixel, which already
 * exceeds a 640px viewport. The header row and the cells read the same
 * constants, so a width moves in one place.
 */
const COLUMN = {
  time: 'lg:w-[90px]',
  venue: 'lg:w-[260px]',
  price: 'lg:w-[80px]',
  age: 'lg:w-[60px]',
  actions: 'lg:w-[100px]',
} as const

/**
 * Padding per density, and only once the row IS a column row. The frame gives
 * the stacked form a flat 8px whatever the density: what density buys on two
 * lines is nothing, because the lines are already as tight as they read.
 */
const ROW_PADDING: Record<Density, string> = {
  compact: 'p-2 lg:px-2 lg:py-1',
  comfortable: 'p-2 lg:px-2 lg:py-1.5',
  expanded: 'p-2 lg:px-2 lg:py-2.5',
}

/** Age is the first column the frame collapses, and support the second. */
function showsAge(density: Density): boolean {
  return density !== 'compact'
}

/** One billed act, linked to its artist page when it has one. */
function ArtistLink({ artist }: { artist: ArtistResponse }) {
  if (!artist.slug) return <>{artist.name}</>
  return (
    <Link
      href={`/artists/${artist.slug}`}
      className="underline decoration-border underline-offset-4 transition-colors hover:text-primary hover:decoration-primary/50"
    >
      {artist.name}
    </Link>
  )
}

/** `w/ Support, Support`, each act linked. */
function SupportText({
  artists,
  className,
}: {
  artists: ArtistResponse[]
  className?: string
}) {
  if (artists.length === 0) return null
  return (
    <span
      className={cn('text-muted-foreground', className)}
      data-testid="row-support"
    >
      w/{' '}
      {artists.map((artist, index) => (
        <span key={artist.id ?? artist.name}>
          {index > 0 ? ', ' : null}
          <ArtistLink artist={artist} />
        </span>
      ))}
    </span>
  )
}

/**
 * One show on the `/shows` list: a dense table row under its day heading.
 *
 * SEPARATE from `ShowCard`, which still serves the home rail and every context
 * list. The two answer different questions. A card repeats the date because it
 * can appear anywhere; these rows sit under a heading that already states the
 * day, so a date column would print the same value on every row of a group.
 *
 * WHAT THIS ROW DOES NOT CARRY, all of it on `ShowCard` and none of it in the
 * frame this row is built to: the admin edit/export/delete controls, the
 * owner's controls, and the expand-music affordance. `SHOW_LIST_FEATURE_POLICY`
 * still grants all three to `discovery`, and the home rail still renders them
 * through `ShowCard`; only this surface drops them.
 *
 * The `<article aria-label>` is load-bearing and not decoration: it is how the
 * save and list-action E2E specs address a specific seeded show
 * (`getByRole('article', { name })`).
 */
export function DayGroupedShowRow({
  show,
  density,
  saveData,
  index,
  showCity,
}: DayGroupedShowRowProps) {
  const { headliners, support } = useMemo(
    () => splitBill(show.artists ?? []),
    [show.artists]
  )

  // The COMPACT register, which `formatShowTimeCompact` documents as the one
  // for "a fixed-width lead column in a row of columns, where the full
  // register's width is the difference between a legible bill and an ellipsis".
  // This is that column. Both registers refuse a guessed zone by answering null.
  const startTime = useMemo(
    () =>
      formatShowTimeCompact(
        show.event_date,
        show.state,
        show.venues?.[0]?.timezone
      ),
    [show.event_date, show.state, show.venues]
  )

  const venue = show.venues?.[0]
  const detailsHref = `/shows/${show.slug || show.id}`

  const supportOnOwnLine = density === 'expanded'
  const showSupport = density !== 'compact' && support.length > 0
  const showAge = showsAge(density)

  const stripe = index % 2 === 0 ? 'bg-muted/20' : undefined

  const metaClass =
    'font-mono text-[11.5px] text-muted-foreground tabular-nums lg:text-[12.5px]'

  const venueLabel = [venue?.name, showCity ? show.city : null, showCity ? show.state : null]
    .filter(Boolean)
    .join(', ')

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
        ONE tree that reflows, not a stacked copy beside a column copy. Two
        breakpoint branches would put every row's content in the DOM twice: a
        screen reader reads both, every link counts twice for a crawler, and a
        50-row page carries double the nodes. `lg:contents` is what lets the
        stacked metadata run become three separate columns without a second copy
        of it.

        Below `lg` the row is two lines: the bill and the actions share the
        first, and the venue and the metadata share the second. The actions ride
        line 1 so the venue keeps the whole left of line 2, which is the field
        that tells two rows apart and the first thing a narrow viewport clips.
      */}
      {/*
        ONE tree that reflows, not a stacked copy beside a column copy. Two
        breakpoint branches would put every row's content in the DOM twice: a
        screen reader reads both, every link counts twice for a crawler, and a
        50-row page carries double the nodes.

        Below `lg` the row is the frame's two lines, and each is a wrapper here.
        At `lg` every wrapper becomes `contents`, so its children flatten into
        the one column row and take their place from `lg:order-*`. That is what
        lets the stacked metadata run become three separate columns without a
        second copy of it.

        The actions ride line 1 rather than line 2, so the venue keeps the whole
        left of its own line: it is the field that tells two rows apart and the
        first thing a narrow viewport clips.
      */}
      <div className="flex flex-col gap-y-0.5 lg:flex-row lg:items-baseline lg:gap-x-2">
        <span className="flex w-full min-w-0 items-baseline gap-x-2 lg:contents">
          <span className="flex min-w-0 flex-1 flex-col gap-0.5 lg:order-2">
            <span className="flex min-w-0 flex-wrap items-baseline gap-x-1.5">
              <Link
                href={detailsHref}
                className="min-w-0 truncate text-sm font-medium transition-colors hover:text-primary lg:text-[13.5px]"
              >
                {headliners.length > 0
                  ? headliners.map(artist => artist.name).join(' / ')
                  : 'TBA'}
              </Link>
              {showSupport && !supportOnOwnLine && (
                <SupportText
                  artists={support}
                  className="min-w-0 text-[12.5px] lg:text-[13px]"
                />
              )}
              <ShowStatusBadge
                show={show}
                cancelledVariant="default"
                className="inline-flex gap-1"
              />
            </span>
            {showSupport && supportOnOwnLine && (
              <SupportText
                artists={support}
                className="text-[12.5px] lg:text-[13px]"
              />
            )}
          </span>

          <span
            className={cn(
              'flex shrink-0 items-center justify-end gap-1 lg:order-6',
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
        </span>

        <span className="flex w-full min-w-0 items-baseline gap-x-2 lg:contents">
          <span
            className={cn(
              'min-w-0 flex-1 truncate text-[12.5px] lg:order-3 lg:flex-none lg:text-[13px]',
              COLUMN.venue
            )}
            // Truncation hides the tail with no other way to read it.
            // `ShowPrice` sets a title for the same reason.
            title={venueLabel || undefined}
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
            {/* City AND state. Two metros can share a name across state lines,
                so the city alone names nothing on a list that spans them. */}
            {showCity && show.city && (
              <span className="text-muted-foreground">
                {' \u00b7 '}
                {[show.city, show.state].filter(Boolean).join(', ')}
              </span>
            )}
          </span>

          <span className="flex shrink-0 items-baseline gap-1 lg:contents">
            <span className={cn(metaClass, 'shrink-0 lg:order-1', COLUMN.time)}>
              {startTime ?? <UnknownCell />}
            </span>
            <span aria-hidden="true" className={cn(metaClass, 'lg:hidden')}>
              {'\u00b7'}
            </span>
            {/* The cell is OURS, not `ShowPrice`'s. With no price to show that
                component returns a bare fragment, so a className handed to it is
                dropped along with the column's width and its `order`, which put
                the fallback dash at the head of the row instead of in the price
                column. */}
            <span className={cn(metaClass, 'shrink-0 lg:order-4', COLUMN.price)}>
              <ShowPrice show={show} fallback={<UnknownCell />} />
            </span>
            {showAge && show.age_requirement && (
              <span aria-hidden="true" className={cn(metaClass, 'lg:hidden')}>
                {'\u00b7'}
              </span>
            )}
            {/* `truncate` rather than wrap: an age that outgrows its column
                ("All ages") would otherwise take a second line and make that one
                row taller than every other, breaking the table's rhythm. */}
            <span
              className={cn(metaClass, 'shrink truncate lg:order-5', COLUMN.age)}
            >
              {showAge && show.age_requirement ? show.age_requirement : null}
            </span>
          </span>
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
 *
 * Takes the density because the row does: a compact list renders no age, and a
 * label over fifty empty cells is a column that is not there.
 */
export function DayGroupedShowListHeader({ density }: { density: Density }) {
  return (
    <div
      className="hidden items-baseline gap-2 border-b border-border px-2 pb-1 font-mono text-[10px] font-bold uppercase tracking-[0.8px] text-muted-foreground lg:flex"
      aria-hidden="true"
      data-testid="show-list-header"
    >
      <span className={cn(COLUMN.time, 'shrink-0')}>Time</span>
      <span className="min-w-0 flex-1">Bill</span>
      <span className={cn(COLUMN.venue, 'shrink-0')}>Venue</span>
      <span className={cn(COLUMN.price, 'shrink-0')}>Price</span>
      <span className={cn(COLUMN.age, 'shrink-0')}>
        {showsAge(density) ? 'Age' : null}
      </span>
      <span className={cn(COLUMN.actions, 'shrink-0')} />
    </div>
  )
}
