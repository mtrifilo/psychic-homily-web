import { describe, it, expect, vi, beforeEach } from 'vitest'
import { waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

// PSY-1296: lock in the canvas a11y attributes (role="img" + aria-label).
// The canvas is created asynchronously by the dynamic-imported force-graph
// chunk — the original effect ran once, missed the not-yet-present canvas,
// and silently never applied the label (caught in live verification). These
// tests pin BOTH paths: canvas already present when the effect runs, and
// canvas appearing later (the MutationObserver path), plus label updates
// after mount.

const h = vi.hoisted(() => ({
  graph: {
    pauseAnimation: vi.fn(),
    resumeAnimation: vi.fn(),
    d3Force: vi.fn(),
    d3ReheatSimulation: vi.fn(),
  },
  // When true, the stub renders its canvas on a later tick — modelling the
  // real chunk's async canvas creation.
  deferCanvas: { value: false },
}))

vi.mock('next/dynamic', async () => {
  const React = await import('react')
  return {
    default: () =>
      React.forwardRef(function ForceGraph2DStub(
        _props: Record<string, unknown>,
        ref: React.Ref<unknown>,
      ) {
        React.useImperativeHandle(ref, () => h.graph)
        const [showCanvas, setShowCanvas] = React.useState(!h.deferCanvas.value)
        React.useEffect(() => {
          if (showCanvas) return
          const t = setTimeout(() => setShowCanvas(true), 5)
          return () => clearTimeout(t)
        }, [showCanvas])
        return showCanvas
          ? React.createElement('canvas', { 'data-testid': 'stub-canvas' })
          : React.createElement('div')
      }),
  }
})

vi.mock('@/features/artists/hooks/useReducedMotion', () => ({
  useReducedMotion: () => false,
}))

import { ForceGraphView, type GraphNode } from './ForceGraphView'

const nodes: GraphNode[] = [
  { id: 1, name: 'Alpha', slug: 'alpha', upcoming_show_count: 0 },
]

const renderGraph = (ariaLabel: string) =>
  renderWithProviders(
    <ForceGraphView
      nodes={nodes}
      links={[]}
      containerWidth={1024}
      ariaLabel={ariaLabel}
      onNodeClick={() => {}}
    />,
  )

describe('ForceGraphView canvas a11y attributes (PSY-1296)', () => {
  beforeEach(() => {
    h.deferCanvas.value = false
  })

  it('labels a canvas that already exists when the effect runs', () => {
    const { container } = renderGraph('graph of things')
    const canvas = container.querySelector('canvas')!
    expect(canvas).toHaveAttribute('role', 'img')
    expect(canvas).toHaveAttribute('aria-label', 'graph of things')
  })

  it('labels a canvas that appears AFTER the effect runs (the async-chunk path)', async () => {
    h.deferCanvas.value = true
    const { container } = renderGraph('late canvas graph')
    expect(container.querySelector('canvas')).toBeNull()

    await waitFor(() => {
      const canvas = container.querySelector('canvas')
      expect(canvas).not.toBeNull()
      expect(canvas).toHaveAttribute('role', 'img')
      expect(canvas).toHaveAttribute('aria-label', 'late canvas graph')
    })
  })

  it('updates the label in place when ariaLabel changes after mount', () => {
    const { container, rerender } = renderGraph('before')
    rerender(
      <ForceGraphView
        nodes={nodes}
        links={[]}
        containerWidth={1024}
        ariaLabel="after"
        onNodeClick={() => {}}
      />,
    )
    expect(container.querySelector('canvas')).toHaveAttribute(
      'aria-label',
      'after',
    )
  })
})
