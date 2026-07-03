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

import { useCallback, useEffect, useRef, useState } from 'react'
import dynamic from 'next/dynamic'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import type { GraphNode } from '@/components/graph/ForceGraphView'
import { useContainerWidth, GRAPH_BREAKPOINT_PX } from '@/components/graph/useContainerWidth'
import { useShow } from '@/features/shows'
import { useArtistGraph } from '@/features/artists/hooks/useArtistGraph'

const GRAPH_HEIGHT_PX = 480
const INTERSECTION_ROOT_MARGIN = '200px'

// Placeholder reserving the graph's natural height (CLS budget). Reused
// as the dynamic-import loading fallback AND the pre-mount / data-loading
// skeleton below, so the two can't drift apart.
function GraphSkeleton() {
  return (
    <div
      className="aspect-[16/9] w-full rounded-lg border border-border/50 bg-muted/10 animate-pulse"
      style={{ minHeight: GRAPH_HEIGHT_PX }}
      aria-hidden="true"
    />
  )
}

// Visible, announced fallback for when the ForceGraphView chunk fails to
// load. next/dynamic does NOT throw a failed chunk fetch to an error
// boundary — it re-invokes `loading` with `error` set — so without this
// the user would sit on the aria-hidden skeleton forever. The graph is an
// optional below-the-fold section, so a failure must be perceivable but
// must not take down the rest of /explore.
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

// ForceGraphView is /explore's only static reach into the shared graph
// chunk: a static import keeps it (and that chunk) in /explore's initial
// JS even though the graph is below the fold and IntersectionObserver-
// gated. dynamic(ssr:false) splits the wrapper into its own async chunk
// fetched only when the graph mounts; the heavy renderer underneath
// (react-force-graph-2d) is already lazy. The canvas never renders
// server-side, so ssr:false costs nothing. See PSY-868.
//
// Peer ForceGraphView consumers (Scene / Venue / Collection graphs) keep
// the static import on purpose: there the graph IS the page's primary
// content, not a below-the-fold widget on a perf-budgeted landing page,
// so the split would only add a chunk round-trip. The homepage section
// (HomeSceneGraph, PSY-1344) is the other below-the-fold consumer and
// splits the same way; extraction of this shared shell is PSY-1347.
const ForceGraphView = dynamic(
  () =>
    import('@/components/graph/ForceGraphView').then(m => ({
      default: m.ForceGraphView,
    })),
  {
    ssr: false,
    // Happy path: the height-reserving skeleton while the chunk downloads.
    // Error path (e.g. a deploy rotated the hashed chunk while the page
    // was open): a perceivable, recoverable state, not an infinite skeleton.
    loading: ({ error, retry }) =>
      error ? <GraphLoadError onRetry={retry} /> : <GraphSkeleton />,
  },
)

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
  const containerRef = useRef<HTMLDivElement>(null)
  const [isMounted, setIsMounted] = useState(false)

  // Lazy-mount via IntersectionObserver. Pre-mount with a 200px root
  // margin so the data is in flight before the placeholder is fully
  // on-screen; once mounted, we never tear down (the user has shown
  // intent by scrolling here).
  useEffect(() => {
    if (isMounted) return
    const node = containerRef.current
    if (!node || typeof IntersectionObserver === 'undefined') {
      // SSR / very old browsers — fall back to immediate mount. React 19.2:
      // defer the setState to a microtask so it lands after the effect returns
      // instead of synchronously in the effect body (set-state-in-effect /
      // cascading render). The two-phase render (placeholder → mounted) is
      // preserved exactly.
      let cancelled = false
      Promise.resolve().then(() => {
        if (!cancelled) setIsMounted(true)
      })
      return () => {
        cancelled = true
      }
    }
    const observer = new IntersectionObserver(
      entries => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            setIsMounted(true)
            observer.disconnect()
            return
          }
        }
      },
      { rootMargin: INTERSECTION_ROOT_MARGIN },
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [isMounted])

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
  const skeleton = <GraphSkeleton />

  return (
    <div ref={containerRef} className="w-full">
      <div ref={measureRefCallback} className="w-full">
        {!isMounted && skeleton}

        {isMounted && containerWidth !== null && !graphAvailable && teaser}

        {isMounted && graphAvailable && (
          <>
            {graphLoading && skeleton}
            {graphData && graphData.nodes.length > 0 && (
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
