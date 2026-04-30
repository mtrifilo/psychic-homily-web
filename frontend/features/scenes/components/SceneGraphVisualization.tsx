'use client'

/**
 * SceneGraphVisualization (PSY-367 → refactored under PSY-365)
 *
 * Thin shape adapter over the shared `ForceGraphView` (PSY-365) — it owns
 * just the scene-specific concerns (a11y label phrasing, click → artist
 * navigation) and delegates the canvas, layout, hulls, isolate shelf, and
 * tooltip to the shared component.
 *
 * Why a wrapper instead of inlining `<ForceGraphView/>` at the call site:
 *   1. Keeps the public import path stable for callers (SceneGraph.tsx).
 *   2. Holds the next-router dependency (`useRouter`) so the shared
 *      ForceGraphView stays router-agnostic and easier to test.
 *   3. Owns the scene-specific aria-label + click semantics; equivalent
 *      pattern lives in `VenueBillNetwork.tsx` for venue scope.
 *
 * Behaviour preserved (PSY-516, PSY-517, PSY-518, PSY-519 patches):
 *   - all props from the original component remain — the SceneGraph tests
 *     that mock this component via `vi.mock('./SceneGraphVisualization')`
 *     do NOT need to change
 *   - height-prop handling (used by SceneGraph fullscreen overlay) flows
 *     through to ForceGraphView unchanged
 *   - hiddenClusterIDs filter behaviour preserved (Set passed through)
 */

import { useCallback } from 'react'
import { useRouter } from 'next/navigation'
import { ForceGraphView } from '@/components/graph/ForceGraphView'
import type { GraphNode } from '@/components/graph/ForceGraphView'
import type { SceneGraphResponse } from '../types'

interface SceneGraphVisualizationProps {
  data: SceneGraphResponse
  containerWidth: number
  /**
   * IDs of clusters the user has hidden via the legend. Hidden clusters'
   * nodes + edges are removed from the canvas; "other" cluster always stays
   * visible (toggling it would hide the long tail without a way back).
   */
  hiddenClusterIDs: Set<string>
  /**
   * Optional explicit canvas height. When omitted, defaults to the inline
   * sizing (400px on narrow viewports, 560px otherwise). PSY-517 passes an
   * overlay-aware height in fullscreen mode so the canvas fills the viewport
   * minus the header/legend reserve.
   */
  height?: number
}

export function SceneGraphVisualization({
  data,
  containerWidth,
  hiddenClusterIDs,
  height,
}: SceneGraphVisualizationProps) {
  const router = useRouter()

  // PSY-361 inheritance: clicking a node "exits" the scene-scale view and
  // re-centers in that artist's global graph. The actual recentering happens
  // on the artist page; here we just navigate.
  const handleNodeClick = useCallback(
    (node: GraphNode) => {
      router.push(`/artists/${node.slug}`)
    },
    [router],
  )

  const ariaLabel = `Scene relationship graph for ${data.scene.city}, ${data.scene.state}: ${data.scene.artist_count} artists, ${data.scene.edge_count} connections.`

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
