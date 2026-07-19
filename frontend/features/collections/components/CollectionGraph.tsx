'use client'

/**
 * CollectionGraph (PSY-366, PSY-555, PSY-1473)
 *
 * Section wrapper for the collection's knowledge subgraph: header, canvas,
 * fullscreen overlay. PSY-555 broadened the graph from artist-only to a
 * full multi-type graph (Option B) — every collection item is now a node.
 *
 * PSY-1473: click SELECTS into a context panel (ArtistContextPanel for
 * artists, EntityContextPanel for the other five types) via
 * CollectionGraphVisualization — the last PSY-1451 surface. Navigation
 * only via the panel's "Open page →".
 *
 * Mirrors the SceneGraph (PSY-367, PSY-516, PSY-517) layout pattern — same
 * callback-ref width measurement, same mobile gate, same body-scroll-lock +
 * Esc-close fullscreen overlay.
 *
 * Differs from SceneGraph in three ways:
 *   1. Clusters are entity TYPES (artist / venue / show / release / label /
 *      festival), not scene buckets — entity type is the most useful
 *      grouping signal for a curator-chosen mixed-type set.
 *   2. No `MIN_GRAPH_NODES=3` empty-state gate — the parent
 *      (CollectionDetail) only renders this when the collection has items.
 *   3. Toggle-driven, not default-visible. The parent owns the toggle.
 *
 * Mobile gating retained: below 640px the canvas is unusable (PSY-369),
 * so the graph slot collapses to a teaser message.
 */

import { useMemo } from 'react'
import { Maximize2, X } from 'lucide-react'
import { useCollectionGraph } from '../hooks'
import type { GraphCluster, GraphNode } from '@/components/graph/ForceGraphView'
import { GraphSkeleton } from '@/components/graph/GraphSkeleton'
import {
  GraphStateCard,
  GRAPH_BOX_HEIGHT_CLASS,
  GRAPH_TEASER_HEIGHT_CLASS,
} from '@/components/graph/GraphStateCard'
import { useContainerWidth, GRAPH_BREAKPOINT_PX } from '@/components/graph/useContainerWidth'
import { useFullscreenGraphOverlay } from '@/components/graph/useFullscreenGraphOverlay'
import { truncatedCountPhrase, sentenceCase } from '@/components/graph/truncatedCountPhrase'
import {
  COLLECTION_ENTITY_TYPES,
  getEntityTypeLabel,
} from '../types'
import { CollectionGraphVisualization } from './CollectionGraphVisualization'

/**
 * PSY-555: stable color-index per entity type, indexing into the cluster
 * palette used by ForceGraphView (the `--chart-1..8` theme tokens since
 * PSY-1083). The same type → index mapping is used everywhere in the
 * component so the color the user sees on the canvas matches the legend
 * hint and the icon row.
 *
 * The order is the same as COLLECTION_ENTITY_TYPES so the node-builder
 * iteration and the cluster-legend ordering stay aligned. Indexes 0–5
 * (the remaining tokens stay in reserve).
 */
const ENTITY_COLOR_INDEX: Record<string, number> = {
  artist: 0,
  venue: 1,
  show: 2,
  release: 3,
  label: 4,
  festival: 5,
}

interface CollectionGraphProps {
  slug: string
  collectionTitle: string
}

export function CollectionGraph({ slug, collectionTitle }: CollectionGraphProps) {
  const { data, isLoading, isError } = useCollectionGraph({ slug, enabled: Boolean(slug) })
  const { refCallback: containerRefCallback, containerWidth } = useContainerWidth()

  const isolateCount = useMemo(() => {
    if (!data) return 0
    return data.nodes.reduce((n, node) => (node.is_isolate ? n + 1 : n), 0)
  }, [data])

  // PSY-555: enrich nodes with cluster_id (the entity type), and produce
  // matching cluster definitions so ForceGraphView paints each type in
  // its own color. We only declare clusters that actually appear in the
  // payload, so single-type collections still look clean.
  const { renderNodes, clusters } = useMemo(() => {
    if (!data) {
      return { renderNodes: [] as GraphNode[], clusters: [] as GraphCluster[] }
    }
    const counts: Record<string, number> = {}
    const enriched: GraphNode[] = data.nodes.map(n => {
      const entityType = n.entity_type ?? 'artist'
      counts[entityType] = (counts[entityType] ?? 0) + 1
      return {
        id: n.id,
        name: n.name,
        slug: n.slug,
        city: n.city,
        state: n.state,
        upcoming_show_count: n.upcoming_show_count,
        is_isolate: n.is_isolate,
        cluster_id: entityType,
      }
    })
    const clusterDefs: GraphCluster[] = COLLECTION_ENTITY_TYPES
      .filter(t => (counts[t] ?? 0) > 0)
      .map(t => ({
        id: t,
        label: getEntityTypeLabel(t),
        size: counts[t],
        color_index: ENTITY_COLOR_INDEX[t] ?? -1,
      }))
    return { renderNodes: enriched, clusters: clusterDefs }
  }, [data])

  // PSY-555: subtitle reflects the multi-type breakdown. Uses
  // entity_counts when present, falling back to artist_count for
  // PSY-366-era response shapes.
  const subtitleParts = useMemo(() => {
    if (!data) return [] as string[]
    const counts =
      data.collection.entity_counts ??
      // legacy fallback — old responses only carry artist_count
      ({ artist: data.collection.artist_count } as Record<string, number>)
    return COLLECTION_ENTITY_TYPES.flatMap(t => {
      const count = counts[t] ?? 0
      if (count <= 0) return []
      // singular vs. plural copy — `getEntityTypeLabel` is "Artist"
      // (singular); for the count display we lowercase + pluralize.
      const labelLower = getEntityTypeLabel(t).toLowerCase()
      const pluralized = count === 1 ? labelLower : `${labelLower}s`
      return [`${count} ${pluralized}`]
    })
  }, [data])

  // Always render the wrapper so the callback ref fires even before data
  // arrives — otherwise we'd never measure the container.
  const nodeCount = data?.nodes.length ?? 0
  const edgeCount = data?.collection.edge_count ?? 0
  // Mobile gate: same 640px threshold as SceneGraph.
  const graphAvailable = containerWidth !== null && containerWidth >= GRAPH_BREAKPOINT_PX

  // Overlay lifecycle (scroll lock, Esc, viewport tracking, auto-close when
  // graphAvailable flips false mid-overlay) lives in the shared hook.
  const {
    isFullscreen,
    open: openFullscreen,
    close: closeFullscreen,
    overlayWidth,
    overlayHeight,
  } = useFullscreenGraphOverlay(graphAvailable)

  // PSY-1476: a capped graph must say so. One `truncatedCountPhrase` call gives
  // both the shared "top N of M items" / "N items" phrase AND whether the cue
  // applies — so the header, canvas aria-label, and caption all read one source
  // and can't state different numbers (and there's no second copy of the guard
  // to drift). "items" is the generic noun for the mixed entity types; the
  // helper's `shown > 0` guard makes an all-dropped payload (0 nodes, positive
  // total — a case collection.go still flags) read "No items", not "Top 0 of N".
  // When truncated the cue REPLACES the per-type breakdown (a per-type count of
  // a capped subset would contradict the cap); otherwise the breakdown stands.
  //
  // NOTE: nodes_truncated conflates the 150-node payload cap, the 600-item build
  // ceiling, and unbuildable (deleted-entity) items — so "top" slightly
  // overstates ranking for the latter two. The counts stay truthful and the
  // dominant >150-item case is a genuine degree-ranked cap.
  const { phrase: itemsCountPhrase, truncated: nodesTruncated } = truncatedCountPhrase({
    shown: nodeCount,
    total: data?.collection.node_total,
    truncated: data?.collection.nodes_truncated,
    singular: 'item',
    plural: 'items',
  })
  const leadSegment = nodesTruncated
    ? sentenceCase(itemsCountPhrase)
    : subtitleParts.length > 0
      ? subtitleParts.join(' · ')
      : 'No items'

  const sectionHeader = (
    <div>
      <h2 className="text-lg font-semibold">Collection graph</h2>
      <p className="text-sm text-muted-foreground">
        {leadSegment}
        {edgeCount > 0 && (
          <>
            {' · '}
            {edgeCount} {edgeCount === 1 ? 'connection' : 'connections'}
          </>
        )}
        {isolateCount > 0 && nodeCount > 1 && (
          <>
            {' · '}
            {isolateCount} unconnected
          </>
        )}
      </p>
    </div>
  )

  const expandButton = graphAvailable && !isFullscreen && data && nodeCount > 0 && (
    <button
      type="button"
      onClick={openFullscreen}
      className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-md border border-border/60 hover:bg-muted/50 transition-colors"
      aria-label="Expand collection graph to fullscreen"
    >
      <Maximize2 className="h-4 w-4" aria-hidden="true" />
      <span>Expand</span>
    </button>
  )

  return (
    <>
      <div
        ref={containerRefCallback}
        // PSY-366: `id="graph"` enables Cmd+K deep-links.
        id="graph"
        className="mt-6 scroll-mt-20"
        aria-hidden={isFullscreen || undefined}
        inert={isFullscreen || undefined}
      >
        <div className="flex flex-wrap items-center justify-between gap-2 mb-3">
          {sectionHeader}
          {expandButton}
        </div>

        {/* Loading reserves the graph box (shared GraphSkeleton, PSY-1347)
            instead of bare text — bare text collapses the slot and shifts
            the page when the canvas lands. */}
        {isLoading && <GraphSkeleton className={GRAPH_BOX_HEIGHT_CLASS} />}

        {/* A settled fetch error leaves `data` undefined — say so instead of
            rendering an empty slot (scene-page convention, PSY-1446). */}
        {!isLoading && !data && isError && (
          <GraphStateCard
            role="alert"
            message="This view couldn't load. Refresh the page to try again."
          />
        )}

        {/* Post-load, pre-measurement: hold the box height until the width
            gate can resolve (HomeSceneGraph precedent). */}
        {!isLoading && data && nodeCount > 0 && containerWidth === null && (
          <GraphSkeleton className={GRAPH_BOX_HEIGHT_CLASS} />
        )}

        {!isLoading && data && nodeCount === 0 && (
          <p className="text-sm text-muted-foreground">
            No items yet — add an artist, venue, release, label, festival, or
            show to this collection to see its graph.
          </p>
        )}

        {/* Sub-640px: shared teaser card (PSY-1446) — says WHY + gives a way
            forward (PSY-1472). Link-out scrolls to the collection's item list.
            Unlike the scene/station/venue anchors (new PSY-1472 constants),
            "#items" is the pre-existing, load-bearing CollectionAnchorNav anchor
            (ANCHOR_SECTIONS in CollectionDetail + the id on CollectionItemsList),
            reused here deliberately rather than duplicated as a new constant. */}
        {!isLoading && data && nodeCount > 0 && !graphAvailable && containerWidth !== null && (
          <GraphStateCard
            className={GRAPH_TEASER_HEIGHT_CLASS}
            message={`${collectionTitle} as a map — how its artists, venues, releases, labels, festivals and shows connect. Needs a larger screen.`}
            linkHref="#items"
            linkLabel="Browse the collection →"
          />
        )}

        {!isLoading && data && nodeCount > 0 && graphAvailable && !isFullscreen && (
          <div className="space-y-3">
            <CollectionGraphVisualization
              nodes={renderNodes}
              sourceNodes={data.nodes}
              links={data.links}
              clusters={clusters}
              containerWidth={containerWidth!}
              collectionTitle={collectionTitle}
              countPhrase={itemsCountPhrase}
              edgeCount={edgeCount}
            />
            <p className="text-xs text-muted-foreground">
              {nodesTruncated
                ? `Showing the ${itemsCountPhrase} in this collection`
                : 'Showing every item in this collection'}{' '}
              and the relationships between them — artists, venues they’ve
              played, releases they’ve made, labels they’re on, festivals
              they’ve played, and shows. Click any node for its details.
            </p>
          </div>
        )}
      </div>

      {isFullscreen && data && graphAvailable && (
        <div
          role="dialog"
          aria-modal="true"
          aria-label={`Collection graph for ${collectionTitle}, fullscreen`}
          // z-[60] sits above the cookie consent banner (z-50) so first-time
          // visitors don't see the banner painted over the canvas (PSY-518).
          className="fixed inset-0 z-[60] bg-background flex flex-col"
          data-testid="collection-graph-overlay"
        >
          <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-3 border-b border-border/50">
            {sectionHeader}
            <button
              type="button"
              onClick={closeFullscreen}
              className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-md border border-border/60 hover:bg-muted/50 transition-colors"
              aria-label="Exit fullscreen collection graph"
            >
              <X className="h-4 w-4" aria-hidden="true" />
              <span>Exit</span>
            </button>
          </div>

          <div className="flex-1 min-h-0 px-4 py-2">
            {overlayHeight !== null && overlayWidth !== null && (
              <CollectionGraphVisualization
                nodes={renderNodes}
                sourceNodes={data.nodes}
                links={data.links}
                clusters={clusters}
                containerWidth={overlayWidth}
                height={overlayHeight}
                collectionTitle={collectionTitle}
                countPhrase={itemsCountPhrase}
                edgeCount={edgeCount}
              />
            )}
          </div>
        </div>
      )}
    </>
  )
}
