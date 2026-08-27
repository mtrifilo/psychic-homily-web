'use client'

/**
 * HomeSceneGraph (PSY-1344) — the "Observatory Lite" homepage section: a
 * bounded, scene-scoped knowledge-graph glimpse under Upcoming Shows
 * (Figma: Product Designs → Home → PSY-1338 Option D, locked 2026-07-03).
 *
 * Deliberately NOT the full graph tool: click-select only (no wheel-zoom,
 * no pan, no scope switcher, no interactive legend — `staticViewport` on
 * ForceGraphView), frozen after settle, with an activity-ranked map of names,
 * tiered labels, next-show chips, and a compact legend. Selecting a name opens
 * the ArtistContextPanel (PSY-1345) with playable audio first and "Open page →"
 * as the navigation path. Full-power interactivity lives on the dedicated
 * /graph page (the re-pointed Observatory, PSY-1079…1086), which HAS since
 * shipped (app/graph/page.tsx; /explore permanently redirects to it). This
 * section's CTA nonetheless still points at the scene page's graph section,
 * `/scenes/{slug}#graph` — scene-scoped, matching what the canvas above it
 * shows. Whether it should re-point at /graph is an open question, not a
 * documented decision; do not read this note as one.
 *
 * Perf posture mirrors InlineGraph (PSY-868/PSY-837): the section
 * lazy-mounts via IntersectionObserver, all data fetching waits for
 * scroll-intent (and, for the graph payload, for a canvas-capable width),
 * and ForceGraphView loads in its own dynamic(ssr:false) chunk so nothing
 * graph-shaped lands in the homepage's initial JS.
 *
 * The section self-hides (renders nothing) when the scenes list errors,
 * is empty, or the section itself throws (GraphSectionErrorBoundary with no
 * fallback — the App Router's next/dynamic throws failed chunk loads to the
 * nearest error boundary; without a local one, a graph chunk-fetch failure
 * would replace the ENTIRE homepage with app/error.tsx). The homepage must
 * never break on a graph problem.
 */

import { useCallback, useMemo, useState } from 'react'
import Link from 'next/link'
import type { GraphNode } from '@/components/graph/ForceGraphView'
import {
  ArtistContextPanel,
} from '@/components/graph/ArtistContextPanel'
import {
  EntityContextPanel,
  graphEntitySelectGestureHint,
} from '@/components/graph/EntityContextPanel'
import { GraphPanelHost } from '@/components/graph/GraphPanelHost'
import {
  isLabelHubNode,
  LABEL_HUB_ENTITY_TYPE,
  LABEL_HUB_SPOKE_EDGE_TYPE,
  labelHubHomeCaption,
} from '@/components/graph/labelHub'
import { useArtistPanelSelection } from '@/components/graph/useArtistPanelSelection'
import { EdgeSwatch } from '@/components/graph/EdgeLegend'
import { edgeTypeLabel, orderEdgeTypes } from '@/components/graph/edgeGrammar'
import {
  PLAYABLE_MARKER_LABEL,
  PLAYABLE_RING_COLOR,
  UPCOMING_SHOW_DOT_COLOR,
  UPCOMING_SHOW_MARKER_LABEL,
} from '@/components/graph/graphMarkers'
import {
  useContainerWidth,
  GRAPH_BREAKPOINT_PX,
} from '@/components/graph/useContainerWidth'
import { useLazyGraphMount } from '@/components/graph/useLazyGraphMount'
import { GraphSkeleton as BaseGraphSkeleton } from '@/components/graph/GraphSkeleton'
import { createLazyForceGraphView } from '@/components/graph/lazyForceGraphView'
import { GraphSectionErrorBoundary } from '@/components/graph/GraphSectionErrorBoundary'
// Deep imports, deliberately NOT the '@/features/scenes' barrel: the barrel
// re-exports the scenes component tree (AtlasGlobe / SceneList / …)
// whose module bodies run top-level dynamic() calls the bundler can't drop,
// so importing it from a statically-mounted homepage component would drag
// scenes module code into the homepage's initial JS. Same precedent as
// InlineGraph's deep import of useArtistGraph (PSY-868).
import { useScenes, useSceneGraph } from '@/features/scenes/hooks/useScenes'
import type { SceneGraphNode } from '@/features/scenes/types'
import { useArtistGraphCard } from '@/features/artists/hooks/useArtistGraphCard'
import { formatShowDate, formatShowMonthParts } from '@/lib/utils/formatters'
import { pickDefaultScene, pickSurpriseScene } from './homeSceneGraphScenes'
import { useGeoDefaultScene } from '@/lib/hooks/common/useGeoDefaultScene'
import { buildHomeSceneGraphMap } from './homeSceneGraphMap'

const GRAPH_HEIGHT_PX = 560

/**
 * Fewer CONNECTED NODES than this renders the "Not enough connected
 * artists" card instead of a near-empty canvas. The threshold value (3)
 * matches the scene page's MIN_GRAPH_NODES (SceneGraph.tsx), but the
 * counted quantity differs: the scene page counts ALL nodes (its isolate
 * shelf keeps isolates on canvas), while the homepage counts connected
 * nodes only, because isolates are filtered out below.
 *
 * "Nodes", not "artists", and the distinction is load-bearing: label hubs
 * are nodes too, so this gate can pass on a map holding as little as ONE
 * artist. Anything downstream that counts artists rather than nodes — the
 * caption, the canvas aria-label — must be singular-safe and must not treat
 * 3 as its floor.
 *
 * The one-artist map is not merely theoretical, but it is also not as cheap
 * as "one artist on two labels": the backend only mints a hub for a roster of
 * `labelHubMinRoster` (3) in-payload artists, so any two hubs drag at least
 * three artists in with them. It takes SATURATION — an artist on ~19 labels
 * outranks everything (its degree is the hub count), pairs with the top hub,
 * and the remaining hubs each admit as neighbors of an already-selected node
 * until HOME_GRAPH_MAX_NODES (20) is reached, before any second artist is
 * examined. The label-heavy scenes this teaser already accommodates (Austin:
 * 300 of 302 edges from one label) are exactly where that shape lives.
 */
const MIN_CONNECTED_NODES = 3

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

// This surface's height-reserving placeholder (CLS budget) — the shared
// `GraphSkeleton` base look (PSY-1347) plus the responsive height contract
// above. Named distinctly from the shared primitive to avoid shadowing it.
// Used by the pre-mount state, the data-loading state, and the dynamic-import
// fallback so they can't drift apart.
function SceneGraphSkeleton() {
  return <BaseGraphSkeleton className={PLACEHOLDER_HEIGHT_CLASS} />
}

/**
 * Chip for a node's soonest upcoming show.
 *
 * DATED, not a bare weekday. `next_show` carries no window — the payload
 * contract in features/scenes/types.ts says it can be a year out — so "Fri"
 * on its own reads as THIS Friday and claims an imminence nothing backs.
 *
 * The YEAR rides along only when the show falls outside the current
 * venue-local year. Dropping it unconditionally (which is what every table
 * caller of formatShowDate does) would trade one false imminence for another:
 * read in August 2026, a July-2027 booking rendered "Fri, Jul 17" looks five
 * weeks PAST. Those callers sit under year-anchored headers that supply the
 * year; this chip is a free-floating canvas overlay with no date context
 * around it. Reading the clock here is safe: this subtree never renders on the
 * server (useLazyGraphMount flips only inside an effect, and ForceGraphView is
 * dynamic(ssr:false)), so there is no hydration boundary to disagree across.
 *
 * TWO LINES rather than one. The 180px cap is not a free parameter: the
 * overlay layer reserves `nodeOverlayOutwardClearance={192}` (set below) and
 * offsets the chip 12px into that gutter, so widening the chip would need the
 * clearance widened with it. The venue name therefore wraps instead. A hover
 * tooltip is not an alternative — ForceGraphView renders every overlay inside
 * a `pointer-events-none` wrapper, which is also why selecting the NODE, not
 * this chip, is what opens the panel. That panel's "Next show" line shares
 * this formatter and this date-then-venue ordering, and additionally appends
 * the venue city.
 */
function ShowDateChip({ node }: { node: SceneGraphNode }) {
  const show = node.next_show
  if (!show) return null
  const { year: showYear } = formatShowMonthParts(
    show.event_date,
    show.venue_state,
    show.venue_timezone
  )
  const { year: currentYear } = formatShowMonthParts(
    new Date().toISOString(),
    show.venue_state,
    show.venue_timezone
  )
  const showDate = formatShowDate(
    show.event_date,
    show.venue_state,
    showYear !== currentYear,
    show.venue_timezone
  )
  const venueName = show.venue_name.trim()

  return (
    // No `block` alongside `line-clamp-2`: both set `display`, they have equal
    // specificity, and Tailwind emits `.line-clamp-2` first, so `block` would
    // win and silently kill the clamp (`-webkit-line-clamp` only applies to
    // `display:-webkit-box`). line-clamp already establishes a block-level box.
    <span className="line-clamp-2 max-w-[180px] rounded border border-green-500 bg-background px-2 py-[3px] font-mono text-[10px] leading-tight text-foreground shadow-sm">
      {showDate}{venueName ? ` · ${venueName}` : ''}
    </span>
  )
}

/**
 * Static mini-legend. Every key is gated on the marker being ON the canvas:
 * the edge keys already derive from the rendered links, and the two node
 * markers now do the same. A key with no marker behind it is the same class
 * of unbacked claim as the copy around it — it tells the visitor to go find
 * something that is not there. Same gating the ego graph's EgoTypeLegend
 * applies to these two markers.
 */
function HomeGraphLegend({
  types,
  hasUpcomingShowDot,
  hasPlayableRing,
}: {
  types: readonly string[]
  hasUpcomingShowDot: boolean
  hasPlayableRing: boolean
}) {
  return (
    // role="group" so the aria-label is actually exposed: a bare <div> is
    // role=generic, and ARIA prohibits an accessible name there, so browsers
    // drop it. Without a role the keys below reach a screen reader as loose,
    // unframed text (every swatch is aria-hidden and cannot supply context),
    // and the RTL getByLabelText assertion passes anyway — a false green.
    <div
      role="group"
      className="flex flex-wrap items-center gap-x-[18px] gap-y-2 text-[11px] text-muted-foreground"
      aria-label="Graph legend"
    >
      {orderEdgeTypes(types).map(type => (
        <span key={type} className="flex items-center gap-1.5">
          <EdgeSwatch type={type} />
          {edgeTypeLabel(type).toLowerCase()}
        </span>
      ))}
      {hasUpcomingShowDot && (
      <span className="flex items-center gap-1.5">
        <span
          className="size-[7px] rounded-full"
          style={{ backgroundColor: UPCOMING_SHOW_DOT_COLOR }}
          aria-hidden="true"
        />
        {/* Wording — and why it is a predicate rather than a time window, and
            what bounding it would cost — lives on the constant in
            graphMarkers, which the ego legend renders too, so this key and
            that one cannot describe one marker two ways. */}
        {UPCOMING_SHOW_MARKER_LABEL}
      </span>
      )}
      {hasPlayableRing && (
      <span className="flex items-center gap-1.5">
        <span
          className="size-[9px] rounded-full border-[1.5px]"
          style={{ borderColor: PLAYABLE_RING_COLOR }}
          aria-hidden="true"
        />
        {PLAYABLE_MARKER_LABEL}
      </span>
      )}
    </div>
  )
}

// Shared lazy ForceGraphView (PSY-1359): its own dynamic(ssr:false) chunk so
// nothing graph-shaped lands in the homepage's initial JS (PSY-868). A failed
// chunk fetch throws to GraphSectionErrorBoundary below (the App Router never
// re-invokes `loading` with an error); `loading` is only the happy-path skeleton.
const ForceGraphView = createLazyForceGraphView(<SceneGraphSkeleton />)

export function HomeSceneGraph() {
  // Lazy-mount on scroll intent (shared hook — PSY-1347, incl. the React 19
  // defer-to-microtask fallback when IntersectionObserver is unavailable).
  // Once mounted, never tears down; the section's data hooks only run behind
  // this gate.
  const { containerRef, isMounted } = useLazyGraphMount()

  return (
    <div ref={containerRef} className="w-full">
      {isMounted ? (
        // Self-hide on any render/chunk error (no fallback) — a graph problem
        // must never dent the homepage; the throw is reported to Sentry, not
        // bubbled to app/error.tsx.
        <GraphSectionErrorBoundary sentryTag="home-scene-graph">
          <HomeSceneGraphSection />
        </GraphSectionErrorBoundary>
      ) : (
        <SceneGraphSkeleton />
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
    [scenesQuery.data?.scenes]
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
    [scenes, geoSuggestion]
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

  // Curated map-of-names: one pure derivation owns the activity rank, ≤20
  // cap, link pruning, typography terciles, and headline-show selection so
  // those visible encodings cannot drift apart. Placeholder data is excluded
  // above, so a Surprise-me rotation never ranks the outgoing scene under the
  // incoming heading.
  const graphMap = useMemo(
    () =>
      buildHomeSceneGraphMap(
        settledGraphData?.nodes ?? [],
        settledGraphData?.links ?? []
      ),
    [settledGraphData]
  )
  const connectedNodes = graphMap.nodes
  const hasEnoughConnectedNodes = connectedNodes.length >= MIN_CONNECTED_NODES

  // Node selection → context panel (PSY-1345), via the shared
  // select/deselect/close/focus-return wiring (PSY-1451). The resolver only
  // trusts settled data for the current scene: placeholder frames resolve to
  // null (their connectedNodes belong to the outgoing scene), and the scene
  // rotation that caused them clears the selection below anyway.
  const {
    selectedNode: currentSelectedNode,
    canvasWrapRef,
    panelRef,
    handleNodeClick: selectNode,
    handleBackgroundClick,
    handlePanelClose,
    clearSelection,
  } = useArtistPanelSelection({
    resolveNode: selected =>
      settledGraphData
        ? (connectedNodes.find(node => node.id === selected.id) ?? null)
        : null,
  })

  // Clear the selection whenever the scene identity changes — the selected
  // artist belongs to the outgoing scene's graph (Surprise-me AND data-driven
  // re-ranks both count). React 19.2: the previous-value-guard idiom (adjust
  // state during render) rather than a synchronous setState in an effect,
  // which trips react-hooks/set-state-in-effect and adds a cascading render.
  const [prevSceneSlug, setPrevSceneSlug] = useState<string | undefined>(
    scene?.slug
  )
  if (scene?.slug !== prevSceneSlug) {
    setPrevSceneSlug(scene?.slug)
    clearSelection()
  }
  const showChipOverlays = useMemo(
    () =>
      new Map(
        graphMap.showChipNodes.map(node => [
          node.id,
          <ShowDateChip key={node.id} node={node} />,
        ])
      ),
    [graphMap.showChipNodes]
  )
  const edgeTypes = useMemo(
    () => [...new Set(graphMap.links.map(link => link.type).filter(Boolean))],
    [graphMap.links]
  )
  // Marker presence on THIS canvas, not on the payload: the node cap can drop
  // every artist that carried a marker. The legend and the payoff line both
  // read these, so a key and its sentence can never outlive the marker.
  const hasUpcomingShowDot = useMemo(
    () => connectedNodes.some(node => node.upcoming_show_count > 0),
    [connectedNodes]
  )
  const hasPlayableRing = useMemo(
    () => connectedNodes.some(node => node.has_playable_audio),
    [connectedNodes]
  )

  // A label hub is not an artist (PSY-1530): its panel is the shared
  // EntityContextPanel, and asking the artist-card endpoint for a hub's
  // offset node id would 404.
  const selectedIsHub =
    currentSelectedNode !== null && isLabelHubNode(currentSelectedNode)

  const cardQuery = useArtistGraphCard({
    artistId: !selectedIsHub ? (currentSelectedNode?.id ?? null) : null,
    enabled: currentSelectedNode !== null && !selectedIsHub,
  })

  // "N artists on this label" over the teaser's OWN selected spokes — the
  // teaser caps at HOME_GRAPH_MAX_NODES, so this is deliberately the count in
  // the map, not the scene's full roster.
  const hubRosterInGraph = useMemo(() => {
    if (!currentSelectedNode || !selectedIsHub) return 0
    const roster = new Set<number>()
    for (const link of graphMap.links) {
      if (link.type !== LABEL_HUB_SPOKE_EDGE_TYPE) continue
      if (link.source_id === currentSelectedNode.id) roster.add(link.target_id)
      else if (link.target_id === currentSelectedNode.id)
        roster.add(link.source_id)
    }
    return roster.size
  }, [currentSelectedNode, selectedIsHub, graphMap.links])

  // Artist-only count for the caption/aria: hubs are a second population, and
  // calling a label an artist would be a false claim.
  //
  // This count can be 1 even though the canvas only renders at all above
  // MIN_CONNECTED_NODES (3): that gate counts every connected NODE, label hubs
  // included. See the constant's own doc for the shape that produces it (hub
  // saturation, not "one artist on two labels"). Readers must be singular-safe.
  const connectedArtistCount = useMemo(
    () => graphMap.nodes.filter(node => !isLabelHubNode(node)).length,
    [graphMap.nodes],
  )
  const connectedArtistNoun = connectedArtistCount === 1 ? 'artist' : 'artists'

  const handleSurprise = useCallback(() => {
    const next = pickSurpriseScene(scenes, scene?.slug ?? null)
    // Selection clearing rides the scene-slug effect above.
    if (next) setSurpriseSlug(next.slug)
  }, [scenes, scene?.slug])

  // Click selects / second click deselects (shared hook); the first click
  // also pins the current scene (see pinnedSlug) so a late geo resolution
  // won't yank it.
  const handleNodeClick = useCallback(
    (node: GraphNode) => {
      setPinnedSlug(prev => prev ?? scene?.slug ?? null)
      selectNode(node)
    },
    [scene?.slug, selectNode]
  )

  // Self-hide on scenes failure/emptiness: a broken graph source must not
  // dent the homepage. (scenes.length === 0 is only meaningful once the
  // query settled — while loading we hold the skeleton instead.)
  if (scenesQuery.isError) return null
  if (!scenesQuery.isLoading && scenes.length === 0) return null
  if (!scene) return <SceneGraphSkeleton />

  const sceneHref = `/scenes/${scene.slug}`
  const sceneGraphHref = `${sceneHref}#graph`

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
          The {scene.city} scene graph
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
            href={sceneGraphHref}
            className="font-medium text-muted-foreground transition-colors hover:text-primary hover:underline underline-offset-4"
          >
            Open the graph →
          </Link>
        </div>
      </div>

      {/* Three claims this caption is deliberately careful NOT to make. Same
          rule as the PSY-1732 note on CommunityPulseResponse in
          features/home/types.ts; it binds the heading above and the canvas
          aria-label below just as much as this paragraph.

          1. NO time window. The payload is the scene's artist RELATIONSHIP
             graph, not a dated slice. The upcoming-show accent (the green dot
             and the dated show chip) is per-node, never a filter, and it is
             itself unbounded — `upcoming_show_count` and `next_show` can be a
             year out — so no wording anywhere in this section may imply "this
             week", "this month", or "soon".
          2. "N OF the most connected", not "THE N most connected". Neither
             ranking stage sorts on connectivity alone: the backend picks the
             roster by approved show count (scene.go ranked_roster), and
             buildHomeSceneGraphMap then ranks on degree + upcoming_show_count
             and admits nodes in connected pairs. A strict superlative would be
             false whenever a well-booked artist outranks a better-connected one.
          3. "Ties LIKE shared bills and labels" is an example, not the set.
             allowedSceneEdgeTypes (scene.go) also passes member_of and
             side_project, so a scene can render with no bill or label edge at
             all. The legend below names whichever types actually appear; this
             sentence must not contradict it. */}
      {graphAvailable && settledGraphData && hasEnoughConnectedNodes && (
        <p className="text-xs text-muted-foreground">
          {connectedArtistCount} of the most connected artists tied to{' '}
          {scene.city}. Ties like shared bills and labels link them; every name
          is clickable.
        </p>
      )}

      <div ref={refCallback} className="w-full">
        {/* Pre-measurement: hold the (responsive) height so the section
            can't shift the radio section below when the state settles. */}
        {containerWidth === null && <SceneGraphSkeleton />}

        {/* Static teaser below the canvas-usability gate (PSY-511): no
            canvas touch handling at small widths — link out instead. */}
        {containerWidth !== null && !graphAvailable && (
          <div
            className={`w-full rounded-lg border border-border/50 bg-muted/20 flex flex-col items-center justify-center text-center p-6 gap-3 ${PLACEHOLDER_HEIGHT_CLASS}`}
          >
            {/* "Every show, artist, venue and label here is connected" was
                three claims deep in unbacked. Shows and venues are not nodes
                in this payload at all (SceneGraphNode is artists plus label
                hubs; venues survive only as cluster ids), universal
                connectivity is contradicted by `is_isolate` — a first-class
                field this very component filters on, and whose absence the
                sibling branch below announces as "Not enough connected
                artists" — and this branch fetches no graph at all
                (useSceneGraph is gated on `graphAvailable`). So the copy now
                describes the graph the visitor would get, in the same
                "ties like" example form the caption uses. */}
            <p className="text-sm text-muted-foreground max-w-xs">
              The {scene.city} scene graph links artists by ties like shared
              bills and labels. It is best on a larger screen.
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
            hasEnoughConnectedNodes ? (
              <GraphPanelHost
                canvasWrapRef={canvasWrapRef}
                panel={
                  currentSelectedNode && selectedIsHub ? (
                    <EntityContextPanel
                      className="absolute top-2 right-2 z-40"
                      entityType={LABEL_HUB_ENTITY_TYPE}
                      name={currentSelectedNode.name}
                      slug={currentSelectedNode.slug}
                      meta={labelHubHomeCaption(currentSelectedNode) ?? null}
                      primary={
                        hubRosterInGraph > 0
                          ? {
                              kind: 'emphasis',
                              text: `${hubRosterInGraph} ${hubRosterInGraph === 1 ? 'artist' : 'artists'} on this label in this map`,
                            }
                          : null
                      }
                      onClose={handlePanelClose}
                      panelRef={panelRef}
                    />
                  ) : currentSelectedNode ? (
                    <ArtistContextPanel
                      className="absolute top-2 right-2 z-40"
                      artistName={currentSelectedNode.name}
                      artistSlug={currentSelectedNode.slug}
                      card={cardQuery.data}
                      isError={cardQuery.isError}
                      onClose={handlePanelClose}
                      panelRef={panelRef}
                    />
                  ) : null
                }
              >
                <ForceGraphView
                  // Remount per scene: a rotation BACK to a cached scene
                  // arrives with isPlaceholderData false (no skeleton frame,
                  // no unmount), and the mounted canvas's one-shot zoomToFit
                  // is already spent — with zoom/pan disabled there'd be no
                  // gesture to recover a mis-framed swap. A fresh mount
                  // re-arms the fit and drops stale hover state.
                  key={scene.slug}
                  nodes={connectedNodes}
                  links={graphMap.links}
                  clusters={settledGraphData.clusters}
                  containerWidth={containerWidth}
                  height={GRAPH_HEIGHT_PX}
                  staticViewport
                  nodeLabelStyles={graphMap.labelStyles}
                  forceNodeLabels
                  nodeOverlays={showChipOverlays}
                  nodeOverlayPlacement="outward"
                  nodeOverlayOutwardClearance={192}
                  showAccessibleNodeControls
                  // Count the CONNECTED nodes actually on the canvas, not the
                  // payload's full artist_count (which includes the isolates
                  // filtered out above) — the caption promises "lines connect
                  // artists", so the label must not overstate. Pluralized off
                  // the shared noun because the count can be 1 (see the memo).
                  ariaLabel={`Knowledge graph of the ${scene.city} scene: ${connectedArtistCount} connected ${connectedArtistNoun}. ${graphEntitySelectGestureHint}`}
                  onNodeClick={handleNodeClick}
                  onBackgroundClick={handleBackgroundClick}
                  // Pin the focus-dim to the selection (PSY-1478) —
                  // grammar in graphFocus.resolveFocusForeground.
                  focusNodeId={currentSelectedNode?.id ?? null}
                />
              </GraphPanelHost>
            ) : (
              <div
                className={`w-full rounded-lg border border-border/50 bg-muted/10 flex items-center justify-center text-sm text-muted-foreground ${PLACEHOLDER_HEIGHT_CLASS}`}
              >
                {/* "names", not "artists": the gate above counts
                    connectedNodes, which includes label hubs. Saying
                    "artists" here asserts a count this branch never took —
                    the same node-vs-artist conflation the aria-label carried. */}
                Not enough connected names in {scene.city} yet — try another
                scene.
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
            <SceneGraphSkeleton />
          ))}
      </div>

      {/* Static mini-legend + payoff line. Only rendered with the canvas —
          teaser/empty/error states carry their own copy and click guidance
          would be false there. */}
      {graphAvailable && settledGraphData && hasEnoughConnectedNodes && (
        <div className="space-y-3">
          <HomeGraphLegend
            types={edgeTypes}
            hasUpcomingShowDot={hasUpcomingShowDot}
            hasPlayableRing={hasPlayableRing}
          />
          {/* Every word here is load-bearing against buildHomeSceneGraphMap.

              "RANK BY", not "=". Size is a TERCILE of the activity ranking
              (tierSize = ceil(n/3)), not the score itself, so two names with
              an identical degree + upcoming_show_count can land in different
              tiers when the cut falls between them. An equality claim would be
              false on any tie; a ranking claim is exactly what the tiers are.

              "ACROSS THE SCENE". `degrees` counts every link in the payload,
              but only links whose BOTH endpoints survive the
              HOME_GRAPH_MAX_NODES cap are drawn. A top-tier name whose
              partners were all cut renders with fewer visible lines than a
              smaller name below it, and the caption right above tells the
              reader lines are the ties — so without this qualifier the
              sentence invites counting lines and catching the map
              contradicting itself.

              Neither input is dated, so no wording here may imply recency. */}
          <p className="text-xs text-muted-foreground">
            Name size = rank by connections across the scene plus upcoming
            shows. Click any artist for context
            {/* Gated on the same predicate as the legend key: pointing at a
                violet ring on a canvas that has none sends the visitor
                looking for a marker that is not there. */}
            {hasPlayableRing && '; violet-ring artists include a listen'} — no
            zooming required.
          </p>
        </div>
      )}
    </section>
  )
}
