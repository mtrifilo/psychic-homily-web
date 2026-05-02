'use client'

/**
 * ForceGraphView (PSY-365 — extracted from SceneGraphVisualization, PSY-367)
 *
 * Shared cluster-aware force-directed canvas. Renders any "scene-shaped"
 * graph payload — nodes, links, optional clusters — using the same d3-force
 * config, hull rendering, and click/hover behaviour that PSY-367 shipped for
 * the scene graph. The scene-graph and venue-bill-network surfaces both
 * compose this primitive instead of forking a 500-LOC canvas component.
 *
 * Why extract on the second instance, not the third (per Code Complete
 * "rule of three"): the venue surface does NOT compute clusters yet (the
 * scene-graph "primary venue per artist" signal collapses at venue scope),
 * but the layout, hull, isolate-shelf, callback-ref width measurement, and
 * a11y setup are 100% reusable. Forking those into a parallel
 * `VenueBillNetworkVisualization` would mean two copies of every PSY-516,
 * PSY-517, PSY-518, PSY-519 patch going forward — a maintenance trap. The
 * extraction shrinks SceneGraphVisualization to a thin shape adapter and
 * lets VenueBillNetwork ship with no canvas code of its own.
 *
 * Behaviour preserved from SceneGraphVisualization (do not regress):
 *   - cluster-aware d3-force: per-cluster centroid `forceX`/`forceY`,
 *     stronger intra-cluster `forceLink`, weaker between-cluster
 *   - convex hulls behind clusters at zoom ≤ 1.0×, fading to invisible at
 *     zoom ≥ 1.6× (`d3-polygon` polygonHull)
 *   - per-cluster fill from the Okabe-Ito 8-color palette; ungrouped /
 *     "other" nodes get a neutral grey
 *   - isolated nodes pinned to a perimeter shelf so they don't drift
 *   - prefers-reduced-motion pauses the simulation immediately on mount
 *   - canvas gets `role="img"` + a caller-provided `aria-label`
 *   - click handler delegates to caller (scene + venue both navigate to
 *     `/artists/{slug}` — that's the PSY-361 "exit to global graph"
 *     pattern, the caller controls it)
 *   - hover tooltip shows name, location, cluster (when present), and
 *     upcoming show count
 */

import { useCallback, useMemo, useRef, useEffect, useState } from 'react'
import dynamic from 'next/dynamic'
import { Loader2 } from 'lucide-react'
import { polygonHull } from 'd3-polygon'
import { useReducedMotion } from '@/features/artists/hooks/useReducedMotion'

// ──────────────────────────────────────────────
// Public types — the generic graph payload shape
// ──────────────────────────────────────────────

/**
 * Cluster definition. v1 uses the Okabe-Ito 8-color palette indexed by
 * `colorIndex` (0..7); -1 = "other" / ungrouped (rendered in neutral grey).
 * Callers that don't compute clusters can omit the array entirely.
 */
export interface GraphCluster {
  /** Stable cluster id; matches GraphNode.cluster_id. */
  id: string
  /** Human-readable label rendered in legends/tooltips. */
  label: string
  /** Number of nodes in this cluster — used for legend display. */
  size: number
  /** 0..7 = Okabe-Ito index; -1 = "other" (neutral grey). */
  color_index: number
}

/**
 * Generic graph node. Required fields are the minimum needed to render a
 * label, route to the entity, and apply cluster/isolate styling. Extra
 * fields (e.g. tooltip metadata) flow through `tooltipExtras` so callers
 * can surface their own signals without expanding the shared interface.
 */
export interface GraphNode {
  id: number
  name: string
  slug: string
  city?: string
  state?: string
  upcoming_show_count: number
  /** Matches GraphCluster.id. "other" or empty string when ungrouped. */
  cluster_id?: string
  /** True when the node has zero in-scope edges (post any filter). */
  is_isolate?: boolean
}

/**
 * Generic graph link. `type` is the PSY-362 edge-type grammar (shared_bills,
 * shared_label, etc.). `is_cross_cluster` lets the simulation pick a weaker
 * link strength for ties that span cluster boundaries. Score and detail are
 * carried through unchanged for the future tooltip enhancement.
 */
export interface GraphLink {
  source_id: number
  target_id: number
  type: string
  score?: number
  detail?: Record<string, unknown> | unknown
  is_cross_cluster?: boolean
}

// ──────────────────────────────────────────────
// Cluster palette — Okabe-Ito 8-color, colorblind-safe
// ──────────────────────────────────────────────
const OKABE_ITO_PALETTE = [
  '#0173B2', // blue
  '#DE8F05', // orange
  '#029E73', // green
  '#D55E00', // vermillion
  '#CC78BC', // pink
  '#CA9161', // brown
  '#56B4E9', // sky blue
  '#ECE133', // yellow
] as const

const OTHER_CLUSTER_COLOR = '#94A3B8' // slate-400 — neutral grey for ungrouped

export function clusterColor(colorIndex: number): string {
  if (colorIndex < 0 || colorIndex >= OKABE_ITO_PALETTE.length) {
    return OTHER_CLUSTER_COLOR
  }
  return OKABE_ITO_PALETTE[colorIndex]
}

// ──────────────────────────────────────────────
// Force-layout constants — tuned per PSY-368 spike §3
// ──────────────────────────────────────────────
const FORCE_X_STRENGTH = 0.15
const FORCE_Y_STRENGTH = 0.15
const LINK_STRENGTH_INTRA = 0.7
const LINK_STRENGTH_CROSS = 0.1
const CHARGE_STRENGTH = -120
const NODE_RADIUS = 8
const ISOLATE_RADIUS = 5

const HULL_FADE_START = 1.0
const HULL_FADE_END = 1.6
const HULL_FILL_ALPHA_MAX = 0.12

const OTHER_CLUSTER_ID = 'other'

// ──────────────────────────────────────────────
// Skeleton + dynamic-import boilerplate
// ──────────────────────────────────────────────
function GraphSkeleton() {
  return (
    <div
      className="flex items-center justify-center bg-muted/20 rounded-lg border border-border/50"
      style={{ height: 500 }}
    >
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

// computeCentroids arranges visible cluster centroids on a circle around
// origin; radius scales with the smaller viewport dimension so clusters
// stay inside the visible area regardless of aspect ratio.
function computeCentroids(
  clusterIDs: string[],
  containerWidth: number,
  graphHeight: number,
): Map<string, { x: number; y: number }> {
  const out = new Map<string, { x: number; y: number }>()
  if (clusterIDs.length === 0) return out
  const radius = Math.min(containerWidth, graphHeight) * 0.32
  const stepRad = (Math.PI * 2) / clusterIDs.length
  clusterIDs.forEach((id, i) => {
    const angle = i * stepRad - Math.PI / 2 // start at top
    out.set(id, {
      x: Math.cos(angle) * radius,
      y: Math.sin(angle) * radius,
    })
  })
  return out
}

// ──────────────────────────────────────────────
// Internal render shapes (post-filter, with d3-force runtime fields)
// ──────────────────────────────────────────────

interface RenderNode extends GraphNode {
  cluster_id: string // normalized — always populated
  is_isolate: boolean // normalized — always populated
  // d3-force runtime fields
  x?: number
  y?: number
  fx?: number | null
  fy?: number | null
}

interface RenderLink {
  source: number | RenderNode
  target: number | RenderNode
  type: string
  is_cross_cluster: boolean
}

// ──────────────────────────────────────────────
// Public component
// ──────────────────────────────────────────────

export interface ForceGraphViewProps {
  /** Graph nodes. The component normalizes `cluster_id` to "other" when missing. */
  nodes: GraphNode[]
  /** Graph links. `is_cross_cluster` defaults to false when omitted. */
  links: GraphLink[]
  /** Optional cluster definitions; pass an empty array to disable hulls + cluster forces. */
  clusters?: GraphCluster[]
  /** Container width measured by the parent — drives canvas width + isolate-shelf bounds. */
  containerWidth: number
  /** Optional explicit canvas height; defaults to 400px (<768) / 560px otherwise. */
  height?: number
  /**
   * Cluster IDs the parent has hidden via the legend. Hidden clusters'
   * nodes + edges are filtered out before layout.
   */
  hiddenClusterIDs?: Set<string>
  /** aria-label for the canvas (PSY-369). */
  ariaLabel: string
  /** Click handler — receives the underlying GraphNode the user clicked. */
  onNodeClick: (node: GraphNode) => void
}

export function ForceGraphView({
  nodes,
  links,
  clusters = [],
  containerWidth,
  height,
  hiddenClusterIDs,
  ariaLabel,
  onNodeClick,
}: ForceGraphViewProps) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const graphRef = useRef<any>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const reducedMotion = useReducedMotion()
  const [hoveredNode, setHoveredNode] = useState<RenderNode | null>(null)
  const [tooltipPos, setTooltipPos] = useState({ x: 0, y: 0 })

  const graphHeight = height ?? (containerWidth < 768 ? 400 : 560)

  const clustersByID = useMemo(() => {
    const map = new Map<string, GraphCluster>()
    for (const c of clusters) map.set(c.id, c)
    return map
  }, [clusters])

  // Filter out hidden clusters' nodes + any edges that touch them. Normalize
  // cluster_id and is_isolate so the rest of the component never has to
  // special-case the underspecified payload from venue-shape callers.
  const renderData = useMemo(() => {
    const nodeKept = new Set<number>()
    const renderNodes: RenderNode[] = []
    for (const n of nodes) {
      const clusterID = n.cluster_id || OTHER_CLUSTER_ID
      if (hiddenClusterIDs && hiddenClusterIDs.has(clusterID)) continue
      nodeKept.add(n.id)
      renderNodes.push({
        ...n,
        cluster_id: clusterID,
        is_isolate: n.is_isolate ?? false,
      })
    }
    const renderLinks: RenderLink[] = []
    for (const l of links) {
      if (!nodeKept.has(l.source_id) || !nodeKept.has(l.target_id)) continue
      renderLinks.push({
        source: l.source_id,
        target: l.target_id,
        type: l.type,
        is_cross_cluster: l.is_cross_cluster ?? false,
      })
    }
    return { nodes: renderNodes, links: renderLinks }
  }, [nodes, links, hiddenClusterIDs])

  // Cluster centroids steer non-isolate nodes via forceX/forceY; the isolate
  // shelf parks artists with no edges along the bottom margin.
  const centroids = useMemo(() => {
    const visibleClusterIDs = clusters
      .filter(c => !hiddenClusterIDs || !hiddenClusterIDs.has(c.id))
      .map(c => c.id)
    return computeCentroids(visibleClusterIDs, containerWidth, graphHeight)
  }, [clusters, hiddenClusterIDs, containerWidth, graphHeight])

  // Configure cluster-aware d3 forces. Re-runs whenever the graph data, the
  // viewport, or the visible cluster set changes — d3-force-3d's API allows
  // swapping individual forces in place via `d3Force(name, force)`.
  useEffect(() => {
    const fg = graphRef.current
    if (!fg) return

    // Pin isolates to a shelf along the bottom of the canvas.
    const isolates = renderData.nodes.filter(n => n.is_isolate)
    const shelfY = graphHeight * 0.42
    const shelfStart = -containerWidth * 0.4
    const shelfEnd = containerWidth * 0.4
    const stride =
      isolates.length > 1 ? (shelfEnd - shelfStart) / (isolates.length - 1) : 0
    isolates.forEach((node, i) => {
      node.fx = shelfStart + stride * i
      node.fy = shelfY
    })
    for (const node of renderData.nodes) {
      if (!node.is_isolate) {
        node.fx = null
        node.fy = null
      }
    }

    // Cluster centroid forces (only meaningful when at least one centroid exists).
    fg.d3Force('clusterX', (alpha: number) => {
      for (const node of renderData.nodes) {
        if (node.is_isolate || node.x == null) continue
        const c = centroids.get(node.cluster_id)
        if (!c) continue
        node.x += (c.x - node.x) * FORCE_X_STRENGTH * alpha
      }
    })
    fg.d3Force('clusterY', (alpha: number) => {
      for (const node of renderData.nodes) {
        if (node.is_isolate || node.y == null) continue
        const c = centroids.get(node.cluster_id)
        if (!c) continue
        node.y += (c.y - node.y) * FORCE_Y_STRENGTH * alpha
      }
    })

    const linkForce = fg.d3Force('link')
    if (linkForce && typeof linkForce.strength === 'function') {
      linkForce.strength((link: RenderLink) => {
        return link.is_cross_cluster ? LINK_STRENGTH_CROSS : LINK_STRENGTH_INTRA
      })
    }

    const chargeForce = fg.d3Force('charge')
    if (chargeForce && typeof chargeForce.strength === 'function') {
      chargeForce.strength(CHARGE_STRENGTH)
    }

    fg.d3ReheatSimulation()
  }, [renderData, centroids, containerWidth, graphHeight])

  // a11y: expose the canvas as an image with a descriptive label, mirroring
  // ArtistGraphVisualization (PSY-369).
  useEffect(() => {
    if (!containerRef.current) return
    const canvas = containerRef.current.querySelector('canvas')
    if (!canvas) return
    canvas.setAttribute('role', 'img')
    canvas.setAttribute('aria-label', ariaLabel)
  }, [ariaLabel])

  // Reduced-motion: pause the simulation immediately on mount.
  useEffect(() => {
    if (reducedMotion && graphRef.current) {
      graphRef.current.pauseAnimation()
    }
  }, [reducedMotion])

  const handleNodeClickInternal = useCallback(
    (node: RenderNode) => {
      onNodeClick(node)
    },
    [onNodeClick],
  )

  const handleNodeHover = useCallback(
    (node: RenderNode | null, event?: MouseEvent) => {
      setHoveredNode(node)
      if (node && event) {
        setTooltipPos({ x: event.clientX, y: event.clientY })
      }
    },
    [],
  )

  // Per-cluster fill from the Okabe-Ito palette with 70% alpha so cross-cluster
  // edges drawn on top remain visible.
  const nodeCanvasObject = useCallback(
    (node: RenderNode, ctx: CanvasRenderingContext2D, globalScale: number) => {
      const x = node.x ?? 0
      const y = node.y ?? 0
      const cluster = clustersByID.get(node.cluster_id)
      const fill = clusterColor(cluster?.color_index ?? -1)
      const radius = node.is_isolate ? ISOLATE_RADIUS : NODE_RADIUS

      ctx.beginPath()
      ctx.arc(x, y, radius, 0, Math.PI * 2)
      ctx.fillStyle = fill + 'B3' // ≈ 70% alpha
      ctx.fill()
      ctx.lineWidth = 1
      ctx.strokeStyle = node.is_isolate ? 'rgba(148, 163, 184, 0.5)' : fill
      ctx.stroke()

      if (node.upcoming_show_count > 0) {
        ctx.beginPath()
        ctx.arc(x + radius - 1.5, y - radius + 1.5, 2.5, 0, Math.PI * 2)
        ctx.fillStyle = '#22c55e'
        ctx.fill()
      }

      if (globalScale > 1.0) {
        const label = node.name.length > 22 ? node.name.slice(0, 20) + '…' : node.name
        const fontSize = Math.max(9, Math.min(13, 11 / globalScale))
        ctx.font = `${fontSize}px sans-serif`
        ctx.textAlign = 'center'
        ctx.textBaseline = 'top'
        ctx.fillStyle = 'rgba(228, 228, 231, 0.9)'
        ctx.fillText(label, x, y + radius + 3)
      }
    },
    [clustersByID],
  )

  // Convex hulls behind each cluster — drawn under the edges via
  // `linkCanvasObjectMode='before'`. polygonHull needs ≥3 points; for clusters
  // with fewer visible nodes we draw a simple disk to indicate region.
  const drawHulls = useCallback(
    (_link: RenderLink, ctx: CanvasRenderingContext2D, globalScale: number) => {
      // Only draw the hull layer once per render pass — gate on the first
      // link so we don't repaint it for every link.
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      if ((ctx as any).__forceGraphHullPainted) return
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(ctx as any).__forceGraphHullPainted = true

      if (globalScale >= HULL_FADE_END) return

      const fadeT = Math.max(
        0,
        Math.min(1, (HULL_FADE_END - globalScale) / (HULL_FADE_END - HULL_FADE_START)),
      )
      const alpha = HULL_FILL_ALPHA_MAX * fadeT

      // Group node positions by cluster (skip isolates and "other" — neither
      // wants a region indicator).
      const byCluster = new Map<string, [number, number][]>()
      for (const node of renderData.nodes) {
        if (node.is_isolate) continue
        if (node.cluster_id === OTHER_CLUSTER_ID) continue
        if (node.x == null || node.y == null) continue
        const points = byCluster.get(node.cluster_id) ?? []
        points.push([node.x, node.y])
        byCluster.set(node.cluster_id, points)
      }

      for (const [clusterID, points] of byCluster) {
        const cluster = clustersByID.get(clusterID)
        if (!cluster) continue
        const fill = clusterColor(cluster.color_index)

        if (points.length >= 3) {
          const hull = polygonHull(points)
          if (!hull) continue
          ctx.beginPath()
          ctx.moveTo(hull[0][0], hull[0][1])
          for (let i = 1; i < hull.length; i++) {
            ctx.lineTo(hull[i][0], hull[i][1])
          }
          ctx.closePath()
          ctx.fillStyle = fill + alphaToHex(alpha)
          ctx.fill()
        } else {
          const cx = points.reduce((s, p) => s + p[0], 0) / points.length
          const cy = points.reduce((s, p) => s + p[1], 0) / points.length
          ctx.beginPath()
          ctx.arc(cx, cy, 28, 0, Math.PI * 2)
          ctx.fillStyle = fill + alphaToHex(alpha)
          ctx.fill()
        }
      }
    },
    [renderData.nodes, clustersByID],
  )

  // Reset the per-frame hull-painted flag at the start of each render pass.
  const handleRenderFramePre = useCallback(
    (ctx: CanvasRenderingContext2D) => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(ctx as any).__forceGraphHullPainted = false
    },
    [],
  )

  return (
    <div
      ref={containerRef}
      className="relative rounded-lg border border-border/50 overflow-hidden bg-background"
    >
      <ForceGraph2D
        ref={graphRef}
        graphData={renderData}
        width={containerWidth}
        height={graphHeight}
        nodeId="id"
        nodeCanvasObject={nodeCanvasObject}
        nodePointerAreaPaint={(node: RenderNode, color: string, ctx: CanvasRenderingContext2D) => {
          ctx.beginPath()
          ctx.arc(node.x ?? 0, node.y ?? 0, node.is_isolate ? 8 : 11, 0, Math.PI * 2)
          ctx.fillStyle = color
          ctx.fill()
        }}
        onNodeClick={handleNodeClickInternal}
        onNodeHover={handleNodeHover}
        linkSource="source"
        linkTarget="target"
        linkColor={(link: RenderLink) =>
          link.is_cross_cluster
            ? 'rgba(148, 163, 184, 0.35)'
            : 'rgba(148, 163, 184, 0.6)'
        }
        linkWidth={(link: RenderLink) => (link.is_cross_cluster ? 0.6 : 1.1)}
        linkCanvasObjectMode={() => 'before'}
        linkCanvasObject={drawHulls}
        onRenderFramePre={handleRenderFramePre}
        cooldownTicks={200}
        d3AlphaDecay={0.04}
        d3VelocityDecay={0.3}
        minZoom={0.4}
        maxZoom={3}
        backgroundColor="transparent"
      />

      {hoveredNode && (
        <div
          className="fixed z-50 px-3 py-2 text-xs rounded-md bg-popover border border-border shadow-lg text-popover-foreground pointer-events-none"
          style={{ left: tooltipPos.x + 12, top: tooltipPos.y - 10 }}
        >
          <div className="font-medium text-sm">{hoveredNode.name}</div>
          {(hoveredNode.city || hoveredNode.state) && (
            <div className="text-muted-foreground">
              {[hoveredNode.city, hoveredNode.state].filter(Boolean).join(', ')}
            </div>
          )}
          {(() => {
            const cluster = clustersByID.get(hoveredNode.cluster_id)
            if (!cluster) return null
            return (
              <div className="text-muted-foreground mt-0.5">
                Cluster: <span className="text-foreground">{cluster.label}</span>
              </div>
            )
          })()}
          {hoveredNode.upcoming_show_count > 0 && (
            <div className="mt-1 text-green-400">
              {hoveredNode.upcoming_show_count} upcoming{' '}
              {hoveredNode.upcoming_show_count === 1 ? 'show' : 'shows'}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// alphaToHex converts a 0..1 alpha into the two-char hex pair appended to a
// 6-char hex color (e.g., "#0173B2" + alphaToHex(0.12) → "#0173B21F"). Inline
// to avoid pulling in a color util just for this.
export function alphaToHex(alpha: number): string {
  const clamped = Math.max(0, Math.min(1, alpha))
  return Math.round(clamped * 255)
    .toString(16)
    .padStart(2, '0')
}
