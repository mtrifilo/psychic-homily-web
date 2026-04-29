'use client'

/**
 * SceneGraph (PSY-367, PSY-516, PSY-517)
 *
 * Section wrapper for the scene-scale graph: header, cluster legend, and the
 * canvas. Mobile-gated below the Tailwind `sm` breakpoint per
 * PSY-369→PSY-511.
 *
 * PSY-516: graph is visible by default at ≥640px (previously hidden behind a
 * "View map" / "Hide map" toggle). Dogfood feedback flagged the toggle as
 * friction on a feature whose value is the immediate visual scan. Mobile
 * gating and the empty-state (`<3` connected artists) gate are unchanged.
 *
 * PSY-517: the slot vacated by PSY-516's removed toggle now hosts an
 * Expand / Exit button that opens a CSS viewport overlay (not the Browser
 * Fullscreen API) containing the same graph + cluster legend. Esc closes;
 * body scroll is locked while open. The overlay inherits the inline
 * `graphAvailable` gate, so mobile users never see the Expand button.
 *
 * Decision: inline on the existing `/scenes/{slug}` page rather than a
 * separate `/scenes/{slug}/graph` route. Reasons:
 *   - Discoverable: users browsing scenes naturally encounter it; no need to
 *     learn a separate URL.
 *   - Keeps the scene page authoritative for "what the scene is" — the graph
 *     is one of many lenses, alongside venues, artists, pulse, genres.
 *
 * Trade-off accepted: the canvas mounts on every Phoenix-scale scene visit
 * and pushes other sections down, but `react-force-graph-2d` is dynamic-
 * imported with `ssr: false`, the canvas pauses after `cooldownTicks=200`,
 * and mobile already gates it off.
 */

import { useState, useCallback, useMemo, useEffect } from 'react'
import { Eye, EyeOff, Maximize2, X } from 'lucide-react'
import { useSceneGraph } from '../hooks/useScenes'
import { SceneGraphVisualization } from './SceneGraphVisualization'

const GRAPH_BREAKPOINT_PX = 640
const MIN_GRAPH_NODES = 3

// Overlay vertical reserve: header bar (~60px) + cluster pill row (~60px) +
// padding/margins. Subtracted from window.innerHeight to give the canvas the
// remaining vertical real estate. Tuned to keep the canvas full-bleed without
// clipping the legend or the title bar.
const OVERLAY_VERTICAL_RESERVE_PX = 140

// Same Okabe-Ito mapping as SceneGraphVisualization, repeated here to avoid an
// internal dependency on the canvas component for legend rendering. Keep in
// sync if either palette changes.
const OKABE_ITO_PALETTE = [
  '#0173B2',
  '#DE8F05',
  '#029E73',
  '#D55E00',
  '#CC78BC',
  '#CA9161',
  '#56B4E9',
  '#ECE133',
] as const
const OTHER_CLUSTER_COLOR = '#94A3B8'

function clusterColor(colorIndex: number): string {
  if (colorIndex < 0 || colorIndex >= OKABE_ITO_PALETTE.length) return OTHER_CLUSTER_COLOR
  return OKABE_ITO_PALETTE[colorIndex]
}

interface SceneGraphProps {
  slug: string
  city: string
  state: string
}

export function SceneGraph({ slug, city, state }: SceneGraphProps) {
  const { data, isLoading } = useSceneGraph({ slug, enabled: Boolean(slug) })
  const [hiddenClusters, setHiddenClusters] = useState<Set<string>>(new Set())
  const [containerWidth, setContainerWidth] = useState<number | null>(null)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [overlayHeight, setOverlayHeight] = useState<number | null>(null)
  const [overlayWidth, setOverlayWidth] = useState<number | null>(null)

  // Callback ref instead of useRef + useEffect. Using useEffect with `[]` deps
  // would only fire on the *initial* mount — and the initial mount often
  // returns null (waiting for data), so containerRef.current is null and the
  // effect bails. Subsequent renders that DO produce a DOM node never re-run
  // the effect because the deps haven't changed. A callback ref fires whenever
  // the underlying DOM node mounts/unmounts, so we always measure the right
  // node. (This is the React 19 supported pattern; the cleanup return is
  // honored automatically.)
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

  // Overlay-mode side effects: body scroll lock, Esc-to-close, and a live
  // viewport-size listener that keeps the canvas full-bleed when the user
  // resizes the window. All gated on `isFullscreen` so the listener +
  // overflow style only exist while the overlay is open. We snapshot the
  // previous body overflow value and restore it on close — blindly setting
  // it to '' would clobber any inline value a parent layout had set.
  useEffect(() => {
    if (!isFullscreen) return

    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'

    const updateDimensions = () => {
      setOverlayWidth(window.innerWidth)
      setOverlayHeight(Math.max(200, window.innerHeight - OVERLAY_VERTICAL_RESERVE_PX))
    }
    updateDimensions()

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setIsFullscreen(false)
      }
    }

    document.addEventListener('keydown', handleKeyDown)
    window.addEventListener('resize', updateDimensions)

    return () => {
      document.body.style.overflow = previousOverflow
      document.removeEventListener('keydown', handleKeyDown)
      window.removeEventListener('resize', updateDimensions)
    }
  }, [isFullscreen])

  const isolateCount = useMemo(() => {
    if (!data) return 0
    return data.nodes.reduce((n, node) => (node.is_isolate ? n + 1 : n), 0)
  }, [data])

  if (isLoading) return null

  const nodeCount = data?.nodes.length ?? 0
  const edgeCount = data?.scene.edge_count ?? 0
  const hasEnoughForGraph = nodeCount >= MIN_GRAPH_NODES
  // Mobile gating: < sm breakpoint (640px) hides the graph entirely; the
  // existing scene page list view remains the only surface (PSY-369 / PSY-511).
  // `containerWidth === null` (pre-measurement) also gates off.
  const graphAvailable =
    hasEnoughForGraph && containerWidth !== null && containerWidth >= GRAPH_BREAKPOINT_PX

  // Section is rendered (with the header) so users get scale info even when
  // the graph is unavailable (e.g. mobile). Empty state: scene has < 3
  // connected artists — render nothing rather than a confusing skeleton.
  if (!data || nodeCount === 0) return null

  const toggleCluster = (clusterID: string) => {
    setHiddenClusters(prev => {
      const next = new Set(prev)
      if (next.has(clusterID)) {
        next.delete(clusterID)
      } else {
        next.add(clusterID)
      }
      return next
    })
  }

  // Cluster legend pills — same markup inline and in the overlay so the
  // toggle behavior, ARIA, and color mapping stay in one place.
  const clusterLegend = data.clusters.length > 0 && (
    <div className="flex flex-wrap gap-1.5">
      {data.clusters.map(cluster => {
        const hidden = hiddenClusters.has(cluster.id)
        return (
          <button
            key={cluster.id}
            onClick={() => toggleCluster(cluster.id)}
            aria-pressed={!hidden}
            className={`inline-flex items-center gap-1.5 text-xs px-2 py-0.5 rounded-full border transition-opacity ${
              hidden ? 'opacity-40' : 'opacity-100'
            }`}
            style={{
              borderColor: clusterColor(cluster.color_index),
              color: clusterColor(cluster.color_index),
            }}
            title={hidden ? `Show ${cluster.label}` : `Hide ${cluster.label}`}
          >
            <span
              className="inline-block w-2 h-2 rounded-full"
              style={{ backgroundColor: clusterColor(cluster.color_index) }}
            />
            <span className="text-foreground/85">
              {cluster.label} ({cluster.size})
            </span>
            {hidden ? (
              <EyeOff className="h-3 w-3" aria-hidden="true" />
            ) : (
              <Eye className="h-3 w-3" aria-hidden="true" />
            )}
          </button>
        )
      })}
    </div>
  )

  // The Expand button lives inside the header's gap-2 row; it's only rendered
  // when graphAvailable, which inherits the mobile gate (containerWidth must
  // be ≥ 640px). That single source of truth means mobile users never see the
  // Expand button — there's no separate mobile branch to maintain.
  const expandButton = graphAvailable && !isFullscreen && (
    <button
      type="button"
      onClick={() => setIsFullscreen(true)}
      className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-md border border-border/60 hover:bg-muted/50 transition-colors"
      aria-label="Expand scene graph to fullscreen"
    >
      <Maximize2 className="h-4 w-4" aria-hidden="true" />
      <span>Expand</span>
    </button>
  )

  const sceneHeader = (
    <div>
      <h2 className="text-lg font-semibold">Scene graph</h2>
      <p className="text-sm text-muted-foreground">
        {nodeCount} {nodeCount === 1 ? 'artist' : 'artists'}
        {edgeCount > 0 && (
          <>
            {' · '}
            {edgeCount} {edgeCount === 1 ? 'connection' : 'connections'}
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

  return (
    <>
      <div
        ref={containerRefCallback}
        className="mt-2"
        // While the overlay is open, hide the inline copy from assistive tech
        // and inert it for keyboard focus — the overlay's own header is the
        // single source of truth for scene-graph navigation in that mode.
        // `inert` is a React 19 boolean prop; aria-hidden is the sibling
        // affordance for screen readers.
        aria-hidden={isFullscreen || undefined}
        inert={isFullscreen || undefined}
      >
        <div className="flex flex-wrap items-center justify-between gap-2 mb-2">
          {sceneHeader}
          {expandButton}
        </div>

        {graphAvailable && !isFullscreen && (
          <div className="space-y-3">
            {/* Cluster legend — click a row to toggle that cluster's visibility.
                "Other" stays clickable so users can hide the long tail at will. */}
            {clusterLegend}

            <SceneGraphVisualization
              data={data}
              // Safe non-null: graphAvailable requires containerWidth !== null
              containerWidth={containerWidth!}
              hiddenClusterIDs={hiddenClusters}
            />

            <p className="text-xs text-muted-foreground">
              Showing artists who&apos;ve played approved shows in {city}, {state}. Clusters
              group artists by their most-frequent venue here. Click a cluster pill above to
              hide it; click any artist to open their page.
            </p>
          </div>
        )}
      </div>

      {isFullscreen && graphAvailable && (
        <div
          // Full-opacity backdrop so nothing peeks through; the overlay is its
          // own surface, not a translucent modal. z-50 mirrors other overlays
          // in the codebase. role=dialog + aria-modal communicates focus
          // expectations to assistive tech without trapping focus (the
          // overlay is keyboard-dismissable via Esc).
          role="dialog"
          aria-modal="true"
          aria-label={`Scene graph for ${city}, ${state}, fullscreen`}
          className="fixed inset-0 z-50 bg-background flex flex-col"
          data-testid="scene-graph-overlay"
        >
          <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-3 border-b border-border/50">
            {sceneHeader}
            <button
              type="button"
              onClick={() => setIsFullscreen(false)}
              className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-md border border-border/60 hover:bg-muted/50 transition-colors"
              aria-label="Exit fullscreen scene graph"
            >
              <X className="h-4 w-4" aria-hidden="true" />
              <span>Exit</span>
            </button>
          </div>

          <div className="px-4 py-2 border-b border-border/30">{clusterLegend}</div>

          <div className="flex-1 min-h-0 px-4 py-2">
            {overlayHeight !== null && overlayWidth !== null && (
              <SceneGraphVisualization
                data={data}
                containerWidth={overlayWidth}
                hiddenClusterIDs={hiddenClusters}
                height={overlayHeight}
              />
            )}
          </div>
        </div>
      )}
    </>
  )
}
