'use client'

/**
 * CollectionGraph (PSY-366, PSY-555)
 *
 * Section wrapper for the collection's knowledge subgraph: header, canvas,
 * fullscreen overlay. PSY-555 broadened the graph from artist-only to a
 * full multi-type graph (Option B) — every collection item is now a node.
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

import { useState, useCallback, useEffect, useMemo } from 'react'
import { useRouter } from 'next/navigation'
import { Maximize2, X } from 'lucide-react'
import { useCollectionGraph } from '../hooks'
import { ForceGraphView } from '@/components/graph/ForceGraphView'
import type { GraphCluster, GraphNode } from '@/components/graph/ForceGraphView'
import {
  COLLECTION_ENTITY_TYPES,
  getEntityTypeLabel,
  getEntityUrl,
} from '../types'

const GRAPH_BREAKPOINT_PX = 640
const OVERLAY_VERTICAL_RESERVE_PX = 140

/**
 * PSY-555: stable color-index per entity type, indexing into the
 * Okabe-Ito 8-color palette already used by ForceGraphView. The same
 * type → index mapping is used everywhere in the component so the color
 * the user sees on the canvas matches the legend hint and the icon row.
 *
 * The order is the same as COLLECTION_ENTITY_TYPES so the node-builder
 * iteration and the cluster-legend ordering stay aligned.
 *
 * Indexes 0–5 (skipping 6/yellow which has poor contrast on dark mode).
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
  const router = useRouter()
  const { data, isLoading } = useCollectionGraph({ slug, enabled: Boolean(slug) })
  const [containerWidth, setContainerWidth] = useState<number | null>(null)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [overlayHeight, setOverlayHeight] = useState<number | null>(null)
  const [overlayWidth, setOverlayWidth] = useState<number | null>(null)

  // Callback ref + ResizeObserver. Same PSY-519 pattern as SceneGraph and
  // RelatedArtists — useEffect with `[]` deps would only fire on the initial
  // mount when ref.current is still null and never re-run.
  const containerRefCallback = useCallback((node: HTMLDivElement | null) => {
    if (!node) return
    setContainerWidth(node.getBoundingClientRect().width)
    const observer = new ResizeObserver(entries => {
      for (const entry of entries) {
        setContainerWidth(entry.contentRect.width)
      }
    })
    observer.observe(node)
    return () => observer.disconnect()
  }, [])

  // Fullscreen overlay side effects (PSY-517 pattern).
  useEffect(() => {
    if (!isFullscreen) return

    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'

    const updateDimensions = () => {
      setOverlayWidth(window.innerWidth)
      setOverlayHeight(Math.max(200, window.innerHeight - OVERLAY_VERTICAL_RESERVE_PX))
    }
    updateDimensions()

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setIsFullscreen(false)
    }

    document.addEventListener('keydown', handleKeyDown)
    window.addEventListener('resize', updateDimensions)

    return () => {
      document.body.style.overflow = previousOverflow
      document.removeEventListener('keydown', handleKeyDown)
      window.removeEventListener('resize', updateDimensions)
    }
  }, [isFullscreen])

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

  // PSY-555: route to the right entity detail page based on the node's
  // entity_type. The render-node's `cluster_id` carries the entity type
  // (set above when enriching). Falls back to /artists/ for legacy nodes
  // that don't carry it — shouldn't happen in production, matches the
  // PSY-366 baseline.
  const navigateToNode = useCallback(
    (node: GraphNode) => {
      const entityType = node.cluster_id ?? 'artist'
      router.push(getEntityUrl(entityType, node.slug))
    },
    [router],
  )

  const handleNodeClickOverlay = useCallback(
    (node: GraphNode) => {
      setIsFullscreen(false)
      navigateToNode(node)
    },
    [navigateToNode],
  )

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

  const sectionHeader = (
    <div>
      <h2 className="text-lg font-semibold">Collection graph</h2>
      <p className="text-sm text-muted-foreground">
        {subtitleParts.length > 0 ? subtitleParts.join(' · ') : 'No items'}
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
      onClick={() => setIsFullscreen(true)}
      className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-md border border-border/60 hover:bg-muted/50 transition-colors"
      aria-label="Expand collection graph to fullscreen"
    >
      <Maximize2 className="h-4 w-4" aria-hidden="true" />
      <span>Expand</span>
    </button>
  )

  const ariaLabel = `Knowledge graph for collection ${collectionTitle}: ${nodeCount} items, ${edgeCount} connections.`

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

        {isLoading && (
          <p className="text-sm text-muted-foreground">Loading graph…</p>
        )}

        {!isLoading && data && nodeCount === 0 && (
          <p className="text-sm text-muted-foreground">
            No items yet — add an artist, venue, release, label, festival, or
            show to this collection to see its graph.
          </p>
        )}

        {!isLoading && data && nodeCount > 0 && !graphAvailable && containerWidth !== null && (
          <p className="text-sm text-muted-foreground">
            Open on a larger screen to view the collection graph.
          </p>
        )}

        {!isLoading && data && nodeCount > 0 && graphAvailable && !isFullscreen && (
          <div className="space-y-3">
            <ForceGraphView
              nodes={renderNodes}
              links={data.links}
              clusters={clusters}
              containerWidth={containerWidth!}
              ariaLabel={ariaLabel}
              onNodeClick={navigateToNode}
            />
            <p className="text-xs text-muted-foreground">
              Showing every item in this collection and the relationships
              between them — artists, venues they’ve played, releases
              they’ve made, labels they’re on, festivals they’ve
              played, and shows. Click any node to open its page.
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
              onClick={() => setIsFullscreen(false)}
              className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-md border border-border/60 hover:bg-muted/50 transition-colors"
              aria-label="Exit fullscreen collection graph"
            >
              <X className="h-4 w-4" aria-hidden="true" />
              <span>Exit</span>
            </button>
          </div>

          <div className="flex-1 min-h-0 px-4 py-2">
            {overlayHeight !== null && overlayWidth !== null && (
              <ForceGraphView
                nodes={renderNodes}
                links={data.links}
                clusters={clusters}
                containerWidth={overlayWidth}
                height={overlayHeight}
                ariaLabel={ariaLabel}
                onNodeClick={handleNodeClickOverlay}
              />
            )}
          </div>
        </div>
      )}
    </>
  )
}
