import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderWithProviders } from '@/test/utils'

// PSY-1443: in staticViewport mode the node-label pass must bypass the
// `globalScale <= 1.0` zoom gate. Static surfaces (homepage teaser) disable
// zoom entirely, so a fitted zoom at/below 1.0 would otherwise mean NO
// visitor could ever see a label — anonymous unlabeled circles at rest.
// Non-static surfaces keep the gate unchanged. jsdom has no real canvas, so
// we capture the `onRenderFramePost` callback at the ForceGraph2D boundary
// and drive it with a mock 2D context.

const h = vi.hoisted(() => ({
  lastProps: {} as Record<string, unknown>,
}))

vi.mock('next/dynamic', async () => {
  const React = await import('react')
  return {
    default: () =>
      React.forwardRef(function ForceGraph2DStub(props: Record<string, unknown>, _ref: React.Ref<unknown>) {
        h.lastProps = props
        return React.createElement('div', { 'data-testid': 'force-graph' })
      }),
  }
})

import { ForceGraphView, type GraphNode } from './ForceGraphView'

const nodes: GraphNode[] = [
  { id: 1, name: 'Alpha', slug: 'alpha', upcoming_show_count: 0 },
  { id: 2, name: 'Beta', slug: 'beta', upcoming_show_count: 0 },
]

const renderGraph = (staticViewport?: boolean) =>
  renderWithProviders(
    <ForceGraphView
      nodes={nodes}
      links={[{ source_id: 1, target_id: 2, type: 'similar' }]}
      containerWidth={1024}
      ariaLabel="test graph"
      onNodeClick={() => {}}
      staticViewport={staticViewport}
    />,
  )

function makeCtx() {
  return {
    save: vi.fn(),
    restore: vi.fn(),
    measureText: vi.fn(() => ({ width: 40 })),
    strokeText: vi.fn(),
    fillText: vi.fn(),
    font: '',
    textAlign: '',
    textBaseline: '',
    lineJoin: '',
    lineWidth: 0,
    strokeStyle: '',
    fillStyle: '',
  } as unknown as CanvasRenderingContext2D
}

function paintLabels(globalScale: number) {
  const frame = h.lastProps.onRenderFramePost as (
    ctx: CanvasRenderingContext2D,
    globalScale: number,
  ) => void
  expect(typeof frame).toBe('function')
  const ctx = makeCtx()
  frame(ctx, globalScale)
  return ctx.fillText as ReturnType<typeof vi.fn>
}

beforeEach(() => {
  h.lastProps = {}
})

describe('ForceGraphView static-viewport label gate (PSY-1443)', () => {
  it('renders labels at fitted zoom <= 1.0 in static-viewport mode, collision-culled', () => {
    renderGraph(true)
    const fillText = paintLabels(0.6)
    const drawn = fillText.mock.calls.map((c) => c[0])
    // The simulation hasn't run in jsdom, so both nodes sit at (0,0): their
    // label boxes overlap and the collision cull draws only the first (stable
    // order among equal-degree nodes). Static mode bypasses the zoom gate but
    // NOT the overlap culling.
    expect(drawn).toEqual(['Alpha'])
  })

  it('renders labels at exactly zoom 1.0 (the default when fit is skipped) in static mode', () => {
    renderGraph(true)
    const fillText = paintLabels(1.0)
    expect(fillText.mock.calls.map((c) => c[0])).toContain('Alpha')
  })

  it('keeps the zoom gate on non-static surfaces: no labels at zoom <= 1.0', () => {
    renderGraph(false)
    expect(paintLabels(1.0)).not.toHaveBeenCalled()
  })

  it('non-static surfaces still label past the gate (zoom > 1.0)', () => {
    renderGraph(false)
    expect(paintLabels(1.5).mock.calls.map((c) => c[0])).toContain('Alpha')
  })
})
