import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { BACKGROUND_ALPHA } from './graphFocus'

// PSY-1225: cover the PORT wiring — that ForceGraphView feeds the shared graphFocus
// neighborhood set into its link fade (linkColor) and node dim (nodeCanvasObject) on hover.
// The neighborhood MATH is unit-tested in graphFocus.test.ts; the VISUAL result is verified
// by screenshots (jsdom can't render canvas). This suite guards the glue in between: a
// refactor that dropped focusedIds from a callback would silently stop the fade with nothing
// failing. We exercise the callbacks the (mocked) canvas would call, the same prop-capture
// approach as ArtistGraphVisualization.test.tsx.

// Capture the props ForceGraph2D is rendered with so the test can drive onNodeHover and
// re-read the focus-dependent callbacks exactly as the real canvas would.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let forceGraphProps: any = null

// ForceGraph2D loads via next/dynamic (ssr:false). Mock it to a synchronous stub that
// records its props — no Suspense/async-import dance, no real canvas/three dependency.
vi.mock('next/dynamic', () => ({
  default: () =>
    function ForceGraph2DStub(props: Record<string, unknown>) {
      forceGraphProps = props
      return <div data-testid="force-graph" />
    },
}))

// With the canvas mocked there is no graph2ScreenCoords and nodes carry no settled coords,
// so the real nodeTooltipPlacement returns null for every hover and hoveredNode would never
// set. Stub it to place any node and reject null (hover-out), so onNodeHover drives the
// parent's focus state.
vi.mock('./nodeTooltip', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./nodeTooltip')>()
  return {
    ...actual,
    nodeTooltipPlacement: (_graph: unknown, _container: unknown, node: unknown) =>
      node ? { x: 12, y: 12, flipX: false, flipY: false } : null,
  }
})

import { ForceGraphView, type GraphNode, type GraphLink } from './ForceGraphView'

// Two disjoint 1-hop pairs: 1—2 and 3—4. Hovering node 1 puts {1,2} in the foreground;
// the 3—4 link (and node 3) are background. Untyped links (type '') so the resting +
// faded colors are deterministic rgba() strings, independent of theme-palette resolution.
const nodes: GraphNode[] = [
  { id: 1, name: 'Alpha', slug: 'alpha', upcoming_show_count: 0 },
  { id: 2, name: 'Beta', slug: 'beta', upcoming_show_count: 0 },
  { id: 3, name: 'Gamma', slug: 'gamma', upcoming_show_count: 0 },
  { id: 4, name: 'Delta', slug: 'delta', upcoming_show_count: 0 },
]
const links: GraphLink[] = [
  { source_id: 1, target_id: 2, type: '' },
  { source_id: 3, target_id: 4, type: '' },
]

// A RenderNode-shaped object for the nodeCanvasObject dim assertions.
const renderNode = (id: number) => ({
  id, name: `n${id}`, slug: `n${id}`, upcoming_show_count: 0,
  cluster_id: 'other', is_isolate: false, x: 0, y: 0,
})

// An untyped link in the shape linkColor reads (source/target, post-renderData mapping).
const link = (source: number, target: number, is_cross_cluster = false) => ({
  source, target, type: '', is_cross_cluster,
})

// Minimal canvas ctx that records every globalAlpha assignment so the dim can be asserted
// (nodeCanvasObject resets globalAlpha to 1 at the end, so the live value can't be read after).
function makeFakeCtx() {
  const alphas: number[] = []
  let alpha = 1
  return {
    get globalAlpha() { return alpha },
    set globalAlpha(v: number) { alpha = v; alphas.push(v) },
    beginPath() {}, arc() {}, fill() {}, stroke() {},
    fillStyle: '', strokeStyle: '', lineWidth: 0,
    alphas,
  }
}

const renderGraph = () =>
  renderWithProviders(
    <ForceGraphView
      nodes={nodes}
      links={links}
      containerWidth={1024}
      ariaLabel="test graph"
      onNodeClick={() => {}}
    />,
  )

describe('ForceGraphView — hover-focus port (PSY-1225)', () => {
  beforeEach(() => { forceGraphProps = null })
  afterEach(() => { vi.clearAllMocks() })

  it('uses resting link styling when nothing is hovered (no focus)', () => {
    renderGraph()
    expect(forceGraphProps).not.toBeNull()
    // No focus → intra grey 0.6, cross grey 0.35 (unchanged from before PSY-1225).
    expect(forceGraphProps.linkColor(link(1, 2, false))).toBe('rgba(148, 163, 184, 0.6)')
    expect(forceGraphProps.linkColor(link(3, 4, true))).toBe('rgba(148, 163, 184, 0.35)')
  })

  it('fades links outside the hovered node’s 1-hop neighborhood', () => {
    renderGraph()
    // Hover node 1 → foreground {1, 2}.
    act(() => forceGraphProps.onNodeHover(renderNode(1)))
    // 1—2 is foreground (both endpoints in the set) → full color.
    expect(forceGraphProps.linkColor(link(1, 2))).toBe('rgba(148, 163, 184, 0.6)')
    // 3—4 touches neither foreground node → faded to the background alpha.
    expect(forceGraphProps.linkColor(link(3, 4))).toBe(`rgba(148, 163, 184, ${BACKGROUND_ALPHA})`)
  })

  it('dims nodes outside the foreground set via globalAlpha, leaving foreground nodes full', () => {
    renderGraph()
    act(() => forceGraphProps.onNodeHover(renderNode(1)))

    // Background node (3): first globalAlpha assignment is the dim.
    const bgCtx = makeFakeCtx()
    forceGraphProps.nodeCanvasObject(renderNode(3), bgCtx as unknown as CanvasRenderingContext2D)
    expect(bgCtx.alphas[0]).toBe(BACKGROUND_ALPHA)
    expect(bgCtx.alphas[bgCtx.alphas.length - 1]).toBe(1) // reset at the end

    // Foreground node (1, the hovered node): never dimmed.
    const fgCtx = makeFakeCtx()
    forceGraphProps.nodeCanvasObject(renderNode(1), fgCtx as unknown as CanvasRenderingContext2D)
    expect(fgCtx.alphas.every(a => a === 1)).toBe(true)
  })

  it('clears focus on hover-out (links return to resting styling)', () => {
    renderGraph()
    act(() => forceGraphProps.onNodeHover(renderNode(1)))
    expect(forceGraphProps.linkColor(link(3, 4))).toBe(`rgba(148, 163, 184, ${BACKGROUND_ALPHA})`)
    // Hover-out → focusedIds null → 3—4 back to its resting intra grey.
    act(() => forceGraphProps.onNodeHover(null))
    expect(forceGraphProps.linkColor(link(3, 4))).toBe('rgba(148, 163, 184, 0.6)')
  })
})
