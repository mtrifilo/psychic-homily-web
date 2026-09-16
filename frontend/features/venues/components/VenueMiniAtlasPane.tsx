'use client'

import { useMemo } from 'react'
import dynamic from 'next/dynamic'
import Link from 'next/link'
import { atlasCityHref } from '@/features/scenes/atlasCityEntry'
import type { CityState } from '@/components/filters'
import { cityLabel } from '@/components/filters/cityParams'
import { useMediaQuery } from '@/lib/hooks/common/useMediaQuery'
import type { VenueWithShowCount } from '../types'
import { MiniAtlasSkeleton } from './MiniAtlasSkeleton'
import {
  MINI_ATLAS_HEIGHT_PX,
  MINI_ATLAS_WIDTH_PX,
  miniAtlasPins,
  miniAtlasSummary,
} from '../venueMiniAtlas'

/**
 * The width the pane appears at, matching Tailwind's `xl`.
 *
 * Below it the pane is not rendered at all rather than hidden: a hidden map
 * still downloads MapLibre, still holds a WebGL context, and still draws tiles
 * for a reader who will never see it.
 */
const MINI_ATLAS_MIN_VIEWPORT_PX = 1280
const MINI_ATLAS_MEDIA_QUERY = `(min-width: ${MINI_ATLAS_MIN_VIEWPORT_PX}px)`

/**
 * Whether this viewport gets the pane.
 *
 * False on the server and until hydration, which is what keeps the pane out of
 * the server HTML and off every narrow viewport. The table's column is capped
 * at `xl` and the space beside it is already empty there, so the pane arriving
 * after hydration moves nothing: it fills space, it does not take it.
 */
export function useMiniAtlasViewport(): boolean {
  return useMediaQuery(MINI_ATLAS_MEDIA_QUERY)
}

function MiniAtlasLoadError({ onRetry }: { onRetry?: () => void }) {
  return (
    <div
      role="alert"
      className="absolute inset-0 flex flex-col items-center justify-center gap-2 p-4 text-center text-xs text-muted-foreground"
    >
      <p>The map couldn&apos;t load. Every room is in the table.</p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="text-primary underline-offset-4 hover:underline"
        >
          Try again
        </button>
      )}
    </div>
  )
}

// MapLibre is a ~900 kB chunk and window-bound. `ssr: false` plus this being
// the only import path keeps it out of the /venues initial JS: it is fetched
// when the pane mounts, which only happens at 1280 and up.
//
// next/dynamic re-invokes `loading` with `error`/`retry` set on a failed chunk
// fetch (it does NOT throw to an error boundary) — without that branch a
// rotated hashed chunk would strand the reader on a skeleton forever.
const VenueMiniAtlas = dynamic(() => import('./VenueMiniAtlas'), {
  ssr: false,
  loading: ({ error, retry }) =>
    error ? <MiniAtlasLoadError onRetry={retry} /> : <MiniAtlasSkeleton />,
})

export interface VenueMiniAtlasPaneProps {
  /** This page's rows, in the order the table lists them. */
  venues: readonly VenueWithShowCount[]
  /** The ONE city the page is about, as the facet spells it. */
  scopeCity: CityState
  /** The room under the pointer, shared with the table. */
  hoveredVenueId: number | null
  onHoverVenue: (venueId: number | null) => void
  /** Pin click: the page scrolls that room's row into view and focuses it. */
  onSelectVenue: (venueId: number) => void
}

/**
 * The map pane beside the city table at 1280 and up.
 *
 * Its box is the frame's exact size and is rendered before the map arrives, so
 * the lazy chunk lands inside a space that is already the right shape.
 */
export function VenueMiniAtlasPane({
  venues,
  scopeCity,
  hoveredVenueId,
  onHoverVenue,
  onSelectVenue,
}: VenueMiniAtlasPaneProps) {
  const pins = useMemo(() => miniAtlasPins(venues), [venues])
  const label = cityLabel(scopeCity)

  // No coordinates anywhere in this page of rooms: an empty street map says
  // nothing the table does not, so the pane stays away rather than reserving
  // space for a blank.
  if (pins.length === 0) return null

  return (
    <aside
      data-testid="venue-mini-atlas"
      className="shrink-0"
      style={{ width: MINI_ATLAS_WIDTH_PX }}
    >
      <div
        className="relative overflow-hidden rounded-md border border-border bg-muted/20"
        style={{ height: MINI_ATLAS_HEIGHT_PX }}
      >
        <VenueMiniAtlas
          pins={pins}
          hoveredVenueId={hoveredVenueId}
          onHoverVenue={onHoverVenue}
          onSelectVenue={onSelectVenue}
        />
      </div>
      <p className="sr-only">
        {miniAtlasSummary(pins.length, venues.length, label)}
      </p>
      <div className="mt-2 text-right">
        <Link
          href={atlasCityHref(scopeCity.city, scopeCity.state)}
          className="inline-flex min-h-11 items-center text-xs text-primary hover:underline underline-offset-4"
          data-testid="venue-mini-atlas-open"
        >
          Open {label} in the Atlas &#8599;
        </Link>
      </div>
    </aside>
  )
}
