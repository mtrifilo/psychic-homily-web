'use client'

/**
 * The Map of the Scene canvas (PSY-1725) — the whole catalog at rest, drawn
 * from the nightly snapshot.
 *
 * WHY THIS IS NOT ForceGraphView. Every other canvas surface hands d3-force a
 * graph and lets it find a layout; this one is handed a layout and must not
 * touch it. The nightly job runs the force simulation once for everybody, so
 * here `warmupTicks` and `cooldownTicks` are BOTH zero and every node is pinned
 * with fx/fy: react-force-graph is used purely as a canvas + camera, and the
 * physics never runs on a visitor's main thread (PSY-1724 spike — the measured
 * cost at 5k nodes / 20k edges is drawing, not simulating).
 *
 * It reuses the shared vocabulary modules rather than either canvas component:
 * `graphPalette` (theme-resolved colors), `graphLabels` (tier ladder + halo
 * recipe), `graphMarkers` (hub square, upcoming-show dot), `graphFocus` (the
 * hover neighbourhood + dim grammar). What is local is only what is genuinely
 * new: precomputed hull polygons, mono region captions, and the grid label cull
 * that keeps a catalog-sized candidate set off the per-frame path.
 */

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ComponentType,
  type Ref,
} from 'react'
import dynamic from 'next/dynamic'
import type { ForceGraphMethods, ForceGraphProps } from 'react-force-graph-2d'

import {
  LABEL_HUB_HALF_EXTENT,
} from '@/components/graph/labelHub'
import {
  drawLabelHubMarker,
  drawUpcomingShowDot,
} from '@/components/graph/graphMarkers'
import {
  TOOL_LABEL_TIERS,
  renderGraphLabels,
  truncateLabel,
  type GraphLabelSpec,
} from '@/components/graph/graphLabels'
import {
  BACKGROUND_ALPHA,
  buildAdjacency,
  resolveFocusForeground,
} from '@/components/graph/graphFocus'
import {
  alphaToHex,
  clusterColor,
  useGraphPalette,
  withHexAlpha,
  type GraphPalette,
} from '@/components/graph/graphPalette'
import { GraphSkeleton } from '@/components/graph/GraphSkeleton'

import { sceneMapColorIndex, type SceneMap, type SceneMapNode } from '../sceneMap'
import { cullLabelsToGrid, sceneMapLabelTiers } from '../sceneMapLabels'

// Same generic-erasure cast the ego graph uses: `next/dynamic` strips
// react-force-graph-2d's generic parameters, so the callback props would fall
// back to the library's loose `NodeObject<{}>` and fail variance checks under
// `strictFunctionTypes`. Runtime behaviour is unchanged.
const ForceGraph2D = dynamic(() => import('react-force-graph-2d'), {
  ssr: false,
  loading: () => <GraphSkeleton className="h-full" />,
}) as unknown as ComponentType<
  ForceGraphProps<MapNode, MapLink> & {
    ref?: Ref<ForceGraphMethods<MapNode, MapLink> | null>
  }
>

/**
 * A snapshot node with the coordinate fields d3-force reads. `fx`/`fy` are set
 * once at construction and never cleared: on this surface a pin is permanent,
 * because the position IS the data.
 */
interface MapNode extends SceneMapNode {
  x: number
  y: number
  fx: number
  fy: number
}

interface MapLink {
  source: number
  target: number
  kind: 'similarity' | 'spoke'
}

/**
 * Artist dot radius, a little under ForceGraphView's NODE_RADIUS of 8: a
 * whole-catalog map fits at a much lower zoom than an ego graph, so the dots
 * have to leave room for each other at the density the fitted view produces.
 */
const ARTIST_RADIUS = 7
/** Hub half-extent, from the shared PSY-1530 geometry. */
const HUB_HALF_EXTENT = LABEL_HUB_HALF_EXTENT

/**
 * Ramp slot for the label family, matching the `--chart-6` the ego palette
 * locks to labels so the app keeps one color language.
 */
const LABEL_HUB_COLOR_INDEX = 5

/**
 * Hull fill alpha at rest. Lower than ForceGraphView's 0.12 on purpose: this
 * map draws every region at once rather than a handful of clusters, so washes
 * that read as "a region" individually stack into a bruise collectively. Light
 * mode gets a further reduction — the newsprint background is much closer in
 * luminance to a tinted wash than the dark theme's near-black, so the same
 * alpha reads roughly twice as loud there (re-checked on both themes per the
 * locked design).
 */
const HULL_FILL_ALPHA_DARK = 0.14
const HULL_FILL_ALPHA_LIGHT = 0.09
/** Hull outline alpha, well above the fill so the boundary carries the shape. */
const HULL_STROKE_ALPHA = 0.55
/** Hull outline width in SCREEN px — counter-scaled at draw time. */
const HULL_STROKE_WIDTH_PX = 1.5

/** Edge ink at rest. Backbone edges are structure, not content: barely there. */
const EDGE_ALPHA_HEX = '2e'
const SPOKE_ALPHA_HEX = '4d'

/** Region caption size in SCREEN px — counter-scaled by zoom at draw time. */
const REGION_CAPTION_FONT_SIZE = 11
const REGION_CAPTION_ALPHA = 0.75

/**
 * Screen-space grid cell for the label cull, in px. Sized to a comfortable
 * name-plus-gap at the middle tier, so a fully labelled screen reads as an
 * evenly spaced field of names rather than a dense band over the biggest
 * community with bare space everywhere else.
 */
const LABEL_GRID_CELL_WIDTH = 120
const LABEL_GRID_CELL_HEIGHT = 34

/** Padding for the initial fit, in px. Room for the labels that hang below. */
const ZOOM_FIT_PADDING = 60

export interface SceneMapCanvasProps {
  map: SceneMap
  containerWidth: number
  /** An artist dot was clicked — the host re-roots exactly as a search does. */
  onSelectArtist: (node: SceneMapNode) => void
  /** A label hub was clicked — the host opens (or toggles shut) its panel. */
  onSelectHub: (node: SceneMapNode) => void
  /** The hub whose panel is open, kept in focus while it is. */
  selectedHubId: number | null
  /** Background click — the host closes any open panel. */
  onBackgroundClick: () => void
  ariaLabel: string
  describedById?: string
}

/** Map height, matching GRAPH_BOX_HEIGHT_CLASS so state swaps don't jump. */
function mapHeight(containerWidth: number): number {
  return containerWidth < 768 ? 400 : 560
}

export function SceneMapCanvas({
  map,
  containerWidth,
  onSelectArtist,
  onSelectHub,
  selectedHubId,
  onBackgroundClick,
  ariaLabel,
  describedById,
}: SceneMapCanvasProps) {
  const palette = useGraphPalette()
  const graphRef = useRef<ForceGraphMethods<MapNode, MapLink> | null>(null)
  const [hoveredId, setHoveredId] = useState<number | null>(null)

  // The dynamic chunk can attach the canvas AFTER the first render (cold mount),
  // so the initial fit has to depend on the attachment rather than on mount —
  // the `next/dynamic` ref gotcha the ego graph documents (PSY-1548).
  const [canvasReady, setCanvasReady] = useState(false)
  const attachGraphRef = useCallback(
    (instance: ForceGraphMethods<MapNode, MapLink> | null) => {
      graphRef.current = instance
      setCanvasReady(instance !== null)
    },
    [],
  )

  const height = mapHeight(containerWidth)

  // Render data is built ONCE per snapshot. The nodes handed to the engine are
  // the exact objects it keeps, so they carry their own pins; rebuilding them
  // per render would detach the pins from the running instance.
  const graphData = useMemo(() => {
    const nodes: MapNode[] = map.nodes.map(node => ({
      ...node,
      x: node.x,
      y: node.y,
      fx: node.x,
      fy: node.y,
    }))
    const links: MapLink[] = map.edges.map(edge => ({
      source: edge.source,
      target: edge.target,
      kind: edge.kind,
    }))
    return { nodes, links }
  }, [map])

  const nodeIds = useMemo(
    () => new Set(map.nodes.map(node => node.id)),
    [map],
  )
  const adjacency = useMemo(() => buildAdjacency(map.edges), [map])

  // Hover previews over the open hub panel, and a stale hover falls back to the
  // panel's node rather than releasing the dim — the shared PSY-1478 grammar.
  const focusedIds = useMemo(
    () =>
      resolveFocusForeground(
        adjacency,
        [hoveredId, selectedHubId],
        null,
        id => nodeIds.has(id),
      ),
    [adjacency, hoveredId, selectedHubId, nodeIds],
  )

  const tierStyles = useMemo(
    () => sceneMapLabelTiers(map.nodes, TOOL_LABEL_TIERS),
    [map],
  )

  const hullFillAlpha = isDarkPalette(palette) ? HULL_FILL_ALPHA_DARK : HULL_FILL_ALPHA_LIGHT

  // ── Hulls + region captions ────────────────────────────────────────────
  //
  // Drawn from `onRenderFramePre`, which runs before the links and nodes with
  // the ctx already in graph coordinates. ForceGraphView reaches the same layer
  // through a `linkCanvasObject` guarded by a once-per-frame flag on the ctx,
  // because it needs d3-polygon to COMPUTE a hull from live node positions.
  // Here the polygons arrive precomputed, so the pre-frame hook does the job
  // directly with no per-link dispatch and no flag to reset.
  const handleRenderFramePre = useCallback(
    (ctx: CanvasRenderingContext2D, globalScale: number) => {
      // While a neighbourhood is focused the regions fade with the rest of the
      // background: the washes are their own fill pass, so globalAlpha does not
      // touch them, and left lit they bury the focused set (worst in light mode).
      const focusFactor = focusedIds ? BACKGROUND_ALPHA : 1
      for (const region of map.regions) {
        if (region.hull.length < 3) continue
        const fill = clusterColor(
          palette,
          sceneMapColorIndex(region.community, palette.chart.length),
        )
        ctx.beginPath()
        ctx.moveTo(region.hull[0][0], region.hull[0][1])
        for (let i = 1; i < region.hull.length; i += 1) {
          ctx.lineTo(region.hull[i][0], region.hull[i][1])
        }
        ctx.closePath()
        ctx.fillStyle = withHexAlpha(fill, alphaToHex(hullFillAlpha * focusFactor))
        ctx.fill()
        // Counter-scaled so the boundary is a constant hairline at any zoom.
        // A graph-space width would vanish at the fitted whole-map view, which
        // is the ONE view the region outlines exist for.
        ctx.lineWidth = HULL_STROKE_WIDTH_PX / globalScale
        ctx.strokeStyle = withHexAlpha(fill, alphaToHex(HULL_STROKE_ALPHA * focusFactor))
        ctx.stroke()
      }
    },
    [map, palette, hullFillAlpha, focusedIds],
  )

  // ── Nodes ──────────────────────────────────────────────────────────────
  const nodeCanvasObject = useCallback(
    (node: MapNode, ctx: CanvasRenderingContext2D) => {
      const foreground = !focusedIds || focusedIds.has(node.id)
      ctx.save()
      ctx.globalAlpha = foreground ? 1 : BACKGROUND_ALPHA

      if (node.kind === 'label') {
        // Hubs take the label family's own hue from the shared ramp; SHAPE, not
        // hue, is what separates them from artists (PSY-1530).
        drawLabelHubMarker(
          ctx,
          node.x,
          node.y,
          HUB_HALF_EXTENT,
          clusterColor(palette, LABEL_HUB_COLOR_INDEX),
          palette.labelHalo,
        )
      } else {
        ctx.beginPath()
        ctx.arc(node.x, node.y, ARTIST_RADIUS, 0, Math.PI * 2)
        ctx.fillStyle = clusterColor(
          palette,
          sceneMapColorIndex(node.community, palette.chart.length),
        )
        ctx.fill()
        if (node.hasUpcomingShow) {
          drawUpcomingShowDot(ctx, node.x, node.y, ARTIST_RADIUS)
        }
      }

      ctx.restore()
    },
    [palette, focusedIds],
  )

  const nodePointerAreaPaint = useCallback(
    (node: MapNode, color: string, ctx: CanvasRenderingContext2D) => {
      ctx.beginPath()
      if (node.kind === 'label') {
        ctx.rect(
          node.x - HUB_HALF_EXTENT,
          node.y - HUB_HALF_EXTENT,
          HUB_HALF_EXTENT * 2,
          HUB_HALF_EXTENT * 2,
        )
      } else {
        // A hit area wider than the dot: at fitted zoom a 5-unit artist dot is
        // a couple of screen px, and the map's whole point is that a visitor
        // can start anywhere on it.
        ctx.arc(node.x, node.y, ARTIST_RADIUS * 2, 0, Math.PI * 2)
      }
      ctx.fillStyle = color
      ctx.fill()
    },
    [],
  )

  // ── Labels + region captions ───────────────────────────────────────────
  const handleRenderFramePost = useCallback(
    (ctx: CanvasRenderingContext2D, globalScale: number) => {
      drawRegionCaptions(ctx, palette, map, globalScale, focusedIds != null)

      // Labels are ALWAYS ON here — the zoom gate the other surfaces apply
      // would blank the map entirely, because a fitted whole-catalog view sits
      // far below it. Tier sizes are screen px, so they are counter-scaled by
      // the zoom; the collision boxes stay in graph space.
      const candidates: GraphLabelSpec[] = []
      for (const node of graphData.nodes) {
        if (focusedIds && !focusedIds.has(node.id)) continue
        const tier = tierStyles.get(node.id)
        if (!tier) continue
        const radius = node.kind === 'label' ? HUB_HALF_EXTENT : ARTIST_RADIUS
        candidates.push({
          x: node.x,
          y: node.y + radius + 3 / globalScale,
          text: truncateLabel(node.name),
          fontSize: tier.fontSize / globalScale,
          fontWeight: tier.fontWeight,
          // Rank 0 is the most central node, and `renderGraphLabels` keeps the
          // HIGHER priority on a collision, so the rank has to be inverted here
          // exactly as it is for the tier ladder.
          priority: -node.rank,
          // A hub is the visual anchor of a whole roster; an unlabelled one is
          // an unexplained square.
          force: node.kind === 'label',
        })
      }
      candidates.sort((a, b) => (b.priority ?? 0) - (a.priority ?? 0))

      // Forced labels skip the grid entirely. `renderGraphLabels` draws them
      // regardless of collisions, so culling one here would only silently
      // revoke the guarantee `force` exists to give.
      const forced = candidates.filter(spec => spec.force)
      const culled = cullLabelsToGrid(
        candidates.filter(spec => !spec.force),
        LABEL_GRID_CELL_WIDTH / globalScale,
        LABEL_GRID_CELL_HEIGHT / globalScale,
      )
      renderGraphLabels(ctx, palette, [...forced, ...culled])
    },
    [graphData, tierStyles, palette, map, focusedIds],
  )

  // ── Links ──────────────────────────────────────────────────────────────
  const linkColor = useCallback(
    (link: MapLink) => {
      const base =
        link.kind === 'spoke'
          ? clusterColor(palette, LABEL_HUB_COLOR_INDEX)
          : palette.mutedForeground
      const alphaHex = link.kind === 'spoke' ? SPOKE_ALPHA_HEX : EDGE_ALPHA_HEX
      if (!focusedIds) return withHexAlpha(base, alphaHex)
      // Both endpoints in the focused set = a connection INSIDE the
      // neighbourhood; anything else recedes with the rest of the background.
      const source = endpointNodeId(link.source)
      const target = endpointNodeId(link.target)
      const inFocus = focusedIds.has(source) && focusedIds.has(target)
      return withHexAlpha(base, inFocus ? 'cc' : alphaToHex(BACKGROUND_ALPHA / 2))
    },
    [palette, focusedIds],
  )

  // ── Interaction ────────────────────────────────────────────────────────
  const handleNodeClick = useCallback(
    (node: MapNode) => {
      if (node.kind === 'label') onSelectHub(node)
      else onSelectArtist(node)
    },
    [onSelectArtist, onSelectHub],
  )

  const handleNodeHover = useCallback((node: MapNode | null) => {
    setHoveredId(node?.id ?? null)
  }, [])

  // The one camera move on this surface: fit the whole map once the canvas is
  // attached. Instant (duration 0) — a map that flies in from an arbitrary
  // engine scale is motion for its own sake, and the layout is already final.
  // `canvasReady` is the dependency that makes this fire on a COLD mount, where
  // the dynamic chunk attaches the canvas after the first render; without it
  // the effect runs once against a null ref and the map keeps the engine's own
  // scale (the PSY-1548 gotcha). Calling zoomToFit is a DOM side effect, which
  // is what an effect is for.
  useEffect(() => {
    if (!canvasReady) return
    graphRef.current?.zoomToFit(0, ZOOM_FIT_PADDING)
  }, [canvasReady, map, containerWidth, height])

  return (
    <div
      className="w-full overflow-hidden rounded-lg"
      style={{ height }}
      role="img"
      aria-label={ariaLabel}
      aria-describedby={describedById}
    >
      <ForceGraph2D
        ref={attachGraphRef}
        graphData={
          graphData as unknown as React.ComponentProps<typeof ForceGraph2D>['graphData']
        }
        width={containerWidth}
        height={height}
        nodeId="id"
        nodeCanvasObject={nodeCanvasObject}
        nodePointerAreaPaint={nodePointerAreaPaint}
        onNodeClick={handleNodeClick}
        onNodeHover={handleNodeHover}
        onBackgroundClick={onBackgroundClick}
        linkSource="source"
        linkTarget="target"
        linkColor={linkColor}
        linkWidth={1.5}
        onRenderFramePre={handleRenderFramePre}
        onRenderFramePost={handleRenderFramePost}
        // ZERO client simulation — the whole point of the nightly snapshot.
        // Every node is pinned, so even a stray tick could not move anything,
        // but the engine is switched off rather than merely constrained: this
        // is the surface where the node count makes a warmup measurable.
        warmupTicks={0}
        cooldownTicks={0}
        minZoom={0.1}
        maxZoom={12}
        enableNodeDrag={false}
        backgroundColor="transparent"
      />
    </div>
  )
}

/** d3-force swaps a bare endpoint id for the resolved node after wiring. */
function endpointNodeId(endpoint: number | { id: number }): number {
  return typeof endpoint === 'number' ? endpoint : endpoint.id
}

/**
 * Whether the resolved palette is the dark theme, inferred from the background
 * token's luminance. The palette carries colors, not a theme name, and this is
 * the one decision on the map that needs to know which way round the two are:
 * a tinted wash costs far more contrast on newsprint than on near-black.
 */
export function isDarkPalette(palette: GraphPalette): boolean {
  const hex = palette.labelHalo
  if (!/^#[0-9a-fA-F]{6}$/.test(hex)) return true
  const r = parseInt(hex.slice(1, 3), 16)
  const g = parseInt(hex.slice(3, 5), 16)
  const b = parseInt(hex.slice(5, 7), 16)
  return (r * 299 + g * 587 + b * 114) / 1000 < 128
}

/**
 * "Around {artist}" captions over each region, in the mono face the rest of the
 * app uses for data (PSY-647). Inert by design in v1 — they name a
 * neighbourhood, they are not a control — so they are drawn, not hit-tested.
 */
function drawRegionCaptions(
  ctx: CanvasRenderingContext2D,
  palette: GraphPalette,
  map: SceneMap,
  globalScale: number,
  isFocused: boolean,
): void {
  const anchored = map.regions.filter(region => region.captionAnchor !== null)
  if (anchored.length === 0) return

  ctx.save()
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.lineJoin = 'round'
  const fontSize = REGION_CAPTION_FONT_SIZE / globalScale
  ctx.font = `400 ${fontSize}px ${palette.monoFontFamily}`
  ctx.globalAlpha = REGION_CAPTION_ALPHA * (isFocused ? BACKGROUND_ALPHA : 1)
  ctx.lineWidth = fontSize / 4
  for (const region of anchored) {
    const [x, y] = region.captionAnchor!
    // Same halo-under-fill recipe the node labels use: a caption sitting over
    // its own hull wash needs the backdrop just as much.
    ctx.strokeStyle = palette.labelHalo
    ctx.strokeText(region.label, x, y)
    ctx.fillStyle = palette.labelText
    ctx.fillText(region.label, x, y)
  }
  ctx.restore()
}
