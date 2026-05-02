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
 * Click semantics (PSY-361 inheritance): clicking a node navigates to that
 * artist's page, which is itself the entry point to their global graph
 * with the recentering breadcrumb. The "exit to broader exploration" wiring
 * is therefore on the artist page, not duplicated here.
 */

import { useState, useCallback, useMemo, useEffect } from 'react'
import { Maximize2, X } from 'lucide-react'
import { useVenueBillNetwork } from '../hooks/useVenues'
import type { VenueBillNetworkWindow } from '../types'
import { SceneGraphVisualizationStyleAdapter } from './VenueBillNetworkAdapter'

const GRAPH_BREAKPOINT_PX = 640
const MIN_GRAPH_NODES = 3 // mirror SceneGraph — under 3 connected artists is too sparse
const MIN_GRAPH_SHOWS = 10 // PSY-365 ticket: empty state for "<10 shows at the venue"
const OVERLAY_VERTICAL_RESERVE_PX = 140 // matches SceneGraph

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
  const [containerWidth, setContainerWidth] = useState<number | null>(null)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [overlayHeight, setOverlayHeight] = useState<number | null>(null)
  const [overlayWidth, setOverlayWidth] = useState<number | null>(null)

  const { data, isLoading } = useVenueBillNetwork({
    venueIdOrSlug,
    window,
    year: window === 'year' ? yearSelection : undefined,
    enabled: Boolean(venueIdOrSlug),
  })

  // Callback ref pattern (PSY-516 / PSY-519): a `useRef + useEffect[]` shape
  // silently fails when the first render returns null (data still loading).
  // The callback ref fires whenever the underlying DOM node mounts/unmounts,
  // so we always measure the right node regardless of conditional returns.
  const containerRefCallback = useCallback((node: HTMLDivElement | null) => {
    if (!node) return
    setContainerWidth(node.getBoundingClientRect().width)
    const observer = new ResizeObserver(entries => {
      for (const entry of entries) {
        setContainerWidth(entry.contentRect.width)
      }
    })
    observer.observe(node)
    return () => observer.disconnect()
  }, [])

  // Overlay-mode side effects (mirror SceneGraph): body scroll lock, Esc
  // close, and live viewport-size listener for the canvas.
  useEffect(() => {
    if (!isFullscreen) return

    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'

    const updateDimensions = () => {
      setOverlayWidth(globalThis.window.innerWidth)
      setOverlayHeight(
        Math.max(200, globalThis.window.innerHeight - OVERLAY_VERTICAL_RESERVE_PX),
      )
    }
    updateDimensions()

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setIsFullscreen(false)
      }
    }

    document.addEventListener('keydown', handleKeyDown)
    globalThis.window.addEventListener('resize', updateDimensions)

    return () => {
      document.body.style.overflow = previousOverflow
      document.removeEventListener('keydown', handleKeyDown)
      globalThis.window.removeEventListener('resize', updateDimensions)
    }
  }, [isFullscreen])

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

  if (isLoading) return null
  if (!data) return null

  const nodeCount = data.nodes.length
  const edgeCount = data.venue.edge_count
  const showCount = data.venue.show_count

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

  // Even when graphAvailable is false, keep the section header + filter
  // visible at desktop widths so the user understands WHY the graph isn't
  // showing and can re-scope the window. Mobile users see nothing — matches
  // the SceneGraph mobile gate.
  if (containerWidth !== null && containerWidth < GRAPH_BREAKPOINT_PX && tooSparse) {
    return null
  }

  // Time-window filter — three radio-style buttons + the year picker.
  // Inline keeps the markup co-located with the `setWindow` handler; the
  // toggle behavior is small enough that pulling it into a sub-component
  // would be premature.
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

  const sectionHeader = (
    <div>
      <h2 className="text-lg font-semibold">Who plays together here</h2>
      <p className="text-sm text-muted-foreground">
        {nodeCount} {nodeCount === 1 ? 'artist' : 'artists'}
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
      onClick={() => setIsFullscreen(true)}
      className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-md border border-border/60 hover:bg-muted/50 transition-colors"
      aria-label="Expand venue bill network to fullscreen"
    >
      <Maximize2 className="h-4 w-4" aria-hidden="true" />
      <span>Expand</span>
    </button>
  )

  return (
    <>
      <div
        ref={containerRefCallback}
        // PSY-366: `id="graph"` enables Cmd+K deep-links from the command
        // palette (`/venues/{slug}#graph`). `scroll-mt-20` accounts for the
        // sticky header on the entity layout.
        id="graph"
        className="mt-8 px-4 md:px-0 scroll-mt-20"
        // Hide inline copy from assistive tech while the overlay is open —
        // the overlay's own header is the single source of truth in that mode.
        aria-hidden={isFullscreen || undefined}
        inert={isFullscreen || undefined}
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

        {graphAvailable && !isFullscreen && (
          <div className="space-y-3 mt-2">
            <SceneGraphVisualizationStyleAdapter
              data={data}
              venueName={venueName}
              containerWidth={containerWidth!}
            />

            <p className="text-xs text-muted-foreground">
              Showing artists who&apos;ve played approved shows at {venueName}. Edge weight =
              shared shows AT THIS VENUE in the active window. Click any artist to open their
              page.
            </p>
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
              onClick={() => setIsFullscreen(false)}
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
