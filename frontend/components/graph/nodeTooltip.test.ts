import { describe, expect, it } from 'vitest'

import { nodeTooltipPlacement, tooltipPlacementStyle } from './nodeTooltip'

// PSY-1217 — the node→screen positioning + flip logic shared by ArtistGraph
// and ForceGraphView. The canvas/d3-force pipeline can't render in jsdom, so
// the placement math is unit-tested here as a pure function (mirrors the
// graphLabels.ts / edgeGrammar.ts precedent).

// graph2ScreenCoords is identity by default so assertions read off the input;
// a panned variant below proves we route through it rather than the raw coords.
const graph = { graph2ScreenCoords: (x: number, y: number) => ({ x, y }) }
const container = { clientWidth: 1000, clientHeight: 500 }

describe('nodeTooltipPlacement', () => {
  it('maps a node with settled coords to its screen anchor (no flip in the interior)', () => {
    expect(nodeTooltipPlacement(graph, container, { x: 100, y: 200 })).toEqual({
      x: 100,
      y: 200,
      flipX: false,
      flipY: false,
    })
  })

  it('flips toward the interior once the node passes 60% of the container on each axis', () => {
    // 60% of 1000 = 600 (x), 60% of 500 = 300 (y); 700/400 are past both.
    expect(nodeTooltipPlacement(graph, container, { x: 700, y: 400 })).toMatchObject({
      flipX: true,
      flipY: true,
    })
  })

  it('does not flip exactly at the 60% threshold (strict >)', () => {
    expect(nodeTooltipPlacement(graph, container, { x: 600, y: 300 })).toMatchObject({
      flipX: false,
      flipY: false,
    })
  })

  it('routes through graph2ScreenCoords, not the raw graph coords', () => {
    // In real use the mapping isn't identity (pan/zoom). Assert we pass through it.
    const panned = { graph2ScreenCoords: (x: number, y: number) => ({ x: x + 50, y: y - 20 }) }
    expect(nodeTooltipPlacement(panned, container, { x: 100, y: 200 })).toMatchObject({
      x: 150,
      y: 180,
    })
  })

  it('returns null for a node without settled d3-force coords', () => {
    expect(nodeTooltipPlacement(graph, container, { x: undefined, y: undefined })).toBeNull()
    expect(nodeTooltipPlacement(graph, container, {})).toBeNull()
  })

  it('returns null when the graph or container ref is not ready', () => {
    expect(nodeTooltipPlacement(null, container, { x: 1, y: 2 })).toBeNull()
    expect(nodeTooltipPlacement(graph, null, { x: 1, y: 2 })).toBeNull()
  })

  it('returns null on hover-out (null node)', () => {
    expect(nodeTooltipPlacement(graph, container, null)).toBeNull()
  })

  it('returns null for NaN coords emitted on an early simulation tick', () => {
    // d3-force can hand back NaN before the layout seats; `x == null` would let it
    // through and render left:NaN — the corner-glitch PSY-1215 killed. Number
    // .isFinite rejects it.
    expect(nodeTooltipPlacement(graph, container, { x: NaN, y: 10 })).toBeNull()
    expect(nodeTooltipPlacement(graph, container, { x: 10, y: NaN })).toBeNull()
  })

  it('does not flip when the container has not been measured yet (0×0)', () => {
    // clientWidth*0.6 === 0 would otherwise make `x > 0` true and flip every node.
    const unmeasured = { clientWidth: 0, clientHeight: 0 }
    expect(nodeTooltipPlacement(graph, unmeasured, { x: 100, y: 50 })).toMatchObject({
      flipX: false,
      flipY: false,
    })
  })

  it('returns null when the graph ref lacks graph2ScreenCoords (transient mount)', () => {
    const noMethod = {} as Parameters<typeof nodeTooltipPlacement>[0]
    expect(nodeTooltipPlacement(noMethod, container, { x: 1, y: 2 })).toBeNull()
  })
})

describe('tooltipPlacementStyle', () => {
  it('anchors down-right of the node by default (no flip)', () => {
    expect(tooltipPlacementStyle({ x: 100, y: 200 })).toEqual({
      left: 100,
      top: 200,
      transform: 'translateX(8px) translateY(8px)',
    })
  })

  it('flips toward the node top-left when both flags are set', () => {
    expect(tooltipPlacementStyle({ x: 100, y: 200, flipX: true, flipY: true })).toEqual({
      left: 100,
      top: 200,
      transform: 'translateX(calc(-100% - 8px)) translateY(calc(-100% - 8px))',
    })
  })

  it('flips independently per axis', () => {
    expect(tooltipPlacementStyle({ x: 0, y: 0, flipX: true, flipY: false }).transform).toBe(
      'translateX(calc(-100% - 8px)) translateY(8px)',
    )
  })
})
