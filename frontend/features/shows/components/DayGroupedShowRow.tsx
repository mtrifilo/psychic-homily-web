'use client'

import Link from 'next/link'
import { useMemo, useState } from 'react'
import {
  ChevronDown,
  ChevronUp,
  ExternalLink,
  Pencil,
  Trash2,
  X,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import type { Density } from '@/lib/hooks/common/useDensity'
import { Button } from '@/components/ui/button'
import { replayOnHydrate } from '@/lib/hydration/clickReplay'
import { useAuthContext } from '@/lib/context/AuthContext'
// Imported by path, not through the `components/shared` barrel: that barrel is
// ~30 client components and any route reaching it pulls the lot into its module
// graph (the reason `SceneWeekView` imports `ShareButton` the same way).
import { SaveButton } from '@/components/shared/SaveButton'
import { ShowPrice } from '@/components/shared/ShowPrice'
import type { BatchedSaveData } from '@/components/shared/batchedSaveData'
import { formatShowTimeCompact } from '@/lib/utils/formatters'
import { SHOW_LIST_FEATURE_POLICY } from './showListFeaturePolicy'
import { DeleteShowDialog } from './DeleteShowDialog'
import { ExportShowButton } from './ExportShowButton'
import { ShowArtistMusicPanel, showHasArtistMusic } from './ShowArtistMusic'
import { ShowForm } from './ShowForm'
import { ShowStatusBadge } from './ShowStatusBadge'
import { canDeleteShow, splitBill } from '../utils'
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
  /** Gates the admin controls, exactly as it does on `ShowCard`. */
  isAdmin: boolean
  /** The viewer, for the owner's delete control. */
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
  actions: 'lg:min-w-[100px]',
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
 * The ACTIONS column carries exactly what `ShowCard` carries for the same
 * viewer, gated on the same `SHOW_LIST_FEATURE_POLICY.discovery` flags: save,
 * outbound, expand-music, and the admin and owner controls. The frame draws
 * that column simplified; it was never a decision to remove capability, and the
 * open players behind the expand control are a locked decision. The column
 * wraps rather than dropping anything.
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
  const { user } = useAuthContext()
  // DERIVED from the density, not seeded from it. `useDensity` reads
  // localStorage through a server snapshot, so it is always 'comfortable' on
  // the server and the hydration render, and this row first mounts on the
  // server: `useState(density === 'expanded')` would latch `false` and never
  // re-run, so a viewer whose stored density is 'expanded' would silently lose
  // the auto-opened music for the whole session. The override is the reader's
  // own toggle, which outranks the preference. Same shape as `ShowCard`.
  const [expandOverride, setExpandOverride] = useState<boolean | null>(null)
  const [isEditing, setIsEditing] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)

  const artists = useMemo(() => show.artists ?? [], [show.artists])
  const { headliners, support } = useMemo(() => splitBill(artists), [artists])
  const hasArtistMusic = useMemo(
    () => showHasArtistMusic(artists),
    [artists]
  )

  const resolvedUserId = userId || user?.id
  const canDelete = canDeleteShow({
    submittedBy: show.submitted_by,
    viewerId: resolvedUserId,
    isAdmin,
  })

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

  const isExpanded = expandOverride ?? density === 'expanded'
  const setIsExpanded = setExpandOverride

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

          {/* A MINIMUM width, not a cap. The frame drew this column for two
              controls; an admin carries five, six where the dev-only export
              button renders. At a fixed width they would wrap to a second line
              inside the box and make admin rows taller than the rest, which
              `lg:items-baseline` would then align to the first line. */}
          <span
            className={cn(
              'flex shrink-0 items-center justify-end gap-0.5 lg:order-6',
              COLUMN.actions
            )}
          >
            {SHOW_LIST_FEATURE_POLICY.discovery.showExpandMusic &&
              hasArtistMusic && (
                <Button
                  // In server HTML since PSY-1624: rows paint before hydration,
                  // so this is clickable while still dead without the replay
                  // root.
                  {...replayOnHydrate}
                  variant="ghost"
                  size="sm"
                  onClick={() => setIsExpanded(!isExpanded)}
                  className="h-7 w-7 p-0"
                  aria-label={
                    isExpanded ? 'Hide artist music' : 'Discover artist music'
                  }
                >
                  {isExpanded ? (
                    <ChevronUp className="h-4 w-4" />
                  ) : (
                    <ChevronDown className="h-4 w-4" />
                  )}
                </Button>
              )}

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

            {SHOW_LIST_FEATURE_POLICY.discovery.showAdminActions && isAdmin && (
              <Button
                variant={isEditing ? 'secondary' : 'ghost'}
                size="sm"
                onClick={() => setIsEditing(!isEditing)}
                className="h-7 w-7 p-0"
                aria-label={isEditing ? 'Cancel editing' : 'Edit show'}
              >
                {isEditing ? (
                  <X className="h-4 w-4" />
                ) : (
                  <Pencil className="h-3.5 w-3.5" />
                )}
              </Button>
            )}

            {SHOW_LIST_FEATURE_POLICY.discovery.showAdminActions && isAdmin && (
              <ExportShowButton
                showId={show.id}
                showTitle={show.title}
                variant="ghost"
                size="sm"
                className="h-7 w-7 p-0"
                iconOnly
              />
            )}

            {SHOW_LIST_FEATURE_POLICY.discovery.showOwnerActions && canDelete && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setIsDeleteDialogOpen(true)}
                className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
                aria-label="Delete show"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
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

      {isExpanded && hasArtistMusic && (
        <ShowArtistMusicPanel
          artists={artists}
          className="mt-3 border-t border-border/50 pt-3"
        />
      )}

      {isEditing && (
        <div className="mt-3 border-t border-border/50 pt-3">
          <ShowForm
            mode="edit"
            initialData={show}
            onSuccess={() => setIsEditing(false)}
            onCancel={() => setIsEditing(false)}
          />
        </div>
      )}

      {/* Mounted only for a viewer who can open it. `DeleteShowDialog` calls
          `useShowDelete` at mount, so rendering it unconditionally would put 50
          mutations and 50 dialog roots on a page for readers who can never use
          one. */}
      {canDelete && (
        <DeleteShowDialog
          show={show}
          open={isDeleteDialogOpen}
          onOpenChange={setIsDeleteDialogOpen}
        />
      )}
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
