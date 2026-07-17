'use client'

/**
 * VenueBillNetworkAdapter (PSY-365) — venue analog of
 * `SceneGraphVisualization`.
 *
 * Thin shape adapter that maps the backend `VenueBillNetworkResponse` onto
 * the shared `ForceGraphView`'s generic node/cluster/link shape. Keeps the
 * venue-specific aria-label phrasing and panel wiring out of the canvas
 * primitive, mirroring the pattern in
 * `features/scenes/components/SceneGraphVisualization.tsx`.
 *
 * Locked grammar (PSY-1451): on Section-class surfaces a node click SELECTS
 * into the shared ArtistContextPanel; navigation happens only via the
 * panel's "Open page →". The select/deselect/close/focus-return conventions
 * live in the shared `useArtistPanelSelection` hook (shared with
 * HomeSceneGraph, SceneGraphVisualization, and StationGraphVisualization).
 * Selection is instance-local, so toggling fullscreen (which swaps between
 * two separate instances of this component) intentionally resets it — same
 * accepted behavior as the peer surfaces and the edge-inspect
 * ConnectionPanel. Esc layering with the fullscreen overlay is handled by
 * the panel's Radix DismissableLayer (PSY-1355): it preventDefaults in the
 * capture phase, so the overlay's own Esc listener (which skips
 * defaultPrevented) closes only on the NEXT press.
 *
 * The "StyleAdapter" suffix in the export name is intentional: it signals
 * that this component does *no* layout / canvas work itself, only data
 * shaping, so future readers don't expect to find d3-force config here.
 */

import { ForceGraphView } from '@/components/graph/ForceGraphView'
import {
  ArtistContextPanel,
  graphSelectGestureHint,
} from '@/components/graph/ArtistContextPanel'
import { useArtistPanelSelection } from '@/components/graph/useArtistPanelSelection'
import { resolveNodeInVisibleClusters } from '@/components/graph/resolveNodeInVisibleClusters'
// Deep import, deliberately NOT the '@/features/artists' barrel — the barrel
// re-exports the artists component tree, which would drag unrelated module
// code into the venue page's graph chunk (SceneGraphVisualization
// precedent, PSY-868).
import { useArtistGraphCard } from '@/features/artists/hooks/useArtistGraphCard'
import type { VenueBillNetworkResponse } from '../types'

// The venue surface has no cluster legend pills, so nothing is ever hidden —
// the shared resolver still runs so a data refresh (window change) that
// drops the selected node puts the panel away.
const NO_HIDDEN_CLUSTERS: ReadonlySet<string> = new Set()

interface SceneGraphVisualizationStyleAdapterProps {
  data: VenueBillNetworkResponse
  venueName: string
  containerWidth: number
  height?: number
}

export function SceneGraphVisualizationStyleAdapter({
  data,
  venueName,
  containerWidth,
  height,
}: SceneGraphVisualizationStyleAdapterProps) {
  // Node selection → context panel: shared select/deselect/close/focus-return
  // wiring (PSY-1451). The shared resolver checks the CURRENT payload — a
  // window-filter refetch that drops the node must put the panel away rather
  // than strand it naming an off-canvas artist.
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
      resolveNodeInVisibleClusters(selected, data.nodes, NO_HIDDEN_CLUSTERS),
  })

  // Venue bill-network node IDs are artist IDs (PSY-1334 precedent), so the
  // shared artist graph-card endpoint resolves directly.
  const cardQuery = useArtistGraphCard({
    artistId: selectedNode?.id ?? null,
    enabled: selectedNode !== null,
  })

  // a11y: surface artist + edge counts in the canvas description so screen
  // reader users get the same scale info that sighted users see in the
  // header. Window phrasing matches the filter labels. The trailing shared
  // hint names the select gesture — click no longer navigates, so the label
  // must set that expectation (scene convention).
  const windowPhrase =
    data.venue.window === 'last_12m'
      ? 'last 12 months'
      : data.venue.window === 'year' && data.venue.year
        ? `year ${data.venue.year}`
        : 'all time'
  const ariaLabel = `Co-bill network for ${venueName} (${windowPhrase}): ${data.venue.artist_count} artists, ${data.venue.edge_count} co-bills. ${graphSelectGestureHint}`

  return (
    <div ref={canvasWrapRef} tabIndex={-1} className="relative outline-none">
      <ForceGraphView
        nodes={data.nodes}
        links={data.links}
        clusters={data.clusters}
        containerWidth={containerWidth}
        height={height}
        ariaLabel={ariaLabel}
        onNodeClick={handleNodeClick}
        onBackgroundClick={handleBackgroundClick}
        // The aria-label advertises the select gesture, so keyboard and
        // screen-reader users need an equivalent: the focus-revealed node
        // button list drives the same handleNodeClick path (scene surface
        // convention).
        showAccessibleNodeControls
        // PSY-1083: co-bill edges are typed (shared_bills) — opt into the
        // shared edge legend so the venue surface teaches the same grammar.
        showEdgeLegend
        // PSY-1334: click an edge to inspect why the pair is connected.
        showConnectionPanel
        // Edge click opens the ConnectionPanel — deselect so the two
        // inspectors never stack.
        onConnectionInspectOpen={handleConnectionInspectOpen}
      />
      {selectedNode && (
        <ArtistContextPanel
          // Top-LEFT, not HomeSceneGraph's top-right: this surface floats the
          // EdgeLegend at top-2 right-2 (inside ForceGraphView) and the
          // ConnectionPanel at bottom-2 left-2 — top-left is the free corner
          // (SceneGraphVisualization precedent).
          className="absolute top-2 left-2 z-40"
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
