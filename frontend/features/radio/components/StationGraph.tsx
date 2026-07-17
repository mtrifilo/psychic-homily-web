'use client'

/**
 * StationGraph (PSY-1299)
 *
 * Section wrapper for the station co-occurrence graph: header, per-show
 * cluster legend pills, inline canvas, and the fullscreen overlay. Mirrors
 * `SceneGraph` (PSY-367/516/517) — same mobile gate (<640px hidden, lists
 * remain the accessible surface per PSY-369/511), same visible-by-default
 * inline placement, same CSS viewport overlay with Esc-to-close.
 *
 * Data is the PSY-1295 backbone-filtered within-station subgraph; clusters
 * group artists by the station show they're most played on. Defaults-only:
 * the endpoint's window/limit params are not exposed in the UI yet.
 */

import { useState, useMemo, useEffect, useRef } from 'react'
import { Maximize2, X } from 'lucide-react'
import { ClusterLegend } from '@/components/graph/ClusterLegend'
import { GraphSkeleton } from '@/components/graph/GraphSkeleton'
import {
  GraphStateCard,
  GRAPH_BOX_HEIGHT_CLASS,
  GRAPH_TEASER_HEIGHT_CLASS,
} from '@/components/graph/GraphStateCard'
import { useContainerWidth, GRAPH_BREAKPOINT_PX } from '@/components/graph/useContainerWidth'
import { useFullscreenGraphOverlay } from '@/components/graph/useFullscreenGraphOverlay'
import { GRAPH_HASH, useUrlHash } from '@/lib/hooks/common/useUrlHash'
import { useStationGraph } from '../hooks/useStationGraph'
import { StationGraphVisualization } from './StationGraphVisualization'

const MIN_GRAPH_NODES = 3

interface StationGraphProps {
  slug: string
  stationName: string
}

export function StationGraph({ slug, stationName }: StationGraphProps) {
  // The hook owns the empty-slug guard (enabled: Boolean(slug) internally).
  const { data, isLoading, isError } = useStationGraph({ slug })
  const [hiddenClusters, setHiddenClusters] = useState<Set<string>>(new Set())
  const { refCallback: containerRefCallback, containerWidth } = useContainerWidth()

  const isolateCount = useMemo(() => {
    if (!data) return 0
    return data.nodes.reduce((n, node) => (node.is_isolate ? n + 1 : n), 0)
  }, [data])

  const nodeCount = data?.nodes.length ?? 0
  const edgeCount = data?.station.edge_count ?? 0
  const hasEnoughForGraph = nodeCount >= MIN_GRAPH_NODES
  // Mobile gating: <640px hides the graph entirely; the playlists feed +
  // shows directory remain the only surfaces (PSY-369 / PSY-511).
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

  // Deliver the `#graph` deep-link: the anchor mounts only after two async
  // fetches (station, then graph), so the browser's native fragment scroll
  // fires before the target exists. Scroll once when the data lands.
  const hash = useUrlHash()
  const scrolledToHash = useRef(false)
  useEffect(() => {
    if (hash !== GRAPH_HASH || scrolledToHash.current || !data) return
    scrolledToHash.current = true
    document.getElementById('graph')?.scrollIntoView()
  }, [hash, data])

  // Loading reserves the graph box (shared GraphSkeleton, PSY-1347) instead
  // of returning null — a null here shifts every section below when the
  // canvas lands. The header stays put so only the box swaps on settle.
  if (isLoading) {
    return (
      <div id="graph" className="scroll-mt-20">
        <h2 className="text-lg font-semibold mb-2">Airplay graph</h2>
        <GraphSkeleton className={GRAPH_BOX_HEIGHT_CLASS} />
      </div>
    )
  }

  // A settled fetch error leaves `data` undefined. Rendering nothing here
  // would make an API failure indistinguishable from a sparse station —
  // keep the section shell and say so (scene-page convention, PSY-1446).
  if (!data && isError) {
    return (
      <div id="graph" className="scroll-mt-20">
        <h2 className="text-lg font-semibold mb-2">Airplay graph</h2>
        <GraphStateCard
          role="alert"
          message="This view couldn't load. Refresh the page to try again."
        />
      </div>
    )
  }

  // Sparse state: a station needs at least MIN_GRAPH_NODES charted artists to
  // be worth a graph section — below that, render nothing at all (a bare
  // "2 artists" header with no canvas under it reads as broken). Note the
  // graph QUERY still fetches; only the render is gated.
  if (!data || nodeCount < MIN_GRAPH_NODES) return null

  const windowLabel = data.station.window === 'all_time' ? 'all time' : 'the last 12 months'

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

  // Cluster legend pills — one per station show; the shared component keeps
  // toggle behavior, ARIA, and colors identical inline and in the overlay.
  const clusterLegend = (
    <ClusterLegend
      clusters={data.clusters}
      hiddenClusterIDs={hiddenClusters}
      onToggle={toggleCluster}
    />
  )

  // Expand only renders when graphAvailable, which inherits the mobile gate —
  // mobile users never see the button (single source of truth, per SceneGraph).
  const expandButton = graphAvailable && !isFullscreen && (
    <button
      type="button"
      onClick={openFullscreen}
      className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-md border border-border/60 hover:bg-muted/50 transition-colors"
      aria-label="Expand airplay graph to fullscreen"
    >
      <Maximize2 className="h-4 w-4" aria-hidden="true" />
      <span>Expand</span>
    </button>
  )

  const stationHeader = (
    <div>
      <h2 className="text-lg font-semibold">Airplay graph</h2>
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
        // `id="graph"` enables `#graph` deep-links, matching the scene page.
        id="graph"
        className="scroll-mt-20"
        // While the overlay is open, hide + inert the inline copy so the
        // overlay is the single graph surface for assistive tech (PSY-517).
        aria-hidden={isFullscreen || undefined}
        inert={isFullscreen || undefined}
      >
        <div className="flex flex-wrap items-center justify-between gap-2 mb-2">
          {stationHeader}
          {expandButton}
        </div>

        {/* Pre-measurement: hold the box height so the settle can't shift
            the sections below (HomeSceneGraph precedent). */}
        {containerWidth === null && (
          <GraphSkeleton className={GRAPH_BOX_HEIGHT_CLASS} />
        )}

        {/* Sub-640px: shared teaser card instead of the old silent hide —
            the playlists feed + shows directory on this page remain the
            small-screen surfaces, so no link-out target. */}
        {containerWidth !== null && containerWidth < GRAPH_BREAKPOINT_PX && (
          <GraphStateCard
            className={GRAPH_TEASER_HEIGHT_CLASS}
            message="The interactive airplay graph is best on a larger screen."
          />
        )}

        {graphAvailable && !isFullscreen && (
          <div className="space-y-3">
            {clusterLegend}

            <StationGraphVisualization
              data={data}
              // Safe non-null: graphAvailable requires containerWidth !== null
              containerWidth={containerWidth!}
              hiddenClusterIDs={hiddenClusters}
            />

            <p className="text-xs text-muted-foreground">
              Artists most played on {stationName} over {windowLabel}, linked when they
              appear on the same episodes (strongest connections only). Clusters group
              artists by the show that plays them most. Click a cluster pill above to
              hide it; click any artist for their details.
            </p>
          </div>
        )}
      </div>

      {isFullscreen && graphAvailable && (
        <div
          // z-[60] sits above the cookie-consent banner (z-50) so first-time
          // visitors don't see the banner painted over the canvas (PSY-518).
          role="dialog"
          aria-modal="true"
          aria-label={`Airplay graph for ${stationName}, fullscreen`}
          className="fixed inset-0 z-[60] bg-background flex flex-col"
          data-testid="station-graph-overlay"
        >
          <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-3 border-b border-border/50">
            {stationHeader}
            <button
              type="button"
              onClick={closeFullscreen}
              className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-md border border-border/60 hover:bg-muted/50 transition-colors"
              aria-label="Exit fullscreen airplay graph"
            >
              <X className="h-4 w-4" aria-hidden="true" />
              <span>Exit</span>
            </button>
          </div>

          <div className="px-4 py-2 border-b border-border/30">{clusterLegend}</div>

          <div className="flex-1 min-h-0 px-4 py-2">
            {overlayHeight !== null && overlayWidth !== null && (
              <StationGraphVisualization
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
