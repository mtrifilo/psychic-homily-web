'use client'

import { useCallback, useMemo, useRef, useEffect, useState, type ComponentType, type MutableRefObject } from 'react'
import Link from 'next/link'
import dynamic from 'next/dynamic'
import { Loader2 } from 'lucide-react'
import type { ForceGraphMethods, ForceGraphProps } from 'react-force-graph-2d'
import { buildLinkLabel, edgeLineDash, edgeWidth } from '@/components/graph/edgeGrammar'
import { useGraphPalette, withHexAlpha } from '@/components/graph/graphPalette'
import { degreeMap, renderGraphLabels, type GraphLabelSpec } from '@/components/graph/graphLabels'
import { buildAdjacency, endpointId, focusForeground } from '@/components/graph/graphFocus'
import { nodeTooltipPlacement, tooltipPlacementStyle, type TooltipAnchor, type TooltipPlacement } from '@/components/graph/nodeTooltip'
import { EdgeLegend } from '@/components/graph/EdgeLegend'
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
// Pointer-events grammar:
//   - Outer wrapper: pointer-events-none — the tooltip is just a visual
//     hint and must not steal hover/click events from the canvas
//     underneath (otherwise the cursor sliding from a node onto the
//     tooltip would dismiss the tooltip and break re-center clicks on
//     adjacent nodes).
//   - Link inside: pointer-events-auto — selectively re-enables the
//     escape hatch to the full artist detail page. Works on desktop
//     (hover surfaces tooltip, click goes through) and mobile
//     (long-press surfaces tooltip per PSY-369 grammar, tap goes
//     through).
export interface ArtistNodeTooltipProps {
  node: {
    name: string
    slug: string
    city?: string
    state?: string
    upcoming_show_count: number
  }
  position: TooltipAnchor
}

export function ArtistNodeTooltip({ node, position }: ArtistNodeTooltipProps) {
  return (
    <div
      className="absolute z-50 px-3 py-2 text-xs rounded-md bg-popover border border-border shadow-lg text-popover-foreground pointer-events-none"
      // left/top sit at the node; the transform offsets the tooltip 8px off the
      // node and flips it toward the container interior near the right/bottom edge
      // (shared with ForceGraphView via tooltipPlacementStyle — PSY-1217).
      style={tooltipPlacementStyle(position)}
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

// PSY-1210 hover-focus: nodes/links outside the foreground set fade using this alpha.
// BACKGROUND_ALPHA is the canvas globalAlpha for nodes; BACKGROUND_ALPHA_HEX is the same
// value as a 2-char hex pair for withHexAlpha on link colors — derived, so tuning the
// constant moves both. (They share the source number, not the PERCEIVED opacity: the node
// globalAlpha multiplies the node's already-semi-transparent fill, so backgrounded nodes
// read a touch fainter than the flat-alpha links. Note withHexAlpha passes any non-6-hex
// color through UNCHANGED, so if an --edge-* token ever became oklch/rgb the background
// links would silently render at FULL color (no fade) while nodes still dim — the same
// latent gap the resting cross-connection dim already has. All current tokens are 6-hex.)
const BACKGROUND_ALPHA = 0.15
const BACKGROUND_ALPHA_HEX = Math.round(BACKGROUND_ALPHA * 255).toString(16).padStart(2, '0')

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

  // Reset hover when the center changes (re-center) — the React-recommended
  // "adjust state during render" pattern (not an effect). onNodeHover only fires
  // on the next under-pointer change, so without this the previous artist's
  // tooltip would linger at stale coords above the re-centering overlay (PSY-1215).
  const [hoverCenterId, setHoverCenterId] = useState(data.center.id)
  if (data.center.id !== hoverCenterId) {
    setHoverCenterId(data.center.id)
    setHoveredNode(null)
  }
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

    // Filter links by active types and build graph links
    for (const link of data.links) {
      if (!activeTypes.has(link.type)) continue

      const isCross =
        link.source_id !== data.center.id && link.target_id !== data.center.id

      links.push({
        source: link.source_id,
        target: link.target_id,
        type: link.type,
        score: link.score,
        votes_up: link.votes_up,
        votes_down: link.votes_down,
        detail: link.detail,
        isCrossConnection: isCross,
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

    return { nodes: filteredNodes, links }
  }, [data, activeTypes])

  // Set node size based on score
  useEffect(() => {
    if (graphRef.current) {
      // Fix center node position
      const centerNode = graphData.nodes.find(n => n.isCenter)
      if (centerNode) {
        centerNode.fx = 0
        centerNode.fy = 0
      }
    }
  }, [graphData])

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
  // CAVEAT: force-graph's rAF loop reschedules unconditionally, so resumeAnimation here
  // (on mount via palette, then on hover) keeps the loop running even under
  // prefers-reduced-motion — i.e. the pauseAnimation effect above doesn't actually hold.
  // That's PRE-EXISTING (the [palette] resume already ran on mount before this change);
  // hover-focus thus DOES render for reduced-motion, but the pause being defeated is the
  // real issue, tracked in PSY-1226. (The canvas is a visual enhancement; the accessible
  // path is the RelatedArtists list.)
  useEffect(() => {
    graphRef.current?.resumeAnimation()
  }, [palette, hoveredNode])

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
    const placement = nodeTooltipPlacement(graphRef.current, containerRef.current, node)
    if (placement) {
      setTooltipPos(placement)
      setHoveredNode(node)
    } else {
      setHoveredNode(null)
    }
  }, [])

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
        // on zoom (re-hover re-anchors it) (PSY-1215).
        onZoom={() => setHoveredNode(null)}
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
        <ArtistNodeTooltip node={hoveredNode} position={tooltipPos} />
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
          in-canvas legend stays display-only here). */}
      <EdgeLegend className="absolute top-2 right-2" types={Array.from(activeTypes)} />
    </div>
  )
}
