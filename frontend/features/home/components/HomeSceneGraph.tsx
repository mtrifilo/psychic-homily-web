'use client'

/**
 * HomeSceneGraph (PSY-1344) — the "Observatory Lite" homepage section: a
 * bounded, scene-scoped knowledge-graph glimpse under Upcoming Shows
 * (Figma: Product Designs → Home → PSY-1338 Option D, locked 2026-07-03).
 *
 * Deliberately NOT the full graph tool: click-select only (no wheel-zoom,
 * no pan, no scope switcher, no interactive legend — `staticViewport` on
 * ForceGraphView), frozen after settle, one static caption. Clicking a
 * node opens the ArtistContextPanel (PSY-1345) — next show, labels, radio,
 * connections — with "Open page →" as the navigation path. Full-power
 * interactivity lives on the dedicated /graph page (the re-pointed
 * Observatory, PSY-1079…1086); until that ships the CTA links to the
 * scene page's graph section.
 *
 * Perf posture mirrors InlineGraph (PSY-868/PSY-837): the section
 * lazy-mounts via IntersectionObserver, all data fetching waits for
 * scroll-intent (and, for the graph payload, for a canvas-capable width),
 * and ForceGraphView loads in its own dynamic(ssr:false) chunk so nothing
 * graph-shaped lands in the homepage's initial JS.
 *
 * The section self-hides (renders nothing) when the scenes list errors,
 * is empty, or the section itself throws (SectionErrorBoundary — the App
 * Router's next/dynamic throws failed chunk loads to the nearest error
 * boundary; without a local one, a graph chunk-fetch failure would
 * replace the ENTIRE homepage with app/error.tsx). The homepage must
 * never break on a graph problem.
 */

import { Component, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import * as Sentry from '@sentry/nextjs'
import dynamic from 'next/dynamic'
import Link from 'next/link'
import type { GraphNode } from '@/components/graph/ForceGraphView'
import { ArtistContextPanel } from '@/components/graph/ArtistContextPanel'
import {
  useContainerWidth,
  GRAPH_BREAKPOINT_PX,
} from '@/components/graph/useContainerWidth'
// Deep imports, deliberately NOT the '@/features/scenes' barrel: the barrel
// re-exports the scenes component tree (SceneDetailView / AtlasGlobe / …)
// whose module bodies run top-level dynamic() calls the bundler can't drop,
// so importing it from a statically-mounted homepage component would drag
// scenes module code into the homepage's initial JS. Same precedent as
// InlineGraph's deep import of useArtistGraph (PSY-868).
import { useScenes, useSceneGraph } from '@/features/scenes/hooks/useScenes'
import { useArtistGraphCard } from '@/features/artists/hooks/useArtistGraphCard'
import { sceneArtistCountPhrase } from '@/features/scenes/components/sceneGraphCopy'
import {
  pickDefaultScene,
  pickSurpriseScene,
} from './homeSceneGraphScenes'
import { useGeoDefaultScene } from '../hooks/useGeoDefaultScene'

const GRAPH_HEIGHT_PX = 560
const INTERSECTION_ROOT_MARGIN = '200px'

/**
 * One shared height contract for every non-canvas box (skeleton, teaser,
 * empty, error): 240px below Tailwind's `sm` (≈ the 640px canvas gate),
 * 560px above — the `sm` value MUST equal `GRAPH_HEIGHT_PX` (Tailwind
 * arbitrary values can't read the const). Boxes agreeing on height keeps
 * the GRAPH AREA from shifting LatestRadioShows as states settle; the
 * pre-mount skeleton deliberately reserves only the graph box, not the
 * heading row/caption (~100px), so a small one-time shift remains at
 * section mount. A container-vs-viewport mismatch survives only in the
 * narrow band where the padded column measures <640px on a ≥640px
 * viewport.
 */
const PLACEHOLDER_HEIGHT_CLASS = 'h-[240px] sm:h-[560px]'

// Height-reserving placeholder (CLS budget) — shared by the pre-mount
// state, the data-loading state, and the dynamic-import fallback.
function GraphSkeleton() {
  return (
    <div
      className={`w-full rounded-lg border border-border/50 bg-muted/10 animate-pulse ${PLACEHOLDER_HEIGHT_CLASS}`}
      aria-hidden="true"
    />
  )
}

// PSY-868 pattern: split ForceGraphView (and react-force-graph underneath)
// into an async chunk fetched only when the section actually mounts. The
// App Router never re-invokes `loading` with an error — a failed chunk
// fetch throws from React.lazy instead, which SectionErrorBoundary eats.
const ForceGraphView = dynamic(
  () =>
    import('@/components/graph/ForceGraphView').then(m => ({
      default: m.ForceGraphView,
    })),
  {
    ssr: false,
    loading: () => <GraphSkeleton />,
  },
)

/**
 * Self-hide boundary: any render/chunk error inside the section collapses
 * it to nothing instead of bubbling to app/error.tsx and taking the whole
 * homepage down. Class component — React error boundaries have no hook
 * equivalent.
 */
class SectionErrorBoundary extends Component<
  { children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false }
  static getDerivedStateFromError() {
    return { failed: true }
  }
  componentDidCatch(error: unknown) {
    // Self-hiding must not mean silent: without this, a systematic chunk
    // failure (deploy skew, CDN flake) kills the section for everyone and
    // nothing reports it (app/global-error.tsx would have captured it).
    Sentry.captureException(error, {
      tags: { section: 'home-scene-graph' },
    })
  }
  render() {
    return this.state.failed ? null : this.props.children
  }
}

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
      {isMounted ? (
        <SectionErrorBoundary>
          <HomeSceneGraphSection />
        </SectionErrorBoundary>
      ) : (
        <GraphSkeleton />
      )}
    </div>
  )
}

// Inner component so the data hooks only run once scroll-intent exists —
// the outer shell can't call them conditionally.
function HomeSceneGraphSection() {
  const { refCallback, containerWidth } = useContainerWidth()
  const scenesQuery = useScenes()
  const scenes = useMemo(
    () => scenesQuery.data?.scenes ?? [],
    [scenesQuery.data?.scenes],
  )
  // Geo-personalize the default (PSY-1346): a visitor in a scene-city lands on
  // THEIR scene, not just the liveliest one. Non-blocking (like the shows
  // filter's useGeoDefaultCity): geo is null until it resolves, so the section
  // shows its liveliest default immediately and swaps to the geo scene when the
  // suggestion arrives — a warm session cache resolves synchronously, so the
  // common case shows the geo scene from the first render with no swap.
  // "Surprise me" still wins below.
  const geoSuggestion = useGeoDefaultScene()
  const defaultScene = useMemo(
    () => pickDefaultScene(scenes, geoSuggestion),
    [scenes, geoSuggestion],
  )

  // The user's "Surprise me" pick; null = the liveliest-scene default.
  const [surpriseSlug, setSurpriseSlug] = useState<string | null>(null)
  // The scene the visitor engaged (first node click), pinned so a LATE
  // (cold-cache) geo resolution can't swap the graph out from under them —
  // the ticket's "geo must never override user interaction" rule. A node
  // click isn't a scene pick like "Surprise me", but it is interaction; without
  // this, clicking a node on the liveliest graph before /api/geo resolves would
  // silently close the panel and remount a different scene. Surprise-me's slug
  // still wins over the pin (an explicit re-pick).
  const [pinnedSlug, setPinnedSlug] = useState<string | null>(null)
  const scene =
    scenes.find(s => s.slug === (surpriseSlug ?? pinnedSlug)) ?? defaultScene

  // Node selection → context panel (PSY-1345). Cleared whenever the scene
  // identity changes (the selected artist belongs to the outgoing scene's
  // graph — Surprise-me AND data-driven re-ranks both count) and on
  // Esc/click-away/close. React 19.2: clear via the previous-value-guard
  // idiom (adjust state during render) rather than a synchronous setState
  // in an effect, which trips react-hooks/set-state-in-effect and adds a
  // cascading render.
  const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null)
  const canvasWrapRef = useRef<HTMLDivElement>(null)
  const [prevSceneSlug, setPrevSceneSlug] = useState<string | undefined>(
    scene?.slug,
  )
  if (scene?.slug !== prevSceneSlug) {
    setPrevSceneSlug(scene?.slug)
    setSelectedNode(null)
  }

  // Below the canvas gate the teaser never reads graphData — don't pay
  // the (dense, liveliest-scene) graph round-trip for a payload the
  // mobile render discards.
  const graphAvailable =
    containerWidth !== null && containerWidth >= GRAPH_BREAKPOINT_PX
  const graphQuery = useSceneGraph({
    slug: scene?.slug ?? '',
    enabled: Boolean(scene) && graphAvailable,
  })
  // useSceneGraph carries placeholderData: keepPreviousData, so right
  // after a "Surprise me" rotation the hook reports the PREVIOUS scene's
  // payload as success. Rendering that under the new scene's heading
  // mislabels the canvas (and its aria-label) — treat placeholder frames
  // as loading and only trust settled data for the current scene.
  const settledGraphData = graphQuery.isPlaceholderData
    ? undefined
    : graphQuery.data

  const cardQuery = useArtistGraphCard({
    artistId: selectedNode?.id ?? null,
    enabled: selectedNode !== null,
  })

  const handleSurprise = useCallback(() => {
    const next = pickSurpriseScene(scenes, scene?.slug ?? null)
    // Selection clearing rides the scene-slug effect above.
    if (next) setSurpriseSlug(next.slug)
  }, [scenes, scene?.slug])

  // Click selects (opens the context panel); navigation happens via the
  // panel's "Open page →". Clicking the already-selected node deselects —
  // a second click reads as "put it away". The first click also pins the
  // current scene (see pinnedSlug) so a late geo resolution won't yank it.
  const handleNodeClick = useCallback(
    (node: GraphNode) => {
      setPinnedSlug(prev => prev ?? scene?.slug ?? null)
      setSelectedNode(prev => (prev?.id === node.id ? null : node))
    },
    [scene?.slug],
  )

  const handlePanelClose = useCallback(() => {
    setSelectedNode(null)
    // PSY-1313 lesson: return focus via an EXPLICIT ref — after the panel
    // unmounts, focus would otherwise drop to document.body.
    canvasWrapRef.current?.focus()
  }, [])

  // Self-hide on scenes failure/emptiness: a broken graph source must not
  // dent the homepage. (scenes.length === 0 is only meaningful once the
  // query settled — while loading we hold the skeleton instead.)
  if (scenesQuery.isError) return null
  if (!scenesQuery.isLoading && scenes.length === 0) return null
  if (!scene) return <GraphSkeleton />

  const sceneHref = `/scenes/${scene.slug}`

  return (
    <section
      aria-labelledby="home-scene-graph-heading"
      className="flex w-full flex-col gap-4"
    >
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <h2
          id="home-scene-graph-heading"
          aria-live="polite"
          className="text-2xl font-semibold tracking-tight text-foreground"
        >
          {scene.city} music graph
        </h2>
        <div className="flex items-center gap-4 text-sm">
          {scenes.length > 1 && (
            <button
              type="button"
              onClick={handleSurprise}
              className="font-medium text-muted-foreground transition-colors hover:text-primary hover:underline underline-offset-4"
            >
              Surprise me <span aria-hidden="true">↻</span>
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
        {/* Pre-measurement: hold the (responsive) height so the section
            can't shift the radio section below when the state settles. */}
        {containerWidth === null && <GraphSkeleton />}

        {/* Static teaser below the canvas-usability gate (PSY-511): no
            canvas touch handling at small widths — link out instead. */}
        {containerWidth !== null && !graphAvailable && (
          <div
            className={`w-full rounded-lg border border-border/50 bg-muted/20 flex flex-col items-center justify-center text-center p-6 gap-3 ${PLACEHOLDER_HEIGHT_CLASS}`}
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

        {graphAvailable &&
          // Settled data for the CURRENT scene wins — including when a
          // background refetch of the same scene later errors (data is
          // retained; a working canvas must not be swapped for an error
          // card). Otherwise: error card, else loading/placeholder
          // skeleton. The branches are mutually exclusive by construction.
          (settledGraphData ? (
            settledGraphData.nodes.length > 0 ? (
              <div ref={canvasWrapRef} tabIndex={-1} className="relative outline-none">
                <ForceGraphView
                // Remount per scene: a rotation BACK to a cached scene
                // arrives with isPlaceholderData false (no skeleton frame,
                // no unmount), and the mounted canvas's one-shot zoomToFit
                // is already spent — with zoom/pan disabled there'd be no
                // gesture to recover a mis-framed swap. A fresh mount
                // re-arms the fit and drops stale hover state.
                key={scene.slug}
                nodes={settledGraphData.nodes}
                links={settledGraphData.links}
                clusters={settledGraphData.clusters}
                containerWidth={containerWidth}
                height={GRAPH_HEIGHT_PX}
                staticViewport
                ariaLabel={`Knowledge graph of the ${scene.city} scene: ${sceneArtistCountPhrase(settledGraphData.scene)}. Click a node for that artist’s details.`}
                onNodeClick={handleNodeClick}
                onBackgroundClick={handlePanelClose}
              />
                {selectedNode && (
                  <ArtistContextPanel
                    className="absolute top-2 right-2 z-40"
                    artistName={selectedNode.name}
                    artistSlug={selectedNode.slug}
                    card={cardQuery.data}
                    isError={cardQuery.isError}
                    onClose={handlePanelClose}
                  />
                )}
              </div>
            ) : (
              <div
                className={`w-full rounded-lg border border-border/50 bg-muted/10 flex items-center justify-center text-sm text-muted-foreground ${PLACEHOLDER_HEIGHT_CLASS}`}
              >
                Not enough connected artists in {scene.city} yet — try
                another scene.
              </div>
            )
          ) : graphQuery.isError ? (
            <div
              className={`w-full rounded-lg border border-border/50 bg-muted/10 flex items-center justify-center text-sm text-muted-foreground ${PLACEHOLDER_HEIGHT_CLASS}`}
            >
              The graph couldn’t load.{' '}
              <Link
                href={sceneHref}
                className="ml-1 text-primary hover:underline underline-offset-4"
              >
                See the {scene.city} scene →
              </Link>
            </div>
          ) : (
            <GraphSkeleton />
          ))}
      </div>

      {/* Static mini-legend + payoff line (locked on PSY-1338: no
          interactive legend on the homepage). Only rendered with the
          canvas — the teaser/empty/error states carry their own copy and
          "click any artist" would be a false instruction there. */}
      {graphAvailable && settledGraphData && settledGraphData.nodes.length > 0 && (
        <p className="text-sm text-muted-foreground">
          Lines connect artists — shared bills, label ties, band members.
          Click any artist for their next show, labels, and radio plays.
        </p>
      )}
    </section>
  )
}
