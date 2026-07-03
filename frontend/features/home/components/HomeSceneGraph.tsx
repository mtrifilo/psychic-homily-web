'use client'

/**
 * HomeSceneGraph (PSY-1344) — the "Observatory Lite" homepage section: a
 * bounded, scene-scoped knowledge-graph glimpse under Upcoming Shows
 * (Figma: Product Designs → Home → PSY-1338 Option D, locked 2026-07-03).
 *
 * Deliberately NOT the full graph tool: click-select only (no wheel-zoom,
 * no pan, no scope switcher, no interactive legend — `staticViewport` on
 * ForceGraphView), frozen after settle, one static caption. Full-power
 * interactivity lives on the dedicated /graph page (the re-pointed
 * Observatory, PSY-1079…1086); until that ships the CTA links to the
 * scene page's graph section.
 *
 * Perf posture mirrors InlineGraph (PSY-868/PSY-837): the section
 * lazy-mounts via IntersectionObserver, all data fetching waits for
 * scroll-intent, and ForceGraphView loads in its own dynamic(ssr:false)
 * chunk so nothing graph-shaped lands in the homepage's initial JS.
 *
 * The section self-hides (renders nothing) when the scenes list errors
 * or is empty — the homepage must never break on a graph-data problem.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import dynamic from 'next/dynamic'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import type { GraphNode } from '@/components/graph/ForceGraphView'
import {
  useContainerWidth,
  GRAPH_BREAKPOINT_PX,
} from '@/components/graph/useContainerWidth'
import {
  useScenes,
  useSceneGraph,
  sceneArtistCountPhrase,
} from '@/features/scenes'
import {
  pickDefaultScene,
  pickSurpriseScene,
} from './homeSceneGraphScenes'

const GRAPH_HEIGHT_PX = 480
const INTERSECTION_ROOT_MARGIN = '200px'

// Height-reserving placeholder (CLS budget) — shared by the pre-mount
// state, the data-loading state, and the dynamic-import fallback.
function GraphSkeleton() {
  return (
    <div
      className="w-full rounded-lg border border-border/50 bg-muted/10 animate-pulse"
      style={{ height: GRAPH_HEIGHT_PX }}
      aria-hidden="true"
    />
  )
}

// Perceivable fallback when the ForceGraphView chunk fails to load
// (next/dynamic re-invokes `loading` with `error` set rather than throwing
// to an error boundary — without this the aria-hidden skeleton spins
// forever). Same recovery shape as InlineGraph's GraphLoadError.
function GraphLoadError({ onRetry }: { onRetry?: () => void }) {
  return (
    <div
      role="alert"
      className="w-full rounded-lg border border-border/50 bg-muted/10 flex flex-col items-center justify-center gap-3 text-center p-6"
      style={{ height: GRAPH_HEIGHT_PX }}
    >
      <p className="text-sm text-muted-foreground">
        The graph couldn’t load.
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

// PSY-868 pattern: split ForceGraphView (and react-force-graph underneath)
// into an async chunk fetched only when the section actually mounts.
const ForceGraphView = dynamic(
  () =>
    import('@/components/graph/ForceGraphView').then(m => ({
      default: m.ForceGraphView,
    })),
  {
    ssr: false,
    loading: ({ error, retry }) =>
      error ? <GraphLoadError onRetry={retry} /> : <GraphSkeleton />,
  },
)

export function HomeSceneGraph() {
  const containerRef = useRef<HTMLDivElement>(null)
  const [isMounted, setIsMounted] = useState(false)

  // Lazy-mount on scroll intent (InlineGraph's observer shape, incl. the
  // React 19 defer-to-microtask fallback when IntersectionObserver is
  // unavailable). Once mounted, never tears down.
  useEffect(() => {
    if (isMounted) return
    const node = containerRef.current
    if (!node || typeof IntersectionObserver === 'undefined') {
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
        if (entries.some(e => e.isIntersecting)) {
          setIsMounted(true)
          observer.disconnect()
        }
      },
      { rootMargin: INTERSECTION_ROOT_MARGIN },
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [isMounted])

  return (
    <div ref={containerRef} className="w-full">
      {isMounted ? <HomeSceneGraphSection /> : <GraphSkeleton />}
    </div>
  )
}

// Inner component so the data hooks only run once scroll-intent exists —
// the outer shell can't call them conditionally.
function HomeSceneGraphSection() {
  const router = useRouter()
  const { refCallback, containerWidth } = useContainerWidth()
  const scenesQuery = useScenes()
  const scenes = scenesQuery.data?.scenes ?? []

  // The user's "Surprise me" pick; null = the liveliest-scene default.
  const [surpriseSlug, setSurpriseSlug] = useState<string | null>(null)
  const scene =
    scenes.find(s => s.slug === surpriseSlug) ?? pickDefaultScene(scenes)

  const graphQuery = useSceneGraph({
    slug: scene?.slug ?? '',
    enabled: Boolean(scene),
  })
  const graphData = graphQuery.data

  const handleSurprise = useCallback(() => {
    const next = pickSurpriseScene(scenes, scene?.slug ?? null)
    if (next) setSurpriseSlug(next.slug)
  }, [scenes, scene?.slug])

  const handleNodeClick = useCallback(
    (node: GraphNode) => {
      router.push(`/artists/${node.slug}`)
    },
    [router],
  )

  // Self-hide on scenes failure/emptiness: a broken graph source must not
  // dent the homepage. (scenes.length === 0 is only meaningful once the
  // query settled — while loading we hold the skeleton instead.)
  if (scenesQuery.isError) return null
  if (!scenesQuery.isLoading && scenes.length === 0) return null
  if (!scene) return <GraphSkeleton />

  const sceneHref = `/scenes/${scene.slug}`
  const graphAvailable =
    containerWidth !== null && containerWidth >= GRAPH_BREAKPOINT_PX

  return (
    <section
      aria-labelledby="home-scene-graph-heading"
      className="flex w-full flex-col gap-4"
    >
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <h2
          id="home-scene-graph-heading"
          className="text-2xl font-semibold tracking-tight text-foreground"
        >
          The {scene.city} scene, mapped
        </h2>
        <div className="flex items-center gap-4 text-sm">
          {scenes.length > 1 && (
            <button
              type="button"
              onClick={handleSurprise}
              className="font-medium text-muted-foreground transition-colors hover:text-primary hover:underline underline-offset-4"
            >
              Surprise me ↻
            </button>
          )}
          <Link
            href={sceneHref}
            className="font-medium text-muted-foreground transition-colors hover:text-primary hover:underline underline-offset-4"
          >
            Explore the full graph →
          </Link>
        </div>
      </div>

      <div ref={refCallback} className="w-full">
        {/* Pre-measurement: hold the height so the section can't collapse
            and shift the radio section below (CLS budget). */}
        {containerWidth === null && <GraphSkeleton />}

        {/* Static teaser below the canvas-usability gate (PSY-511): no
            canvas touch handling at small widths — link out instead. */}
        {containerWidth !== null && !graphAvailable && (
          <div
            className="w-full rounded-lg border border-border/50 bg-muted/20 flex flex-col items-center justify-center text-center p-6 gap-3"
            style={{ height: GRAPH_HEIGHT_PX / 2 }}
          >
            <p className="text-sm text-muted-foreground max-w-xs">
              Every show, artist, venue and label here is connected. The
              interactive graph is best on a larger screen.
            </p>
            <Link
              href={sceneHref}
              className="text-sm text-primary hover:underline underline-offset-4"
            >
              See the {scene.city} scene →
            </Link>
          </div>
        )}

        {graphAvailable && (
          <>
            {graphQuery.isLoading && <GraphSkeleton />}
            {graphData && graphData.nodes.length > 0 && (
              <ForceGraphView
                nodes={graphData.nodes}
                links={graphData.links}
                clusters={graphData.clusters}
                containerWidth={containerWidth}
                height={GRAPH_HEIGHT_PX}
                staticViewport
                ariaLabel={`Knowledge graph of the ${scene.city} scene: ${sceneArtistCountPhrase(graphData.scene)}. Click a node to open that artist’s page.`}
                onNodeClick={handleNodeClick}
              />
            )}
            {!graphQuery.isLoading &&
              graphData &&
              graphData.nodes.length === 0 && (
                <div
                  className="w-full rounded-lg border border-border/50 bg-muted/10 flex items-center justify-center text-sm text-muted-foreground"
                  style={{ height: GRAPH_HEIGHT_PX / 2 }}
                >
                  Not enough connected artists in {scene.city} yet — try
                  another scene.
                </div>
              )}
            {graphQuery.isError && (
              <div
                className="w-full rounded-lg border border-border/50 bg-muted/10 flex items-center justify-center text-sm text-muted-foreground"
                style={{ height: GRAPH_HEIGHT_PX / 2 }}
              >
                The graph couldn’t load.{' '}
                <Link
                  href={sceneHref}
                  className="ml-1 text-primary hover:underline underline-offset-4"
                >
                  See the {scene.city} scene →
                </Link>
              </div>
            )}
          </>
        )}
      </div>

      <p className="text-sm text-muted-foreground">
        Every show, artist, venue and label on the site is connected — this
        is one scene’s slice. Click any artist to dig in.
      </p>
    </section>
  )
}
