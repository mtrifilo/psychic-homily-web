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

import { LABEL_HUB_HALF_EXTENT } from '@/components/graph/labelHub'
import {
  drawLabelHubMarker,
  drawUpcomingShowDot,
} from '@/components/graph/graphMarkers'
import {
  TOOL_LABEL_TIERS,
  renderGraphLabels,
  truncateLabel,
} from '@/components/graph/graphLabels'
import {
  BACKGROUND_ALPHA,
  buildAdjacency,
  endpointId,
  resolveFocusForeground,
} from '@/components/graph/graphFocus'
import {
  alphaToHex,
  clusterColor,
  useGraphPalette,
  withHexAlpha,
} from '@/components/graph/graphPalette'
import { GraphSkeleton } from '@/components/graph/GraphSkeleton'
import { graphCanvasHeight } from '@/components/graph/GraphStateCard'

import { sceneMapColorIndex, type SceneMap, type SceneMapNode } from '../sceneMap'
import {
  selectSceneMapLabels,
  sceneMapLabelTiers,
  type SceneMapLabelCandidate,
  type WorldBounds,
} from '../sceneMapLabels'

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
/** Edge ink inside a hovered neighbourhood, and outside it. */
const FOCUSED_EDGE_ALPHA_HEX = 'cc'
const DIMMED_EDGE_ALPHA_HEX = alphaToHex(BACKGROUND_ALPHA / 2)

/** Region caption size in SCREEN px — counter-scaled by zoom at draw time. */
const REGION_CAPTION_FONT_SIZE = 11
const REGION_CAPTION_ALPHA = 0.75

/**
 * Priority a label hub gets over an artist when they contend for a grid cell.
 * Far larger than any rank spread, so a hub always wins its cell — but it is a
 * BOOST, not an exemption: two hubs in one cell still yield to each other.
 */
const HUB_LABEL_PRIORITY_BOOST = 1e9

/** Padding for the initial fit, in px. Room for the labels that hang below. */
const ZOOM_FIT_PADDING = 60

/** Viewport-cull margin, as a fraction of the visible extent on each side. */
const VIEWPORT_CULL_MARGIN = 0.05

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

  const height = graphCanvasHeight(containerWidth)

  // Render data is built ONCE per snapshot, and it is a COPY on purpose.
  // react-force-graph hands these exact objects to d3-force, which writes its
  // own bookkeeping onto them (`vx`/`vy`/`index`, and it swaps each link's
  // numeric endpoints for node references). Handing it `map.nodes` directly
  // would let the engine mutate the decoded snapshot that the list view and
  // every memo below also read.
  //
  // Deliberately NOT keyed on the palette. Rebuilding these objects on a theme
  // toggle would hand the engine an entirely new graph — a full data digest and
  // re-initialisation to change nothing but colour. The theme-dependent fill
  // lives in its own lookup below.
  const graphData = useMemo(() => {
    const nodes: MapNode[] = map.nodes.map(node => ({
      ...node,
      fx: node.x,
      fy: node.y,
    }))
    const links: MapLink[] = map.edges.map(edge => ({ ...edge }))
    return { nodes, links }
  }, [map])

  // Node fill by id, resolved per snapshot + theme rather than per node per
  // frame: it depends only on the node's community and the palette, so
  // recomputing it inside the paint callback would be pure waste on the
  // hottest path in the component.
  const fillById = useMemo(() => {
    const fills = new Map<number, string>()
    const hubFill = clusterColor(palette, LABEL_HUB_COLOR_INDEX)
    for (const node of map.nodes) {
      fills.set(
        node.id,
        node.kind === 'label'
          ? hubFill
          : clusterColor(palette, sceneMapColorIndex(node.community, palette.chart.length)),
      )
    }
    return fills
  }, [map, palette])

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

  // Everything about a label that does NOT depend on zoom or hover, computed
  // once per snapshot and pre-sorted by priority.
  //
  // This memo is the difference between a map that scrolls and one that does
  // not. `onRenderFramePost` runs every animation frame; building this list
  // there would allocate one object per node, sort it, and re-truncate every
  // name SIXTY TIMES A SECOND — at catalog scale, hundreds of thousands of
  // allocations per second for a result that changes only when the nightly
  // snapshot does. Per frame, only the zoom-dependent numbers are applied, and
  // only to the labels that survive the cull.
  const labelCandidates = useMemo(() => {
    const candidates: SceneMapLabelCandidate[] = []
    for (const node of map.nodes) {
      const tier = tierStyles.get(node.id)
      if (!tier) continue
      const isHub = node.kind === 'label'
      candidates.push({
        id: node.id,
        x: node.x,
        y: node.y,
        radius: isHub ? HUB_HALF_EXTENT : ARTIST_RADIUS,
        text: truncateLabel(node.name),
        fontSize: tier.fontSize,
        fontWeight: tier.fontWeight,
        // Rank 0 is the most central node, and `renderGraphLabels` keeps the
        // HIGHER priority on a collision, so the rank has to be inverted here
        // exactly as it is for the tier ladder.
        //
        // A hub gets a large boost rather than `force`. Forcing meant every
        // hub in the catalog drew through every collision at the fitted zoom —
        // hundreds of label names stacked on each other, and suppressing the
        // artist names underneath, which is the exact pile-up the shared label
        // module exists to end. The boost still wins a hub its cell against any
        // artist; it just cannot win the same cell twice.
        priority: isHub ? HUB_LABEL_PRIORITY_BOOST - node.rank : -node.rank,
        force: false,
      })
    }
    // Sorted ONCE. The per-frame cull consumes this order as its tie-break, so
    // the same artists win the same cells on every frame and the labels never
    // flicker between neighbours.
    candidates.sort((a, b) => b.priority - a.priority || a.id - b.id)
    return candidates
  }, [map, tierStyles])

  // The labels that are not up for culling: every label hub, and every named
  // region. Both go through `renderGraphLabels` with the artist names so their
  // boxes are RESERVED — a separate text pass would draw them and reserve
  // nothing, and an artist name landing on top of a region name is exactly the
  // pile-up that module exists to prevent.
  //
  // A hub is the visual anchor of a whole roster, so an unlabelled one is an
  // unexplained square; a region that has a name must show it. The captions'
  // low alpha is what keeps them behind the artist names rather than competing.
  const forcedCandidates = useMemo(() => {
    const forced: SceneMapLabelCandidate[] = []
    for (const region of map.regions) {
      if (!region.captionAnchor) continue
      forced.push({
        // Region ids are made negative so they cannot collide with a node id.
        // Nothing resolves a caption BY id — `selectSceneMapLabels` exempts
        // forced candidates from the focus filter precisely because a region
        // is not a node and has no focus state to look up.
        id: -1 - region.community,
        x: region.captionAnchor[0],
        y: region.captionAnchor[1],
        radius: 0,
        text: region.label,
        fontSize: REGION_CAPTION_FONT_SIZE,
        fontWeight: 400,
        priority: Number.POSITIVE_INFINITY,
        force: true,
        fontFamily: palette.monoFontFamily,
        alpha: REGION_CAPTION_ALPHA,
        memberCount: region.memberCount,
      })
    }
    // Sorted so the per-frame ceiling truncates the TAIL: the biggest regions
    // are the ones a visitor most needs named.
    forced.sort((a, b) => (b.memberCount ?? 0) - (a.memberCount ?? 0) || a.id - b.id)
    return forced
  }, [map, palette.monoFontFamily])

  // Hull paint styles, resolved per theme and focus state rather than per
  // frame: the alpha only ever takes two values, so recomputing a colour
  // string per region per frame is pure garbage.
  //
  // Light mode takes a lower fill — the newsprint background sits far closer
  // in luminance to a tinted wash than the dark theme's near-black, so the same
  // alpha reads roughly twice as loud there.
  const hullStyles = useMemo(() => {
    const focusFactor = focusedIds ? BACKGROUND_ALPHA : 1
    const fillAlpha =
      (palette.isDark ? HULL_FILL_ALPHA_DARK : HULL_FILL_ALPHA_LIGHT) * focusFactor
    const strokeAlpha = HULL_STROKE_ALPHA * focusFactor
    return map.regions
      .filter(region => region.hull.length >= 3)
      .map(region => {
        const base = clusterColor(
          palette,
          sceneMapColorIndex(region.community, palette.chart.length),
        )
        return {
          hull: region.hull,
          fill: withHexAlpha(base, alphaToHex(fillAlpha)),
          stroke: withHexAlpha(base, alphaToHex(strokeAlpha)),
        }
      })
  }, [map, palette, focusedIds])

  // ── Hulls ──────────────────────────────────────────────────────────────
  //
  // Drawn from `onRenderFramePre`, which runs before the links and nodes with
  // the ctx already in graph coordinates. ForceGraphView reaches the same layer
  // through a `linkCanvasObject` guarded by a once-per-frame flag on the ctx,
  // because it needs d3-polygon to COMPUTE a hull from live node positions.
  // Here the polygons arrive precomputed, so the pre-frame hook does the job
  // directly with no per-link dispatch and no flag to reset.
  const handleRenderFramePre = useCallback(
    (ctx: CanvasRenderingContext2D, globalScale: number) => {
      // The library does NOT wrap the pre-frame hook in save/restore (it calls
      // it bare, before tickFrame), so anything set here would otherwise leak
      // into the link and node passes that follow. Same self-contained
      // save/restore contract `drawIsolateShelfBand` already keeps.
      ctx.save()
      // Counter-scaled so the boundary is a constant hairline at any zoom. A
      // graph-space width would vanish at the fitted whole-map view, which is
      // the ONE view the region outlines exist for.
      ctx.lineWidth = HULL_STROKE_WIDTH_PX / globalScale
      for (const style of hullStyles) {
        ctx.beginPath()
        ctx.moveTo(style.hull[0][0], style.hull[0][1])
        for (let i = 1; i < style.hull.length; i += 1) {
          ctx.lineTo(style.hull[i][0], style.hull[i][1])
        }
        ctx.closePath()
        ctx.fillStyle = style.fill
        ctx.fill()
        ctx.strokeStyle = style.stroke
        ctx.stroke()
      }
      ctx.restore()
    },
    [hullStyles],
  )

  // ── Nodes ──────────────────────────────────────────────────────────────
  const nodeCanvasObject = useCallback(
    (node: MapNode, ctx: CanvasRenderingContext2D) => {
      // No save/restore per node, and NOT because the library provides one:
      // `paintNodes` saves once before the loop, but the default
      // `nodeCanvasObjectMode` is 'replace', whose branch restores INSIDE the
      // loop — so the first node empties the stack and every later restore is a
      // no-op. The real reason is narrower: this callback touches exactly one
      // piece of ctx state, and resets it unconditionally below. Anything else
      // added here (setLineDash, shadowBlur, textAlign, filter) WILL leak into
      // the label pass and the next frame, so wrap the body if that changes.
      ctx.globalAlpha = !focusedIds || focusedIds.has(node.id) ? 1 : BACKGROUND_ALPHA

      if (node.kind === 'label') {
        // Hubs take the label family's own hue from the shared ramp; SHAPE, not
        // hue, is what separates them from artists (PSY-1530).
        drawLabelHubMarker(
          ctx,
          node.x,
          node.y,
          HUB_HALF_EXTENT,
          fillById.get(node.id) ?? palette.otherCluster,
          palette.labelHalo,
        )
      } else {
        ctx.beginPath()
        ctx.arc(node.x, node.y, ARTIST_RADIUS, 0, Math.PI * 2)
        ctx.fillStyle = fillById.get(node.id) ?? palette.otherCluster
        ctx.fill()
        if (node.hasUpcomingShow) {
          drawUpcomingShowDot(ctx, node.x, node.y, ARTIST_RADIUS)
        }
      }

      ctx.globalAlpha = 1
    },
    [fillById, palette.labelHalo, palette.otherCluster, focusedIds],
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
  //
  // Region captions and node names go through ONE pass, so a region name and
  // an artist name can never be drawn over each other — a second text pass
  // would reserve no collision box, which is the pile-up `graphLabels` exists
  // to prevent.
  //
  // Labels are ALWAYS ON here. The zoom gate every other surface applies would
  // blank the map entirely, because a fitted whole-catalog view sits far below
  // it — this map's labels ARE its content, not a hover reward.
  const handleRenderFramePost = useCallback(
    (ctx: CanvasRenderingContext2D, globalScale: number) => {
      renderGraphLabels(
        ctx,
        palette,
        selectSceneMapLabels(
          labelCandidates,
          forcedCandidates,
          globalScale,
          focusedIds,
          visibleWorldBounds(ctx),
        ),
      )
    },
    [labelCandidates, forcedCandidates, palette, focusedIds],
  )

  // ── Links ──────────────────────────────────────────────────────────────
  // The four colours an edge can ever be, resolved once per theme.
  //
  // react-force-graph calls the accessor below once per link per redraw, and it
  // then hashes what comes back to batch the strokes. Building the string in
  // the accessor meant a regex test and two string allocations per edge per
  // frame; returning one of four cached instances makes the batching cheaper
  // too, since identical strings compare by reference.
  const linkColors = useMemo(() => {
    const spokeBase = clusterColor(palette, LABEL_HUB_COLOR_INDEX)
    return {
      restSimilarity: withHexAlpha(palette.mutedForeground, EDGE_ALPHA_HEX),
      restSpoke: withHexAlpha(spokeBase, SPOKE_ALPHA_HEX),
      focusedSimilarity: withHexAlpha(palette.mutedForeground, FOCUSED_EDGE_ALPHA_HEX),
      focusedSpoke: withHexAlpha(spokeBase, FOCUSED_EDGE_ALPHA_HEX),
      dimmedSimilarity: withHexAlpha(palette.mutedForeground, DIMMED_EDGE_ALPHA_HEX),
      dimmedSpoke: withHexAlpha(spokeBase, DIMMED_EDGE_ALPHA_HEX),
    }
  }, [palette])

  const linkColor = useCallback(
    (link: MapLink) => {
      const isSpoke = link.kind === 'spoke'
      if (!focusedIds) return isSpoke ? linkColors.restSpoke : linkColors.restSimilarity
      // Both endpoints in the focused set = a connection INSIDE the
      // neighbourhood; anything else recedes with the rest of the background.
      const inFocus =
        focusedIds.has(endpointId(link.source)) && focusedIds.has(endpointId(link.target))
      if (inFocus) return isSpoke ? linkColors.focusedSpoke : linkColors.focusedSimilarity
      return isSpoke ? linkColors.dimmedSpoke : linkColors.dimmedSimilarity
    },
    [linkColors, focusedIds],
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

/**
 * The graph-space rectangle currently on screen, read back from the canvas.
 *
 * Inside a render-frame hook the ctx transform IS the graph transform (pan +
 * zoom + device pixel ratio), so inverting it maps the canvas corners to world
 * coordinates without duplicating the camera maths. Returns null if the browser
 * cannot invert it, which the label pass reads as "no viewport bound" and
 * degrades to the grid and the cap alone.
 */
function visibleWorldBounds(ctx: CanvasRenderingContext2D): WorldBounds | null {
  let minX: number
  let maxX: number
  let minY: number
  let maxY: number
  // The WHOLE read is guarded, not just the inversion. This runs inside a
  // render-frame callback, where an exception would kill the frame loop rather
  // than surface anywhere useful — and it touches three separate optional
  // platform APIs (getTransform, DOMMatrix.inverse, transformPoint). Returning
  // null degrades to "no viewport bound", which the label pass handles.
  // Plain point literals rather than `new DOMPoint(...)`: transformPoint takes
  // a DOMPointInit, so this needs no constructor that a runtime might lack.
  try {
    const transform = ctx.getTransform?.()
    if (!transform) return null
    const inverse = transform.inverse()
    const topLeft = inverse.transformPoint({ x: 0, y: 0 })
    const bottomRight = inverse.transformPoint({
      x: ctx.canvas.width,
      y: ctx.canvas.height,
    })
    minX = Math.min(topLeft.x, bottomRight.x)
    maxX = Math.max(topLeft.x, bottomRight.x)
    minY = Math.min(topLeft.y, bottomRight.y)
    maxY = Math.max(topLeft.y, bottomRight.y)
  } catch {
    return null
  }
  if (!Number.isFinite(minX) || !Number.isFinite(minY)) return null
  if (!Number.isFinite(maxX) || !Number.isFinite(maxY)) return null
  // A margin, so a label anchored just off-screen still draws the part of
  // itself that reaches into view instead of popping in at the edge.
  const marginX = (maxX - minX) * VIEWPORT_CULL_MARGIN
  const marginY = (maxY - minY) * VIEWPORT_CULL_MARGIN
  return {
    minX: minX - marginX,
    maxX: maxX + marginX,
    minY: minY - marginY,
    maxY: maxY + marginY,
  }
}
