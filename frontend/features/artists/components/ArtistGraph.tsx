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

// Edge type colors
//
// PSY-363: festival_cobill (vermillion #D55E00) was added to the existing 6
// after running the same colorblind audit method PSY-362 used (Brettel/Vienot
// CVD matrices, RGB Euclidean threshold of 30). Worst-case distance against
// any existing color is 98.2 vs member_of under deuteranopia — well above the
// threshold. Full audit results in docs/learnings/graph-colorblind-audit.md.
const EDGE_COLORS: Record<string, string> = {
  similar: '#a1a1aa',              // zinc-400 (neutral)
  shared_bills: '#60a5fa',         // blue-400
  shared_label: '#c084fc',         // purple-400
  side_project: '#4ade80',         // green-400
  member_of: '#fbbf24',            // amber-400
  radio_cooccurrence: '#2dd4bf',   // teal-400
  festival_cobill: '#D55E00',      // vermillion (Okabe-Ito)
}

const EDGE_LABELS: Record<string, string> = {
  similar: 'Similar',
  shared_bills: 'Shared Bills',
  shared_label: 'Shared Label',
  side_project: 'Side Project',
  member_of: 'Member Of',
  radio_cooccurrence: 'Radio Co-occurrence',
  festival_cobill: 'Festival co-lineup',
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

// PSY-363 — Helpers for pulling values out of the `detail` JSONB blob
// returned by the backend. Loosely-typed because the column is JSONB
// and the shape varies per edge type.
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

// buildLinkLabel produces the hover-tooltip text for an edge. It is
// edge-type aware so each signal can surface its raw underlying data.
// Exported so the format of each edge type's tooltip can be unit tested.
//
// PSY-363: festival_cobill case. Examples of expected output:
//   "3 shared festivals: ACL, Coachella, Lollapalooza (last: 2025)"
//   "3 shared festivals (last: 2025)"          (when names sparse)
//   "3 shared festivals"                        (when both names + year sparse)
//   "Festival co-lineup"                        (when count missing too)
//
// Other edge types fall back to the static EDGE_LABELS map. PSY-362 will
// expand each case with full underlying-signal text for similar /
// shared_bills / shared_label / radio_cooccurrence; this ticket only
// adds the festival_cobill case.
export function buildLinkLabel(link: Pick<GraphLink, 'type' | 'detail'>): string {
  switch (link.type) {
    case 'festival_cobill': {
      const count = detailNumber(link.detail, 'count')
      const names = detailString(link.detail, 'festival_names')
      const year = detailNumber(link.detail, 'most_recent_year')
      if (count === undefined) {
        return EDGE_LABELS.festival_cobill ?? 'Festival co-lineup'
      }
      const noun = count === 1 ? 'festival' : 'festivals'
      const headline = names
        ? `${count} shared ${noun}: ${names}`
        : `${count} shared ${noun}`
      return year !== undefined ? `${headline} (last: ${year})` : headline
    }
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

  const linkWidth = useCallback(
    (link: GraphLink) => {
      if (link.type === 'similar') {
        return Math.max(1, link.score * 3)
      }
      if (link.type === 'shared_bills') {
        return Math.max(1, link.score * 3)
      }
      if (link.type === 'radio_cooccurrence') {
        return Math.max(1, link.score * 3)
      }
      // PSY-363: festival_cobill has magnitude (count of shared festivals,
      // recency-weighted), so weight encodes signal strength like the other
      // count-based edges.
      if (link.type === 'festival_cobill') {
        return Math.max(1, link.score * 3)
      }
      return 1
    },
    []
  )

  const linkLineDash = useCallback(
    (link: GraphLink) => {
      if (link.type === 'shared_label') return [5, 5]
      if (link.type === 'side_project' || link.type === 'member_of') return [2, 4]
      if (link.type === 'radio_cooccurrence') return [8, 3]
      // PSY-363: long-dash pattern for festival_cobill. Color (vermillion)
      // is sufficiently distinct under all 3 CVD types per the audit, but
      // the dash provides redundant encoding (WCAG 2.2 §1.4.1).
      if (link.type === 'festival_cobill') return [10, 4]
      return []
    },
    []
  )

  // PSY-363: hover tooltip for edges. Pulls underlying signal data out of
  // the loosely-typed `detail` JSONB so users can see *why* an edge exists.
  // When PSY-362 lands it will introduce a richer top-level helper that
  // covers every edge type; for now this only handles festival_cobill,
  // which is what this ticket adds. Other types fall back to the static
  // EDGE_LABELS string.
  const linkLabel = useCallback(
    (link: GraphLink) => buildLinkLabel(link),
    []
  )

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
                borderStyle:
                  type === 'shared_label' || type === 'festival_cobill'
                    ? 'dashed'
                    : type === 'side_project' || type === 'member_of'
                      ? 'dotted'
                      : 'solid',
              }}
            />
            <span className="text-muted-foreground">{EDGE_LABELS[type] || type}</span>
          </div>
        ))}
      </div>
    </div>
  )
}
