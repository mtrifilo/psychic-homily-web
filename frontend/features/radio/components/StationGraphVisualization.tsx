'use client'

/**
 * StationGraphVisualization (PSY-1299, click-selects under PSY-1451)
 *
 * Thin shape adapter over the shared `ForceGraphView` for the station
 * co-occurrence graph — the station analog of `SceneGraphVisualization`.
 * It owns only the station-specific concerns (a11y label phrasing,
 * node-select → context-panel wiring) and delegates canvas, layout, hulls,
 * isolate shelf, and tooltips to the shared component.
 *
 * Locked grammar (PSY-1451): a node click SELECTS into the shared
 * ArtistContextPanel; navigation happens only via the panel's "Open page →".
 * The select/deselect/close/focus-return conventions live in the shared
 * `useArtistPanelSelection` hook. Selection is instance-local, so toggling
 * fullscreen (which swaps between two separate instances of this component)
 * intentionally resets it — same accepted behavior as the scene surface and
 * the edge-inspect ConnectionPanel. Esc layering with the fullscreen overlay
 * is handled by the panel's Radix DismissableLayer (PSY-1355): it
 * preventDefaults in the capture phase, so the overlay's own Esc listener
 * (which skips defaultPrevented) closes only on the NEXT press.
 *
 * The edge legend is intentionally OFF: every station edge is the single
 * `radio_cooccurrence` type, so a one-row legend adds noise. The cluster
 * pills (per-show colors, owned by StationGraph.tsx) carry the legend duty —
 * which also leaves the top-right corner free for the context panel
 * (HomeSceneGraph's placement; the scene surface uses top-left because its
 * EdgeLegend floats top-right inside ForceGraphView).
 */

import { ForceGraphView } from '@/components/graph/ForceGraphView'
import { SECTION_LABEL_TIERS } from '@/components/graph/graphLabels'
import {
  ArtistContextPanel,
  graphSelectGestureHint,
} from '@/components/graph/ArtistContextPanel'
import { useArtistPanelSelection } from '@/components/graph/useArtistPanelSelection'
import { resolveNodeInVisibleClusters } from '@/components/graph/resolveNodeInVisibleClusters'
// Deep import, deliberately NOT the '@/features/artists' barrel — the barrel
// re-exports the artists component tree, which would drag unrelated module
// code into the station page's graph chunk (SceneGraphVisualization
// precedent, PSY-868).
import { useArtistGraphCard } from '@/features/artists/hooks/useArtistGraphCard'
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
  // Node selection → context panel: shared select/deselect/close/focus-return
  // wiring (PSY-1451). The shared resolver checks the CURRENT payload and
  // cluster filter — hiding a cluster pill or a data refresh that drops the
  // node must put the panel away rather than strand it naming an off-canvas
  // artist.
  const {
    selectedNode,
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

  const cardQuery = useArtistGraphCard({
    artistId: selectedNode?.id ?? null,
    enabled: selectedNode !== null,
  })

  // The trailing shared hint names the select gesture — click no longer
  // navigates, so the label must set that expectation (scene convention).
  const ariaLabel = `Airplay graph for ${data.station.name}: ${data.station.artist_count} artists, ${data.station.edge_count} connections. Use the shows and playlists lists to browse without the canvas. ${graphSelectGestureHint}`

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
        onBackgroundClick={handleBackgroundClick}
        // The aria-label advertises the select gesture, so keyboard and
        // screen-reader users need an equivalent: the focus-revealed node
        // button list drives the same handleNodeClick path (scene surface
        // convention).
        showAccessibleNodeControls
        // PSY-1334: click an edge to inspect why the pair is connected
        // ("played together on N radio shows across M stations").
        showConnectionPanel
        // Edge click opens the ConnectionPanel — deselect so the two
        // inspectors never stack.
        onConnectionInspectOpen={handleConnectionInspectOpen}
        // Locked grammar decision 4 (PSY-1454): the Section-class isolate
        // shelf reads as a labeled group (containment band + "+{N} not yet
        // connected artists" caption).
        showIsolateShelfLabel
        // Section-class tier ladder: labels size by degree tercile over the
        // rendered set, so hubs read before leaves at rest (locked spec).
        labelTiers={SECTION_LABEL_TIERS}
      />
      {selectedNode && (
        <ArtistContextPanel
          className="absolute top-2 right-2 z-40"
          artistName={selectedNode.name}
          artistSlug={selectedNode.slug}
          card={cardQuery.data}
          isError={cardQuery.isError}
          onClose={handlePanelClose}
          panelRef={panelRef}
        />
      )}
    </div>
  )
}
