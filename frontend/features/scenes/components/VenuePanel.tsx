'use client'

import { useMemo } from 'react'
import Link from 'next/link'
import { DismissableLayer } from '@radix-ui/react-dismissable-layer'
import { X } from 'lucide-react'
import { Button } from '@/components/ui/button'
// Deep import, not the `@/components/shared` barrel: the barrel drags in every
// shared component (and their AuthContext/router dependencies) for one button,
// and it is the path the Atlas suites already mock.
import { FollowButton } from '@/components/shared/FollowButton'
import { dedupVenueShows } from '@/features/shows'
import { useVenueShows } from '@/features/venues/hooks'
import type { VenueShow, VenueWithShowCount } from '@/features/venues/types'
import { formatPrice, formatShowTime } from '@/lib/utils/formatters'
import { showDisplayTitle } from '@/lib/utils/showDisplayTitle'
import { formatTimeAgo } from '@/lib/formatTimeAgo'
import {
  CITY_VENUE_PANEL_BOTTOM_INSET_PX,
  CITY_VENUE_PANEL_WIDTH_PX,
  VENUE_PANEL_SHOW_ROWS,
  formatPanelShowDate,
  venuePanelIdentityLine,
  venuePanelShowCount,
} from '../cityView'

// Match the venue page's request EXACTLY (same limit, same timezone source).
// `venueQueryKeys.shows()` keys only on venue id + time filter — NOT on limit
// or timezone — so a differently-parameterized request here would share a
// cache entry with `VenueShowsList` and whichever landed first would silently
// answer for both. Same parameters, one cache entry, no collision.
const VENUE_SHOWS_FETCH_LIMIT = 50
const VIEWER_TIMEZONE = Intl.DateTimeFormat().resolvedOptions().timeZone

interface VenuePanelProps {
  /** The selected venue, straight from the rail's already-fetched page. */
  venue: VenueWithShowCount
  onClose: () => void
  /**
   * PSY-1541's artist drill-in seam. A show row is a button, not a link,
   * because the drill-in opens IN the Atlas rather than navigating away; until
   * 1541 lands there is nothing to open, so rows render inert when this is
   * undefined rather than pretending to be actionable.
   */
  onShowSelect?: (show: VenueShow) => void
}

/**
 * The Atlas city view's venue panel (PSY-1540) — "what's coming up here",
 * answered without leaving the map.
 *
 * Non-modal by construction, following the graph inspectors
 * (`GraphPanelShell`): Escape dismisses via Radix `DismissableLayer` so the
 * panel joins the one global layer stack (a ⌘K palette stacked over it still
 * wins Escape), outside-click and focus-out are explicitly prevented so the
 * map stays fully interactive behind it, and there is no focus trap.
 *
 * It floats over the map's RIGHT edge and stops short of the bottom
 * (CITY_VENUE_PANEL_BOTTOM_INSET_PX) so it can never cover the map's
 * bottom-left OpenStreetMap attribution, which the ODbL requires stay visible.
 */
export function VenuePanel({ venue, onClose, onShowSelect }: VenuePanelProps) {
  const { data, isLoading, isError } = useVenueShows({
    venueId: venue.id,
    timezone: VIEWER_TIMEZONE,
    timeFilter: 'upcoming',
    limit: VENUE_SHOWS_FETCH_LIMIT,
  })

  const fetched = data?.shows
  const shows = useMemo(
    () => (fetched ? dedupVenueShows(fetched) : []),
    [fetched],
  )
  const showCount = venuePanelShowCount({
    total: data?.total,
    listed: shows.length,
    fetched: fetched?.length ?? 0,
    limit: VENUE_SHOWS_FETCH_LIMIT,
  })
  const visible = shows.slice(0, VENUE_PANEL_SHOW_ROWS)
  const identity = venuePanelIdentityLine(venue)
  const venueHref = `/venues/${venue.slug || venue.id}`

  return (
    <DismissableLayer
      asChild
      onDismiss={onClose}
      // Pointer/focus dismissal is deliberately off: the whole point of the
      // panel is that you can keep panning and clicking the map beside it, and
      // an outside-click close would slam it shut on the first map drag.
      onPointerDownOutside={(e) => e.preventDefault()}
      onFocusOutside={(e) => e.preventDefault()}
    >
      <section
        aria-label={`${venue.name} — upcoming shows`}
        data-testid="atlas-venue-panel"
        style={{
          width: CITY_VENUE_PANEL_WIDTH_PX,
          bottom: CITY_VENUE_PANEL_BOTTOM_INSET_PX,
        }}
        className="absolute right-3 top-3 z-20 flex max-w-[calc(100%-1.5rem)] flex-col overflow-hidden rounded-md border border-border bg-background/95 shadow-lg backdrop-blur"
      >
        <header className="border-b border-border px-4 pb-3 pt-3">
          <div className="flex items-start justify-between gap-2">
            <p className="font-mono text-[11px] uppercase tracking-wide text-muted-foreground">
              Venue
            </p>
            <button
              type="button"
              onClick={onClose}
              aria-label={`Close ${venue.name} panel`}
              className="-mr-1 -mt-1 shrink-0 rounded-sm p-1 text-muted-foreground transition-colors hover:bg-muted/50 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <X className="h-3.5 w-3.5" aria-hidden="true" />
            </button>
          </div>

          <h2 className="mt-1 text-lg font-semibold leading-tight text-foreground">
            {venue.name}
          </h2>

          {identity && (
            <p
              data-testid="venue-panel-identity"
              className="mt-1 font-mono text-[11px] leading-4 text-muted-foreground"
            >
              {identity}
            </p>
          )}

          <div className="mt-3 flex flex-wrap items-center gap-2">
            <FollowButton entityType="venues" entityId={venue.id} />
            {/* Structure only. Confirming a venue's info — and the edit /
                contributor counts the mock pairs it with — is PSY-1542's
                surface; rendering the control live here would promise a write
                that goes nowhere. */}
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled
              title="Confirming venue info isn’t available yet"
            >
              ✓ Confirm info
            </Button>
          </div>

          {/* Provenance. The timestamp is REAL (the venue row's updated_at).
              The mock's "N edits by M contributors · ingest + community" is
              omitted, not faked — those counts are PSY-1542's, exactly as the
              rail's own provenance footer already handles it. */}
          <p
            data-testid="venue-panel-provenance"
            className="mt-2 font-mono text-[11px] leading-4 text-muted-foreground"
          >
            <span>UPDATED</span>{' '}
            {venue.updated_at ? formatTimeAgo(venue.updated_at) : 'unknown'}
          </p>
        </header>

        <div className="min-h-0 flex-1 overflow-y-auto">
          <h3 className="px-4 pb-1 pt-3 font-mono text-[11px] uppercase tracking-wide text-muted-foreground">
            Upcoming
            {!isLoading && !isError && (
              <> — {showCount} {showCount === 1 ? 'show' : 'shows'}</>
            )}
          </h3>

          {isLoading ? (
            <p className="px-4 py-4 text-sm text-muted-foreground">
              Loading shows…
            </p>
          ) : isError ? (
            <p className="px-4 py-4 text-sm text-destructive">
              Couldn’t load this venue’s shows.
            </p>
          ) : visible.length === 0 ? (
            <p className="px-4 py-4 text-sm text-muted-foreground">
              Nothing on the calendar yet.
            </p>
          ) : (
            <>
              <ul>
                {visible.map((show) => (
                  <li key={show.id}>
                    <ShowRow
                      show={show}
                      venue={venue}
                      onSelect={onShowSelect}
                    />
                  </li>
                ))}
              </ul>
              {showCount > visible.length && (
                <Link
                  href={venueHref}
                  className="block px-4 py-2 font-mono text-[11px] text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
                >
                  view all {showCount} →
                </Link>
              )}
            </>
          )}

          {/* The mock's FIELD NOTES teaser is absent on purpose. Field notes
              are SHOW-scoped by construction — `CreateFieldNote` writes
              entity_type='show', and the generic comment endpoint exposes no
              `kind`, so a venue-scoped field note cannot exist. A read surface
              for rows nothing can write would be a permanently empty section
              dressed as a feature. Whether a venue teaser should roll up the
              notes from shows AT the venue is a product decision, not one to
              guess at here. */}
        </div>

        <footer className="border-t border-border px-4 py-2.5">
          <Link
            href={venueHref}
            className="font-mono text-xs text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            Open venue page →
          </Link>
        </footer>
      </section>
    </DismissableLayer>
  )
}

function ShowRow({
  show,
  venue,
  onSelect,
}: {
  show: VenueShow
  venue: VenueWithShowCount
  onSelect?: (show: VenueShow) => void
}) {
  // Per-show `state` first, venue's as the fallback — the same precedence
  // VenueShowsList uses, so the two surfaces can't disagree about a date.
  const state = show.state ?? venue.state
  const date = formatPanelShowDate(show.event_date, state, venue.timezone)
  const time = formatShowTime(show.event_date, state, venue.timezone)
  const title = showDisplayTitle(
    show.title,
    show.artists.map((a) => a.name),
    { cap: 3 },
  )
  const meta = [time, show.price !== null ? formatPrice(show.price) : null]
    .filter(Boolean)
    .join(' · ')

  const body = (
    <>
      <span className="w-16 shrink-0 pt-px font-mono text-[11px] leading-4 text-primary">
        {date}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm leading-snug text-foreground">
          {title}
        </span>
        {meta && (
          <span className="mt-0.5 block font-mono text-[11px] leading-4 text-muted-foreground">
            {meta}
          </span>
        )}
      </span>
    </>
  )

  if (!onSelect) {
    return (
      <div className="flex gap-3 border-b border-border/60 px-4 py-2.5">
        {body}
      </div>
    )
  }

  return (
    <button
      type="button"
      onClick={() => onSelect(show)}
      className="flex w-full gap-3 border-b border-border/60 px-4 py-2.5 text-left transition-colors hover:bg-muted/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
    >
      {body}
      <span
        aria-hidden="true"
        className="shrink-0 self-center font-mono text-xs text-muted-foreground"
      >
        →
      </span>
    </button>
  )
}
