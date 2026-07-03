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
 * PSY-1320: cluster lenses. A "Clusters: Venue | Community" pill toggle
 * switches the backend's `cluster_by` param between most-frequent-venue
 * clusters and the persisted Leiden community partition (PSY-1262). Venue
 * remains the DEFAULT until PSY-1323's input-graph rebalance makes community
 * clusters scene-meaningful (amended decision, 2026-07-02 gamma sweep).
 * Hidden-cluster state resets on mode switch (IDs are mode-scoped).
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

import { useState, useMemo } from 'react'
import { Maximize2, X } from 'lucide-react'
import { ClusterLegend } from '@/components/graph/ClusterLegend'
import { useContainerWidth, GRAPH_BREAKPOINT_PX } from '@/components/graph/useContainerWidth'
import { useFullscreenGraphOverlay } from '@/components/graph/useFullscreenGraphOverlay'
import { useSceneGraph, type SceneGraphClusterBy } from '../hooks/useScenes'
import { SceneGraphVisualization } from './SceneGraphVisualization'

const MIN_GRAPH_NODES = 3

const CLUSTER_MODES: { value: SceneGraphClusterBy; label: string }[] = [
  { value: 'venue', label: 'Venue' },
  { value: 'community', label: 'Community' },
]

interface SceneGraphProps {
  slug: string
  city: string
  state: string
}

export function SceneGraph({ slug, city, state }: SceneGraphProps) {
  // PSY-1320: venue is the default lens for now — the recorded default-to-
  // community decision is amended until PSY-1323's input-graph rebalance
  // makes community clusters scene-meaningful (2026-07-02 gamma sweep:
  // both eyeball scenes rendered 100% "other").
  const [clusterBy, setClusterBy] = useState<SceneGraphClusterBy>('venue')
  const { data, isLoading, isError, isPlaceholderData } = useSceneGraph({
    slug,
    clusterBy,
    enabled: Boolean(slug),
  })
  const [hiddenClusters, setHiddenClusters] = useState<Set<string>>(new Set())

  const switchClusterBy = (mode: SceneGraphClusterBy) => {
    if (mode === clusterBy) return
    setClusterBy(mode)
    // Cluster IDs are mode-scoped (`v_*` vs `c_*`), so a hidden set carried
    // across modes would silently hide nothing — reset with the switch.
    setHiddenClusters(new Set())
  }
  // Width measurement uses a callback ref, not useRef + useEffect — the full
  // rationale lives in useContainerWidth.ts.
  const { refCallback: containerRefCallback, containerWidth } = useContainerWidth()

  const isolateCount = useMemo(() => {
    if (!data) return 0
    return data.nodes.reduce((n, node) => (node.is_isolate ? n + 1 : n), 0)
  }, [data])

  const nodeCount = data?.nodes.length ?? 0
  const edgeCount = data?.scene.edge_count ?? 0
  const hasEnoughForGraph = nodeCount >= MIN_GRAPH_NODES
  // Mobile gating: < sm breakpoint (640px) hides the graph entirely; the
  // existing scene page list view remains the only surface (PSY-369 / PSY-511).
  // `containerWidth === null` (pre-measurement) also gates off.
  const graphAvailable =
    hasEnoughForGraph && containerWidth !== null && containerWidth >= GRAPH_BREAKPOINT_PX

  // Overlay lifecycle (scroll lock, Esc, viewport tracking, auto-close when
  // graphAvailable flips false mid-overlay) lives in the shared hook.
  const {
    isFullscreen,
    open: openFullscreen,
    close: closeFullscreen,
    overlayWidth,
    overlayHeight,
  } = useFullscreenGraphOverlay(graphAvailable)

  if (isLoading) return null

  // Cluster-by mode toggle — radio-style pills (VenueBillNetwork's window
  // filter pattern), rendered above the legend inline and in the overlay.
  // Defined before the data guards because the error state below needs it.
  const clusterByToggle = (
    <div className="flex flex-wrap items-center gap-2 text-xs">
      <span className="text-muted-foreground" aria-hidden="true">
        Clusters:
      </span>
      {CLUSTER_MODES.map(mode => {
        const isActive = clusterBy === mode.value
        return (
          <button
            key={mode.value}
            type="button"
            onClick={() => switchClusterBy(mode.value)}
            aria-pressed={isActive}
            className={`px-2 py-0.5 rounded-full border text-xs transition-colors ${
              isActive
                ? 'bg-primary/10 border-primary text-primary'
                : 'border-border/60 text-muted-foreground hover:bg-muted/50'
            }`}
          >
            {mode.label}
          </button>
        )
      })}
    </div>
  )

  // A settled fetch error leaves `data` undefined (keepPreviousData only
  // bridges the pending window). Unmounting here would strand the user: the
  // toggle is their only path back to the mode that worked, so keep the
  // section shell + toggle rendered with an inline notice instead of
  // vanishing (code-review finding, PSY-1320).
  if (!data && isError) {
    return (
      <div id="graph" className="mt-2 scroll-mt-20">
        <h2 className="text-lg font-semibold mb-2">Scene graph</h2>
        <div className="space-y-2">
          {clusterByToggle}
          <p className="text-sm text-muted-foreground">
            This view couldn&apos;t load. Try switching clusters above.
          </p>
        </div>
      </div>
    )
  }

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

  // Cluster legend pills — the shared component keeps toggle behavior, ARIA,
  // and color mapping identical inline and in the overlay.
  const clusterLegend = (
    <ClusterLegend
      clusters={data.clusters}
      hiddenClusterIDs={hiddenClusters}
      onToggle={toggleCluster}
    />
  )

  const clusterCaption =
    clusterBy === 'community'
      ? 'Clusters group artists by similarity community — computed nightly from shared bills, shared labels, and radio co-play across the whole catalog.'
      : 'Clusters group artists by their most-frequent venue here.'

  // Mid-switch, keepPreviousData still renders the OLD mode's clusters while
  // the caption/pills already claim the new mode. Dim the stale content and
  // block legend clicks (a hide clicked now would store an old-mode ID into
  // the just-reset hidden set); the mode toggle stays interactive.
  const transitionDim = isPlaceholderData ? 'opacity-60 pointer-events-none' : ''

  // The Expand button lives inside the header's gap-2 row; it's only rendered
  // when graphAvailable, which inherits the mobile gate (containerWidth must
  // be ≥ 640px). That single source of truth means mobile users never see the
  // Expand button — there's no separate mobile branch to maintain.
  const expandButton = graphAvailable && !isFullscreen && (
    <button
      type="button"
      onClick={openFullscreen}
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
        // PSY-366: `id="graph"` enables Cmd+K deep-links from the command
        // palette (`/scenes/{slug}#graph`). `scroll-mt-20` accounts for the
        // sticky header on the entity layout.
        id="graph"
        className="mt-2 scroll-mt-20"
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
            {clusterByToggle}

            <div className={`space-y-3 ${transitionDim}`} aria-busy={isPlaceholderData}>
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
                Showing artists who&apos;ve played approved shows in {city}, {state}.{' '}
                {clusterCaption} Click a cluster pill above to hide it; click any artist to
                open their page.
              </p>
            </div>
          </div>
        )}
      </div>

      {isFullscreen && graphAvailable && (
        <div
          // Full-opacity backdrop so nothing peeks through; the overlay is its
          // own surface, not a translucent modal. z-[60] sits above the cookie
          // consent banner (z-50) so first-time visitors don't see the banner
          // painted over the bottom of the canvas (PSY-518). role=dialog +
          // aria-modal communicates focus expectations to assistive tech
          // without trapping focus (the overlay is keyboard-dismissable via Esc).
          role="dialog"
          aria-modal="true"
          aria-label={`Scene graph for ${city}, ${state}, fullscreen`}
          className="fixed inset-0 z-[60] bg-background flex flex-col"
          data-testid="scene-graph-overlay"
        >
          <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-3 border-b border-border/50">
            {sceneHeader}
            <button
              type="button"
              onClick={closeFullscreen}
              className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-md border border-border/60 hover:bg-muted/50 transition-colors"
              aria-label="Exit fullscreen scene graph"
            >
              <X className="h-4 w-4" aria-hidden="true" />
              <span>Exit</span>
            </button>
          </div>

          <div className="px-4 py-2 border-b border-border/30 space-y-2">
            {clusterByToggle}
            <div className={transitionDim} aria-busy={isPlaceholderData}>
              {clusterLegend}
            </div>
          </div>

          <div
            className={`flex-1 min-h-0 px-4 py-2 ${transitionDim}`}
            aria-busy={isPlaceholderData}
          >
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
