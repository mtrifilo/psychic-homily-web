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

import { useState, useCallback, useMemo, useEffect } from 'react'
import { Eye, EyeOff, Maximize2, X } from 'lucide-react'
import { clusterColorCSS } from '@/components/graph/graphPalette'
import { useStationGraph } from '../hooks/useStationGraph'
import { StationGraphVisualization } from './StationGraphVisualization'

const GRAPH_BREAKPOINT_PX = 640
const MIN_GRAPH_NODES = 3

// Overlay vertical reserve: header bar + cluster pill row + padding, matching
// the SceneGraph overlay tuning.
const OVERLAY_VERTICAL_RESERVE_PX = 140

interface StationGraphProps {
  slug: string
  stationName: string
}

export function StationGraph({ slug, stationName }: StationGraphProps) {
  // The hook owns the empty-slug guard (enabled: Boolean(slug) internally).
  const { data, isLoading } = useStationGraph({ slug })
  const [hiddenClusters, setHiddenClusters] = useState<Set<string>>(new Set())
  const [containerWidth, setContainerWidth] = useState<number | null>(null)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [overlayHeight, setOverlayHeight] = useState<number | null>(null)
  const [overlayWidth, setOverlayWidth] = useState<number | null>(null)

  // Callback ref (not useRef+useEffect): the initial mount often returns null
  // while data loads, so an effect with [] deps would never measure the node
  // that mounts later. See SceneGraph.tsx for the full rationale.
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

  // Overlay side effects: body scroll lock, Esc-to-close, live viewport-size
  // tracking. Snapshot + restore the previous body overflow value.
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
  const edgeCount = data?.station.edge_count ?? 0
  const hasEnoughForGraph = nodeCount >= MIN_GRAPH_NODES
  // Mobile gating: <640px hides the graph entirely; the playlists feed +
  // shows directory remain the only surfaces (PSY-369 / PSY-511).
  const graphAvailable =
    hasEnoughForGraph && containerWidth !== null && containerWidth >= GRAPH_BREAKPOINT_PX

  // Empty state: fewer than 3 artists with plays — render nothing rather
  // than a confusing near-empty canvas.
  if (!data || nodeCount === 0) return null

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

  // Cluster legend pills — one per station show; same markup inline and in
  // the overlay so toggle behavior, ARIA, and colors stay in one place.
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
              borderColor: clusterColorCSS(cluster.color_index),
              color: clusterColorCSS(cluster.color_index),
            }}
            title={hidden ? `Show ${cluster.label}` : `Hide ${cluster.label}`}
          >
            <span
              className="inline-block w-2 h-2 rounded-full"
              style={{ backgroundColor: clusterColorCSS(cluster.color_index) }}
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

  // Expand only renders when graphAvailable, which inherits the mobile gate —
  // mobile users never see the button (single source of truth, per SceneGraph).
  const expandButton = graphAvailable && !isFullscreen && (
    <button
      type="button"
      onClick={() => setIsFullscreen(true)}
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
              hide it; click any artist to open their page.
            </p>
          </div>
        )}
      </div>

      {isFullscreen && graphAvailable && (
        <div
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
              onClick={() => setIsFullscreen(false)}
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
