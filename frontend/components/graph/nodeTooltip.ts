import type { CSSProperties } from 'react'

/**
 * Shared node-tooltip placement for the canvas graphs (PSY-1217).
 *
 * react-force-graph-2d's `onNodeHover` gives `(node, previousNode)` with no
 * MouseEvent, so the tooltip is anchored on the NODE, not the cursor:
 * `graph2ScreenCoords` maps the node's graph position to canvas pixels, which
 * are also the tooltip's coords because the tooltip is position:ABSOLUTE inside
 * the relative graph container. Absolute — not fixed — so it stays correct
 * inside a transformed Radix dialog, whose transform would otherwise be the
 * containing block for a fixed tooltip and offset it to the dialog corner.
 *
 * Extracted from ArtistGraph's PSY-1215 fix so ArtistGraph and ForceGraphView
 * share one source of truth instead of byte-diverging copies (PSY-1217).
 */

/**
 * A tooltip anchor in CONTAINER-relative pixels, with the per-axis flip flags
 * OPTIONAL — the input shape `tooltipPlacementStyle` accepts. A bare `{ x, y }`
 * renders the down-right, no-flip resting case. Both the style fn and
 * `ArtistNodeTooltip`'s `position` prop take this one shape so the field set
 * lives in a single place (PSY-1217).
 */
export interface TooltipAnchor {
  /** Node x in container pixels (the tooltip's `left`). */
  x: number
  /** Node y in container pixels (the tooltip's `top`). */
  y: number
  /** Anchor to the node's LEFT (node sits past 60% of the container width). */
  flipX?: boolean
  /** Anchor ABOVE the node (node sits past 60% of the container height). */
  flipY?: boolean
}

/**
 * A fully-resolved anchor — what `nodeTooltipPlacement` computes, with both flip
 * flags decided. Assignable to TooltipAnchor wherever the optional shape is taken.
 */
export interface TooltipPlacement extends TooltipAnchor {
  flipX: boolean
  flipY: boolean
}

/** The slice of ForceGraphMethods this module needs (the graphRef is typed `any`). */
interface ScreenCoordsGraph {
  graph2ScreenCoords: (x: number, y: number) => { x: number; y: number }
}

/** The hovered node — only its settled simulation coords matter for placement. */
interface PlaceableNode {
  x?: number
  y?: number
}

/**
 * Map a hovered node to its container-relative tooltip anchor + flip flags, or
 * `null` when it can't be placed yet (no graph/container ref, or the node has
 * no settled d3-force coords). Callers hide the tooltip on `null` rather than
 * render it at a stale/origin position (PSY-1215). The flip flags steer the
 * tooltip toward the container interior near the right/bottom edges so it
 * doesn't run off the (overflow-hidden) container or the dialog.
 */
export function nodeTooltipPlacement(
  graph: ScreenCoordsGraph | null | undefined,
  container: Pick<HTMLElement, 'clientWidth' | 'clientHeight'> | null | undefined,
  node: PlaceableNode | null | undefined,
): TooltipPlacement | null {
  // Defensive at the boundary: `graph` is an `any`-typed library ref (the method
  // may be momentarily absent during a transient mount), and d3-force can hand
  // back NaN coords on the first few simulation ticks. Number.isFinite rejects
  // both undefined (unsettled) and NaN, so we never feed `left: NaN` to the style
  // — the exact corner-glitch PSY-1215 set out to kill.
  if (
    !graph ||
    typeof graph.graph2ScreenCoords !== 'function' ||
    !container ||
    !node ||
    !Number.isFinite(node.x) ||
    !Number.isFinite(node.y)
  ) {
    return null
  }
  const { x, y } = graph.graph2ScreenCoords(node.x as number, node.y as number)
  const { clientWidth, clientHeight } = container
  return {
    x,
    y,
    // Flip toward the interior past 60% of each axis — but only once the container
    // has been measured. A 0×0 container (mounted one frame before layout) would
    // make `x > 0` true for every node and wrongly flip them all up-left.
    flipX: clientWidth > 0 && x > clientWidth * 0.6,
    flipY: clientHeight > 0 && y > clientHeight * 0.6,
  }
}

/**
 * The position:absolute inline style that anchors a tooltip at `placement`:
 * left/top sit at the node; the transform offsets the tooltip 8px off the node
 * and flips it toward the container interior near the right/bottom edge. The
 * flip flags are optional (default: down-right, no flip) so callers can pass a
 * bare `{ x, y }` for the resting case.
 */
export function tooltipPlacementStyle(placement: TooltipAnchor): CSSProperties {
  return {
    left: placement.x,
    top: placement.y,
    transform: [
      placement.flipX ? 'translateX(calc(-100% - 8px))' : 'translateX(8px)',
      placement.flipY ? 'translateY(calc(-100% - 8px))' : 'translateY(8px)',
    ].join(' '),
  }
}
