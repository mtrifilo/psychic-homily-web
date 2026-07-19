'use client'

/**
 * VenueBillNetwork (PSY-365)
 *
 * Section wrapper for the venue-rooted co-bill graph: header, time-window
 * filter, fullscreen overlay, and the canvas. Mirrors `SceneGraph.tsx`'s
 * shape so future graph patches (PSY-516, PSY-517, PSY-518, PSY-519
 * inheritance) can be applied symmetrically across both surfaces.
 *
 * Mobile-gated below the Tailwind `sm` breakpoint (640px) per PSY-369→511.
 * Sparse-venue empty state mirrors the scene graph: the section returns
 * `null` when the venue has fewer than `MIN_GRAPH_SHOWS` approved shows OR
 * fewer than `MIN_GRAPH_NODES` connected artists in the active window.
 *
 * Time-window filter (PSY-365 ticket scope):
 *   - All-time (default — matches the scene graph's all-approved precedent
 *     and the orchestrator's settled decision)
 *   - Last 12 months (rolling)
 *   - By year (with a year picker)
 *
 * Click semantics (locked grammar, PSY-1451): clicking a node SELECTS into
 * the shared ArtistContextPanel; navigation happens only via the panel's
 * "Open page →". The selection wiring lives in the adapter
 * (`VenueBillNetworkAdapter.tsx`), so the panel mounts inside whichever
 * container renders the canvas — inline section or fullscreen overlay.
 */

import { useState, useMemo } from 'react'
import { Maximize2, X } from 'lucide-react'
import { GraphSkeleton } from '@/components/graph/GraphSkeleton'
import {
  GraphStateCard,
  GRAPH_BOX_HEIGHT_CLASS,
  GRAPH_TEASER_HEIGHT_CLASS,
} from '@/components/graph/GraphStateCard'
import { useContainerWidth, GRAPH_BREAKPOINT_PX } from '@/components/graph/useContainerWidth'
import { useFullscreenGraphOverlay } from '@/components/graph/useFullscreenGraphOverlay'
import { truncatedCountPhrase, sentenceCase } from '@/components/graph/truncatedCountPhrase'
import { useVenueBillNetwork } from '../hooks/useVenues'
import type { VenueBillNetworkWindow } from '../types'
import { SceneGraphVisualizationStyleAdapter } from './VenueBillNetworkAdapter'

const MIN_GRAPH_NODES = 3 // mirror SceneGraph — under 3 connected artists is too sparse
const MIN_GRAPH_SHOWS = 10 // PSY-365 ticket: empty state for "<10 shows at the venue"

/**
 * The scroll-anchor id for the mobile teaser's "Browse shows" link-out
 * (PSY-1472). Single-sourced here so this component's `linkHref` and the
 * VenueDetail wrapper's `id` can't drift apart.
 */
export const VENUE_SHOWS_ANCHOR = 'venue-shows'

// ------------------------------------------------------------------
// Year picker bounds
// ------------------------------------------------------------------
// Lower bound: the platform doesn't have meaningful pre-2010 data; capping
// the dropdown prevents the user from selecting a year that always returns
// zero events. Upper bound: current year + 1 to allow future-dated shows.
const YEAR_PICKER_FIRST = 2010

interface VenueBillNetworkProps {
  /** Venue ID or slug — passed straight to the API and the cache key. */
  venueIdOrSlug: string | number
  /** Venue display name, used in the header copy. */
  venueName: string
}

export function VenueBillNetwork({ venueIdOrSlug, venueName }: VenueBillNetworkProps) {
  const [window, setWindowState] = useState<VenueBillNetworkWindow>('all')
  const [yearSelection, setYearSelection] = useState<number>(() => new Date().getFullYear())
  const { refCallback: containerRefCallback, containerWidth } = useContainerWidth()

  const { data, isLoading, isError } = useVenueBillNetwork({
    venueIdOrSlug,
    window,
    year: window === 'year' ? yearSelection : undefined,
    enabled: Boolean(venueIdOrSlug),
  })

  // Pre-compute years for the picker. Stable across renders so the <select>
  // doesn't churn its option DOM on every parent re-render.
  const yearOptions = useMemo(() => {
    const currentYear = new Date().getFullYear()
    const years: number[] = []
    for (let y = currentYear + 1; y >= YEAR_PICKER_FIRST; y--) {
      years.push(y)
    }
    return years
  }, [])

  const isolateCount = useMemo(() => {
    if (!data) return 0
    return data.nodes.reduce((n, node) => (node.is_isolate ? n + 1 : n), 0)
  }, [data])

  const nodeCount = data?.nodes.length ?? 0
  const edgeCount = data?.venue.edge_count ?? 0
  const showCount = data?.venue.show_count ?? 0

  // Sparse-venue / sparse-window empty state. Two gates:
  //   1. Venue has < 10 approved shows in the active window — too thin for
  //      the network to be informative (PSY-365 acceptance criterion).
  //   2. Connected-artist count below MIN_GRAPH_NODES — same threshold as
  //      the SceneGraph empty state so the gating is symmetrical.
  const connectedNodeCount = nodeCount - isolateCount
  const tooSparse = showCount < MIN_GRAPH_SHOWS || connectedNodeCount < MIN_GRAPH_NODES

  // Mobile gating: < sm breakpoint hides the canvas. `containerWidth === null`
  // (pre-measurement) also gates off so we never flash an oversized canvas.
  const graphAvailable =
    !tooSparse && containerWidth !== null && containerWidth >= GRAPH_BREAKPOINT_PX

  // Overlay lifecycle (scroll lock, Esc, viewport tracking, auto-close when
  // graphAvailable flips false mid-overlay) lives in the shared hook.
  const {
    isFullscreen,
    open: openFullscreen,
    close: closeFullscreen,
    overlayWidth,
    overlayHeight,
  } = useFullscreenGraphOverlay(graphAvailable)

  // Time-window filter — three radio-style buttons + the year picker.
  // Inline keeps the markup co-located with the `setWindow` handler; the
  // toggle behavior is small enough that pulling it into a sub-component
  // would be premature. Defined before the loading/error guards because the
  // error state below needs it (the filter is the user's path back to a
  // window that worked — same stranding rationale as SceneGraph's toggle).
  const windowFilter = (
    <div className="flex flex-wrap items-center gap-2 text-xs">
      <span className="text-muted-foreground" aria-hidden="true">
        Window:
      </span>
      {(['all', '12m', 'year'] as VenueBillNetworkWindow[]).map(w => {
        const isActive = window === w
        const label = w === 'all' ? 'All-time' : w === '12m' ? 'Last 12 months' : 'By year'
        return (
          <button
            key={w}
            type="button"
            onClick={() => setWindowState(w)}
            aria-pressed={isActive}
            className={`px-2 py-0.5 rounded-full border text-xs transition-colors ${
              isActive
                ? 'bg-primary/10 border-primary text-primary'
                : 'border-border/60 text-muted-foreground hover:bg-muted/50'
            }`}
          >
            {label}
          </button>
        )
      })}
      {window === 'year' && (
        <select
          value={yearSelection}
          onChange={e => setYearSelection(Number(e.target.value))}
          aria-label="Select year"
          className="px-2 py-0.5 rounded-md border border-border/60 bg-background text-foreground text-xs"
        >
          {yearOptions.map(y => (
            <option key={y} value={y}>
              {y}
            </option>
          ))}
        </select>
      )}
    </div>
  )

  // Loading reserves the graph box (shared GraphSkeleton, PSY-1347) instead
  // of returning null — a null here shifts every section below when the
  // canvas lands. keepPreviousData means this only fires on the initial load.
  if (isLoading) {
    return (
      <div className="mt-8 px-4 md:px-0">
        <h2 className="text-lg font-semibold mb-2">Who plays together here</h2>
        <GraphSkeleton className={GRAPH_BOX_HEIGHT_CLASS} />
      </div>
    )
  }

  // A settled fetch error leaves `data` undefined (keepPreviousData only
  // bridges the pending window). Keep the section shell + window filter
  // rendered — the filter is the user's path back to a window that worked —
  // with a visible notice instead of vanishing (scene-page convention).
  if (!data) {
    if (isError) {
      return (
        <div className="mt-8 px-4 md:px-0">
          <h2 className="text-lg font-semibold mb-2">Who plays together here</h2>
          <div className="space-y-2">
            {windowFilter}
            <GraphStateCard
              role="alert"
              message="This view couldn't load. Try a different window above."
            />
          </div>
        </div>
      )
    }
    return null
  }

  // Mobile + sparse: render nothing visible, but do NOT `return null` here.
  // Returning null would unmount the `useContainerWidth` ref node, and the
  // hook's cleanup resets the measured width to null on unmount — so the node
  // would remount → remeasure (< breakpoint) → return null → unmount … in an
  // infinite loop (React #185 "Maximum update depth exceeded"). It only
  // reproduced on mobile: this branch needs a sub-breakpoint measured width,
  // and the graph canvas never renders below the breakpoint, so desktop never
  // reached it. Instead we keep a stable, zero-height measuring wrapper
  // mounted (see the return below) and gate the section CONTENT on this flag.
  // This matches the six peer graph surfaces (SceneGraph, StationGraph,
  // CollectionGraph, InlineGraph, HomeSceneGraph, BillComposition): the ref
  // node lives for the component's lifetime; only its children are width-gated.
  // Desktop still shows the header + filter (below) so the user understands
  // WHY the graph isn't drawing and can re-scope the window.
  const hideSection =
    containerWidth !== null && containerWidth < GRAPH_BREAKPOINT_PX && tooSparse

  // PSY-1476: a capped roster must say so — "150 artists" on a 312-artist
  // venue reads as the whole history. Mirrors the scene graph's shipped
  // treatment (sceneArtistCountPhrase → truncatedCountPhrase): the leading
  // count becomes "top N of M artists" when roster_truncated, sentence-cased
  // here (a digit-leading plain count is a no-op). Reads the contract field
  // `artist_count` (which the backend guarantees equals len(nodes)) — the
  // field `artist_total`/`roster_truncated` are defined against (both added by
  // #1563). Computed ONCE here and threaded to the adapter as `countPhrase`,
  // so the header and the canvas aria-label read one value and can't diverge.
  const { phrase: artistCountPhrase } = truncatedCountPhrase({
    shown: data?.venue.artist_count ?? 0,
    total: data?.venue.artist_total,
    truncated: data?.venue.roster_truncated,
    singular: 'artist',
    plural: 'artists',
  })
  const artistPhrase = sentenceCase(artistCountPhrase)

  const sectionHeader = (
    <div>
      <h2 className="text-lg font-semibold">Who plays together here</h2>
      <p className="text-sm text-muted-foreground">
        {artistPhrase}
        {edgeCount > 0 && (
          <>
            {' · '}
            {edgeCount} {edgeCount === 1 ? 'co-bill' : 'co-bills'}
          </>
        )}
        {isolateCount > 0 && (
          <>
            {' · '}
            {isolateCount} unconnected
          </>
        )}
      </p>
    </div>
  )

  const expandButton = graphAvailable && !isFullscreen && (
    <button
      type="button"
      onClick={openFullscreen}
      className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-md border border-border/60 hover:bg-muted/50 transition-colors"
      aria-label="Expand venue bill network to fullscreen"
    >
      <Maximize2 className="h-4 w-4" aria-hidden="true" />
      <span>Expand</span>
    </button>
  )

  return (
    <>
      {/* Measuring wrapper — ALWAYS mounted so the useContainerWidth ref never
          detaches (see the `hideSection` note above). Its box is intentionally
          bare (no padding/margin) so the measured width is a pure function of
          the grid track, never of `hideSection`; a state-dependent box could
          straddle the breakpoint and reintroduce the measure↔gate loop.
          PSY-1034/PSY-949: `min-w-0` is load-bearing here — the ResizeObserver
          feeds THIS element's measured width to the graph as an explicit pixel
          width. Without it the canvas's min-content width can push this node
          wider than its grid track, re-firing the RO with a larger width and
          ballooning the layout. Do NOT remove it. aria-hidden/inert (PSY-517):
          while the fullscreen overlay is open the inline copy is hidden from
          assistive tech so the overlay is the single graph surface. */}
      <div
        ref={containerRefCallback}
        className="min-w-0"
        aria-hidden={isFullscreen || undefined}
        inert={isFullscreen || undefined}
      >
        {!hideSection && (
          <div
            // PSY-366: `id="graph"` enables Cmd+K deep-links from the command
            // palette (`/venues/{slug}#graph`). `scroll-mt-20` accounts for the
            // sticky header on the entity layout. `min-w-0` mirrors the wrapper
            // as a belt-and-suspenders cap on the canvas's min-content width.
            id="graph"
            className="mt-8 px-4 md:px-0 scroll-mt-20 min-w-0"
          >
            <div className="flex flex-wrap items-center justify-between gap-2 mb-2">
              {sectionHeader}
              {expandButton}
            </div>

            <div className="mb-3">{windowFilter}</div>

            {tooSparse && containerWidth !== null && containerWidth >= GRAPH_BREAKPOINT_PX && (
              <p className="text-sm text-muted-foreground mt-2">
                Not enough booked-together activity yet to draw the network. Try a
                wider window or check back as more shows are approved.
              </p>
            )}

            {/* Pre-measurement: hold the box height so the settle can't
                shift the sections below (HomeSceneGraph precedent). */}
            {!tooSparse && containerWidth === null && (
              <GraphSkeleton className={`mt-2 ${GRAPH_BOX_HEIGHT_CLASS}`} />
            )}

            {/* Sub-640px, non-sparse: shared teaser card (PSY-1446) — says WHY
                + gives a way forward (PSY-1472). Link-out scrolls to the
                venue's show list on this page (#venue-shows, VenueDetail). */}
            {!tooSparse &&
              containerWidth !== null &&
              containerWidth < GRAPH_BREAKPOINT_PX && (
                <GraphStateCard
                  className={`mt-2 ${GRAPH_TEASER_HEIGHT_CLASS}`}
                  message={`Who plays ${venueName} together, mapped by shared bills here. Needs a larger screen.`}
                  linkHref={`#${VENUE_SHOWS_ANCHOR}`}
                  linkLabel={`Browse shows at ${venueName} →`}
                />
              )}

            {graphAvailable && !isFullscreen && (
              <div className="space-y-3 mt-2">
                <SceneGraphVisualizationStyleAdapter
                  data={data}
                  venueName={venueName}
                  countPhrase={artistCountPhrase}
                  containerWidth={containerWidth!}
                />

                <p className="text-xs text-muted-foreground">
                  Showing artists who&apos;ve played approved shows at {venueName}. Edge weight =
                  shared shows AT THIS VENUE in the active window. Click any artist for their
                  details.
                </p>
              </div>
            )}
          </div>
        )}
      </div>

      {isFullscreen && graphAvailable && (
        <div
          role="dialog"
          aria-modal="true"
          aria-label={`Venue bill network for ${venueName}, fullscreen`}
          className="fixed inset-0 z-[60] bg-background flex flex-col"
          data-testid="venue-bill-network-overlay"
        >
          <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-3 border-b border-border/50">
            {sectionHeader}
            <button
              type="button"
              onClick={closeFullscreen}
              className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-md border border-border/60 hover:bg-muted/50 transition-colors"
              aria-label="Exit fullscreen venue bill network"
            >
              <X className="h-4 w-4" aria-hidden="true" />
              <span>Exit</span>
            </button>
          </div>

          <div className="px-4 py-2 border-b border-border/30">{windowFilter}</div>

          <div className="flex-1 min-h-0 px-4 py-2">
            {overlayHeight !== null && overlayWidth !== null && (
              <SceneGraphVisualizationStyleAdapter
                data={data}
                venueName={venueName}
                countPhrase={artistCountPhrase}
                containerWidth={overlayWidth}
                height={overlayHeight}
              />
            )}
          </div>
        </div>
      )}
    </>
  )
}
