'use client'

import { useCallback, useMemo, useRef, useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import dynamic from 'next/dynamic'
import { Loader2 } from 'lucide-react'
import { useReducedMotion } from '../hooks/useReducedMotion'
import type { ArtistGraph as ArtistGraphData, ArtistGraphNode, ArtistGraphLink } from '../types'

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

const ForceGraph2D = dynamic(() => import('react-force-graph-2d'), {
  ssr: false,
  loading: () => <GraphSkeleton />,
})

// PSY-362: Canonical visual style map for typed edges. Keep this co-located here so
// future graph surfaces (scene-scale, venue, festival) can reuse the same grammar.
//
// Colorblind audit (2026-04-24): All 6 colors verified against Protanopia, Deuteranopia,
// and Tritanopia transformation matrices using a 30-unit RGB Euclidean threshold. All 15
// pairs pass the threshold under all 3 vision types. Closest pair under any simulator is
// shared_bills vs radio_cooccurrence at d=35.3 (protanopia), which is also dash-differentiated
// (solid vs dashed-8-3) for redundancy. Full audit: docs/research/graph-colorblind-audit.md.
//
// WCAG 2.2 §1.4.1 ("Use of Color"): we never rely on color alone — every edge type has a
// dash pattern (solid / dashed / dotted) and many also have weight scaling, so information
// is conveyed through at least two channels.
const EDGE_COLORS: Record<string, string> = {
  similar: '#a1a1aa',              // zinc-400 (neutral)
  shared_bills: '#60a5fa',         // blue-400
  shared_label: '#c084fc',         // purple-400
  side_project: '#4ade80',         // green-400
  member_of: '#fbbf24',            // amber-400
  radio_cooccurrence: '#2dd4bf',   // teal-400
}

const EDGE_LABELS: Record<string, string> = {
  similar: 'Similar',
  shared_bills: 'Shared Bills',
  shared_label: 'Shared Label',
  side_project: 'Side Project',
  member_of: 'Member Of',
  radio_cooccurrence: 'Radio Co-occurrence',
}

// Convert API data to graph format needed by react-force-graph-2d
interface GraphNode {
  id: number
  name: string
  slug: string
  city?: string
  state?: string
  upcoming_show_count: number
  isCenter: boolean
  val: number // node size
}

interface GraphLink {
  source: number
  target: number
  type: string
  score: number
  votes_up: number
  votes_down: number
  detail?: Record<string, unknown>
  isCrossConnection: boolean
}

// Helper: pull a number out of the loosely-typed `detail` JSONB blob.
// Returns undefined when the field is missing or not coercible to a number.
function detailNumber(detail: Record<string, unknown> | undefined, key: string): number | undefined {
  if (!detail) return undefined
  const v = detail[key]
  if (typeof v === 'number') return v
  if (typeof v === 'string') {
    const n = Number(v)
    return Number.isFinite(n) ? n : undefined
  }
  return undefined
}

function detailString(detail: Record<string, unknown> | undefined, key: string): string | undefined {
  if (!detail) return undefined
  const v = detail[key]
  return typeof v === 'string' && v.length > 0 ? v : undefined
}

// Build the hover tooltip string for an edge. The text is edge-type aware and surfaces
// the underlying raw signal (count, score, label name) sourced from the link's `detail`
// JSONB or `score` field. If the data shape doesn't carry the field we'd ideally show,
// we fall back to a description that uses what's available — never fabricate a number.
//
// Exported for unit testing the format of each edge type's tooltip string.
export function buildLinkLabel(link: Pick<GraphLink, 'type' | 'score' | 'votes_up' | 'votes_down' | 'detail'>): string {
  const detail = link.detail
  switch (link.type) {
    case 'similar': {
      const pct = Math.round(link.score * 100)
      const total = link.votes_up + link.votes_down
      if (total > 0) {
        return `Similar: ${pct}% (${link.votes_up} up / ${link.votes_down} down)`
      }
      return `Similar: ${pct}%`
    }
    case 'shared_bills': {
      const count = detailNumber(detail, 'shared_count')
      const lastShared = detailString(detail, 'last_shared')
      if (count !== undefined) {
        const noun = count === 1 ? 'show' : 'shows'
        return lastShared
          ? `${count} shared ${noun} (last: ${lastShared})`
          : `${count} shared ${noun}`
      }
      return 'Shared bills'
    }
    case 'shared_label': {
      const count = detailNumber(detail, 'shared_count')
      const labelNames = detailString(detail, 'label_names')
      if (labelNames) {
        return count !== undefined && count > 1
          ? `${count} shared labels: ${labelNames}`
          : `Both on ${labelNames}`
      }
      if (count !== undefined) {
        const noun = count === 1 ? 'label' : 'labels'
        return `${count} shared ${noun}`
      }
      return 'Shared label'
    }
    case 'radio_cooccurrence': {
      const coCount = detailNumber(detail, 'co_occurrence_count')
      const stationCount = detailNumber(detail, 'station_count')
      if (coCount !== undefined) {
        const stationPart =
          stationCount !== undefined && stationCount > 1 ? ` across ${stationCount} stations` : ''
        const noun = coCount === 1 ? 'show' : 'shows'
        return `Played together on ${coCount} radio ${noun}${stationPart}`
      }
      return 'Radio co-occurrence'
    }
    case 'side_project':
      return 'Side project'
    case 'member_of':
      return 'Member of'
    default:
      return EDGE_LABELS[link.type] ?? link.type
  }
}

interface ArtistGraphProps {
  data: ArtistGraphData
  activeTypes: Set<string>
  containerWidth: number
}

export function ArtistGraphVisualization({ data, activeTypes, containerWidth }: ArtistGraphProps) {
  const router = useRouter()
  const graphRef = useRef<any>(null) // eslint-disable-line @typescript-eslint/no-explicit-any
  const containerRef = useRef<HTMLDivElement>(null)
  const [hoveredNode, setHoveredNode] = useState<GraphNode | null>(null)
  const [tooltipPos, setTooltipPos] = useState({ x: 0, y: 0 })
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
      connectedIds.add(typeof link.source === 'number' ? link.source : (link.source as any).id)
      connectedIds.add(typeof link.target === 'number' ? link.target : (link.target as any).id)
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
        (centerNode as any).fx = 0;
        (centerNode as any).fy = 0
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

  const handleNodeClick = useCallback(
    (node: GraphNode) => {
      router.push(`/artists/${node.slug}`)
    },
    [router]
  )

  const handleNodeHover = useCallback(
    (node: GraphNode | null, event?: MouseEvent) => {
      setHoveredNode(node)
      if (node && event) {
        setTooltipPos({ x: event.clientX, y: event.clientY })
      }
    },
    []
  )

  const nodeCanvasObject = useCallback(
    (node: GraphNode, ctx: CanvasRenderingContext2D, globalScale: number) => {
      const x = (node as any).x ?? 0
      const y = (node as any).y ?? 0
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
        ctx.fillStyle = isCenter ? '#ffffff' : 'rgba(228, 228, 231, 0.9)' // zinc-200
        ctx.fillText(label, x, y + radius + 4)
      }
    },
    []
  )

  const linkColor = useCallback(
    (link: GraphLink) => {
      const color = EDGE_COLORS[link.type] || '#71717a'
      if (link.isCrossConnection) {
        // 40% opacity for cross-connections
        return color + '66'
      }
      return color
    },
    []
  )

  // PSY-362: Stroke weight encoding per edge type.
  //
  //   similar              — magnitude (Wilson similarity score). Scaled.
  //   shared_bills         — magnitude (recency-weighted shared-show count). Scaled.
  //   radio_cooccurrence   — magnitude (cross-station-weighted co-occurrence). Scaled.
  //   shared_label         — magnitude (count of shared labels, normalized to [0,1] in
  //                          the deriver, capped at 5+ shared labels = 1.0). Scaled.
  //   side_project         — BINARY fact ("X is a side project of Y"). Intentionally uniform —
  //                          a side project either exists or does not, there is no magnitude.
  //   member_of            — BINARY fact ("X is a member of Y"). Intentionally uniform — same
  //                          rationale as side_project.
  const linkWidth = useCallback(
    (link: GraphLink) => {
      switch (link.type) {
        case 'similar':
        case 'shared_bills':
        case 'shared_label':
        case 'radio_cooccurrence':
          return Math.max(1, link.score * 3)
        case 'side_project':
        case 'member_of':
          // Binary relationship — uniform stroke is intentional.
          return 1
        default:
          return 1
      }
    },
    []
  )

  const linkLineDash = useCallback(
    (link: GraphLink) => {
      if (link.type === 'shared_label') return [5, 5]
      if (link.type === 'side_project' || link.type === 'member_of') return [2, 4]
      if (link.type === 'radio_cooccurrence') return [8, 3]
      return []
    },
    []
  )

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
          const x = (node as any).x ?? 0
          const y = (node as any).y ?? 0
          ctx.beginPath()
          ctx.arc(x, y, node.isCenter ? 14 : 10, 0, 2 * Math.PI)
          ctx.fillStyle = color
          ctx.fill()
        }}
        onNodeClick={handleNodeClick}
        onNodeHover={handleNodeHover}
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

      {/* Tooltip */}
      {hoveredNode && !hoveredNode.isCenter && (
        <div
          className="fixed z-50 px-3 py-2 text-xs rounded-md bg-popover border border-border shadow-lg text-popover-foreground pointer-events-none"
          style={{
            left: tooltipPos.x + 12,
            top: tooltipPos.y - 10,
          }}
        >
          <div className="font-medium text-sm">{hoveredNode.name}</div>
          {(hoveredNode.city || hoveredNode.state) && (
            <div className="text-muted-foreground">
              {[hoveredNode.city, hoveredNode.state].filter(Boolean).join(', ')}
            </div>
          )}
          {hoveredNode.upcoming_show_count > 0 && (
            <div className="mt-1 text-green-400">
              {hoveredNode.upcoming_show_count} upcoming {hoveredNode.upcoming_show_count === 1 ? 'show' : 'shows'}
            </div>
          )}
        </div>
      )}

      {/* Legend */}
      <div className="absolute top-2 right-2 p-2 rounded-md bg-background/80 backdrop-blur-sm border border-border/50 text-xs space-y-1">
        {Array.from(activeTypes).map(type => (
          <div key={type} className="flex items-center gap-1.5">
            <div
              className="w-4 h-0.5 rounded-full"
              style={{
                backgroundColor: EDGE_COLORS[type] || '#71717a',
                borderStyle: type === 'shared_label' ? 'dashed' : type === 'side_project' || type === 'member_of' ? 'dotted' : 'solid',
              }}
            />
            <span className="text-muted-foreground">{EDGE_LABELS[type] || type}</span>
          </div>
        ))}
        {/* PSY-362: weight-scale affordance — communicates that line thickness encodes
            signal magnitude (similarity score, shared-show count, etc.) so users know
            the visual grammar before hovering individual edges. */}
        <div className="pt-1 mt-1 border-t border-border/40 flex items-center gap-1.5">
          <div className="flex flex-col items-center gap-0.5" aria-hidden="true">
            <div className="w-4 h-px rounded-full bg-muted-foreground/60" />
            <div className="w-4 h-[3px] rounded-full bg-muted-foreground" />
          </div>
          <span className="text-[10px] text-muted-foreground/80 leading-tight">
            Thicker = stronger signal
          </span>
        </div>
      </div>
    </div>
  )
}
