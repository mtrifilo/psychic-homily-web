import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

// PSY-1321: the initial viewport must frame the settled layout when — and
// only when — that layout is out of view. These tests pin the contract:
// fit on the first engine stop with out-of-view content; spend-without-fit
// when the content is already framed (inline mounts stay untouched); stay
// ARMED over empty/bbox-less graphs; re-arm on dimension change; permanent
// user cancel via canvas pointer/wheel; instant fit on the reduced-motion
// path (whose paused engine never reaches onEngineStop).

// Default stub viewport: zoom 1 centered at origin over an 800×560 canvas →
// visible x ∈ [-400, 400], y ∈ [-280, 280]; the out-of-view bbox
// stays out even at the re-arm test's 1400×900 overlay dims (±700 × ±450).
const OUT_OF_VIEW_BBOX = { x: [-800, 800], y: [-100, 100] }
const IN_VIEW_BBOX = { x: [-100, 100], y: [-100, 100] }

const h = vi.hoisted(() => ({
  graph: {
    pauseAnimation: vi.fn(),
    resumeAnimation: vi.fn(),
    d3Force: vi.fn(),
    d3ReheatSimulation: vi.fn(),
    zoomToFit: vi.fn(),
    zoom: vi.fn(() => 1),
    centerAt: vi.fn(() => ({ x: 0, y: 0 })),
    getGraphBbox: vi.fn(() => ({ x: [-800, 800], y: [-100, 100] })),
  },
  lastProps: { value: null as Record<string, unknown> | null },
  reducedMotion: { value: false },
}))

vi.mock('next/dynamic', async () => {
  const React = await import('react')
  return {
    default: () =>
      React.forwardRef(function ForceGraph2DStub(
        props: Record<string, unknown>,
        ref: React.Ref<unknown>,
      ) {
        React.useImperativeHandle(ref, () => h.graph)
        // Captured in an effect (not during render) to satisfy
        // react-hooks/immutability; tests read it after render anyway.
        React.useEffect(() => {
          h.lastProps.value = props
        })
        return React.createElement('canvas', { 'data-testid': 'stub-canvas' })
      }),
  }
})

vi.mock('@/features/artists/hooks/useReducedMotion', () => ({
  useReducedMotion: () => h.reducedMotion.value,
}))

import { ForceGraphView, type GraphNode } from './ForceGraphView'

const nodes: GraphNode[] = [
  { id: 1, name: 'Alpha', slug: 'alpha', upcoming_show_count: 0 },
  { id: 2, name: 'Beta', slug: 'beta', upcoming_show_count: 0 },
]

const links = [{ source_id: 1, target_id: 2, type: 'shared_bills' }]

const stopEngine = () => {
  ;(h.lastProps.value!.onEngineStop as () => void)()
}

const baseProps = {
  nodes,
  links,
  containerWidth: 800,
  ariaLabel: 'test graph',
  onNodeClick: () => {},
}

const renderGraph = (props: Partial<React.ComponentProps<typeof ForceGraphView>> = {}) =>
  renderWithProviders(<ForceGraphView {...baseProps} {...props} />)

describe('ForceGraphView zoomToFit (PSY-1321)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.lastProps.value = null
    h.reducedMotion.value = false
    h.graph.zoom.mockReturnValue(1)
    h.graph.centerAt.mockReturnValue({ x: 0, y: 0 })
    h.graph.getGraphBbox.mockReturnValue(OUT_OF_VIEW_BBOX)
  })

  it('fits once on the first engine stop when content is out of view, not on later stops', () => {
    renderGraph()
    expect(h.graph.zoomToFit).not.toHaveBeenCalled()

    stopEngine()
    expect(h.graph.zoomToFit).toHaveBeenCalledTimes(1)
    expect(h.graph.zoomToFit).toHaveBeenCalledWith(400, 40)

    // A reheat → second settle must not re-yank the viewport.
    stopEngine()
    expect(h.graph.zoomToFit).toHaveBeenCalledTimes(1)
  })

  it('spends the shot WITHOUT fitting when the content is already in view (inline mounts untouched)', () => {
    h.graph.getGraphBbox.mockReturnValue(IN_VIEW_BBOX)
    renderGraph()

    stopEngine()
    stopEngine()
    expect(h.graph.zoomToFit).not.toHaveBeenCalled()
  })

  it('stays ARMED over an empty graph, then fits once real data settles', () => {
    const { rerender } = renderGraph({ nodes: [], links: [] })
    stopEngine()
    expect(h.graph.zoomToFit).not.toHaveBeenCalled()

    rerender(<ForceGraphView {...baseProps} />)
    stopEngine()
    expect(h.graph.zoomToFit).toHaveBeenCalledTimes(1)
  })

  it('stays ARMED when node positions are not initialized yet (NaN-shaped bbox)', () => {
    // force-graph never returns null for a non-empty graph — uninitialized
    // positions yield {x:[undefined,undefined],...}; the guard must catch
    // that shape or the fit corrupts the viewport with centerAt(NaN, NaN).
    h.graph.getGraphBbox.mockReturnValueOnce({
      x: [undefined, undefined],
      y: [undefined, undefined],
    } as never)
    renderGraph()
    stopEngine()
    expect(h.graph.zoomToFit).not.toHaveBeenCalled()

    stopEngine()
    expect(h.graph.zoomToFit).toHaveBeenCalledTimes(1)
  })

  it('a canvas wheel cancels the pending fit (trackpad zoom is the common desktop takeover)', () => {
    renderGraph()
    fireEvent.wheel(screen.getByTestId('stub-canvas'))

    stopEngine()
    expect(h.graph.zoomToFit).not.toHaveBeenCalled()
  })

  it('re-arms when the canvas dimensions change (the overlay path)', () => {
    const { rerender } = renderGraph()
    stopEngine()
    expect(h.graph.zoomToFit).toHaveBeenCalledTimes(1)

    rerender(<ForceGraphView {...baseProps} containerWidth={1400} height={900} />)
    stopEngine()
    expect(h.graph.zoomToFit).toHaveBeenCalledTimes(2)
  })

  it('a canvas pointerdown cancels fitting for the REST of the mount, surviving dimension changes', () => {
    const { rerender } = renderGraph()
    fireEvent.pointerDown(screen.getByTestId('stub-canvas'))

    stopEngine()
    expect(h.graph.zoomToFit).not.toHaveBeenCalled()

    // Dimension change must NOT re-arm over a user-owned viewport.
    rerender(<ForceGraphView {...baseProps} containerWidth={1400} height={900} />)
    stopEngine()
    expect(h.graph.zoomToFit).not.toHaveBeenCalled()
  })

  it('a pointerdown on the edge legend (a non-canvas overlay child) does NOT cancel the fit', () => {
    renderGraph({ showEdgeLegend: true })
    // The legend renders because the payload carries a typed link.
    // ^Shared Bills pins the row toggle (named by its visible content); the
    // PSY-1334 solo button's aria-label starts with "Show only".
    fireEvent.pointerDown(screen.getByRole('button', { name: /^shared bills/i }))

    stopEngine()
    expect(h.graph.zoomToFit).toHaveBeenCalledTimes(1)
  })

  it('applies an instant fit for reduced-motion users (paused engine never stops)', () => {
    h.reducedMotion.value = true
    renderGraph()
    expect(h.graph.pauseAnimation).toHaveBeenCalled()
    expect(h.graph.zoomToFit).toHaveBeenCalledWith(0, 40)
  })

  // PSY-1380: react-force-graph's default forceCenter balances the centroid of
  // ALL nodes to (0,0). When most nodes are isolates pinned to the bottom shelf,
  // that shoves the few connected nodes far up to compensate, ballooning the
  // bbox vertically until zoomToFit zooms out and the graph reads as empty. The
  // cluster forces already anchor non-isolates, so the center force is removed.
  it('removes the built-in center force so the isolate shelf cannot balloon the layout (PSY-1380)', () => {
    renderGraph()
    expect(h.graph.d3Force).toHaveBeenCalledWith('center', null)
  })
})
