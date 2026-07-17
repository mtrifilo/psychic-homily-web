'use client'

/**
 * SceneGraphVisualization (PSY-367 → refactored under PSY-365)
 *
 * Thin shape adapter over the shared `ForceGraphView` (PSY-365) — it owns
 * just the scene-specific concerns (a11y label phrasing, node-select →
 * context-panel wiring) and delegates the canvas, layout, hulls, isolate
 * shelf, and tooltip to the shared component.
 *
 * Why a wrapper instead of inlining `<ForceGraphView/>` at the call site:
 *   1. Keeps the public import path stable for callers (SceneGraph.tsx).
 *   2. Owns the scene surface's node-selection state, so the
 *      ArtistContextPanel mounts inside whichever container renders the
 *      canvas — the inline section or the fullscreen overlay's stacking
 *      context — with no per-container wiring in SceneGraph.tsx.
 *   3. Owns the scene-specific aria-label + click semantics; equivalent
 *      pattern lives in `VenueBillNetwork.tsx` for venue scope.
 *
 * Locked grammar (PSY-1451, decision 2026-07-11): on Section-class surfaces
 * a node click SELECTS into the shared ArtistContextPanel; navigation
 * happens only via the panel's "Open page →". Conventions match
 * HomeSceneGraph (PSY-1345): a second click on the selected node deselects,
 * background click and Esc close, and focus returns to the canvas wrap on
 * close. Esc layering with the fullscreen overlay is handled by the panel's
 * Radix DismissableLayer (PSY-1355): it preventDefaults in the capture
 * phase, so the overlay's own Esc listener (which skips defaultPrevented)
 * closes only on the NEXT press.
 *
 * Behaviour preserved (PSY-516, PSY-517, PSY-518, PSY-519 patches):
 *   - all props from the original component remain — the SceneGraph tests
 *     that mock this component via `vi.mock('./SceneGraphVisualization')`
 *     do NOT need to change
 *   - height-prop handling (used by SceneGraph fullscreen overlay) flows
 *     through to ForceGraphView unchanged
 *   - hiddenClusterIDs filter behaviour preserved (Set passed through)
 */

import { useCallback, useRef, useState } from 'react'
import { ForceGraphView } from '@/components/graph/ForceGraphView'
import type { GraphNode } from '@/components/graph/ForceGraphView'
import { ArtistContextPanel } from '@/components/graph/ArtistContextPanel'
// Deep import, deliberately NOT the '@/features/artists' barrel — the barrel
// re-exports the artists component tree, which would drag unrelated module
// code into the scene page's graph chunk (HomeSceneGraph precedent, PSY-868).
import { useArtistGraphCard } from '@/features/artists/hooks/useArtistGraphCard'
import type { SceneGraphResponse } from '../types'
import { sceneArtistCountPhrase } from './sceneGraphCopy'

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
  // Node selection → context panel (PSY-1451, HomeSceneGraph conventions).
  const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null)
  const canvasWrapRef = useRef<HTMLDivElement>(null)

  // Resolve the selection against the CURRENT payload and cluster filter —
  // a legend hide or a cluster-mode refetch that drops the node must put the
  // panel away rather than strand it naming an off-canvas artist. The filter
  // mirrors ForceGraphView's own node cull (empty cluster_id falls back to
  // "other"). React 19.2: clear stale state via the previous-value-guard
  // idiom (adjust state during render), not a setState-in-effect —
  // HomeSceneGraph precedent.
  const currentSelectedNode = selectedNode
    ? (data.nodes.find(
        node =>
          node.id === selectedNode.id &&
          !hiddenClusterIDs.has(node.cluster_id || 'other')
      ) ?? null)
    : null
  if (selectedNode && !currentSelectedNode) {
    setSelectedNode(null)
  }

  const cardQuery = useArtistGraphCard({
    artistId: currentSelectedNode?.id ?? null,
    enabled: currentSelectedNode !== null,
  })

  // Click selects (opens the context panel); navigation happens via the
  // panel's "Open page →". Clicking the already-selected node deselects —
  // a second click reads as "put it away".
  const handleNodeClick = useCallback((node: GraphNode) => {
    setSelectedNode(prev => (prev?.id === node.id ? null : node))
  }, [])

  const handlePanelClose = useCallback(() => {
    setSelectedNode(null)
    // PSY-1313 lesson: return focus via an EXPLICIT ref — after the panel
    // unmounts, focus would otherwise drop to document.body.
    canvasWrapRef.current?.focus()
  }, [])

  // PSY-1296: describe a capped graph honestly — assistive tech hears the
  // exact phrase the visual header shows (shared sceneGraphCopy source), so
  // the two surfaces can't state different numbers for the same graph. The
  // trailing sentence names the select gesture (HomeSceneGraph phrasing) —
  // click no longer navigates, so the label must set that expectation.
  const ariaLabel = `Scene relationship graph for ${data.scene.city}, ${data.scene.state}: ${sceneArtistCountPhrase(data.scene)}, ${data.scene.edge_count} ${data.scene.edge_count === 1 ? 'connection' : 'connections'}. Click a node for that artist’s details.`

  return (
    <div ref={canvasWrapRef} tabIndex={-1} className="relative outline-none">
      <ForceGraphView
        nodes={data.nodes}
        links={data.links}
        clusters={data.clusters}
        containerWidth={containerWidth}
        height={height}
        hiddenClusterIDs={hiddenClusterIDs}
        ariaLabel={ariaLabel}
        onNodeClick={handleNodeClick}
        onBackgroundClick={handlePanelClose}
        // PSY-1083: scene edges are typed (shared_bills / shared_label /
        // member_of / side_project) — opt into the shared edge legend.
        showEdgeLegend
        // PSY-1334: click an edge to inspect why the pair is connected.
        showConnectionPanel
      />
      {currentSelectedNode && (
        <ArtistContextPanel
          // Top-LEFT, not HomeSceneGraph's top-right: this surface floats the
          // EdgeLegend at top-2 right-2 (inside ForceGraphView) and the
          // ConnectionPanel at bottom-2 left-2 — top-left is the free corner.
          className="absolute top-2 left-2 z-40"
          artistName={currentSelectedNode.name}
          artistSlug={currentSelectedNode.slug}
          card={cardQuery.data}
          isError={cardQuery.isError}
          onClose={handlePanelClose}
        />
      )}
    </div>
  )
}
