'use client'

/**
 * CollectionGraph (PSY-366)
 *
 * Section wrapper for the collection-scoped artist-relationship graph: header,
 * canvas, fullscreen overlay. Mirrors the SceneGraph (PSY-367, PSY-516,
 * PSY-517) layout pattern — same callback-ref width measurement, same
 * mobile gate, same body-scroll-lock + Esc-close fullscreen overlay.
 *
 * Differs from SceneGraph in three ways:
 *   1. No clusters — collections have no natural cluster signal (curator-
 *      chosen items don't share a venue or scene). `clusters={[]}` is
 *      passed to ForceGraphView; nodes default to the "other" bucket.
 *   2. No `MIN_GRAPH_NODES=3` empty-state gate — the parent
 *      (CollectionDetail) only renders this when artistItemCount > 0, so
 *      the user has explicit intent. A single artist shows as one dot;
 *      that's honest, not a crash.
 *   3. Toggle-driven, not default-visible — collections may have many
 *      non-artist items, and an always-on graph would compete with the
 *      items list for attention. The parent owns the toggle state.
 *
 * Mobile gating retained: below 640px the canvas is unusable (PSY-369),
 * so the graph slot collapses to a teaser message + "Open on a larger
 * screen" affordance.
 */

import { useState, useCallback, useEffect, useMemo } from 'react'
import { useRouter } from 'next/navigation'
import { Maximize2, X } from 'lucide-react'
import { useCollectionGraph } from '../hooks'
import { ForceGraphView } from '@/components/graph/ForceGraphView'
import type { GraphNode } from '@/components/graph/ForceGraphView'

const GRAPH_BREAKPOINT_PX = 640
const OVERLAY_VERTICAL_RESERVE_PX = 140

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

  const handleNodeClick = useCallback(
    (node: GraphNode) => {
      router.push(`/artists/${node.slug}`)
    },
    [router],
  )

  const handleNodeClickOverlay = useCallback(
    (node: GraphNode) => {
      setIsFullscreen(false)
      router.push(`/artists/${node.slug}`)
    },
    [router],
  )

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
        {nodeCount} {nodeCount === 1 ? 'artist' : 'artists'}
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

  const ariaLabel = `Relationship graph for collection ${collectionTitle}: ${nodeCount} artists, ${edgeCount} connections.`

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
            No artist items yet — add an artist to this collection to see its graph.
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
              nodes={data.nodes}
              links={data.links}
              clusters={[]}
              containerWidth={containerWidth!}
              ariaLabel={ariaLabel}
              onNodeClick={handleNodeClick}
            />
            <p className="text-xs text-muted-foreground">
              Showing artists in this collection and their stored relationships
              (shared bills, shared label, member of, side project, similar, radio
              co-occurrence). Click any artist to open their page.
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
                nodes={data.nodes}
                links={data.links}
                clusters={[]}
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
