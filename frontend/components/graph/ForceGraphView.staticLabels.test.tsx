import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

// PSY-1443: in staticViewport mode the node-label pass must bypass the
// zoom gate (LABEL_MIN_SCALE, 0.7 since PSY-1445). Static surfaces (homepage
// teaser) disable zoom entirely, so a fitted zoom at/below the gate would
// otherwise mean NO visitor could ever see a label — anonymous unlabeled
// circles at rest. Non-static surfaces keep the gate. jsdom has no real
// canvas, so we capture the `onRenderFramePost` callback at the ForceGraph2D
// boundary and drive it with a mock 2D context.

const h = vi.hoisted(() => ({
  lastProps: {} as Record<string, unknown>,
}))

vi.mock('next/dynamic', async () => {
  const React = await import('react')
  return {
    default: () =>
      React.forwardRef(function ForceGraph2DStub(
        props: Record<string, unknown>,
        _ref: React.Ref<unknown>
      ) {
        React.useImperativeHandle(_ref, () => ({
          graph2ScreenCoords: (x: number, y: number) => ({
            x: x + 100,
            y: y + 50,
          }),
          resumeAnimation: () => {},
        }))
        // Test harness capture: assertions read the dynamic-boundary props
        // after React completes this render.
        // eslint-disable-next-line react-hooks/immutability
        h.lastProps = props
        return React.createElement('div', { 'data-testid': 'force-graph' })
      }),
  }
})

import {
  ForceGraphView,
  type ForceGraphViewProps,
  type GraphNode,
} from './ForceGraphView'
import { LABEL_MIN_SCALE } from './graphLabels'

const nodes: GraphNode[] = [
  { id: 1, name: 'Alpha', slug: 'alpha', upcoming_show_count: 0 },
  { id: 2, name: 'Beta', slug: 'beta', upcoming_show_count: 0 },
]

const renderGraph = (
  staticViewport?: boolean,
  extraProps: Partial<ForceGraphViewProps> = {}
) =>
  renderWithProviders(
    <ForceGraphView
      nodes={nodes}
      links={[{ source_id: 1, target_id: 2, type: 'similar' }]}
      containerWidth={1024}
      ariaLabel="test graph"
      onNodeClick={() => {}}
      staticViewport={staticViewport}
      {...extraProps}
    />
  )

function makeCtx() {
  return {
    save: vi.fn(),
    restore: vi.fn(),
    measureText: vi.fn(() => ({ width: 40 })),
    strokeText: vi.fn(),
    fillText: vi.fn(),
    fillRect: vi.fn(),
    beginPath: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
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
    globalScale: number
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
  it('renders labels below the zoom gate in static-viewport mode, collision-culled', () => {
    renderGraph(true)
    const fillText = paintLabels(LABEL_MIN_SCALE - 0.2)
    const drawn = fillText.mock.calls.map(c => c[0])
    // The simulation hasn't run in jsdom, so both nodes sit at (0,0): their
    // label boxes overlap and the collision cull draws only the first (stable
    // order among equal-degree nodes). Static mode bypasses the zoom gate but
    // NOT the overlap culling.
    expect(drawn).toEqual(['Alpha'])
  })

  it('renders labels at exactly zoom 1.0 (the default when fit is skipped) in static mode', () => {
    renderGraph(true)
    const fillText = paintLabels(1.0)
    expect(fillText.mock.calls.map(c => c[0])).toContain('Alpha')
  })

  it('always draws every curated-map label and applies per-node typography', () => {
    renderGraph(true, {
      forceNodeLabels: true,
      nodeLabelStyles: new Map([
        [1, { fontSize: 17, fontWeight: 600 }],
        [2, { fontSize: 13, fontWeight: 500 }],
      ]),
    })
    const fillText = paintLabels(1)
    expect(fillText.mock.calls.map(call => call[0])).toEqual(['Alpha', 'Beta'])
  })

  it('keeps all curated-map labels visible while hover-focus is active', () => {
    renderGraph(true, { forceNodeLabels: true, links: [] })
    act(() => {
      ;(h.lastProps.onNodeHover as (node: GraphNode) => void)(
        { ...nodes[0], x: 0, y: 0 } as GraphNode,
      )
    })
    expect(paintLabels(1).mock.calls.map(call => call[0])).toEqual(['Alpha', 'Beta'])
  })

  it('includes a curated visible label in the node pointer target', () => {
    renderGraph(true, {
      forceNodeLabels: true,
      nodeLabelStyles: new Map([[1, { fontSize: 17, fontWeight: 600 }]]),
    })
    const paintPointerArea = h.lastProps.nodePointerAreaPaint as (
      node: GraphNode,
      color: string,
      ctx: CanvasRenderingContext2D,
    ) => void
    const ctx = makeCtx()
    paintPointerArea({ ...nodes[0], x: 100, y: 50 } as GraphNode, '#hit', ctx)
    expect(ctx.fillRect).toHaveBeenCalled()
  })

  it('anchors opt-in DOM overlays to the node’s canvas position', () => {
    renderGraph(true, {
      nodes: [
        {
          ...nodes[0],
          x: 12,
          y: 34,
        } as GraphNode,
      ],
      links: [],
      nodeOverlays: new Map([
        [1, <span key="crescent-ballroom">Fri · Crescent Ballroom</span>],
      ]),
    })
    act(() => {
      paintLabels(1)
    })
    const overlay = screen.getByText('Fri · Crescent Ballroom').parentElement
    expect(overlay).toHaveStyle({ left: '112px', top: '84px' })
  })

  it('anchors overlays even when a dynamic graph is below the label zoom gate', () => {
    renderGraph(false, {
      nodes: [{ ...nodes[0], x: 12, y: 34 } as GraphNode],
      links: [],
      nodeOverlays: new Map([
        [1, <span key="valley-bar">Sat · Valley Bar</span>],
      ]),
    })
    act(() => {
      paintLabels(LABEL_MIN_SCALE)
    })
    expect(screen.getByText('Sat · Valley Bar').parentElement).toHaveStyle({
      left: '112px',
      top: '84px',
    })
  })

  it('keeps outward overlays inside the clipped graph frame', () => {
    renderGraph(true, {
      nodes: [{ ...nodes[0], x: 12, y: 34 } as GraphNode],
      links: [],
      nodeOverlays: new Map([
        [1, <span key="crescent-ballroom">Fri · Crescent Ballroom</span>],
      ]),
      nodeOverlayPlacement: 'outward',
      nodeOverlayOutwardClearance: 192,
    })
    act(() => {
      paintLabels(1)
    })
    expect(
      screen.getByText('Fri · Crescent Ballroom').parentElement
    ).toHaveStyle({
      left: '192px',
      top: '84px',
      transform: 'translate(calc(-100% - 12px), -50%)',
    })
  })

  it('offers keyboard controls for every node when the curated map opts in', () => {
    const onNodeClick = vi.fn()
    renderGraph(true, { showAccessibleNodeControls: true, onNodeClick })
    const control = screen.getByRole('button', { name: 'Beta' })
    expect(control).toHaveClass('focus-visible:ring-2')
    expect(control.closest('ul')).toHaveClass('focus-within:opacity-100')
    control.click()
    expect(onNodeClick).toHaveBeenCalledWith(expect.objectContaining({ id: 2 }))
  })

  it('keeps the zoom gate on non-static surfaces: no labels at zoom <= LABEL_MIN_SCALE (PSY-1445)', () => {
    renderGraph(false)
    expect(paintLabels(LABEL_MIN_SCALE)).not.toHaveBeenCalled()
  })

  it('non-static surfaces label past the gate (zoom > LABEL_MIN_SCALE) — earlier than the old 1.0 gate', () => {
    renderGraph(false)
    expect(
      paintLabels(LABEL_MIN_SCALE + 0.1).mock.calls.map(c => c[0])
    ).toContain('Alpha')
  })
})
