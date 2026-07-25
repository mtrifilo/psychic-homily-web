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
 *      context — with no per-container wiring in SceneGraph.tsx. Selection
 *      is instance-local, so toggling fullscreen (which swaps between two
 *      separate instances) intentionally resets it — same accepted behavior
 *      as the edge-inspect ConnectionPanel, whose state also lives
 *      per-instance inside ForceGraphView.
 *   3. Owns the scene-specific aria-label + click semantics; equivalent
 *      pattern lives in `VenueBillNetwork.tsx` for venue scope.
 *
 * Locked grammar (PSY-1451): on Section-class surfaces a node click SELECTS
 * into the shared ArtistContextPanel; navigation happens only via the
 * panel's "Open page →". The select/deselect/close/focus-return conventions
 * live in the shared `useArtistPanelSelection` hook (PSY-1451 — shared with
 * HomeSceneGraph and StationGraphVisualization). Esc layering with the
 * fullscreen overlay is handled by the panel's Radix DismissableLayer
 * (PSY-1355): it preventDefaults in the capture phase, so the overlay's own
 * Esc listener (which skips defaultPrevented) closes only on the NEXT press.
 *
 * Behaviour preserved (PSY-516, PSY-517, PSY-518, PSY-519 patches):
 *   - all props from the original component remain — the SceneGraph tests
 *     that mock this component via `vi.mock('./SceneGraphVisualization')`
 *     do NOT need to change
 *   - height-prop handling (used by SceneGraph fullscreen overlay) flows
 *     through to ForceGraphView unchanged
 *   - hiddenClusterIDs filter behaviour preserved (Set passed through)
 */

import { useMemo } from 'react'
import { ForceGraphView } from '@/components/graph/ForceGraphView'
import { SECTION_LABEL_TIERS } from '@/components/graph/graphLabels'
import { ArtistContextPanel } from '@/components/graph/ArtistContextPanel'
import {
  EntityContextPanel,
  graphEntitySelectGestureHint,
} from '@/components/graph/EntityContextPanel'
import { GraphPanelHost } from '@/components/graph/GraphPanelHost'
import {
  isLabelHubNode,
  LABEL_HUB_ENTITY_TYPE,
  labelHubHomeCaption,
} from '@/components/graph/labelHub'
import { useArtistPanelSelection } from '@/components/graph/useArtistPanelSelection'
import { resolveNodeInVisibleClusters } from '@/components/graph/resolveNodeInVisibleClusters'
// Deep import, deliberately NOT the '@/features/artists' barrel — the barrel
// re-exports the artists component tree, which would drag unrelated module
// code into the scene page's graph chunk (HomeSceneGraph precedent, PSY-868).
import { useArtistGraphCard } from '@/features/artists/hooks/useArtistGraphCard'
import type { SceneGraphResponse } from '../types'
import { sceneArtistCountPhrase, sceneLabelCountPhrase } from './sceneGraphCopy'
import { SCENE_EDGE_TYPE_ON_LABEL } from '../types'

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
  // Node selection → context panel: shared select/deselect/close/focus-return
  // wiring (PSY-1451). The shared resolver checks the CURRENT payload and
  // cluster filter — a legend hide or a cluster-mode refetch that drops the
  // node must put the panel away rather than strand it naming an off-canvas
  // artist.
  const {
    selectedNode: currentSelectedNode,
    canvasWrapRef,
    panelRef,
    handleNodeClick,
    handleBackgroundClick,
    handlePanelClose,
    handleConnectionInspectOpen,
  } = useArtistPanelSelection({
    resolveNode: selected =>
      resolveNodeInVisibleClusters(selected, data.nodes, hiddenClusterIDs),
  })

  // A label hub is not an artist: its panel is the shared EntityContextPanel
  // (PSY-1473), and asking the artist-card endpoint for a hub's offset node ID
  // would 404.
  const selectedIsHub =
    currentSelectedNode !== null && isLabelHubNode(currentSelectedNode)

  const cardQuery = useArtistGraphCard({
    artistId: !selectedIsHub ? (currentSelectedNode?.id ?? null) : null,
    enabled: currentSelectedNode !== null && !selectedIsHub,
  })

  // The hub's payoff — "N artists on this label in this scene" — comes from the
  // payload's own spokes, so the panel needs no fetch. Roster artists in a
  // hidden cluster are excluded, because those spokes are culled from the
  // canvas too and a visibly-smaller star must not be captioned with a bigger
  // number. NOT canvas-exact: ForceGraphView owns edge-type hide/solo state
  // internally, so hiding the "On Label" type leaves this count unchanged
  // (the hub itself then leaves the canvas). Hence "in this scene", not "in
  // this graph" — the number describes the scene's roster, not a filtered view.
  const hubRosterInGraph = useMemo(() => {
    if (!currentSelectedNode || !selectedIsHub) return 0
    const visibleArtistIds = new Set(
      data.nodes
        .filter(node => !hiddenClusterIDs.has(node.cluster_id || 'other'))
        .map(node => node.id),
    )
    const roster = new Set<number>()
    for (const link of data.links) {
      if (link.type !== SCENE_EDGE_TYPE_ON_LABEL) continue
      const other =
        link.source_id === currentSelectedNode.id
          ? link.target_id
          : link.target_id === currentSelectedNode.id
            ? link.source_id
            : null
      if (other !== null && visibleArtistIds.has(other)) roster.add(other)
    }
    return roster.size
  }, [currentSelectedNode, selectedIsHub, data.links, data.nodes, hiddenClusterIDs])

  // PSY-1296: describe a capped graph honestly — assistive tech hears the
  // exact phrase the visual header shows (shared sceneGraphCopy source), so
  // the two surfaces can't state different numbers for the same graph. The
  // trailing shared hint names the select gesture — click no longer
  // navigates, so the label must set that expectation.
  // Assistive tech hears the same populations the visual header shows (shared
  // sceneGraphCopy source), including the label hubs.
  const labelPhrase = sceneLabelCountPhrase(data.scene)
  const ariaLabel = `Scene relationship graph for ${data.scene.city}, ${data.scene.state}: ${sceneArtistCountPhrase(data.scene)}${labelPhrase ? `, ${labelPhrase}` : ''}, ${data.scene.edge_count} ${data.scene.edge_count === 1 ? 'connection' : 'connections'}. ${graphEntitySelectGestureHint}`

  return (
    <GraphPanelHost
      canvasWrapRef={canvasWrapRef}
      panel={
        currentSelectedNode && selectedIsHub ? (
          <EntityContextPanel
            className="absolute top-2 left-2 z-40"
            entityType={LABEL_HUB_ENTITY_TYPE}
            name={currentSelectedNode.name}
            slug={currentSelectedNode.slug}
            // Hubs are not gated to scene-local labels, so the panel states the
            // label's home the same way the canvas caption does.
            meta={labelHubHomeCaption(currentSelectedNode) ?? null}
            primary={
              hubRosterInGraph > 0
                ? {
                    kind: 'emphasis',
                    text: `${hubRosterInGraph} ${hubRosterInGraph === 1 ? 'artist' : 'artists'} on this label in this scene`,
                  }
                : null
            }
            onClose={handlePanelClose}
            panelRef={panelRef}
          />
        ) : currentSelectedNode ? (
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
            panelRef={panelRef}
          />
        ) : null
      }
    >
      <ForceGraphView
        nodes={data.nodes}
        links={data.links}
        clusters={data.clusters}
        containerWidth={containerWidth}
        height={height}
        hiddenClusterIDs={hiddenClusterIDs}
        ariaLabel={ariaLabel}
        onNodeClick={handleNodeClick}
        onBackgroundClick={handleBackgroundClick}
        // Pin the focus-dim to the selection (PSY-1478) — grammar in
        // graphFocus.resolveFocusForeground.
        focusNodeId={currentSelectedNode?.id ?? null}
        // The aria-label advertises the select gesture, so keyboard and
        // screen-reader users need an equivalent: the focus-revealed node
        // button list drives the same handleNodeClick path (HomeSceneGraph
        // convention).
        showAccessibleNodeControls
        // PSY-1083: scene edges are typed (shared_bills / shared_label /
        // member_of / side_project) — opt into the shared edge legend.
        showEdgeLegend
        // PSY-1334: click an edge to inspect why the pair is connected.
        showConnectionPanel
        onConnectionInspectOpen={handleConnectionInspectOpen}
        // Locked grammar decision 4 (PSY-1454): the Section-class isolate
        // shelf reads as a labeled group (containment band + "+{N} not yet
        // connected artists" caption).
        showIsolateShelfLabel
        // Section-class tier ladder: labels size by degree tercile over the
        // rendered set, so hubs read before leaves at rest (locked spec).
        labelTiers={SECTION_LABEL_TIERS}
      />
    </GraphPanelHost>
  )
}
