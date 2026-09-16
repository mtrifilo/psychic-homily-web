'use client'

import { useMemo, useState, useSyncExternalStore } from 'react'
import dynamic from 'next/dynamic'
import Link from 'next/link'
import { atlasCityHref } from '@/features/scenes/atlasCityEntry'
import { Skeleton } from '@/components/ui/skeleton'
import type { VenueWithShowCount } from '../types'
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
 * still downloads MapLibre, still holds a WebGL context, and still draws
 * tiles for a reader who will never see it.
 */
const MINI_ATLAS_MIN_VIEWPORT_PX = 1280
const MINI_ATLAS_MEDIA_QUERY = `(min-width: ${MINI_ATLAS_MIN_VIEWPORT_PX}px)`

function subscribeWideViewport(onChange: () => void): () => void {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return () => {}
  }
  const mq = window.matchMedia(MINI_ATLAS_MEDIA_QUERY)
  if (typeof mq.addEventListener === 'function') {
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }
  mq.addListener(onChange)
  return () => mq.removeListener(onChange)
}

function readWideViewport(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false
  }
  return window.matchMedia(MINI_ATLAS_MEDIA_QUERY).matches
}

function readWideViewportOnServer(): boolean {
  return false
}

/**
 * Whether this viewport gets the pane.
 *
 * False on the server and until hydration, which is what keeps the pane out of
 * the server HTML and off every narrow viewport. The table's column is a fixed
 * width at `xl` and the space beside it is already empty there, so the pane
 * arriving after hydration moves nothing: it fills space, it does not take it.
 */
export function useMiniAtlasViewport(): boolean {
  return useSyncExternalStore(
    subscribeWideViewport,
    readWideViewport,
    readWideViewportOnServer,
  )
}

function MiniAtlasSkeleton() {
  return <Skeleton className="absolute inset-0 rounded-md" />
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
  /** The city the page is about, as the facet spells it. */
  cityLabel: string
  city: string
  state: string
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
  cityLabel,
  city,
  state,
  hoveredVenueId,
  onHoverVenue,
  onSelectVenue,
}: VenueMiniAtlasPaneProps) {
  const [ready, setReady] = useState(false)
  const pins = useMemo(() => miniAtlasPins(venues), [venues])

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
          onReady={() => setReady(true)}
        />
        {/* Held over the canvas until the style has painted, so the pane is
            never a flash of empty box. */}
        {!ready && <MiniAtlasSkeleton />}
      </div>
      <p className="sr-only">
        {miniAtlasSummary(pins.length, venues.length, cityLabel)}
      </p>
      <div className="mt-2 text-right">
        <Link
          href={atlasCityHref(city, state)}
          className="inline-flex min-h-11 items-center text-xs text-primary hover:underline underline-offset-4"
          data-testid="venue-mini-atlas-open"
        >
          Open {cityLabel} in the Atlas &#8599;
        </Link>
      </div>
    </aside>
  )
}
