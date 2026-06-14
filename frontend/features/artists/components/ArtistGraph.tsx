'use client'

import { useCallback, useMemo, useRef, useEffect, useState, type ComponentType, type MutableRefObject } from 'react'
import Link from 'next/link'
import dynamic from 'next/dynamic'
import { Loader2 } from 'lucide-react'
import type { ForceGraphMethods, ForceGraphProps } from 'react-force-graph-2d'
import { buildLinkLabel, edgeLineDash, edgeWidth } from '@/components/graph/edgeGrammar'
import { useGraphPalette, withHexAlpha } from '@/components/graph/graphPalette'
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
  position: { x: number; y: number }
}

export function ArtistNodeTooltip({ node, position }: ArtistNodeTooltipProps) {
  return (
    <div
      className="fixed z-50 px-3 py-2 text-xs rounded-md bg-popover border border-border shadow-lg text-popover-foreground pointer-events-none"
      style={{
        left: position.x + 12,
        top: position.y - 10,
      }}
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
  // The hover handler currently never repositions the tooltip — see the note on
  // `handleNodeHover` below. Pinned to origin until that's fixed.
  const [tooltipPos] = useState({ x: 0, y: 0 })
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

  // react-force-graph-2d invokes `onNodeHover` with `(node, previousNode)` —
  // there's no MouseEvent in the signature (see `force-graph` `force-graph.js`
  // line ~633: `fn(obj.d, prevObj.d)`). The previous-node arg is unused; the
  // tooltip is pinned at origin until a follow-up adds pointer-based positioning.
  const handleNodeHover = useCallback((node: GraphNode | null) => {
    setHoveredNode(node)
  }, [])

  const nodeCanvasObject = useCallback(
    (node: GraphNode, ctx: CanvasRenderingContext2D, globalScale: number) => {
      const x = node.x ?? 0
      const y = node.y ?? 0
      const isCenter = node.isCenter
      const radius = isCenter ? 12 : 8
      const fontSize = Math.max(10, Math.min(14, 12 / globalScale))

      // Draw circle
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

      // Draw label (only when zoomed in enough)
      if (globalScale > 0.7) {
        const label = node.name.length > 20 ? node.name.slice(0, 18) + '...' : node.name
        ctx.font = `${isCenter ? 'bold ' : ''}${fontSize}px sans-serif`
        ctx.textAlign = 'center'
        ctx.textBaseline = 'top'
        // PSY-1092: still hardcoded light colors — illegible on the light theme,
        // the same bug PSY-1091 fixed for ForceGraphView. `palette` already
        // exposes theme-aware labelText/labelHalo; deferred here only for the
        // center-node distinction design call. Do not assume this graph is fixed.
        ctx.fillStyle = isCenter ? '#ffffff' : 'rgba(228, 228, 231, 0.9)' // zinc-200
        ctx.fillText(label, x, y + radius + 4)
      }
    },
    []
  )

  // Shared edge grammar (PSY-1083): color from the theme-resolved palette,
  // dash + magnitude-scaled width from edgeGrammar. Cross-connections
  // (edges not touching the center) dim to 40% alpha, as before.
  const linkColor = useCallback(
    (link: GraphLink) => {
      const color = palette.edges[link.type] ?? palette.unknownEdge
      return link.isCrossConnection ? withHexAlpha(color, '66') : color
    },
    [palette]
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
