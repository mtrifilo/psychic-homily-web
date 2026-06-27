'use client'

import { useCallback, useMemo, useRef, useEffect, useState, type ComponentType, type MutableRefObject } from 'react'
import Link from 'next/link'
import dynamic from 'next/dynamic'
import { Loader2 } from 'lucide-react'
import type { ForceGraphMethods, ForceGraphProps } from 'react-force-graph-2d'
import { buildLinkLabel, edgeLineDash, edgeTypeLabel, edgeWidth } from '@/components/graph/edgeGrammar'
import { useGraphPalette, withHexAlpha } from '@/components/graph/graphPalette'
import { degreeMap, renderGraphLabels, type GraphLabelSpec } from '@/components/graph/graphLabels'
import { buildAdjacency, endpointId, focusForeground, BACKGROUND_ALPHA, BACKGROUND_ALPHA_HEX } from '@/components/graph/graphFocus'
import { capEdgesPerNode } from '@/components/graph/edgeCap'
import { nodeTooltipPlacement, tooltipPlacementStyle, type TooltipAnchor, type TooltipPlacement } from '@/components/graph/nodeTooltip'
import { EdgeLegend } from '@/components/graph/EdgeLegend'
import { useDismissTimer } from '@/lib/hooks/common'
import { useReducedMotion } from '../hooks/useReducedMotion'
import type { ArtistGraph as ArtistGraphData } from '../types'

function GraphSkeleton() {
  return (
    <div className="flex items-center justify-center bg-muted/20 rounded-lg border border-border/50" style={{ height: 400 }}>
      <div className="flex flex-col items-center gap-2 text-muted-foreground">
        <Loader2 className="h-6 w-6 animate-spin" />
        <span className="text-sm">Loading graph...</span>
      </div>
    </div>
  )
}

// `next/dynamic` strips the upstream component's generic parameters (see
// react-force-graph-2d's `FCwithRef = <NodeType, LinkType>(...)`), so under
// `strictFunctionTypes` the callback props default to the library's loose
// `NodeObject<{}>` / `LinkObject<{}>` and our local GraphNode/GraphLink
// signatures fail variance checks. Pin the generics with an as-unknown-as
// cast — runtime behaviour is unchanged.
const ForceGraph2D = dynamic(() => import('react-force-graph-2d'), {
  ssr: false,
  loading: () => <GraphSkeleton />,
}) as unknown as ComponentType<
  ForceGraphProps<GraphNode, GraphLink> & {
    ref?: MutableRefObject<ForceGraphMethods<GraphNode, GraphLink> | undefined>
  }
>

// PSY-362's canonical visual style map for typed edges (colors, dashes,
// weights, tooltip copy, colorblind audit) moved to the shared graph layer
// in PSY-1083 — see `components/graph/edgeGrammar.ts` — so every graph
// surface (artist, scene, venue, collection, /explore) speaks the same
// edge language. Canvas colors resolve from the per-theme `--edge-*`
// tokens via `useGraphPalette()`; the dark palette is byte-identical to
// the pre-extraction EDGE_COLORS map.

// Convert API data to graph format needed by react-force-graph-2d.
//
// d3-force (the engine react-force-graph-2d drives) mutates each datum in
// place during simulation: it adds the layout coordinates `x`/`y`, velocity
// `vx`/`vy`, and (when we pin a node) the fixed coordinates `fx`/`fy`. These
// fields are absent on the data we hand in and only appear once the simulation
// ticks, so they're declared optional here rather than cast to `any` at each
// read/write site. See d3-force's `simulation.nodes()` contract.
interface GraphNode {
  id: number
  name: string
  slug: string
  city?: string
  state?: string
  upcoming_show_count: number
  isCenter: boolean
  val: number // node size
  // Simulation-runtime fields (mutated in place by d3-force):
  x?: number
  y?: number
  vx?: number
  vy?: number
  fx?: number
  fy?: number
}

// `source`/`target` start as numeric node ids but d3-force replaces them with
// the resolved `GraphNode` objects after the first tick, so they read as a
// union. The `.id` accessor below narrows the object case.
interface GraphLink {
  source: number | GraphNode
  target: number | GraphNode
  type: string
  score: number
  votes_up: number
  votes_down: number
  detail?: Record<string, unknown>
  isCrossConnection: boolean
}

interface ArtistGraphProps {
  data: ArtistGraphData
  activeTypes: Set<string>
  containerWidth: number
  /**
   * PSY-361: Re-center handler. Called when the user clicks a non-center
   * node. The parent owns the traversal state + URL sync; this component
   * just emits the click. Receives both id and slug so the parent doesn't
   * need a separate slug→id resolution step on click.
   */
  onRecenter?: (node: { id: number; slug: string; name: string }) => void
  /**
   * PSY-361: When true, an overlay spinner is shown over the graph while
   * the parent fetches the new center's payload. We deliberately do NOT
   * unmount the canvas — that would cause the simulation-mid-tick stutter
   * called out in the prior-art doc (§4.2). Instead we keep the previous
   * frame visible behind the overlay so the transition feels continuous.
   */
  isRecentering?: boolean
}

// PSY-361: Hover/long-press tooltip for non-center nodes. Extracted as its
// own component so the "View artist page →" link can be unit-tested
// independently of the canvas-based ForceGraph2D wrapper.
//
// Pointer-events grammar (PSY-1218 — hoverable so the link is reachable):
//   - Outer wrapper: pointer-events-AUTO so the tooltip captures the pointer
//     when the cursor travels from the node onto it. The parent wires
//     onMouseEnter (cancel the pending dismiss → keep open) + onMouseLeave
//     (reschedule the dismiss), so the "View artist page" link is reachable and
//     clickable instead of vanishing the instant the cursor leaves the node.
//     Trade-off: the small area the tooltip covers (offset 8px down-right of the
//     node) can't be hovered/clicked on the canvas while the tooltip is shown —
//     acceptable for a transient hint.
//   - Link inside: pointer-events-auto — the explicit escape hatch to the full
//     artist detail page (kept explicit as defense-in-depth, so the link works even
//     if the wrapper's pointer-events ever change). The hoverable grace period is a
//     desktop-mouse affordance; on touch the tooltip surfaces on long-press (PSY-369)
//     and the tap on the link goes through (the wrapper is pointer-events-auto), but
//     the mouse-grace timing isn't exercised on touch.
export interface ArtistNodeTooltipProps {
  node: {
    name: string
    slug: string
    city?: string
    state?: string
    upcoming_show_count: number
  }
  position: TooltipAnchor
  /**
   * PSY-1218: keep the tooltip open while the pointer is over it (the parent
   * cancels the dismiss timer on enter) and reschedule the dismiss on leave, so
   * the link below is reachable.
   *
   * REQUIRED, not optional: the wrapper is `pointer-events-auto`, so it captures
   * the pointer and the canvas stops firing `onNodeHover` for the area it covers.
   * Without these handlers the tooltip would never receive a dismiss signal and
   * could strand on screen — the exact PSY-1218 bug. Keeping them required makes
   * that illegal state unrepresentable: a caller can't enable the capturing
   * wrapper without also wiring its dismissal.
   */
  onMouseEnter: () => void
  onMouseLeave: () => void
}

export function ArtistNodeTooltip({ node, position, onMouseEnter, onMouseLeave }: ArtistNodeTooltipProps) {
  return (
    <div
      data-testid="artist-node-tooltip"
      className="absolute z-50 px-3 py-2 text-xs rounded-md bg-popover border border-border shadow-lg text-popover-foreground pointer-events-auto"
      // left/top sit at the node; the transform offsets the tooltip 8px off the
      // node and flips it toward the container interior near the right/bottom edge
      // (shared with ForceGraphView via tooltipPlacementStyle — PSY-1217).
      style={tooltipPlacementStyle(position)}
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
    >
      <div className="font-medium text-sm">{node.name}</div>
      {(node.city || node.state) && (
        <div className="text-muted-foreground">
          {[node.city, node.state].filter(Boolean).join(', ')}
        </div>
      )}
      {node.upcoming_show_count > 0 && (
        <div className="mt-1 text-green-400">
          {node.upcoming_show_count} upcoming {node.upcoming_show_count === 1 ? 'show' : 'shows'}
        </div>
      )}
      <Link
        href={`/artists/${node.slug}`}
        className="mt-1.5 inline-block text-primary hover:underline pointer-events-auto"
      >
        View artist page &rarr;
      </Link>
    </div>
  )
}

// Node circle radii — shared between the circle paint (nodeCanvasObject) and the
// label y-offset (nodeLabelsFrame) so the label always sits just below the circle
// edge; keep the two in lockstep (PSY-1209).
const CENTER_NODE_RADIUS = 12
const SATELLITE_NODE_RADIUS = 8

// PSY-1257: radial ego layout. The center is pinned at the origin; every satellite is
// pulled toward a single ring of this radius so the subject reads as the hub and the
// neighbors spread evenly around it instead of clumping into the free-force hairball.
// Depth-1 graph today, so all satellites share one ring; the math keys on hop distance
// (currently always 1) to stay ready for P1 multi-hop expand-on-demand, where outer
// rings encode depth. RADIAL_FORCE_STRENGTH is the per-tick pull toward the ring —
// higher snaps to the ring faster (less organic angular spread). Tuned visually on a
// dense radio ego graph. EGO_CHARGE_STRENGTH stiffens the default node repulsion so the
// satellites distribute EVENLY around the ring instead of bunching on one arc (left-side
// label crowding without it) — the radial force fixes the radius, charge fixes the angle.
const EGO_RING_RADIUS = 165
const RADIAL_FORCE_STRENGTH = 0.4
const EGO_CHARGE_STRENGTH = -210

// PSY-1258: dense relationship types get trimmed to each node's k strongest edges so the
// many-to-many radio signal stops rendering as a teal hairball. Only types listed here are
// capped (the rest are sparse enough to draw in full); k is intentionally tunable — start
// at 5 (the upper end of the research's 3–5 range) and adjust visually on /artists/cola.
// Isolated as its own map so the volatile "which types, what k" decision lives in one place.
const EDGE_CAP_BY_TYPE: Record<string, number> = { radio_cooccurrence: 5 }

// PSY-1218: how long the hoverable tooltip lingers after the cursor leaves the node
// before auto-hiding. The tooltip overlaps the node's pointer-area (8px offset vs a
// 10px hit radius), so this mainly absorbs accidental micro-movements off the node as
// the user settles onto the tooltip, not a long traverse. 300ms is an initial feel
// value (not yet tuned against a hover-timing trace); adjust if it feels sticky or too
// eager. Tuned independently of the nav menu's CLOSE_DELAY_MS (useHoverIntentMenu) —
// different gap geometry — so don't "unify" them.
const TOOLTIP_DISMISS_DELAY_MS = 300

export function ArtistGraphVisualization({
  data,
  activeTypes,
  containerWidth,
  onRecenter,
  isRecentering = false,
}: ArtistGraphProps) {
  const graphRef = useRef<any>(null) // eslint-disable-line @typescript-eslint/no-explicit-any
  const containerRef = useRef<HTMLDivElement>(null)
  const palette = useGraphPalette()
  const [hoveredNode, setHoveredNode] = useState<GraphNode | null>(null)
  // Tooltip position in CONTAINER coords (the tooltip is position:absolute within
  // the relative container). Set from the hovered node's screen position in
  // handleNodeHover; flipX/flipY anchor it toward the container's interior near the
  // right/bottom edges so it doesn't run off the dialog (PSY-1215).
  const [tooltipPos, setTooltipPos] = useState<TooltipPlacement>({ x: 0, y: 0, flipX: false, flipY: false })

  // PSY-1218: hoverable-tooltip dismiss timer (lifecycle extracted to useDismissTimer).
  // On hover-out we DELAY hiding the tooltip (scheduleDismiss) so the cursor can travel
  // onto it and click the "View artist page" link; entering the tooltip cancels the
  // timer (handleTooltipEnter → cancelDismiss), leaving it reschedules.
  //
  // overTooltipRef tracks whether the pointer is over the tooltip (set in
  // handleTooltipEnter/Leave). The dismiss callback bails while it's true, which closes
  // a canvas-vs-DOM ordering gap: when the cursor crosses from the node onto the
  // overlaying tooltip, force-graph fires onNodeHover(null) ONCE on that hover-out — and
  // because canvas hover detection runs in a requestAnimationFrame loop while the DOM
  // onMouseEnter fires synchronously, that single canvas fire can land a frame AFTER
  // onMouseEnter→cancelDismiss, re-arming a dismiss while the pointer rests on the
  // tooltip. The gate bails on it; onMouseLeave reschedules on a real exit, so the link
  // can't vanish from under the cursor.
  //
  // The flag is reset wherever the tooltip is torn down WITHOUT a DOM mouseleave —
  // onZoom and the [data.center.id] re-center effect below, both of which also
  // cancelDismiss. The normal pointer-leave path resets it via onMouseLeave.
  const overTooltipRef = useRef(false)
  const { schedule: scheduleDismiss, cancel: cancelDismiss } = useDismissTimer(() => {
    if (overTooltipRef.current) return
    setHoveredNode(null)
  }, TOOLTIP_DISMISS_DELAY_MS)

  // Reset hover when the center changes (re-center) — the React-recommended
  // "adjust state during render" pattern (not an effect). onNodeHover only fires
  // on the next under-pointer change, so without this the previous artist's
  // tooltip would linger at stale coords above the re-centering overlay (PSY-1215).
  const [hoverCenterId, setHoverCenterId] = useState(data.center.id)
  if (data.center.id !== hoverCenterId) {
    setHoverCenterId(data.center.id)
    setHoveredNode(null)
  }
  // Re-center tears the tooltip down in the render reset above WITHOUT a DOM
  // mouseleave (true for browser back/forward too, since center is URL-driven), so
  // reset the over-tooltip flag and cancel any pending dismiss here — a ref/timer
  // can't be written during render, and an orphaned timer would otherwise fire
  // against the new center. Mirrors onZoom's explicit teardown; the normal
  // pointer-leave path resets the flag via onMouseLeave (PSY-1218 review).
  useEffect(() => {
    overTooltipRef.current = false
    cancelDismiss()
  }, [data.center.id, cancelDismiss])
  const reducedMotion = useReducedMotion()

  const graphHeight = containerWidth < 768 ? 350 : 500

  // Build graph data from API response
  const graphData = useMemo(() => {
    const nodes: GraphNode[] = []
    const links: GraphLink[] = []

    // Center node
    nodes.push({
      id: data.center.id,
      name: data.center.name,
      slug: data.center.slug,
      city: data.center.city,
      state: data.center.state,
      upcoming_show_count: data.center.upcoming_show_count,
      isCenter: true,
      val: 8,
    })

    // Related nodes
    const nodeIds = new Set<number>([data.center.id])
    for (const node of data.nodes) {
      nodeIds.add(node.id)
      nodes.push({
        id: node.id,
        name: node.name,
        slug: node.slug,
        city: node.city,
        state: node.state,
        upcoming_show_count: node.upcoming_show_count,
        isCenter: false,
        val: 4,
      })
    }

    // Active links (filtered by the type toggles) in raw form, carrying the original
    // payload row under `raw` so the per-node cap can rank by score before we project
    // to the render shape. capEdgesPerNode keeps only each node's k strongest edges of a
    // dense type (PSY-1258) — its no-orphan invariant guarantees every still-connected
    // satellite keeps at least its strongest edge, so the connected-node filter below
    // can't make a node vanish under the cap.
    const activeRaw = data.links.filter(link => activeTypes.has(link.type))
    const { links: keptRaw, counts: edgeCounts, cappedTypes } = capEdgesPerNode(
      activeRaw.map(link => ({
        source: link.source_id,
        target: link.target_id,
        type: link.type,
        score: link.score,
        raw: link,
      })),
      EDGE_CAP_BY_TYPE,
    )

    for (const { raw } of keptRaw) {
      links.push({
        source: raw.source_id,
        target: raw.target_id,
        type: raw.type,
        score: raw.score,
        votes_up: raw.votes_up,
        votes_down: raw.votes_down,
        detail: raw.detail,
        isCrossConnection:
          raw.source_id !== data.center.id && raw.target_id !== data.center.id,
      })
    }

    // Find nodes that have visible edges
    const connectedIds = new Set<number>()
    for (const link of links) {
      connectedIds.add(typeof link.source === 'number' ? link.source : link.source.id)
      connectedIds.add(typeof link.target === 'number' ? link.target : link.target.id)
    }

    // Filter nodes to only those with visible edges (always keep center)
    const filteredNodes = nodes.filter(n => n.isCenter || connectedIds.has(n.id))

    return { nodes: filteredNodes, links, edgeCounts, cappedTypes }
  }, [data, activeTypes])

  // Ego layout forces (PSY-1257): pin the center at the origin and add a radial
  // constraint that seats every satellite on a single ring, so the subject reads as the
  // hub and neighbors spread evenly instead of settling into the free-force hairball.
  // Re-runs on graphData change (filter toggle / re-center) to re-register the closure
  // over the current nodes — mirrors ForceGraphView's force-config effect.
  useEffect(() => {
    const fg = graphRef.current
    if (!fg) return

    // Pin the center at the origin (relied on by zoomToFit framing and the radial ring's
    // geometry). `.find()` over the memoized nodes; the single-property write here is the
    // documented d3 pin contract, not an accidental mutation.
    const centerNode = graphData.nodes.find(n => n.isCenter)
    if (centerNode) {
      centerNode.fx = 0
      centerNode.fy = 0
    }

    // Custom radial tick force — a small inline reimplementation of d3-force's forceRadial
    // (d3-force isn't a direct dependency; react-force-graph bundles it but doesn't
    // re-export the factory). It nudges each satellite's velocity toward EGO_RING_RADIUS
    // along its own radius vector, letting the angle settle under charge repulsion. The
    // mutation lives inside the closure d3 calls each tick (NOT the effect body), so — like
    // ForceGraphView's clusterX/clusterY — it doesn't trip react-hooks/immutability. The
    // optional call (`d3Force?.`) no-ops against the test ref stubs, which expose only
    // pause/resume/zoom.
    fg.d3Force?.('radial', (alpha: number) => {
      for (const node of graphData.nodes) {
        if (node.isCenter) continue
        const x = node.x ?? 0
        const y = node.y ?? 0
        const dist = Math.hypot(x, y) || 1e-6
        const k = ((EGO_RING_RADIUS - dist) * RADIAL_FORCE_STRENGTH * alpha) / dist
        node.vx = (node.vx ?? 0) + x * k
        node.vy = (node.vy ?? 0) + y * k
      }
    })

    // Stiffen the default many-body repulsion so satellites spread evenly around the ring
    // (the radial force alone fixes radius, not angle). `.strength?.` no-ops on the test
    // stubs, which don't expose the d3 charge force.
    const charge = fg.d3Force?.('charge')
    charge?.strength?.(EGO_CHARGE_STRENGTH)
  }, [graphData])

  // PSY-1220 parity with ForceGraphView's reheat-dismiss: a filter-toggle (graphData, via
  // activeTypes) or a resize (containerWidth) reheats/reframes the layout and pans nodes under an
  // open tooltip, stranding it at a now-stale position. Dismiss the hover on those shifts (re-hover
  // re-anchors) via the SAME render-phase "adjust state during render" pattern as the re-center
  // reset above. Why render-phase and not an effect like ForceGraphView's dismiss: the EFFECT form
  // trips eslint's react-hooks/set-state-in-effect (a React-Compiler rule) in THIS component, while
  // the identical effect is clean in ForceGraphView — verified by probe (an isolated mount
  // setState-in-effect errors here, passes there). ForceGraphView's `eslint-disable
  // react-hooks/immutability` span (for its in-place d3-force node mutation) opts that whole
  // component out of the compiler-based effect rules; ArtistGraph has no such span, so don't
  // "unify" the two to effects here. graphData is memoized (on data/activeTypes — a stable useState
  // Set in the parent), so a referentially-new value is a real layout change, not a per-render
  // thrash (the existing adjacency/degree memos already rely on that same stability). Re-center is
  // also covered here (data → graphData), harmlessly alongside the center.id reset above.
  const [hoverLayoutSig, setHoverLayoutSig] = useState({ graphData, containerWidth })
  if (hoverLayoutSig.graphData !== graphData || hoverLayoutSig.containerWidth !== containerWidth) {
    setHoverLayoutSig({ graphData, containerWidth })
    setHoveredNode(null)
  }
  // The render-phase dismiss above has no DOM mouseleave, so reset the over-tooltip flag + cancel
  // any pending dismiss on the same shifts — otherwise a stuck overTooltipRef wedges the gate for
  // the next tooltip and an orphaned timer fires against a stale state (mirrors onZoom + the
  // re-center teardown effect). No setState here, so it stays a clean synchronization effect.
  useEffect(() => {
    overTooltipRef.current = false
    cancelDismiss()
  }, [graphData, containerWidth, cancelDismiss])

  // Accessibility: the rendered <canvas> is a visual enhancement; the
  // <RelatedArtists> list view is the keyboard/screen-reader counterpart
  // with proper <Link> semantics. We expose the canvas as an image with a
  // descriptive label so assistive tech announces it sensibly and points
  // users to the accessible list below.
  useEffect(() => {
    if (!containerRef.current) return
    const canvas = containerRef.current.querySelector('canvas')
    if (!canvas) return
    canvas.setAttribute('role', 'img')
    canvas.setAttribute(
      'aria-label',
      `Artist relationship graph for ${data.center.name}. Use the Related Artists list below to navigate.`
    )
  }, [data.center.name])

  // Reduced-motion: pause the continuous force simulation after the
  // initial layout settles so motion-sensitive users get a static
  // snapshot. Tap, pinch zoom, and pan are unaffected — only the
  // background tick animation stops.
  useEffect(() => {
    if (reducedMotion && graphRef.current) {
      graphRef.current.pauseAnimation()
    }
  }, [reducedMotion])

  // Kick a one-off repaint when reactive state the canvas reads changes but wouldn't
  // otherwise force a redraw:
  //   (1) palette (PSY-1209): labels in onRenderFramePost read it, but that callback
  //       doesn't self-trigger a redraw.
  //   (2) hover-focus (PSY-1210): nodeCanvasObject / linkColor / labels read focusedIds;
  //       force-graph's notifyRedraw only sets a flag the rAF loop consumes, and that
  //       loop can be idle/paused, so resumeAnimation is what guarantees the frame renders.
  // Gated on !reducedMotion (PSY-1226): force-graph's rAF loop reschedules unconditionally,
  // so resumeAnimation here would keep the loop running forever and DEFEAT the reduced-motion
  // pauseAnimation above. For reduced-motion users we keep the static snapshot — the canvas is
  // a visual enhancement and the accessible path is the RelatedArtists list. Deliberate
  // trade-offs for them: a theme toggle won't recolor the static graph's labels, and the
  // hover tooltip + hover-focus won't fire (both ride the paused loop's hit-testing).
  useEffect(() => {
    if (!reducedMotion) graphRef.current?.resumeAnimation()
  }, [palette, hoveredNode, reducedMotion])

  // PSY-361: re-frame the viewport after each new center's data lands so
  // the layout is properly centered + scaled. The 500ms transition is
  // smooth without being sluggish; 40px padding matches the canvas border.
  // Keyed on `data.center.id` so this fires once per re-center, not on
  // every filter toggle (filter changes preserve framing intentionally).
  useEffect(() => {
    if (!graphRef.current) return
    // Wait for the simulation to seat the nodes before measuring; without
    // a delay the bounding box is computed before forces have moved
    // anything, so the zoom-to-fit fires on stale positions.
    const timer = setTimeout(() => {
      graphRef.current?.zoomToFit(500, 40)
    }, 250)
    return () => clearTimeout(timer)
  }, [data.center.id])

  // PSY-361: clicking a non-center node re-centers the graph instead of
  // navigating to the artist's full page. The "View artist page →" link
  // inside the hover/long-press tooltip is the explicit nav escape.
  // Clicking the center node is a no-op — there's nothing to re-center to.
  const handleNodeClick = useCallback(
    (node: GraphNode) => {
      if (node.isCenter) return
      onRecenter?.({ id: node.id, slug: node.slug, name: node.name })
    },
    [onRecenter]
  )

  // Anchor the tooltip on the NODE (onNodeHover carries no MouseEvent) via the
  // shared nodeTooltipPlacement helper — see its doc-comment for the
  // graph2ScreenCoords + position:absolute rationale (PSY-1215, extracted in
  // PSY-1217). Set hoveredNode ONLY when a placement is returned so position and
  // node never desync; a null placement (hover-out, or a node without settled
  // coords) hides the tooltip rather than stranding it at a stale/origin position.
  const handleNodeHover = useCallback((node: GraphNode | null) => {
    // The center node has no rich tooltip (suppressed in the render guard below), so
    // treat hovering it like hover-out — otherwise crossing the center on the way to
    // a satellite's tooltip would cancelDismiss + show nothing, instantly killing the
    // grace window the feature exists to provide (PSY-1218 code-review).
    const placement =
      node && !node.isCenter
        ? nodeTooltipPlacement(graphRef.current, containerRef.current, node)
        : null
    if (placement) {
      cancelDismiss()
      setTooltipPos(placement)
      setHoveredNode(node)
    } else {
      // Hover-out, the center node, or an unplaceable node: DON'T hide immediately —
      // delay so the cursor can travel onto the tooltip and reach its link (PSY-1218).
      // Entering the tooltip cancels this; if the cursor never reaches it, it hides.
      scheduleDismiss()
    }
  }, [cancelDismiss, scheduleDismiss])

  // Keep the tooltip open while the pointer is over it (cancel the dismiss) and
  // reschedule the dismiss when it leaves. overTooltipRef gates the dismiss callback
  // against the canvas-vs-DOM race described on the timer above (PSY-1218).
  const handleTooltipEnter = useCallback(() => {
    overTooltipRef.current = true
    cancelDismiss()
  }, [cancelDismiss])
  const handleTooltipLeave = useCallback(() => {
    overTooltipRef.current = false
    scheduleDismiss()
  }, [scheduleDismiss])

  // PSY-1220: while the d3-force sim is live (onEngineTick), re-anchor the open tooltip to the
  // hovered node's CURRENT position — the node drifts during the settle/reheat but onNodeHover
  // doesn't re-fire for a stationary cursor, so the anchored tooltip would strand. Skip while the
  // pointer is over the tooltip (overTooltipRef): re-anchoring then would slide the HOVERABLE
  // tooltip (PSY-1218) out from under the cursor as it reaches for the "View artist page" link.
  // onEngineTick stops once the sim cools, so this is free at rest; the hoveredNode guard makes it
  // a no-op when nothing is hovered. A node that lost its settled coords (filtered out / refetched
  // away) yields null → dismiss.
  const handleEngineTick = useCallback(() => {
    if (!hoveredNode || overTooltipRef.current) return
    const placement = nodeTooltipPlacement(graphRef.current, containerRef.current, hoveredNode)
    if (placement) setTooltipPos(placement)
    else setHoveredNode(null)
  }, [hoveredNode])

  // Hover-focus (PSY-1210): when a node is hovered, IT + its 1-hop neighbors (plus the
  // center, which is the page subject and stays foreground always) are the
  // "foreground"; every other node/link/label fades to the background. Adjacency is
  // rebuilt only when the graph data changes; the foreground set is derived per-hover
  // (null = resting view, no focus). The repaint on hover change is forced by the
  // resumeAnimation effect above — see there for the reduced-motion caveat.
  //
  // The [graphData] dep on adjacency is load-bearing: the memo captures the freshly
  // rebuilt BARE-id links, before d3-force mutates source/target into resolved objects
  // in place (buildAdjacency would accept either shape, but only bare ids occur here).
  const adjacency = useMemo(() => buildAdjacency(graphData.links), [graphData])
  const focusedIds = useMemo(() => {
    if (hoveredNode == null) return null
    // Drop focus if the hovered node was filtered out / refetched away (no longer in
    // graphData): a stale hover would otherwise leave focusedIds matching nothing
    // visible and dim the WHOLE graph to the background alpha (PSY-1210 review).
    if (!graphData.nodes.some(n => n.id === hoveredNode.id)) return null
    // Keep the center (the page subject) foreground always — passed as the helper's
    // alwaysInclude anchor so the rule lives in one tested place (PSY-1210 review).
    return focusForeground(adjacency, hoveredNode.id, data.center.id)
  }, [adjacency, hoveredNode, graphData, data.center.id])

  const nodeCanvasObject = useCallback(
    (node: GraphNode, ctx: CanvasRenderingContext2D) => {
      const x = node.x ?? 0
      const y = node.y ?? 0
      const isCenter = node.isCenter
      const radius = isCenter ? CENTER_NODE_RADIUS : SATELLITE_NODE_RADIUS

      // Hover-focus (PSY-1210): dim nodes outside the foreground set. globalAlpha
      // multiplies every fill/stroke below (incl. the show indicator); it's reset to
      // 1 at the end so the next node + the post-frame labels render at full opacity.
      // Snap (no transition) — focus appears/clears with the hover, no fade animation.
      ctx.globalAlpha = focusedIds != null && !focusedIds.has(node.id) ? BACKGROUND_ALPHA : 1

      // Draw circle. Labels are NOT drawn here — they're rendered in a single
      // collision-culled post-frame pass (nodeLabelsFrame) so overlapping labels
      // can be dropped across all nodes at once (PSY-1209).
      ctx.beginPath()
      ctx.arc(x, y, radius, 0, 2 * Math.PI)

      if (isCenter) {
        ctx.fillStyle = '#6366f1' // indigo-500 accent
        ctx.fill()
        ctx.strokeStyle = '#818cf8' // indigo-400
        ctx.lineWidth = 2
        ctx.stroke()
      } else {
        ctx.fillStyle = 'rgba(63, 63, 70, 0.6)' // zinc-700/60
        ctx.fill()
        ctx.strokeStyle = 'rgba(161, 161, 170, 0.5)' // zinc-400/50
        ctx.lineWidth = 1
        ctx.stroke()
      }

      // Show indicator for upcoming shows
      if (node.upcoming_show_count > 0 && !isCenter) {
        ctx.beginPath()
        ctx.arc(x + radius - 2, y - radius + 2, 3, 0, 2 * Math.PI)
        ctx.fillStyle = '#22c55e' // green-500
        ctx.fill()
      }

      ctx.globalAlpha = 1 // reset so the next node / post-frame labels aren't dimmed
    },
    [focusedIds]
  )

  // Degree (link count) per node id → which label wins a collision (shared with
  // ForceGraphView via degreeMap so the two surfaces can't drift).
  const degreeById = useMemo(() => degreeMap(graphData.links), [graphData])

  // PSY-1258: legend disclosure. Each row shows its DISPLAYED (post-cap) edge count, and
  // when a dense type was actually trimmed we add a footnote naming the cap ("Radio
  // Co-occurrence: each artist's 5 strongest (47 of 312)") so the cap is never silent.
  const legendDisclosure = useMemo(() => {
    const counts = new Map<string, number>()
    for (const [type, tally] of graphData.edgeCounts) counts.set(type, tally.shown)
    if (graphData.cappedTypes.size === 0) {
      return { counts, footnote: undefined as string | undefined }
    }
    const footnote = Array.from(graphData.cappedTypes)
      .map(type => {
        const k = EDGE_CAP_BY_TYPE[type]
        const tally = graphData.edgeCounts.get(type)
        const of = tally ? ` (${tally.shown} of ${tally.total})` : ''
        return `${edgeTypeLabel(type)}: each artist's ${k} strongest${of}`
      })
      .join(' · ')
    return { counts, footnote }
  }, [graphData])

  // Node labels are drawn in one post-frame pass rather than per-node so they can
  // be collision-culled (PSY-1209): in a dense 1-hop graph the per-node labels
  // overlapped into an unreadable pile. The center node is always labeled (force);
  // other labels are kept in degree order and dropped when they'd overlap a
  // higher-priority one. A culled neighbor's name is reachable via the hover tooltip;
  // on hover-focus the background nodes drop their labels and only the foreground set is
  // a label candidate (still collision-culled among themselves — center + hovered are
  // forced) (PSY-1210, below). Same gate (globalScale > 0.7), font, truncation, and
  // y-offset the per-node paint used; the theme-aware halo+fill lives in
  // renderGraphLabels (shared with ForceGraphView).
  const nodeLabelsFrame = useCallback(
    (ctx: CanvasRenderingContext2D, globalScale: number) => {
      if (globalScale <= 0.7) return
      const fontSize = Math.max(10, Math.min(14, 12 / globalScale))
      const specs: GraphLabelSpec[] = graphData.nodes
        // Hover-focus (PSY-1210): when focused, label only the foreground set so the
        // background de-clutters; at rest (focusedIds null) label all, as before.
        .filter(node => focusedIds == null || focusedIds.has(node.id))
        .map(node => {
          const radius = node.isCenter ? CENTER_NODE_RADIUS : SATELLITE_NODE_RADIUS
          return {
            x: node.x ?? 0,
            y: (node.y ?? 0) + radius + 4,
            text: node.name.length > 20 ? node.name.slice(0, 18) + '...' : node.name,
            fontSize,
            bold: node.isCenter,
            // Always label the center and the hovered node. This only applies to the
            // foreground — the .filter above already drops background nodes — so the
            // `isCenter` here stays consistent with the center being in focusForeground's
            // alwaysInclude (a node not in focusedIds is filtered out, never force-labeled
            // over a dimmed circle). PSY-1210.
            force: node.isCenter || node.id === hoveredNode?.id,
            priority: degreeById.get(node.id) ?? 0,
          }
        })
      renderGraphLabels(ctx, palette, specs)
    },
    [graphData, palette, degreeById, focusedIds, hoveredNode]
  )

  // Shared edge grammar (PSY-1083): color from the theme-resolved palette,
  // dash + magnitude-scaled width from edgeGrammar. Cross-connections
  // (edges not touching the center) dim to 40% alpha, as before.
  const linkColor = useCallback(
    (link: GraphLink) => {
      const color = palette.edges[link.type] ?? palette.unknownEdge
      // Hover-focus (PSY-1210): a link is foreground only when BOTH its endpoints are in
      // the foreground set — those render at full color so the focused neighborhood's
      // edges stay crisp; every other link fades to the background. At rest (no focus),
      // keep the resting styling: cross-connections dim to 40% (PSY-1083).
      if (focusedIds) {
        const foreground =
          focusedIds.has(endpointId(link.source)) && focusedIds.has(endpointId(link.target))
        return foreground ? color : withHexAlpha(color, BACKGROUND_ALPHA_HEX)
      }
      return link.isCrossConnection ? withHexAlpha(color, '66') : color
    },
    [palette, focusedIds]
  )

  const linkWidth = useCallback((link: GraphLink) => edgeWidth(link.type, link.score), [])

  const linkLineDash = useCallback((link: GraphLink) => edgeLineDash(link.type), [])

  // PSY-362: hover tooltip on edges. react-force-graph-2d renders this as a native HTML
  // tooltip when the cursor is over a link. The text surfaces the underlying raw signal
  // (similarity score, shared count, label name, radio co-occurrence count) so users can
  // see *why* the graph is drawing this edge.
  const linkLabel = useCallback((link: GraphLink) => buildLinkLabel(link), [])

  return (
    <div
      ref={containerRef}
      className="relative rounded-lg border border-border/50 overflow-hidden bg-background"
    >
      <ForceGraph2D
        ref={graphRef}
        graphData={graphData}
        width={containerWidth}
        height={graphHeight}
        nodeId="id"
        nodeVal="val"
        nodeCanvasObject={nodeCanvasObject}
        onRenderFramePost={nodeLabelsFrame}
        // PSY-1220: keep the open tooltip pinned to its node as the node drifts during settle.
        onEngineTick={handleEngineTick}
        // The hit area is deliberately NOT narrowed for background (hover-focus-faded)
        // nodes (PSY-1210): the fade is a visual de-emphasis, but every node stays fully
        // hoverable/clickable so you can move focus to any node by hovering it (and the
        // node brightens the moment it becomes the hovered/foreground node).
        nodePointerAreaPaint={(node: GraphNode, color: string, ctx: CanvasRenderingContext2D) => {
          const x = node.x ?? 0
          const y = node.y ?? 0
          ctx.beginPath()
          ctx.arc(x, y, node.isCenter ? 14 : 10, 0, 2 * Math.PI)
          ctx.fillStyle = color
          ctx.fill()
        }}
        onNodeClick={handleNodeClick}
        onNodeHover={handleNodeHover}
        // Wheel-zoom moves the node under a stationary pointer without re-firing
        // onNodeHover, stranding the tooltip at a stale screen position — dismiss it
        // on zoom (re-hover re-anchors it) (PSY-1215). This unmounts the tooltip with
        // no DOM mouseleave, so reset the over-tooltip flag and cancel any pending
        // dismiss too, or the flag would wedge the gate for the next tooltip (PSY-1218).
        onZoom={() => { overTooltipRef.current = false; cancelDismiss(); setHoveredNode(null) }}
        // The rich ArtistNodeTooltip below (anchored at the node) replaces the
        // default native name pill for satellite nodes, so suppress the pill there
        // to avoid a redundant second tooltip. KEEP it for the center node, which
        // has no rich tooltip — otherwise hovering the center shows no name at low
        // zoom (PSY-1215). linkLabel (edge tooltip) is kept.
        nodeLabel={(node: GraphNode) => (node.isCenter ? node.name : '')}
        // PSY-361 / PSY-369 spike: disable node drag to remove the
        // tap-vs-drag ambiguity on touch devices. Tap = re-center,
        // long-press = tooltip; nothing for drag to do.
        enableNodeDrag={false}
        linkSource="source"
        linkTarget="target"
        linkColor={linkColor}
        linkWidth={linkWidth}
        linkLineDash={linkLineDash}
        linkLabel={linkLabel}
        linkDirectionalParticles={0}
        cooldownTicks={100}
        d3AlphaDecay={0.04}
        d3VelocityDecay={0.3}
        minZoom={0.5}
        maxZoom={3}
        backgroundColor="transparent"
      />

      {hoveredNode && !hoveredNode.isCenter && (
        <ArtistNodeTooltip
          node={hoveredNode}
          position={tooltipPos}
          onMouseEnter={handleTooltipEnter}
          onMouseLeave={handleTooltipLeave}
        />
      )}

      {/* PSY-361: re-center loading overlay. Sits above the canvas without
          unmounting it — the previous frame stays visible underneath so
          the transition feels continuous. The simulation itself is paused
          by the parent (it stops dispatching new graph data while the
          query is in flight); we just visually attribute the wait. */}
      {isRecentering && (
        <div
          className="absolute inset-0 z-40 flex items-center justify-center bg-background/40 backdrop-blur-[1px]"
          aria-hidden="true"
        >
          <div className="flex items-center gap-2 px-3 py-2 text-xs rounded-md bg-popover border border-border shadow-md">
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
            <span>Re-centering...</span>
          </div>
        </div>
      )}

      {/* Legend — shared EdgeLegend (PSY-1083) in static mode: rows mirror
          the active type toggles (the pill row above the canvas owns
          toggling, including the festival_cobill lazy opt-in fetch, so the
          in-canvas legend stays display-only here). Counts + footnote disclose
          the per-node top-k edge cap (PSY-1258) so it's never silent. */}
      <EdgeLegend
        className="absolute top-2 right-2"
        types={Array.from(activeTypes)}
        counts={legendDisclosure.counts}
        footnote={legendDisclosure.footnote}
      />
    </div>
  )
}
