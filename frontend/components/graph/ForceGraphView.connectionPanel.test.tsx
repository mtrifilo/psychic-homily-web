import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, fireEvent, screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

// PSY-1334: the click-to-inspect ConnectionPanel contract on ForceGraphView —
// edge click opens the pair card (opt-in via showConnectionPanel), background/
// node clicks close it, a second edge click re-targets, and the panel lists
// ALL typed connections for the pair (including types hidden from the sim).

const h = vi.hoisted(() => ({
  graph: {
    pauseAnimation: vi.fn(),
    resumeAnimation: vi.fn(),
    d3Force: vi.fn(),
    d3ReheatSimulation: vi.fn(),
    zoomToFit: vi.fn(),
    zoom: vi.fn(() => 1),
    centerAt: vi.fn(() => ({ x: 0, y: 0 })),
    getGraphBbox: vi.fn(() => ({ x: [-100, 100], y: [-100, 100] })),
  },
  lastProps: { value: null as Record<string, unknown> | null },
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
        React.useEffect(() => {
          h.lastProps.value = props
        })
        return React.createElement('canvas', { 'data-testid': 'stub-canvas' })
      }),
  }
})

vi.mock('@/features/artists/hooks/useReducedMotion', () => ({
  useReducedMotion: () => false,
}))

import { ForceGraphView, type GraphNode } from './ForceGraphView'

const nodes: GraphNode[] = [
  { id: 1, name: 'Dehd', slug: 'dehd', upcoming_show_count: 0 },
  { id: 2, name: 'Lifeguard', slug: 'lifeguard', upcoming_show_count: 0 },
  { id: 3, name: 'Horsegirl', slug: 'horsegirl', upcoming_show_count: 0 },
]

const links = [
  { source_id: 1, target_id: 2, type: 'shared_bills', score: 0.3, detail: { shared_count: 3 } },
  { source_id: 1, target_id: 2, type: 'shared_label', detail: { shared_count: 1, label_names: 'Fire Talk' } },
  { source_id: 2, target_id: 3, type: 'shared_bills', score: 0.1, detail: { shared_count: 1 } },
]

const baseProps = {
  nodes,
  links,
  containerWidth: 800,
  ariaLabel: 'test graph',
  onNodeClick: () => {},
  showConnectionPanel: true,
}

const renderGraph = (props: Partial<React.ComponentProps<typeof ForceGraphView>> = {}) =>
  renderWithProviders(<ForceGraphView {...baseProps} {...props} />)

const clickLink = (source: number, target: number, type = 'shared_bills') => {
  act(() => {
    ;(h.lastProps.value!.onLinkClick as (l: unknown) => void)({ source, target, type })
  })
}

beforeEach(() => {
  h.lastProps.value = null
  vi.clearAllMocks()
})

describe('ForceGraphView connection panel', () => {
  it('opens on edge click with EVERY typed connection for the pair', () => {
    renderGraph()
    clickLink(1, 2)
    expect(
      screen.getByRole('region', { name: 'Why Dehd and Lifeguard are connected' }),
    ).toBeInTheDocument()
    // Both types between the pair render, not just the clicked one.
    expect(screen.getByText('Shared Bills')).toBeInTheDocument()
    expect(screen.getByText('Shared Label')).toBeInTheDocument()
  })

  it('resolves endpoints when d3 has replaced ids with node objects', () => {
    renderGraph()
    act(() => {
      ;(h.lastProps.value!.onLinkClick as (l: unknown) => void)({
        source: { id: 1 },
        target: { id: 2 },
        type: 'shared_bills',
      })
    })
    expect(screen.getByRole('region', { name: /Dehd and Lifeguard/ })).toBeInTheDocument()
  })

  it('re-targets on a second edge click (one panel at a time)', () => {
    renderGraph()
    clickLink(1, 2)
    clickLink(2, 3)
    expect(screen.queryByRole('region', { name: /Dehd and Lifeguard/ })).not.toBeInTheDocument()
    expect(
      screen.getByRole('region', { name: 'Why Lifeguard and Horsegirl are connected' }),
    ).toBeInTheDocument()
  })

  it('closes on background click and on node click', () => {
    renderGraph()
    clickLink(1, 2)
    expect(screen.getByRole('region', { name: /connected/ })).toBeInTheDocument()
    act(() => {
      ;(h.lastProps.value!.onBackgroundClick as () => void)()
    })
    expect(screen.queryByRole('region', { name: /connected/ })).not.toBeInTheDocument()

    clickLink(1, 2)
    expect(screen.getByRole('region', { name: /connected/ })).toBeInTheDocument()
    act(() => {
      ;(h.lastProps.value!.onNodeClick as (n: unknown) => void)(nodes[2])
    })
    expect(screen.queryByRole('region', { name: /connected/ })).not.toBeInTheDocument()
  })

  it('closes via Escape', () => {
    renderGraph()
    clickLink(1, 2)
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('region', { name: /connected/ })).not.toBeInTheDocument()
  })

  it('ignores edge clicks when the surface has not opted in', () => {
    renderGraph({ showConnectionPanel: false })
    clickLink(1, 2)
    expect(screen.queryByRole('region', { name: /connected/ })).not.toBeInTheDocument()
  })

  it('ignores clicks on untyped links (no provenance to show)', () => {
    renderGraph({ links: [{ source_id: 1, target_id: 2, type: '' }] })
    clickLink(1, 2, '')
    expect(screen.queryByRole('region', { name: /connected/ })).not.toBeInTheDocument()
  })

  it('widens the link hit target for the inspect click', () => {
    renderGraph()
    expect(h.lastProps.value!.linkHoverPrecision).toBe(4)
  })

  it('auto-closes when the pair leaves the payload (data refresh)', () => {
    const { rerender } = renderGraph()
    clickLink(1, 2)
    expect(screen.getByRole('region', { name: /Dehd and Lifeguard/ })).toBeInTheDocument()
    rerender(
      <ForceGraphView
        {...baseProps}
        nodes={nodes.filter(n => n.id !== 2)}
        links={links.filter(l => l.source_id !== 2 && l.target_id !== 2)}
      />,
    )
    expect(screen.queryByRole('region', { name: /connected/ })).not.toBeInTheDocument()
  })
})
