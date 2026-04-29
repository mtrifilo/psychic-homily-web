'use client'

/**
 * SceneGraph (PSY-367)
 *
 * Section wrapper for the scene-scale graph: header, "View Map" toggle, cluster
 * legend, and the canvas. Mirrors the BillComposition / RelatedArtists section
 * pattern — graph hidden behind a toggle, mobile-gated below the Tailwind `sm`
 * breakpoint per PSY-369→PSY-511.
 *
 * Decision: inline toggle on the existing `/scenes/{slug}` page rather than a
 * separate `/scenes/{slug}/graph` route. Reasons:
 *   - Mirrors the artist-page precedent (per-artist + bill-composition both
 *     toggle inline). One mental model across the app.
 *   - Discoverable: users browsing scenes naturally encounter it; no need to
 *     learn a separate URL.
 *   - Keeps the scene page authoritative for "what the scene is" — the graph
 *     is one of many lenses, alongside venues, artists, pulse, genres.
 *
 * Trade-off accepted: the canvas competes with other page sections, but
 * collapsing by default + the standalone fixed-height container keep that
 * cost bounded.
 */

import { useState, useCallback, useMemo } from 'react'
import { Network, Eye, EyeOff } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useSceneGraph } from '../hooks/useScenes'
import { SceneGraphVisualization } from './SceneGraphVisualization'

const GRAPH_BREAKPOINT_PX = 640
const MIN_GRAPH_NODES = 3

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
  const [showGraph, setShowGraph] = useState(false)
  const [hiddenClusters, setHiddenClusters] = useState<Set<string>>(new Set())
  const [containerWidth, setContainerWidth] = useState<number | null>(null)

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

  // Section is rendered (with the header) so users get scale info even when the
  // graph is unavailable; the toggle button is only shown when graphAvailable.
  // Empty state: scene has < 3 connected artists — render nothing rather than
  // a confusing skeleton.
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

  return (
    <div ref={containerRefCallback} className="mt-2">
      <div className="flex flex-wrap items-center justify-between gap-2 mb-2">
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
        {graphAvailable && (
          <Button
            variant={showGraph ? 'default' : 'outline'}
            size="sm"
            onClick={() => setShowGraph(prev => !prev)}
          >
            <Network className="h-4 w-4 mr-1.5" />
            {showGraph ? 'Hide map' : 'View map'}
          </Button>
        )}
      </div>

      {showGraph && graphAvailable && (
        <div className="space-y-3">
          {/* Cluster legend — click a row to toggle that cluster's visibility.
              "Other" stays clickable so users can hide the long tail at will. */}
          {data.clusters.length > 0 && (
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
                    title={
                      hidden ? `Show ${cluster.label}` : `Hide ${cluster.label}`
                    }
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
          )}

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
  )
}
