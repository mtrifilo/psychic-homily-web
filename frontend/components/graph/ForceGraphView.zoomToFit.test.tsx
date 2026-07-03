import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

// PSY-1321: the initial viewport must frame the settled layout. These tests
// pin the one-shot contract: fit fires on the first engine stop, does NOT
// re-fire on later stops (reheats), re-arms on a material dimension change,
// and is cancelled when the user takes the viewport (pointer/wheel).

const h = vi.hoisted(() => ({
  graph: {
    pauseAnimation: vi.fn(),
    resumeAnimation: vi.fn(),
    d3Force: vi.fn(),
    d3ReheatSimulation: vi.fn(),
    zoomToFit: vi.fn(),
  },
  // Captured from the stub's props so tests can drive the engine lifecycle.
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

const renderGraph = (props: Partial<React.ComponentProps<typeof ForceGraphView>> = {}) =>
  renderWithProviders(
    <ForceGraphView
      nodes={nodes}
      links={links}
      containerWidth={800}
      ariaLabel="test graph"
      onNodeClick={() => {}}
      {...props}
    />,
  )

describe('ForceGraphView zoomToFit (PSY-1321)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.lastProps.value = null
    h.reducedMotion.value = false
  })

  it('fits once on the first engine stop, not on later stops', () => {
    renderGraph()
    expect(h.graph.zoomToFit).not.toHaveBeenCalled()

    stopEngine()
    expect(h.graph.zoomToFit).toHaveBeenCalledTimes(1)
    expect(h.graph.zoomToFit).toHaveBeenCalledWith(400, 40)

    // A reheat → second settle must not re-yank the viewport.
    stopEngine()
    expect(h.graph.zoomToFit).toHaveBeenCalledTimes(1)
  })

  it('re-arms when the canvas dimensions change (the overlay remount-by-resize path)', () => {
    const { rerender } = renderGraph()
    stopEngine()
    expect(h.graph.zoomToFit).toHaveBeenCalledTimes(1)

    rerender(
      <ForceGraphView
        nodes={nodes}
        links={links}
        containerWidth={1400}
        height={900}
        ariaLabel="test graph"
        onNodeClick={() => {}}
      />,
    )
    stopEngine()
    expect(h.graph.zoomToFit).toHaveBeenCalledTimes(2)
  })

  it('cancels the pending fit once the user touches the viewport', () => {
    const { container } = renderGraph()
    fireEvent.pointerDown(container.firstElementChild!)

    stopEngine()
    expect(h.graph.zoomToFit).not.toHaveBeenCalled()
  })

  it('does not fit an empty graph', () => {
    renderGraph({ nodes: [], links: [] })
    stopEngine()
    expect(h.graph.zoomToFit).not.toHaveBeenCalled()
  })

  it('applies an instant fit for reduced-motion users (paused engine never stops)', () => {
    h.reducedMotion.value = true
    renderGraph()
    expect(h.graph.pauseAnimation).toHaveBeenCalled()
    expect(h.graph.zoomToFit).toHaveBeenCalledWith(0, 40)
  })
})
