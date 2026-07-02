'use client'

/**
 * StationGraphVisualization (PSY-1299)
 *
 * Thin shape adapter over the shared `ForceGraphView` for the station
 * co-occurrence graph — the station analog of `SceneGraphVisualization`.
 * It owns only the station-specific concerns (a11y label phrasing, click →
 * artist navigation) and delegates canvas, layout, hulls, isolate shelf,
 * and tooltips to the shared component.
 *
 * The edge legend is intentionally OFF: every station edge is the single
 * `radio_cooccurrence` type, so a one-row legend adds noise. The cluster
 * pills (per-show colors, owned by StationGraph.tsx) carry the legend duty.
 */

import { useCallback } from 'react'
import { useRouter } from 'next/navigation'
import { ForceGraphView } from '@/components/graph/ForceGraphView'
import type { GraphNode } from '@/components/graph/ForceGraphView'
import type { RadioStationGraphResponse } from '../types'

interface StationGraphVisualizationProps {
  data: RadioStationGraphResponse
  containerWidth: number
  /** Clusters hidden via the legend pills — any cluster, "other" included. */
  hiddenClusterIDs: Set<string>
  /** Explicit canvas height for the fullscreen overlay; inline default when omitted. */
  height?: number
}

export function StationGraphVisualization({
  data,
  containerWidth,
  hiddenClusterIDs,
  height,
}: StationGraphVisualizationProps) {
  const router = useRouter()

  // Same PSY-361 inheritance as the scene graph: clicking a node exits the
  // station-scale view into that artist's page (where the ego graph lives).
  const handleNodeClick = useCallback(
    (node: GraphNode) => {
      router.push(`/artists/${node.slug}`)
    },
    [router],
  )

  const ariaLabel = `Airplay graph for ${data.station.name}: ${data.station.artist_count} artists, ${data.station.edge_count} connections. Use the shows and playlists lists to browse without the canvas.`

  return (
    <ForceGraphView
      nodes={data.nodes}
      links={data.links}
      clusters={data.clusters}
      containerWidth={containerWidth}
      height={height}
      hiddenClusterIDs={hiddenClusterIDs}
      ariaLabel={ariaLabel}
      onNodeClick={handleNodeClick}
    />
  )
}
