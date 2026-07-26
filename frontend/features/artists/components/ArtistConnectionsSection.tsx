'use client'

/**
 * ArtistConnectionsSection — the inline Connections ego map in the artist
 * page's main column (under Discography, before Bill composition).
 *
 * Section-class surface: click SELECTS a node into the shared
 * ArtistContextPanel (useArtistPanelSelection grammar — second click
 * deselects, background click closes, selection pins the focus-dim); no node
 * drag; navigation only via the panel's "Open page →". The [Expand] header
 * action opens the existing ArtistGraphDialog (uncapped, full tool), which
 * the header [Graph] link and the sidebar [Explore graph] link also drive.
 *
 * Data: reuses the SAME useArtistGraph query the sidebar Similar-artists
 * list fetches on every artist page load (identical cache key: no types
 * param), so this section adds no network request. The canvas draws only the
 * top CONNECTIONS_NEIGHBOR_CAP neighbors by max center-edge score (a LAYOUT
 * cap — the dialog stays uncapped) and the count line discloses the cap via
 * the shared truncatedCountPhrase.
 *
 * First paint is settled (PSY-1447 pre-settle grammar), in two halves:
 * the ego LAYOUT needs no warmup ticks because egoRingLayout pins every
 * node deterministically (fx/fy) before the first frame, and the CAMERA is
 * framed with an instant `zoomToFit` (`instantFit`) rather than the dialog's
 * 500ms tween — on an inline section a zoom animation reads as the surface
 * snapping into place after the reader has already looked at it.
 *
 * The section hides ENTIRELY when the artist has no relationships (page
 * convention — primary sections self-hide rather than render an apologetic
 * empty state; the sidebar owns the "suggest similar" affordance). Below the
 * 640px canvas gate it renders the shared PSY-1472 teaser card linking to
 * the sidebar Similar-artists list anchor instead of a canvas.
 */

import { useMemo } from 'react'
import { BracketLink, SectionHeader } from '@/components/shared'
import {
  ArtistContextPanel,
  graphSelectGestureHint,
} from '@/components/graph/ArtistContextPanel'
import { GraphPanelHost } from '@/components/graph/GraphPanelHost'
import { GraphSectionErrorBoundary } from '@/components/graph/GraphSectionErrorBoundary'
import { GraphSkeleton } from '@/components/graph/GraphSkeleton'
import {
  GraphStateCard,
  GRAPH_TEASER_HEIGHT_CLASS,
} from '@/components/graph/GraphStateCard'
import { SECTION_LABEL_TIERS } from '@/components/graph/graphLabels'
import {
  truncatedCountPhrase,
  sentenceCase,
} from '@/components/graph/truncatedCountPhrase'
import { useArtistPanelSelection } from '@/components/graph/useArtistPanelSelection'
import {
  useContainerWidth,
  GRAPH_BREAKPOINT_PX,
} from '@/components/graph/useContainerWidth'
import {
  compareByCenterEdgeScore,
  maxCenterEdgeScoreByNeighbor,
} from './egoNeighborRank'
import { useArtistGraph } from '../hooks/useArtistGraph'
import { useArtistGraphCard } from '../hooks/useArtistGraphCard'
import { ArtistGraphVisualization } from './ArtistGraph'
import type { ArtistGraph } from '../types'

/**
 * The scroll-anchor id for the mobile teaser's "See similar artists"
 * link-out (PSY-1472 convention). Single-sourced here so this component's
 * `linkHref` and the ArtistSimilarSidebar section's `id` can't drift apart.
 */
export const SIMILAR_ARTISTS_ANCHOR = 'similar-artists'

/**
 * Layout cap: the inline canvas draws only the strongest neighbors so the
 * ring stays legible in a 360px box. The dialog (via [Expand]) is uncapped.
 */
export const CONNECTIONS_NEIGHBOR_CAP = 14

/** Locked spec: ~360px canvas — a Section box, shorter than the dialog's. */
const CONNECTIONS_CANVAS_HEIGHT = 360

/**
 * Height reserved for the pre-measurement skeleton: the shared teaser height
 * below the 640px canvas gate (whichever box lands there is that tall), the
 * canvas height above it. The `sm` value tracks CONNECTIONS_CANVAS_HEIGHT by
 * hand (Tailwind arbitrary values can't read the const) and approximates the
 * settled box from BELOW — the rendered surface is ~30px taller, because
 * ArtistGraphVisualization stacks the EgoTypeLegend under the canvas inside
 * its bordered container. Reserving the canvas height rather than the exact
 * total is the same accepted trade-off GRAPH_BOX_HEIGHT_CLASS documents, and
 * the residual is invisible in practice: useContainerWidth measures via a
 * callback ref during commit, so this skeleton is typically replaced before
 * the browser paints it.
 */
const PLACEHOLDER_HEIGHT_CLASS = `${GRAPH_TEASER_HEIGHT_CLASS} sm:h-[360px]`

export interface CappedEgoGraph {
  graph: ArtistGraph
  /** Neighbors actually drawn (post-cap). */
  shown: number
  /** Neighbors connected to the center in the full payload (pre-cap). */
  total: number
}

/**
 * Cap the ego payload to the top `cap` neighbors by the shared ego ranking
 * (egoNeighborRank) — the same order and the same center-connected filter the
 * sidebar Similar-artists list uses in its default "relevant" mode, so the two
 * surfaces can't disagree about who ranks highest. (The sidebar's opt-in
 * Discovery sort modes re-order by DOI; this canvas has no mode toggle and
 * always shows the default ranking.)
 *
 * Kept links are those with both endpoints still on canvas, so
 * cross-connections among kept neighbors survive and dangling ones drop.
 */
export function capEgoNeighbors(graph: ArtistGraph, cap: number): CappedEgoGraph {
  const maxCenterScore = maxCenterEdgeScoreByNeighbor(graph)

  const connected = graph.nodes.filter(node => maxCenterScore.has(node.id))
  const kept = [...connected]
    .sort(compareByCenterEdgeScore(maxCenterScore))
    .slice(0, cap)

  const keptIds = new Set<number>([graph.center.id, ...kept.map(n => n.id)])
  const links = graph.links.filter(
    link => keptIds.has(link.source_id) && keptIds.has(link.target_id)
  )

  return {
    graph: {
      center: graph.center,
      nodes: kept,
      links,
      user_votes: graph.user_votes,
    },
    shown: kept.length,
    total: connected.length,
  }
}

interface ArtistConnectionsSectionProps {
  artistId: number
  artistName: string
  /** Fires when the user clicks [Expand]. Parent opens the graph Dialog. */
  onExpand: () => void
}

export function ArtistConnectionsSection({
  artistId,
  artistName,
  onExpand,
}: ArtistConnectionsSectionProps) {
  const { data: graph, isLoading } = useArtistGraph({
    artistId,
    enabled: artistId > 0,
  })
  const { refCallback, containerWidth } = useContainerWidth()

  const capped = useMemo(
    () => (graph ? capEgoNeighbors(graph, CONNECTIONS_NEIGHBOR_CAP) : null),
    [graph]
  )
  // Every payload type renders (no toggles on this surface) — memoized so
  // ArtistGraphVisualization's graphData memo sees a stable identity.
  const activeTypes = useMemo(
    () => new Set(capped?.graph.links.map(link => link.type) ?? []),
    [capped]
  )

  // Node selection → context panel (shared Section-class wiring). The
  // resolver checks the CAPPED graph, so a payload refresh that drops the
  // node puts the panel away. The center is deliberately NOT resolvable:
  // its context panel would just describe the page the user is already on,
  // so a center click reads as a no-op (the selection clears during render).
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
      capped?.graph.nodes.find(node => node.id === selected.id) ?? null,
  })

  const cardQuery = useArtistGraphCard({
    artistId: selectedNode?.id ?? null,
    enabled: selectedNode !== null,
  })

  // Hide entirely while loading, on error, or with no relationships (page
  // convention — no empty-state card; the query is shared with the sidebar,
  // which owns the contribute affordance).
  if (isLoading || !graph || !capped || capped.shown === 0) return null

  const { phrase } = truncatedCountPhrase({
    shown: capped.shown,
    total: capped.total,
    truncated: capped.total > capped.shown,
    singular: 'connected artist',
    plural: 'connected artists',
  })

  return (
    <section ref={refCallback} className="min-w-0">
      <SectionHeader
        title="Connections"
        as="h2"
        size="md"
        action={<BracketLink label="Expand" onClick={onExpand} />}
      />
      <p className="text-sm text-muted-foreground mb-2">
        {sentenceCase(phrase)} · click a name to see how it connects
      </p>

      {/* Pre-measurement: hold the box height so the settle can't shift the
          sections below (HomeSceneGraph precedent). */}
      {containerWidth === null && (
        <GraphSkeleton className={PLACEHOLDER_HEIGHT_CLASS} />
      )}

      {/* Sub-640px: shared teaser card (PSY-1472) — no canvas on mobile;
          link out to the sidebar Similar-artists list on this page. */}
      {containerWidth !== null && containerWidth < GRAPH_BREAKPOINT_PX && (
        <GraphStateCard
          className={GRAPH_TEASER_HEIGHT_CLASS}
          message={`Who ${artistName} plays with, shares labels with, and gets radio play alongside — mapped. Needs a larger screen.`}
          linkHref={`#${SIMILAR_ARTISTS_ANCHOR}`}
          linkLabel="See similar artists →"
        />
      )}

      {containerWidth !== null && containerWidth >= GRAPH_BREAKPOINT_PX && (
        // Contain a graph chunk-load failure to this section (self-hide, no
        // fallback) — a graph problem must never dent the artist page.
        <GraphSectionErrorBoundary sentryTag="artist-connections-section">
          <GraphPanelHost
            canvasWrapRef={canvasWrapRef}
            panel={
              selectedNode ? (
                <ArtistContextPanel
                  // Top-LEFT: the EdgeLegend owns top-right inside the
                  // canvas and the ConnectionPanel sits bottom-left
                  // (SceneGraphVisualization's corner rationale).
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
            <ArtistGraphVisualization
              data={capped.graph}
              activeTypes={activeTypes}
              containerWidth={containerWidth}
              height={CONNECTIONS_CANVAS_HEIGHT}
              // Section-class pre-settle: frame the camera instantly instead
              // of the dialog's 500ms tween (PSY-1447 grammar).
              instantFit
              // Section-class tier ladder (14/11/9): labels size by degree
              // tercile over the rendered set, center largest (locked spec).
              labelTiers={SECTION_LABEL_TIERS}
              onSelect={handleNodeClick}
              onBackgroundClick={handleBackgroundClick}
              onConnectionInspectOpen={handleConnectionInspectOpen}
              // Pin the focus-dim to the selection (PSY-1478) — grammar in
              // graphFocus.resolveFocusForeground.
              focusNodeId={selectedNode?.id ?? null}
              canvasAriaLabel={`Connections map for ${artistName}: ${phrase}. Use the Similar artists list in the sidebar to browse without the canvas. ${graphSelectGestureHint}`}
            />
          </GraphPanelHost>
        </GraphSectionErrorBoundary>
      )}
    </section>
  )
}
