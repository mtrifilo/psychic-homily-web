'use client'

/**
 * SceneGraphVisualization (PSY-367)
 *
 * Cluster-aware force-directed canvas for the scene-scale artist graph.
 *
 * Built per the PSY-368 spike (`docs/features/scene-graph-layout.md`):
 * - Reuses `react-force-graph-2d` (already in the bundle from PSY-108)
 * - Adds cluster-aware d3-force config (`forceX`/`forceY` per node toward
 *   cluster centroids; weakened `forceLink` strength on between-cluster ties)
 * - Draws faint convex hulls behind clusters at zoom ≤ 1.0× for region
 *   readability, fading out as the user zooms in
 * - Cluster legend with click-to-toggle visibility (mirrors the per-artist
 *   edge-type filter chip pattern)
 * - Pins isolated nodes to a perimeter shelf so they don't drift to infinity
 *
 * Intentionally a sibling of `ArtistGraphVisualization`, not an extension:
 * scene-scale graphs have no center node, no vote affordances, different node
 * colors, and add hulls + a cluster legend, so prop-bagging the existing
 * component would push complexity costs onto the per-artist surface for no
 * reuse benefit (per Code Complete: keep cohesion strong, coupling loose).
 */

import { useCallback, useMemo, useRef, useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import dynamic from 'next/dynamic'
import { Loader2 } from 'lucide-react'
import { polygonHull } from 'd3-polygon'
import { useReducedMotion } from '@/features/artists/hooks/useReducedMotion'
import type { SceneGraphResponse, SceneGraphCluster } from '../types'

function GraphSkeleton() {
  return (
    <div
      className="flex items-center justify-center bg-muted/20 rounded-lg border border-border/50"
      style={{ height: 500 }}
    >
      <div className="flex flex-col items-center gap-2 text-muted-foreground">
        <Loader2 className="h-6 w-6 animate-spin" />
        <span className="text-sm">Loading scene graph...</span>
      </div>
    </div>
  )
}

const ForceGraph2D = dynamic(() => import('react-force-graph-2d'), {
  ssr: false,
  loading: () => <GraphSkeleton />,
})

// ──────────────────────────────────────────────
// Cluster palette (Okabe-Ito 8-color, colorblind-safe)
// Indices align with backend `color_index` (0..7); -1 = "other".
// Same palette as the colorblind audit in graph-colorblind-audit.md.
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

const OTHER_CLUSTER_COLOR = '#94A3B8' // slate-400 — neutral grey for "other"

function clusterColor(colorIndex: number): string {
  if (colorIndex < 0 || colorIndex >= OKABE_ITO_PALETTE.length) {
    return OTHER_CLUSTER_COLOR
  }
  return OKABE_ITO_PALETTE[colorIndex]
}

// ──────────────────────────────────────────────
// Layout constants — tuned to PSY-368 spike §3 parameter ranges, refined for
// Phoenix's actual scale (108 nodes / ~76 in-scope edges). Centroid ring radius
// scales with cluster count so 2 clusters stay close while 8 clusters spread.
// ──────────────────────────────────────────────
const FORCE_X_STRENGTH = 0.15
const FORCE_Y_STRENGTH = 0.15
const LINK_STRENGTH_INTRA = 0.7
const LINK_STRENGTH_CROSS = 0.1
const CHARGE_STRENGTH = -120
const NODE_RADIUS = 8
const ISOLATE_RADIUS = 5

// Hulls fade out as the user zooms in: at ≥ HULL_FADE_END they're invisible,
// at ≤ HULL_FADE_START they're full alpha (12%, see spike §5.2).
const HULL_FADE_START = 1.0
const HULL_FADE_END = 1.6
const HULL_FILL_ALPHA_MAX = 0.12 // 12% per spike doc

// Centroid layout: clusters arranged on a circle around origin; "other" sits
// at the perimeter so its color noise doesn't compete with first-class clusters.
function computeCentroids(
  clusterIDs: string[],
  containerWidth: number,
  graphHeight: number,
): Map<string, { x: number; y: number }> {
  const out = new Map<string, { x: number; y: number }>()
  if (clusterIDs.length === 0) return out
  // Radius scales with the smaller viewport dimension so clusters stay inside
  // the visible area regardless of aspect ratio.
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

interface RenderNode {
  id: number
  name: string
  slug: string
  city?: string
  state?: string
  upcoming_show_count: number
  cluster_id: string
  is_isolate: boolean
  // d3-force runtime fields (populated post-mount)
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

interface SceneGraphVisualizationProps {
  data: SceneGraphResponse
  containerWidth: number
  /**
   * IDs of clusters the user has hidden via the legend. Hidden clusters'
   * nodes + edges are removed from the canvas; "other" cluster always stays
   * visible (toggling it would hide the long tail without a way back).
   */
  hiddenClusterIDs: Set<string>
}

export function SceneGraphVisualization({
  data,
  containerWidth,
  hiddenClusterIDs,
}: SceneGraphVisualizationProps) {
  const router = useRouter()
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const graphRef = useRef<any>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const reducedMotion = useReducedMotion()
  const [hoveredNode, setHoveredNode] = useState<RenderNode | null>(null)
  const [tooltipPos, setTooltipPos] = useState({ x: 0, y: 0 })

  const graphHeight = containerWidth < 768 ? 400 : 560

  // Cluster lookup + ordered cluster IDs (for centroid placement). Order is
  // backend-provided (size desc) so colors stay stable across renders.
  const clustersByID = useMemo(() => {
    const map = new Map<string, SceneGraphCluster>()
    for (const c of data.clusters) map.set(c.id, c)
    return map
  }, [data.clusters])

  // Filter out hidden clusters' nodes + any edges that touch them.
  const renderData = useMemo(() => {
    const nodeKept = new Set<number>()
    const nodes: RenderNode[] = []
    for (const n of data.nodes) {
      if (hiddenClusterIDs.has(n.cluster_id)) continue
      nodeKept.add(n.id)
      nodes.push({ ...n })
    }
    const links: RenderLink[] = []
    for (const l of data.links) {
      if (!nodeKept.has(l.source_id) || !nodeKept.has(l.target_id)) continue
      links.push({
        source: l.source_id,
        target: l.target_id,
        type: l.type,
        is_cross_cluster: l.is_cross_cluster,
      })
    }
    return { nodes, links }
  }, [data, hiddenClusterIDs])

  // Cluster centroids + isolate-shelf positions, recomputed when the graph
  // dimensions or cluster set change. Centroids steer non-isolate nodes via
  // forceX/forceY; the isolate shelf parks artists with no edges along the
  // bottom margin (spike §3.5 "perimeter shelf" recommendation, simplified).
  const centroids = useMemo(() => {
    const visibleClusterIDs = data.clusters
      .filter(c => !hiddenClusterIDs.has(c.id))
      .map(c => c.id)
    return computeCentroids(visibleClusterIDs, containerWidth, graphHeight)
  }, [data.clusters, hiddenClusterIDs, containerWidth, graphHeight])

  // Configure cluster-aware d3 forces. Re-runs whenever the graph data, the
  // viewport, or the visible cluster set changes — d3-force-3d's API allows
  // swapping individual forces in place via `d3Force(name, force)`.
  useEffect(() => {
    const fg = graphRef.current
    if (!fg) return

    // Pin isolates to a shelf along the bottom of the canvas. Set fx/fy
    // directly on the node so d3-force respects the position.
    const isolates = renderData.nodes.filter(n => n.is_isolate)
    const shelfY = graphHeight * 0.42
    const shelfStart = -containerWidth * 0.4
    const shelfEnd = containerWidth * 0.4
    const stride = isolates.length > 1 ? (shelfEnd - shelfStart) / (isolates.length - 1) : 0
    isolates.forEach((node, i) => {
      node.fx = shelfStart + stride * i
      node.fy = shelfY
    })
    // Non-isolates get free placement.
    for (const node of renderData.nodes) {
      if (!node.is_isolate) {
        node.fx = null
        node.fy = null
      }
    }

    // Cluster centroid forces — each non-isolate node pulled toward its cluster's
    // target position. Isolates are pinned, so the force has no effect on them.
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

    // Link force: stronger within-cluster, weaker between-cluster, so clusters
    // stay visually distinct when many cross-cluster ties exist.
    const linkForce = fg.d3Force('link')
    if (linkForce && typeof linkForce.strength === 'function') {
      linkForce.strength((link: RenderLink) => {
        return link.is_cross_cluster ? LINK_STRENGTH_CROSS : LINK_STRENGTH_INTRA
      })
    }

    // Charge: a touch more spread than the per-artist default to prevent
    // node overlap in dense clusters.
    const chargeForce = fg.d3Force('charge')
    if (chargeForce && typeof chargeForce.strength === 'function') {
      chargeForce.strength(CHARGE_STRENGTH)
    }

    // Reheat the simulation so the new forces actually take effect.
    fg.d3ReheatSimulation()
  }, [renderData, centroids, containerWidth, graphHeight])

  // a11y: expose the canvas as an image with a descriptive label, mirroring
  // ArtistGraphVisualization (PSY-369). The list view above is the
  // keyboard/screen-reader counterpart.
  useEffect(() => {
    if (!containerRef.current) return
    const canvas = containerRef.current.querySelector('canvas')
    if (!canvas) return
    canvas.setAttribute('role', 'img')
    canvas.setAttribute(
      'aria-label',
      `Scene relationship graph for ${data.scene.city}, ${data.scene.state}: ${data.scene.artist_count} artists, ${data.scene.edge_count} connections.`,
    )
  }, [data.scene.city, data.scene.state, data.scene.artist_count, data.scene.edge_count])

  // Reduced-motion: pause the simulation immediately on mount so motion-
  // sensitive users get a static layout (matches the per-artist graph).
  useEffect(() => {
    if (reducedMotion && graphRef.current) {
      graphRef.current.pauseAnimation()
    }
  }, [reducedMotion])

  const handleNodeClick = useCallback(
    (node: RenderNode) => {
      router.push(`/artists/${node.slug}`)
    },
    [router],
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
      ctx.fillStyle = fill + 'B3' // 0xB3 ≈ 70% alpha
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

      // Show artist name when zoomed in enough — avoids label collision at low zoom
      // where clusters' shapes do the talking.
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
      if ((ctx as any).__sceneHullPainted) return
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(ctx as any).__sceneHullPainted = true

      if (globalScale >= HULL_FADE_END) return // faded out entirely

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
        if (node.cluster_id === 'other') continue
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
          // 1–2 points: draw a tinted disk centered on the (or midpoint of) point(s).
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
  // react-force-graph-2d calls onRenderFramePre once per animation frame.
  const handleRenderFramePre = useCallback(
    (ctx: CanvasRenderingContext2D) => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(ctx as any).__sceneHullPainted = false
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
        onNodeClick={handleNodeClick}
        onNodeHover={handleNodeHover}
        linkSource="source"
        linkTarget="target"
        linkColor={(link: RenderLink) =>
          link.is_cross_cluster ? 'rgba(148, 163, 184, 0.35)' : 'rgba(148, 163, 184, 0.6)'
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
function alphaToHex(alpha: number): string {
  const clamped = Math.max(0, Math.min(1, alpha))
  return Math.round(clamped * 255)
    .toString(16)
    .padStart(2, '0')
}
