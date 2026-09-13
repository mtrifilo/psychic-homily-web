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
 * 640px canvas gate the whole section is one line of link into the knowledge
 * graph (MobileGraphTeaser); the page header's [Graph] is what still reaches
 * the ego dialog at that width.
 */

import { useMemo, useState } from 'react'
import { BracketLink, SectionHeader } from '@/components/shared'
import {
  ArtistContextPanel,
  graphSelectGestureHint,
} from '@/components/graph/ArtistContextPanel'
import { GraphPanelHost } from '@/components/graph/GraphPanelHost'
import { GraphSectionErrorBoundary } from '@/components/graph/GraphSectionErrorBoundary'
import { GraphSkeleton } from '@/components/graph/GraphSkeleton'
import { MobileGraphTeaser } from '@/components/graph/MobileGraphTeaser'
import { SECTION_LABEL_TIERS } from '@/components/graph/graphLabels'
import {
  truncatedCountPhrase,
  sentenceCase,
} from '@/components/graph/truncatedCountPhrase'
import { useArtistPanelSelection } from '@/components/graph/useArtistPanelSelection'
import {
  useContainerWidth,
  GRAPH_BREAKPOINT_PX,
  GRAPH_CHROME_UNMEASURED_CLASS,
} from '@/components/graph/useContainerWidth'
import {
  compareByCenterEdgeScore,
  maxCenterEdgeScoreByNeighbor,
} from './egoNeighborRank'
import { useArtistGraph } from '../hooks/useArtistGraph'
import { graphRootHref } from '@/features/graph/graphRootLink'
import { useArtistGraphCard } from '../hooks/useArtistGraphCard'
import { ArtistGraphVisualization } from './ArtistGraph'
import type { ArtistGraph } from '../types'

/**
 * The scroll-anchor id hung off the sidebar's Similar-artists list.
 *
 * Nothing in this component links to it: the sub-640px form is one line of
 * knowledge-graph cross-link. It stays exported because ArtistSimilarSidebar
 * stamps it on that section and the canvas aria-label names the list as the
 * no-canvas way to browse.
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
 * Height reserved for the pre-measurement skeleton, and the viewport gate that
 * keeps it from painting where nothing that tall can land: below 640px the
 * settled section is one line of link, so a reserved canvas box there would be
 * a phantom the settle then collapses.
 *
 * The height tracks CONNECTIONS_CANVAS_HEIGHT by hand (Tailwind arbitrary
 * values can't read the const) and approximates the settled box from BELOW —
 * the rendered surface is ~30px taller, because ArtistGraphVisualization
 * stacks the EgoTypeLegend under the canvas inside its bordered container.
 * Reserving the canvas height rather than the exact total is the same accepted
 * trade-off GRAPH_BOX_HEIGHT_CLASS documents, and the residual is invisible in
 * practice: useContainerWidth measures via a callback ref during commit, so
 * this skeleton is typically replaced before the browser paints it.
 */
const PLACEHOLDER_BOX_CLASS = 'hidden sm:block h-[360px]'

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
  /**
   * Roots the sub-640px teaser's map link on this artist. Required, not
   * optional: an omitted slug is indistinguishable from a null one, and the
   * rooting is the whole difference between that link landing on this artist's
   * neighborhood and landing on the map's search prompt. Entity slugs are
   * nullable in this schema, and `graphRootHref` falls back to the unrooted map
   * for a null.
   */
  artistSlug: string | null
  /** Fires when the user clicks [Expand]. Parent opens the graph Dialog. */
  onExpand: () => void
}

export function ArtistConnectionsSection({
  artistId,
  artistName,
  artistSlug,
  onExpand,
}: ArtistConnectionsSectionProps) {
  const { data: graph, isLoading } = useArtistGraph({
    artistId,
    enabled: artistId > 0,
  })
  const { refCallback, containerWidth, isBelowGraphBreakpoint } = useContainerWidth()

  // A caught graph failure is remembered PER ARTIST rather than as a bare
  // boolean, and compared during render rather than synced by an effect: this
  // component sits at a stable position across artist routes, so a bare flag
  // could outlive the artist it was observed for and permanently suppress the
  // interaction clause for a healthy graph.
  const [failedArtistId, setFailedArtistId] = useState<number | null>(null)
  const graphFailed = failedArtistId === artistId

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

  // One container measurement drives every GATING branch below — the count
  // line's interaction clause, the mobile teaser, and the canvas — through the
  // two derivations the width hook owns, so the clause can't promise names to
  // click in a layout that rendered no canvas. Don't add a *gating* breakpoint
  // source of your own; the `sm:` prefixes in this file are viewport-keyed and
  // govern only what an UNMEASURED render paints, which is the band where a
  // column under 640 sits inside a viewport over it.
  //
  // A chunk-load failure is the gate's SECOND input, not an exception to it:
  // the boundary below still self-hides (no `fallback`), but it reports the
  // catch via `onError`, and `graphFailed` retracts the clause with it — the
  // section keeps its header and count, and only the promise of interaction
  // goes away.
  //
  // Pre-measurement (`null`) counts as no canvas: the skeleton paint must not
  // flash a "click a name" instruction that then disappears on mobile.
  const isMeasured = containerWidth !== null
  // `graphAvailable` matches the six sibling graph surfaces; this gate is the
  // measured width of THIS column, not a device or viewport class.
  const graphAvailable = isMeasured && containerWidth >= GRAPH_BREAKPOINT_PX

  return (
    <section ref={refCallback} className="min-w-0">
      {isBelowGraphBreakpoint ? (
        /* The section's sub-640px form: the header, the count line and
           [Expand] all go with the canvas. The page header's [Graph] is the
           surviving path to the ego dialog at this width. */
        <MobileGraphTeaser
          href={graphRootHref(artistSlug)}
        >{`See who ${artistName} plays with on the music map`}</MobileGraphTeaser>
      ) : (
        <>
          {/* Unmeasured, this branch is what a phone's server HTML carries,
              and the measurement is about to replace it with one line. */}
          <SectionHeader
            title="Connections"
            as="h2"
            size="md"
            action={<BracketLink label="Expand" onClick={onExpand} />}
            className={!isMeasured ? GRAPH_CHROME_UNMEASURED_CLASS : undefined}
          />
          {/* The count discloses scale at every width the section keeps its
              header; the interaction clause is narrower still — it is dropped
              whenever no canvas rendered. */}
          <p
            className={`text-sm text-muted-foreground mb-2 ${
              !isMeasured ? GRAPH_CHROME_UNMEASURED_CLASS : ''
            }`}
          >
            {sentenceCase(phrase)}
            {graphAvailable && !graphFailed && ' · click a name to see how it connects'}
          </p>

          {/* Pre-measurement: hold the box height so the settle can't shift the
              sections below (HomeSceneGraph precedent). */}
          {!isMeasured && <GraphSkeleton className={PLACEHOLDER_BOX_CLASS} />}

          {graphAvailable && (
            // Contain a graph chunk-load failure to this section (self-hide, no
            // fallback) — a graph problem must never dent the artist page — and
            // report the catch upward so the count line's interaction clause goes
            // with the canvas instead of promising names nothing renders.
            //
            // Keyed by artist for the same reason `graphFailed` is: the boundary
            // latches `failed` for its lifetime and offers no reset, so without
            // the key a failure on one artist would keep the canvas hidden after
            // navigating to the next one while `graphFailed` had already reset —
            // the same clause/canvas split, inverted.
            <GraphSectionErrorBoundary
              key={artistId}
              sentryTag="artist-connections-section"
              onError={() => setFailedArtistId(artistId)}
            >
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
        </>
      )}
    </section>
  )
}
