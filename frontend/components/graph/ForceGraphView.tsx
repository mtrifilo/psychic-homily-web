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
 *
 * PSY-1083 — typed-edge grammar + interactive legend:
 *   - links that carry a `type` render with the shared edge grammar
 *     (per-type color, dash pattern, magnitude-scaled width, hover
 *     tooltip — see ./edgeGrammar.ts); untyped links keep the original
 *     monochrome styling, so payloads without types regress nothing
 *   - `showEdgeLegend` opts a surface into the interactive EdgeLegend
 *     overlay (per-type counts + show/hide toggles that filter the
 *     simulation)
 *   - cluster fills resolve from the `--chart-1..8` theme tokens instead
 *     of the hardcoded Okabe-Ito set (PSY-1079 spike: Okabe-Ito yellow is
 *     1.21:1 on the newsprint light bg)
 */

import { useCallback, useMemo, useRef, useEffect, useState, type ComponentType, type MutableRefObject } from 'react'
import dynamic from 'next/dynamic'
import { Loader2 } from 'lucide-react'
import { polygonHull } from 'd3-polygon'
import type { ForceGraphMethods, ForceGraphProps } from 'react-force-graph-2d'
import { useReducedMotion } from '@/features/artists/hooks/useReducedMotion'
import { buildLinkLabel, edgeLineDash, edgeWidth } from './edgeGrammar'
import { clusterColor, useGraphPalette, withHexAlpha } from './graphPalette'
import { degreeMap, renderGraphLabels, type GraphLabelSpec } from './graphLabels'
import { buildAdjacency, endpointId, focusForeground, BACKGROUND_ALPHA, BACKGROUND_ALPHA_HEX } from './graphFocus'
import { nodeTooltipPlacement, tooltipPlacementStyle, type TooltipPlacement } from './nodeTooltip'
import { EdgeLegend } from './EdgeLegend'
import { ConnectionPanel } from './ConnectionPanel'
import { aggregatePairConnections, useConnectionInspect } from './useConnectionInspect'

// ──────────────────────────────────────────────
// Public types — the generic graph payload shape
// ──────────────────────────────────────────────

/**
 * Cluster definition. Fills come from the `--chart-1..8` theme tokens
 * indexed by `colorIndex` (0..7); -1 = "other" / ungrouped (rendered in
 * neutral grey). Callers that don't compute clusters can omit the array
 * entirely.
 */
export interface GraphCluster {
  /** Stable cluster id; matches GraphNode.cluster_id. */
  id: string
  /** Human-readable label rendered in legends/tooltips. */
  label: string
  /** Number of nodes in this cluster — used for legend display. */
  size: number
  /** 0..7 = `--chart-{n+1}` token index; -1 = "other" (neutral grey). */
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
  /** Community vote counts (artist-endpoint payloads carry these; the
   * 'similar' tooltip surfaces them when present). */
  votes_up?: number
  votes_down?: number
  detail?: Record<string, unknown> | unknown
  is_cross_cluster?: boolean
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

// zoomToFit pads the NODE bbox — labels aren't measured, so shelf-END labels
// can clip at the canvas edge. Deliberately small: a pad wide enough for the
// labels (~64px) drops the fitted zoom below the 1.0 label gate (PSY-1209)
// on typical overlay layouts, hiding EVERY label — clipped edge labels beat
// none (PSY-1321; both measured on the seeded Phoenix fixture).
const ZOOM_FIT_PADDING_PX = 40

const HULL_FADE_START = 1.0
const HULL_FADE_END = 1.6
const HULL_FILL_ALPHA_MAX = 0.12

const OTHER_CLUSTER_ID = 'other'

// A stable empty-clusters reference for the `clusters` default param. An omitted
// (or inline `[]`) prop would otherwise hand a fresh array each render, giving the
// `centroids` useMemo — and the reheat + tooltip-dismiss effects keyed on it — a new
// identity every render, so a stray re-render mid-hover would needlessly reheat the
// sim and dismiss an open tooltip (PSY-1217 review).
const EMPTY_CLUSTERS: GraphCluster[] = []

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

// `next/dynamic` strips the upstream component's generic parameters (see
// react-force-graph-2d's `FCwithRef = <NodeType, LinkType>(...)`), so under
// `strictFunctionTypes` the callback props default to the library's loose
// `NodeObject<{}>` / `LinkObject<{}>` and our richer RenderNode/RenderLink
// signatures fail variance checks. Pin the generics with an as-unknown-as
// cast — runtime behaviour is unchanged.
const ForceGraph2D = dynamic(() => import('react-force-graph-2d'), {
  ssr: false,
  loading: () => <GraphSkeleton />,
}) as unknown as ComponentType<
  ForceGraphProps<RenderNode, RenderLink> & {
    ref?: MutableRefObject<ForceGraphMethods<RenderNode, RenderLink> | undefined>
  }
>

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
  // Carried through for the typed-edge grammar: magnitude-scaled width
  // (edgeWidth) and the hover tooltip (buildLinkLabel).
  score?: number
  votes_up?: number
  votes_down?: number
  detail?: Record<string, unknown>
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
  /**
   * PSY-1083: render the interactive edge legend (per-type swatch, count,
   * and show/hide toggle) over the canvas. Off by default so existing
   * consumers opt in deliberately.
   */
  showEdgeLegend?: boolean
  /**
   * PSY-1334: click-to-inspect connection panel — clicking a typed edge
   * opens a card listing every connection between that artist pair (the
   * touch-capable, linkable counterpart to the hover tooltip). Off by
   * default, same opt-in convention as showEdgeLegend.
   */
  showConnectionPanel?: boolean
}

export function ForceGraphView({
  nodes,
  links,
  clusters = EMPTY_CLUSTERS,
  containerWidth,
  height,
  hiddenClusterIDs,
  ariaLabel,
  onNodeClick,
  showEdgeLegend = false,
  showConnectionPanel = false,
}: ForceGraphViewProps) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const graphRef = useRef<any>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const reducedMotion = useReducedMotion()
  const palette = useGraphPalette()
  const [hoveredNode, setHoveredNode] = useState<RenderNode | null>(null)
  // Edge types the user has hidden via the legend toggles (PSY-1083).
  // Purely presentational, so the component owns it — parents opt in via
  // `showEdgeLegend` without threading filter state.
  const [hiddenEdgeTypes, setHiddenEdgeTypes] = useState<ReadonlySet<string>>(
    () => new Set<string>(),
  )
  // PSY-1334: soloed edge type (legend "only" affordance). Solo WINS over the
  // hidden set while active; the hidden set is never mutated by soloing, so
  // leaving solo restores exactly the hide state the user had before.
  const [soloEdgeType, setSoloEdgeType] = useState<string | null>(null)
  // PSY-1334: which artist pair the ConnectionPanel is inspecting.
  const connectionInspect = useConnectionInspect()
  // Tooltip anchor in CONTAINER coords (the tooltip is position:absolute within
  // the relative container). Set from the hovered node's screen position in
  // handleNodeHover via the shared nodeTooltipPlacement helper; flipX/flipY steer
  // it toward the container interior near the right/bottom edges (PSY-1217).
  const [tooltipPos, setTooltipPos] = useState<TooltipPlacement>({ x: 0, y: 0, flipX: false, flipY: false })

  const graphHeight = height ?? (containerWidth < 768 ? 400 : 560)

  const clustersByID = useMemo(() => {
    const map = new Map<string, GraphCluster>()
    for (const c of clusters) map.set(c.id, c)
    return map
  }, [clusters])

  // Filter out hidden clusters' nodes + any edges that touch them. Normalize
  // cluster_id and is_isolate so the rest of the component never has to
  // special-case the underspecified payload from venue-shape callers.
  //
  // PSY-1083: the same pass also (a) counts links per edge type for the
  // legend — AFTER the cluster filter (so counts match what's displayable)
  // but BEFORE the edge-type filter (so a hidden type still shows the count
  // you'd get back by re-enabling it) — and (b) drops links of hidden edge
  // types from the simulation. Nodes are NOT dropped when their last edge
  // hides: scope membership is node-level information, edge toggles are
  // edge-level. (The artist graph differs deliberately — it prunes
  // unconnected satellites because its nodes only exist as endpoints.)
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
    const edgeTypeCounts = new Map<string, number>()
    for (const l of links) {
      if (!nodeKept.has(l.source_id) || !nodeKept.has(l.target_id)) continue
      if (l.type) {
        edgeTypeCounts.set(l.type, (edgeTypeCounts.get(l.type) ?? 0) + 1)
        // Solo wins over hidden (PSY-1334): while a type is soloed, only it
        // renders; the hidden set stays intact underneath for when solo ends.
        if (soloEdgeType ? l.type !== soloEdgeType : hiddenEdgeTypes.has(l.type)) continue
      }
      renderLinks.push({
        source: l.source_id,
        target: l.target_id,
        type: l.type,
        is_cross_cluster: l.is_cross_cluster ?? false,
        score: l.score,
        votes_up: l.votes_up,
        votes_down: l.votes_down,
        // `detail` is typed loosely at the payload boundary; the tooltip
        // builder defends against non-object shapes field-by-field.
        detail: l.detail as Record<string, unknown> | undefined,
      })
    }
    return { nodes: renderNodes, links: renderLinks, edgeTypeCounts }
  }, [nodes, links, hiddenClusterIDs, hiddenEdgeTypes, soloEdgeType])

  // Cluster centroids steer non-isolate nodes via forceX/forceY; the isolate
  // shelf parks artists with no edges along the bottom margin.
  const centroids = useMemo(() => {
    const visibleClusterIDs = clusters
      .filter(c => !hiddenClusterIDs || !hiddenClusterIDs.has(c.id))
      .map(c => c.id)
    return computeCentroids(visibleClusterIDs, containerWidth, graphHeight)
  }, [clusters, hiddenClusterIDs, containerWidth, graphHeight])

  // ForceGraphView explicitly reheats the simulation (d3ReheatSimulation in the
  // effect below) whenever the data, edge-type/cluster filter, or viewport changes
  // — which pans the nodes and would strand an already-open tooltip at its now-stale
  // screen position over empty canvas or an unrelated node. Dismiss it on those same
  // changes (identical deps to the reheat effect) so it re-anchors on the next hover.
  // This is the counterpart to ArtistGraph's reset-on-recenter — both dismiss when the
  // layout shifts under an open tooltip, but each keys on its OWN surface's layout
  // signal (ForceGraphView's in-canvas EdgeLegend + viewport here; ArtistGraph's
  // click-to-recenter there), so the triggers are deliberately NOT identical. Surfaced
  // by the PSY-1217 code-review.
  useEffect(() => {
    setHoveredNode(null)
  }, [renderData, centroids, containerWidth, graphHeight])

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
    // d3-force-3d steers the layout by mutating the node datums in place:
    // `fx`/`fy` pin/unpin coordinates, `x`/`y` are the live positions it reads
    // and writes each tick. The graph holds these exact object instances, so we
    // must mutate them here rather than produce new objects — cloning would
    // detach our edits from the running simulation. `renderData` is a useMemo
    // result, which the immutability rule treats as frozen; this mutation is
    // the documented d3 contract, not an accidental write (RenderNode's `x/y/
    // fx/fy` are explicitly typed as runtime fields). Suppressed for this span
    // — note the analyzer's reach shifts with unrelated edits to this
    // component (it stopped flagging these lines mid-PSY-1321, then resumed),
    // so if the directive ever reports as unused, prefer leaving it in place.
    /* eslint-disable react-hooks/immutability */
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
    /* eslint-enable react-hooks/immutability */

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
  // ArtistGraphVisualization (PSY-369). The canvas is created asynchronously
  // by the dynamic-imported force-graph chunk, usually AFTER this effect first
  // runs — a bare querySelector missed it and the label never applied (caught
  // live during PSY-1296 verification). Apply now if present, otherwise watch
  // the container until the canvas appears.
  // canvasReady doubles as the "dynamic chunk has mounted" signal for the
  // reduced-motion fit below: graphRef assignment doesn't trigger effects, so
  // without a state flip the fit effect could run once against a null ref and
  // never retry (adversarial finding — cold-chunk mounts).
  const [canvasReady, setCanvasReady] = useState(false)
  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    const apply = () => {
      const canvas = container.querySelector('canvas')
      if (!canvas) return false
      canvas.setAttribute('role', 'img')
      canvas.setAttribute('aria-label', ariaLabel)
      setCanvasReady(true)
      return true
    }
    if (apply()) return
    const observer = new MutationObserver(() => {
      if (apply()) observer.disconnect()
    })
    observer.observe(container, { childList: true, subtree: true })
    return () => observer.disconnect()
  }, [ariaLabel])

  // PSY-1321: frame the graph once the layout settles — but ONLY when the
  // settled content is out of view. On a fresh mount — the fullscreen-overlay
  // case — the default viewport centers on the coordinate origin while the
  // connected mass settles wherever the centroid ring + charge push it and
  // the isolate shelf pins to the bottom margin, so the initial overlay view
  // could show ONLY the shelf (verified on stage, minneapolis-mn).
  //
  // Contract (each clause pins a code-review finding):
  //   - The shot stays ARMED until there is content + a bbox to frame — an
  //     engine stop over an empty/loading graph must not burn it.
  //   - Content already fully in view → spend the shot WITHOUT fitting, so
  //     inline mounts that frame fine today are byte-identical (and keep
  //     their labels — a full zoomToFit can land below the 1.0 label gate).
  //   - A canvas pointerdown/wheel means the user owns the viewport for the
  //     REST OF THE MOUNT: dimension changes re-arm the shot only until then.
  //     (Canvas-targeted, so EdgeLegend/tooltip clicks don't cancel.)
  //   - The fit pans the canvas under an open tooltip → dismiss it first.
  const needsFitRef = useRef(true)
  const userOwnsViewportRef = useRef(false)
  useEffect(() => {
    // Mount run redundantly re-arms an already-armed ref; every later run IS
    // a dimension change (any-pixel, deliberately not "material": the in-view
    // spend below absorbs trivial resizes, so a threshold would only add a
    // tuning knob). reducedMotion is a re-arm signal too — flipping it OFF
    // resumes the sim and re-settles the layout somewhere new, so a shot
    // spent on the paused snapshot must not stay spent (adversarial finding).
    if (!userOwnsViewportRef.current) {
      needsFitRef.current = true
    }
  }, [containerWidth, graphHeight, reducedMotion])

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const cancelFit = (e: Event) => {
      if (!(e.target instanceof Element) || !e.target.closest('canvas')) return
      userOwnsViewportRef.current = true
      needsFitRef.current = false
    }
    el.addEventListener('pointerdown', cancelFit)
    el.addEventListener('wheel', cancelFit, { passive: true })
    return () => {
      el.removeEventListener('pointerdown', cancelFit)
      el.removeEventListener('wheel', cancelFit)
    }
  }, [])

  const maybeFitViewport = useCallback(
    (animated: boolean) => {
      if (!needsFitRef.current) return
      const fg = graphRef.current
      if (!fg || renderData.nodes.length === 0) return // stay armed
      // Uninitialized positions don't yield a null bbox — force-graph returns
      // {x:[undefined,undefined],...} (d3 min/max over undefined), which would
      // sail into centerAt(NaN, NaN) and corrupt the d3-zoom transform for the
      // rest of the mount (adversarial finding). Validate numerically.
      const bbox = fg.getGraphBbox?.()
      if (
        !bbox ||
        !Number.isFinite(bbox.x[0]) ||
        !Number.isFinite(bbox.x[1]) ||
        !Number.isFinite(bbox.y[0]) ||
        !Number.isFinite(bbox.y[1])
      ) {
        return // positions not initialized yet — stay armed
      }
      needsFitRef.current = false
      const zoom = fg.zoom()
      const center = fg.centerAt()
      const halfW = containerWidth / zoom / 2
      const halfH = graphHeight / zoom / 2
      // 5% per-side slack: a bbox that pokes marginally past the viewport
      // (edge node half-clipped) still counts as in view — a full 400ms
      // zoomToFit for a few clipped pixels is a worse trade than the clip,
      // and on inline mounts the fit could drop below the 1.0 label gate.
      const slackX = halfW * 0.05
      const slackY = halfH * 0.05
      const inView =
        bbox.x[0] >= center.x - halfW - slackX &&
        bbox.x[1] <= center.x + halfW + slackX &&
        bbox.y[0] >= center.y - halfH - slackY &&
        bbox.y[1] <= center.y + halfH + slackY
      if (inView) return
      // The fit pans/zooms under an open tooltip and onEngineTick re-anchoring
      // has already stopped — dismiss like onZoom/onNodeDrag do.
      setHoveredNode(null)
      fg.zoomToFit(animated ? 400 : 0, ZOOM_FIT_PADDING_PX)
    },
    [renderData, containerWidth, graphHeight],
  )

  const handleEngineStop = useCallback(
    () => maybeFitViewport(!reducedMotion),
    [maybeFitViewport, reducedMotion],
  )

  // Reduced-motion: pause the simulation immediately on mount. A paused
  // engine never reaches onEngineStop, so the fit for that path runs from
  // the effect below instead (instant, over unsettled positions — an
  // approximate frame still beats an off-view one). Keyed on the same
  // signals that re-arm the shot, so async-arriving data and dimension
  // changes are covered symmetrically with the animated path.
  useEffect(() => {
    if (reducedMotion && graphRef.current) {
      graphRef.current.pauseAnimation()
    }
  }, [reducedMotion])

  // canvasReady is a dep so a cold chunk load (graphRef null on the first
  // run) retries once the dynamic component actually mounts.
  useEffect(() => {
    if (reducedMotion && canvasReady) {
      maybeFitViewport(false)
    }
  }, [reducedMotion, maybeFitViewport, canvasReady])

  // PSY-1235: restart force-graph's rAF render loop on (re-)mount. On a fresh page load
  // react-force-graph starts its own loop, but after a client back-navigation re-mount the
  // loop comes back DEAD (measured 0 fps vs ~158 fps fresh), which freezes the canvas and
  // every interaction it drives — hover, click, pan, zoom — until a full refresh.
  // resumeAnimation restarts it. Gated on !reducedMotion (PSY-1226) so it does NOT defeat the
  // pause above: for those users the static snapshot is intended (the accessible path is the
  // caller's list view) and a paused loop has no interaction to lose anyway. The effect re-runs
  // on every mount, so a re-mounted graph is revived; it also fires if reducedMotion flips off.
  useEffect(() => {
    if (!reducedMotion) {
      graphRef.current?.resumeAnimation()
    }
  }, [reducedMotion])

  const handleNodeClickInternal = useCallback(
    (node: RenderNode) => {
      // Node click closes an open inspect panel (PSY-1334) — the click either
      // navigates away or shifts attention to the node; a stale pair panel
      // would linger over the new context either way.
      connectionInspect.close()
      onNodeClick(node)
    },
    [onNodeClick, connectionInspect],
  )

  // PSY-1334: edge click opens the ConnectionPanel for that pair. d3-force
  // resolves source/target to node objects in place after mount, so read the
  // ids via endpointId (handles both the bare-id and resolved shapes).
  const handleLinkClick = useCallback(
    (link: RenderLink) => {
      if (!showConnectionPanel || !link.type) return
      connectionInspect.open(endpointId(link.source), endpointId(link.target))
    },
    [showConnectionPanel, connectionInspect],
  )

  // Panel data derives from the RAW props, not renderData: the panel lists
  // ALL typed connections between the pair — including types currently
  // hidden/soloed out of the simulation ("why connected" is about the data,
  // not the current filter view). Resolves to null (panel closed) when the
  // pair's nodes leave the payload — a data refresh that drops a node must
  // not strand a panel naming it.
  const connectionPanelData = useMemo(() => {
    const pair = connectionInspect.pair
    if (!showConnectionPanel || !pair) return null
    const source = nodes.find(n => n.id === pair.sourceId)
    const target = nodes.find(n => n.id === pair.targetId)
    if (!source || !target) return null
    const connections = aggregatePairConnections(links, pair)
    if (connections.length === 0) return null
    return {
      source: { name: source.name, slug: source.slug },
      target: { name: target.name, slug: target.slug },
      connections,
    }
  }, [showConnectionPanel, connectionInspect.pair, nodes, links])

  // react-force-graph-2d invokes `onNodeHover` with `(node, previousNode)` and no
  // MouseEvent, so anchor the tooltip on the NODE via the shared
  // nodeTooltipPlacement helper — see its doc-comment for the graph2ScreenCoords +
  // position:absolute rationale (PSY-1217; previously the tooltip was pinned at
  // origin / top-left). Set hoveredNode ONLY when a placement is returned so
  // position and node never desync; a null placement (hover-out, or a node without
  // settled coords) hides the tooltip rather than stranding it at a stale position.
  const handleNodeHover = useCallback((node: RenderNode | null) => {
    const placement = nodeTooltipPlacement(graphRef.current, containerRef.current, node)
    if (placement) {
      setTooltipPos(placement)
      setHoveredNode(node)
    } else {
      setHoveredNode(null)
    }
  }, [])

  // PSY-1220: while the d3-force sim is live (onEngineTick), re-anchor the open tooltip to the
  // hovered node's CURRENT position via the shared placement helper. The node drifts during the
  // settle/reheat but onNodeHover only re-fires when the object UNDER the cursor changes, so a
  // stationary cursor over a drifting node would strand the anchored tooltip over empty canvas.
  // onEngineTick stops firing once the sim cools, so this costs nothing at rest; the `hoveredNode`
  // guard makes it a no-op when nothing is hovered. A node that lost its settled coords (filtered
  // out) yields null → dismiss. (ForceGraphView's tooltip is pointer-events-none, so unlike
  // ArtistGraph there's no over-the-tooltip case to guard against re-anchoring out from under.)
  const handleEngineTick = useCallback(() => {
    if (!hoveredNode) return
    const placement = nodeTooltipPlacement(graphRef.current, containerRef.current, hoveredNode)
    if (placement) setTooltipPos(placement)
    else setHoveredNode(null)
  }, [hoveredNode])

  // Hover-focus (PSY-1225): when a node is hovered, IT + its 1-hop neighbors (and the
  // links between two foreground nodes) stay foreground; every other node/link/label fades
  // to the background. Ported from ArtistGraph (PSY-1210) via the shared graphFocus helper
  // so the neighborhood math + fade alpha can't drift between the two canvas surfaces.
  // Unlike the artist graph there is NO single center node, so this is purely hover-driven
  // (no alwaysInclude anchor) — the PSY-1225 "no center" open question, resolved per lean.
  //
  // The [renderData] dep on adjacency is load-bearing: the memo captures the freshly-built
  // BARE-id links (renderData maps source/target to numeric ids) before d3-force mutates
  // source/target into resolved objects in place. (buildAdjacency accepts either shape, but
  // only bare ids occur here; the resolved {id} shape only appears later, in the per-frame
  // linkColor lookups, also via endpointId.)
  const adjacency = useMemo(() => buildAdjacency(renderData.links), [renderData])
  const focusedIds = useMemo(() => {
    if (hoveredNode == null) return null
    // Drop focus if the hovered node was filtered out (a hidden cluster / edge-type toggle):
    // a stale hover would otherwise match nothing visible and dim the WHOLE graph to the
    // background alpha. The reset-on-renderData effect above also clears hoveredNode, but
    // only AFTER render — this guards the interim render where renderData changed and that
    // effect hasn't run yet (ArtistGraph resets hover DURING render, so it has no such gap).
    if (!renderData.nodes.some(n => n.id === hoveredNode.id)) return null
    return focusForeground(adjacency, hoveredNode.id)
  }, [adjacency, hoveredNode, renderData])

  // Per-cluster fill from the theme `--chart` tokens with 70% alpha so
  // cross-cluster edges drawn on top remain visible.
  const nodeCanvasObject = useCallback(
    (node: RenderNode, ctx: CanvasRenderingContext2D) => {
      const x = node.x ?? 0
      const y = node.y ?? 0
      const cluster = clustersByID.get(node.cluster_id)
      const fill = clusterColor(palette, cluster?.color_index ?? -1)
      const radius = node.is_isolate ? ISOLATE_RADIUS : NODE_RADIUS

      // Hover-focus (PSY-1225): dim nodes outside the foreground set. globalAlpha multiplies
      // every fill/stroke below (incl. the show indicator); reset to 1 at the end so the next
      // node + the post-frame labels render at full opacity. Snap (no fade animation). Because
      // this callback reads focusedIds, force-graph repaints on hover via onChange→notifyRedraw
      // (the rAF loop runs continuously for non-reduced-motion users), so NO resumeAnimation is
      // needed — and not adding one keeps the reduced-motion pause intact here, avoiding the
      // ArtistGraph/PSY-1226 pause-defeat. (Reduced-motion users get the static snapshot with no
      // hover-focus, consistent with the hover tooltip also being inert while the loop is paused.)
      ctx.globalAlpha = focusedIds != null && !focusedIds.has(node.id) ? BACKGROUND_ALPHA : 1

      ctx.beginPath()
      ctx.arc(x, y, radius, 0, Math.PI * 2)
      ctx.fillStyle = withHexAlpha(fill, 'B3') // ≈ 70% alpha
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

      ctx.globalAlpha = 1 // reset so the next node / post-frame labels aren't dimmed
      // Labels are NOT drawn here — they're rendered in a single collision-culled
      // post-frame pass (nodeLabelsFrame) so overlapping labels can be dropped
      // across all nodes at once (PSY-1209).
    },
    [clustersByID, palette, focusedIds],
  )

  // Degree (link count) per node id → which label wins a collision; isolates
  // (degree 0) lose first. Shared with ArtistGraph via degreeMap so the two
  // surfaces can't drift.
  const degreeById = useMemo(() => degreeMap(renderData.links), [renderData])

  // Node labels in one collision-culled post-frame pass (PSY-1209). Labels are
  // kept in degree order and dropped when they'd overlap a higher-priority one;
  // a culled node's name is still reachable via the hover tooltip below (reveal-
  // on-hover in the canvas is PSY-1210). Same gate (globalScale > 1.0), font,
  // truncation, and y-offset the per-node paint used; the theme-aware halo+fill
  // recipe lives in renderGraphLabels (shared with ArtistGraph).
  const nodeLabelsFrame = useCallback(
    (ctx: CanvasRenderingContext2D, globalScale: number) => {
      if (globalScale <= 1.0) return
      const fontSize = Math.max(9, Math.min(13, 11 / globalScale))
      const specs: GraphLabelSpec[] = renderData.nodes
        // Hover-focus (PSY-1225): when focused, label only the foreground set so the
        // background de-clutters; at rest (focusedIds null) label all, as before. This pass
        // runs in onRenderFramePost, which does NOT self-trigger a repaint on closure change
        // (PSY-1209) — but the nodeCanvasObject/linkColor repaint on the same hover redraws
        // the whole frame, so the new filter is applied without a separate resumeAnimation.
        .filter(node => focusedIds == null || focusedIds.has(node.id))
        .map((node) => {
          const radius = node.is_isolate ? ISOLATE_RADIUS : NODE_RADIUS
          return {
            x: node.x ?? 0,
            y: (node.y ?? 0) + radius + 3,
            text: node.name.length > 22 ? node.name.slice(0, 20) + '…' : node.name,
            fontSize,
            // Always label the hovered node so the node you're pointing at is named even if a
            // higher-degree neighbor would win the collision cull. Only ever true while
            // hovering (hoveredNode null at rest), which is exactly when focus is active.
            force: node.id === hoveredNode?.id,
            priority: degreeById.get(node.id) ?? 0,
          }
        })
      renderGraphLabels(ctx, palette, specs)
    },
    [renderData, palette, degreeById, focusedIds, hoveredNode],
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
      // Hover-focus (PSY-1225): when a node is focused, fade the cluster hulls down with the
      // rest of the background (same BACKGROUND_ALPHA factor the nodes/links use). Without
      // this the colored cluster washes — which are NOT dimmed by globalAlpha since they're
      // their own fill pass — visually dominate the dimmed graph and bury the focused
      // neighborhood (worst in light mode). At rest (no focus) hulls keep their normal
      // zoom-faded alpha. The hovered node's own cluster fades too: the tooltip already names
      // the cluster, and keeping one hull lit would re-introduce a competing bright wash.
      const alpha = HULL_FILL_ALPHA_MAX * fadeT * (focusedIds ? BACKGROUND_ALPHA : 1)

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
        const fill = clusterColor(palette, cluster.color_index)

        if (points.length >= 3) {
          const hull = polygonHull(points)
          if (!hull) continue
          ctx.beginPath()
          ctx.moveTo(hull[0][0], hull[0][1])
          for (let i = 1; i < hull.length; i++) {
            ctx.lineTo(hull[i][0], hull[i][1])
          }
          ctx.closePath()
          ctx.fillStyle = withHexAlpha(fill, alphaToHex(alpha))
          ctx.fill()
        } else {
          const cx = points.reduce((s, p) => s + p[0], 0) / points.length
          const cy = points.reduce((s, p) => s + p[1], 0) / points.length
          ctx.beginPath()
          ctx.arc(cx, cy, 28, 0, Math.PI * 2)
          ctx.fillStyle = withHexAlpha(fill, alphaToHex(alpha))
          ctx.fill()
        }
      }
    },
    [renderData.nodes, clustersByID, palette, focusedIds],
  )

  // Reset the per-frame hull-painted flag at the start of each render pass.
  const handleRenderFramePre = useCallback(
    (ctx: CanvasRenderingContext2D) => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(ctx as any).__forceGraphHullPainted = false
    },
    [],
  )

  // ── PSY-1083: typed-edge grammar ──
  // Links that carry a `type` speak the shared grammar (per-type color,
  // dash, magnitude width, tooltip). Untyped links keep the pre-PSY-1083
  // monochrome styling so payloads without types regress nothing.
  const linkColor = useCallback(
    (link: RenderLink) => {
      // Hover-focus (PSY-1225): a link is foreground only when BOTH its endpoints are in the
      // foreground set — those keep their resting full color so the focused neighborhood's
      // edges stay crisp; every other link fades to the background alpha. Typed links fade
      // via withHexAlpha on the 6-hex token; untyped links carry rgba() colors withHexAlpha
      // can't touch, so they fade via an explicit low-alpha grey using BACKGROUND_ALPHA.
      if (focusedIds) {
        const foreground =
          focusedIds.has(endpointId(link.source)) && focusedIds.has(endpointId(link.target))
        if (!link.type) {
          // Foreground untyped → the resting intra grey (its "full"); background → faded.
          return foreground ? 'rgba(148, 163, 184, 0.6)' : `rgba(148, 163, 184, ${BACKGROUND_ALPHA})`
        }
        const color = palette.edges[link.type] ?? palette.unknownEdge
        return foreground ? color : withHexAlpha(color, BACKGROUND_ALPHA_HEX)
      }
      // Resting (no focus) — styling unchanged from before PSY-1225.
      if (!link.type) {
        return link.is_cross_cluster
          ? 'rgba(148, 163, 184, 0.35)'
          : 'rgba(148, 163, 184, 0.6)'
      }
      const color = palette.edges[link.type] ?? palette.unknownEdge
      // Cross-cluster ties dim to ≈40% alpha — same ratio the artist graph
      // applies to its cross-connections.
      return link.is_cross_cluster ? withHexAlpha(color, '66') : color
    },
    [palette, focusedIds],
  )

  const linkWidth = useCallback((link: RenderLink) => {
    if (!link.type) return link.is_cross_cluster ? 0.6 : 1.1
    return edgeWidth(link.type, link.score)
  }, [])

  // force-graph's native linkLineDash (PSY-1079 spike: cheap, no custom
  // canvas renderer needed). edgeLineDash('') returns [] = solid.
  const linkLineDash = useCallback((link: RenderLink) => edgeLineDash(link.type), [])

  // PSY-362-style hover tooltip on typed edges, surfacing the raw signal
  // behind the connection. Untyped links get no tooltip.
  const linkLabel = useCallback(
    (link: RenderLink) => (link.type ? buildLinkLabel(link) : ''),
    [],
  )

  const handleToggleEdgeType = useCallback((type: string) => {
    setHiddenEdgeTypes(prev => {
      const next = new Set(prev)
      if (next.has(type)) next.delete(type)
      else next.add(type)
      return next
    })
  }, [])

  return (
    <div
      ref={containerRef}
      className="relative rounded-lg border border-border/50 overflow-hidden bg-background"
    >
      <ForceGraph2D
        ref={graphRef}
        // `fx`/`fy` on `RenderNode` are `number | null | undefined`
        // because we intentionally re-release a pinned position by
        // setting them to `null` (d3-force's documented convention
        // for "unfix this node"). The lib's `GraphData` types
        // model these as `number | undefined`, so we cast through
        // the prop boundary. Runtime behaviour is unchanged.
        graphData={renderData as unknown as React.ComponentProps<typeof ForceGraph2D>['graphData']}
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
        // Wheel-zoom moves the node under a stationary pointer without re-firing
        // onNodeHover, stranding the tooltip at a stale screen position — dismiss
        // it on zoom (re-hover re-anchors it). Matches ArtistGraph (PSY-1217).
        onZoom={() => setHoveredNode(null)}
        // Unlike ArtistGraph, ForceGraphView leaves node-drag enabled (default).
        // Dragging a node moves it without re-firing onNodeHover (the same node
        // stays under the cursor) or onZoom, so the anchored tooltip would strand
        // at the node's pre-drag position. Dismiss on drag too (PSY-1217 review).
        onNodeDrag={() => setHoveredNode(null)}
        linkSource="source"
        linkTarget="target"
        linkColor={linkColor}
        linkWidth={linkWidth}
        linkLineDash={linkLineDash}
        linkLabel={linkLabel}
        // PSY-1334: 1px strokes are near-unclickable at the lib's default
        // hover precision — widen the link hit target for the inspect click.
        linkHoverPrecision={4}
        onLinkClick={handleLinkClick}
        // Background click closes the inspect panel (no-op when closed).
        onBackgroundClick={connectionInspect.close}
        linkCanvasObjectMode={() => 'before'}
        linkCanvasObject={drawHulls}
        onRenderFramePre={handleRenderFramePre}
        onRenderFramePost={nodeLabelsFrame}
        // PSY-1220: keep the open tooltip pinned to its node as the node drifts during settle.
        onEngineTick={handleEngineTick}
        // PSY-1321: one-shot initial frame once the layout settles (see needsFitRef).
        onEngineStop={handleEngineStop}
        cooldownTicks={200}
        d3AlphaDecay={0.04}
        d3VelocityDecay={0.3}
        minZoom={0.4}
        maxZoom={3}
        backgroundColor="transparent"
      />

      {/* PSY-1083: interactive edge legend — per-type swatch, live count,
          show/hide toggle. Only for surfaces that opt in, and only when the
          payload actually carries typed edges. */}
      {showEdgeLegend && renderData.edgeTypeCounts.size > 0 && (
        <EdgeLegend
          className="absolute top-2 right-2"
          types={[...renderData.edgeTypeCounts.keys()]}
          counts={renderData.edgeTypeCounts}
          hiddenTypes={hiddenEdgeTypes}
          onToggleType={handleToggleEdgeType}
          soloType={soloEdgeType}
          onSoloType={setSoloEdgeType}
        />
      )}

      {/* PSY-1334: click-to-inspect connection panel. DOM (not canvas), so it
          works on touch, carries links, survives the fullscreen overlay, and
          clicks inside it don't trip the zoomToFit canvas pointerdown cancel. */}
      {connectionPanelData && (
        <ConnectionPanel
          className="absolute bottom-2 left-2 z-40"
          source={connectionPanelData.source}
          target={connectionPanelData.target}
          connections={connectionPanelData.connections}
          onClose={connectionInspect.close}
        />
      )}

      {hoveredNode && (
        <div
          className="absolute z-50 px-3 py-2 text-xs rounded-md bg-popover border border-border shadow-lg text-popover-foreground pointer-events-none"
          // Anchored at the node and flipped toward the interior near the edges —
          // shared with ArtistGraph via tooltipPlacementStyle (PSY-1217).
          style={tooltipPlacementStyle(tooltipPos)}
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
