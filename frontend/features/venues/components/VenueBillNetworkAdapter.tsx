'use client'

/**
 * VenueBillNetworkAdapter (PSY-365) — venue analog of
 * `SceneGraphVisualization`.
 *
 * Thin shape adapter that maps the backend `VenueBillNetworkResponse` onto
 * the shared `ForceGraphView`'s generic node/cluster/link shape. Keeps the
 * venue-specific aria-label phrasing and panel wiring out of the canvas
 * primitive, mirroring the pattern in
 * `features/scenes/components/SceneGraphVisualization.tsx` — with one
 * deliberate difference (PSY-1476): the truncation count phrase is computed
 * ONCE in the parent (VenueBillNetwork) and passed in as `countPhrase`, rather
 * than recomputed here the way scene's canvas recomputes `sceneArtistCountPhrase`.
 * This matches CollectionGraphVisualization (whose canvas isn't handed the
 * truncation fields to recompute from) and keeps this aria-label and the
 * visible header reading one value. Unifying scene onto the same prop later is
 * a possible follow-up.
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
 * The "StyleAdapter" suffix in the export name signals that this component
 * does *no* canvas / d3-force work itself — that all lives in ForceGraphView.
 * It DOES own the venue surface's selection state, the graph-card query, and
 * the floating panel's mount point (the focusable canvas wrap), same as the
 * scene and station adapters.
 */

import { ForceGraphView } from '@/components/graph/ForceGraphView'
import {
  ArtistContextPanel,
  graphSelectGestureHint,
} from '@/components/graph/ArtistContextPanel'
import { GraphPanelHost } from '@/components/graph/GraphPanelHost'
import { useArtistPanelSelection } from '@/components/graph/useArtistPanelSelection'
import { resolveNodeInVisibleClusters } from '@/components/graph/resolveNodeInVisibleClusters'
// Deep import, deliberately NOT the '@/features/artists' barrel — the barrel
// re-exports the artists component tree, which would drag unrelated module
// code into the venue page's graph chunk (SceneGraphVisualization
// precedent, PSY-868).
import { useArtistGraphCard } from '@/features/artists/hooks/useArtistGraphCard'
import { SECTION_LABEL_TIERS } from '@/components/graph/graphLabels'
import type { VenueBillNetworkResponse } from '../types'

// The venue surface has no cluster legend pills, so nothing is ever hidden —
// the shared resolver still runs so a data refresh (window change) that
// drops the selected node puts the panel away.
const NO_HIDDEN_CLUSTERS: ReadonlySet<string> = new Set()

interface SceneGraphVisualizationStyleAdapterProps {
  data: VenueBillNetworkResponse
  venueName: string
  /**
   * The artist-count phrase for the canvas aria-label — "N artists" or, when
   * the roster is capped, "top N of M artists" (PSY-1476). Computed once by the
   * parent (VenueBillNetwork) so the aria-label and the visible header read one
   * value and can't state different numbers.
   */
  countPhrase: string
  containerWidth: number
  height?: number
}

export function SceneGraphVisualizationStyleAdapter({
  data,
  venueName,
  countPhrase,
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
  // PSY-1476: the aria-label carries the SAME truncation disclosure the visible
  // header shows (the `countPhrase` prop, computed once in VenueBillNetwork) —
  // the scene surface's invariant that sighted users and assistive tech never
  // hear different numbers. Read mid-sentence, so lowercase "top N of M
  // artists".
  const ariaLabel = `Co-bill network for ${venueName} (${windowPhrase}): ${countPhrase}, ${data.venue.edge_count} co-bills. ${graphSelectGestureHint}`

  return (
    <GraphPanelHost
      canvasWrapRef={canvasWrapRef}
      panel={
        selectedNode ? (
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
        ) : null
      }
    >
      <ForceGraphView
        nodes={data.nodes}
        links={data.links}
        clusters={data.clusters}
        containerWidth={containerWidth}
        height={height}
        ariaLabel={ariaLabel}
        onNodeClick={handleNodeClick}
        onBackgroundClick={handleBackgroundClick}
        // Pin the focus-dim to the selection (PSY-1478) — grammar in
        // graphFocus.resolveFocusForeground.
        focusNodeId={selectedNode?.id ?? null}
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
        // Locked grammar decision 4 (PSY-1454): the Section-class isolate
        // shelf reads as a labeled group (containment band + "+{N} not yet
        // connected artists" caption) — same opt-in as the scene and
        // station adapters.
        showIsolateShelfLabel
        // Section-class tier ladder: labels size by degree tercile over the
        // rendered set, so hubs read before leaves at rest (locked spec).
        labelTiers={SECTION_LABEL_TIERS}
      />
    </GraphPanelHost>
  )
}
