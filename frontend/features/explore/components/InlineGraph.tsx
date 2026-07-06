'use client'

/**
 * InlineGraph (PSY-837)
 *
 * Lazy-mounted force graph anchored to the featured bill's headliner.
 * The brief calls for the graph initial nodes to come from the bill's
 * lineup; the /explore/featured response only exposes the bill slug +
 * headliner name (not artist IDs), so we resolve the lineup by
 * fetching the show detail when the section scrolls into view, then
 * pass the headliner's artist ID to the existing artist-relationship
 * graph (PSY-365 ForceGraphView via PSY-361 useArtistGraph).
 *
 * Lazy-mount strategy:
 *   1. Render a skeleton placeholder at the natural graph height to
 *      prevent CLS.
 *   2. IntersectionObserver with rootMargin '200px' so the data fetch
 *      kicks off shortly before the placeholder scrolls into view.
 *   3. Only once mounted do we call `useShow` / `useArtistGraph`.
 *
 * Mobile gate (<640px) collapses the graph to a static teaser per
 * PSY-511; canvas touch handling isn't usable at small widths.
 */

import { useCallback } from 'react'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import type { GraphNode } from '@/components/graph/ForceGraphView'
import { useContainerWidth, GRAPH_BREAKPOINT_PX } from '@/components/graph/useContainerWidth'
import { useLazyGraphMount } from '@/components/graph/useLazyGraphMount'
import { GraphSkeleton } from '@/components/graph/GraphSkeleton'
import { createLazyForceGraphView } from '@/components/graph/lazyForceGraphView'
import { GraphSectionErrorBoundary } from '@/components/graph/GraphSectionErrorBoundary'
import { useShow } from '@/features/shows'
import { useArtistGraph } from '@/features/artists/hooks/useArtistGraph'

const GRAPH_HEIGHT_PX = 480

// This surface's lazy-mount placeholder shape (CLS budget): a 16/9 aspect box
// with a min-height floor. The one element is reused as the dynamic-import
// loading fallback AND the pre-mount / data-loading skeleton so they can't
// drift apart. (The shared base look lives in GraphSkeleton — PSY-1347.)
const graphSkeleton = (
  <GraphSkeleton className="aspect-[16/9]" style={{ minHeight: GRAPH_HEIGHT_PX }} />
)

// Visible, announced fallback for when the ForceGraphView chunk fails to load
// (e.g. a deploy rotated the hashed chunk while the page was open). The App
// Router THROWS a failed chunk fetch to the nearest error boundary — it does
// NOT re-invoke `loading` with `error` — so this renders as the fallback of the
// GraphSectionErrorBoundary wrapping the mount below, and `onRetry` is the
// boundary's reset (PSY-1359, reconciled with HomeSceneGraph). The graph is an
// optional below-the-fold section, so a failure must be perceivable but must
// not take down the rest of /explore.
function GraphLoadError({ onRetry }: { onRetry?: () => void }) {
  return (
    <div
      role="alert"
      className="aspect-[16/9] w-full rounded-lg border border-border/50 bg-muted/10 flex flex-col items-center justify-center gap-3 text-center p-6"
      style={{ minHeight: GRAPH_HEIGHT_PX }}
    >
      <p className="text-sm text-muted-foreground">
        The lineup graph couldn&apos;t load.
      </p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="text-sm text-primary hover:underline underline-offset-4"
        >
          Try again
        </button>
      )}
    </div>
  )
}

// Shared lazy ForceGraphView (PSY-1359): its own dynamic(ssr:false) chunk fetched
// only when the graph mounts, so the heavy renderer stays out of /explore's
// initial JS (PSY-868). `loading` is the happy-path skeleton; a failed chunk fetch
// throws to the GraphSectionErrorBoundary at the mount (App Router).
const ForceGraphView = createLazyForceGraphView(graphSkeleton)

interface InlineGraphProps {
  /** Featured bill slug — drives the show-detail fetch for lineup. */
  billSlug: string
  /** Featured bill title — for the canvas aria-label. */
  billTitle: string
  /** Featured bill href — for the mobile teaser fallback link. */
  billHref: string
}

export function InlineGraph({ billSlug, billTitle, billHref }: InlineGraphProps) {
  const router = useRouter()
  // Lazy-mount on scroll intent (shared hook — PSY-1347). Pre-loads 200px
  // before the placeholder is fully on-screen; once mounted the data hooks
  // below start fetching and it never tears down.
  const { containerRef, isMounted } = useLazyGraphMount()

  // Width measurement only kicks in after mount; the shared callback-ref +
  // ResizeObserver hook covers the null-on-first-render mount pattern.
  const { refCallback: measureRefCallback, containerWidth } = useContainerWidth()

  const { data: showData } = useShow(isMounted ? billSlug : '')
  const headlinerId = showData?.artists.find(a => a.is_headliner)?.id
    ?? showData?.artists[0]?.id
    ?? 0

  const { data: graphData, isLoading: graphLoading } = useArtistGraph({
    artistId: headlinerId,
    enabled: isMounted && headlinerId > 0,
  })

  const handleNodeClick = useCallback(
    (node: GraphNode) => {
      router.push(`/artists/${node.slug}`)
    },
    [router],
  )

  const graphAvailable =
    containerWidth !== null && containerWidth >= GRAPH_BREAKPOINT_PX

  // Static teaser for <640px viewports per PSY-511.
  const teaser = (
    <div className="aspect-[16/9] w-full rounded-lg border border-border/50 bg-muted/20 flex flex-col items-center justify-center text-center p-6 gap-3">
      <p className="text-sm text-muted-foreground max-w-xs">
        The interactive knowledge graph is best on a tablet or larger
        screen.
      </p>
      <Link
        href={billHref}
        className="text-sm text-primary hover:underline underline-offset-4"
      >
        View the bill instead →
      </Link>
    </div>
  )

  // Reserves the graph's natural height so the section below doesn't
  // shift when the canvas mounts (CLS budget). Same element the dynamic
  // import shows while the ForceGraphView chunk loads.
  const skeleton = graphSkeleton

  return (
    <div ref={containerRef} className="w-full">
      <div ref={measureRefCallback} className="w-full">
        {!isMounted && skeleton}

        {isMounted && containerWidth !== null && !graphAvailable && teaser}

        {isMounted && graphAvailable && (
          <>
            {graphLoading && skeleton}
            {graphData && graphData.nodes.length > 0 && (
              // A failed ForceGraphView chunk fetch throws here (App Router);
              // the boundary catches it, reports to Sentry, and shows the
              // recoverable GraphLoadError card instead of letting it bubble
              // uncaught and take down /explore (PSY-1359). Retry is a full
              // reload — the only reliable recovery: the dominant failure is a
              // deploy rotating the hashed chunk, so the open page's baked-in
              // chunk URL 404s no matter how often it re-imports; only fresh HTML
              // carries the new URL (and React.lazy caches the rejected import
              // anyway, so a boundary reset would just re-throw).
              <GraphSectionErrorBoundary
                sentryTag="explore-inline-graph"
                fallback={<GraphLoadError onRetry={() => window.location.reload()} />}
              >
                <ForceGraphView
                  nodes={graphData.nodes}
                  links={graphData.links}
                  /* clusters omitted → ForceGraphView's stable EMPTY_CLUSTERS default
                     (a fresh [] here would destabilize centroids per render — PSY-1217) */
                  containerWidth={containerWidth}
                  height={GRAPH_HEIGHT_PX}
                  ariaLabel={`Knowledge graph anchored to ${billTitle}: ${graphData.nodes.length} artists.`}
                  onNodeClick={handleNodeClick}
                />
              </GraphSectionErrorBoundary>
            )}
            {!graphLoading && (!graphData || graphData.nodes.length === 0) && (
              <div className="aspect-[16/9] w-full rounded-lg border border-border/50 bg-muted/10 flex items-center justify-center text-sm text-muted-foreground">
                Not enough related artists to render the graph yet.
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
