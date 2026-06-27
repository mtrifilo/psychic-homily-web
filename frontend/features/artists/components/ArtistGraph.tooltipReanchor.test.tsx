import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, fireEvent, screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { ArtistGraph } from '../types'

// PSY-1220: while the d3-force sim is live, a hovered node drifts but onNodeHover doesn't re-fire
// for a stationary cursor, so the anchored tooltip would strand. ArtistGraph re-anchors it each
// engine tick (onEngineTick) and dismisses on filter/resize reheat (parity with ForceGraphView).
// Drive the canvas callbacks the way the real ForceGraph2D would and assert the tooltip's anchor.

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let forceGraphProps: any = null

vi.mock('next/dynamic', () => ({
  default: () =>
    function ForceGraph2DStub(props: Record<string, unknown>) {
      forceGraphProps = props
      return <div data-testid="force-graph" />
    },
}))

// Reflect the node's CURRENT coords so a moved node yields a NEW anchor — exactly what the
// onEngineTick re-anchor depends on (the real helper does the same via graph2ScreenCoords).
// Center / null node → null (hover-out), matching production.
vi.mock('@/components/graph/nodeTooltip', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/components/graph/nodeTooltip')>()
  return {
    ...actual,
    nodeTooltipPlacement: (_graph: unknown, _container: unknown, node: { x?: number; y?: number; isCenter?: boolean } | null) =>
      node && !node.isCenter ? { x: node.x ?? 0, y: node.y ?? 0, flipX: false, flipY: false } : null,
  }
})

vi.mock('next/link', () => ({
  default: ({ href, children, className }: { href: string; children: React.ReactNode; className?: string }) => (
    <a href={href} className={className}>{children}</a>
  ),
}))

import { ArtistGraphVisualization } from './ArtistGraph'

const graphData: ArtistGraph = {
  center: { id: 1, name: 'Gatecreeper', slug: 'gatecreeper', city: 'Phoenix', state: 'AZ', upcoming_show_count: 3 },
  nodes: [{ id: 2, name: 'Frozen Soul', slug: 'frozen-soul', city: 'Fort Worth', state: 'TX', upcoming_show_count: 1 }],
  links: [{ source_id: 1, target_id: 2, type: 'similar', score: 0.85, votes_up: 8, votes_down: 2 }],
  user_votes: {},
}

// A fresh satellite node per test (mutable x/y — d3-force mutates these in place during settle).
const makeSatellite = () => ({
  id: 2, name: 'Frozen Soul', slug: 'frozen-soul', city: 'Fort Worth', state: 'TX',
  upcoming_show_count: 1, isCenter: false, val: 4, x: 100, y: 100,
})

const tooltip = () => screen.queryByTestId('artist-node-tooltip')
const renderViz = (activeTypes = new Set(['similar']), containerWidth = 1024) =>
  renderWithProviders(<ArtistGraphVisualization data={graphData} activeTypes={activeTypes} containerWidth={containerWidth} />)

describe('ArtistGraph tooltip re-anchor + reheat dismiss (PSY-1220)', () => {
  beforeEach(() => { forceGraphProps = null })
  afterEach(() => { forceGraphProps = null })

  it('re-anchors the open tooltip to the node’s new position on each engine tick (settle drift)', () => {
    renderViz()
    const node = makeSatellite()
    act(() => forceGraphProps.onNodeHover(node))
    expect(tooltip()).toBeInTheDocument()
    expect(tooltip()!.style.left).toBe('100px')

    // The sim ticks and d3-force drifts the node; onEngineTick must follow it.
    node.x = 175
    node.y = 180
    act(() => forceGraphProps.onEngineTick())
    expect(tooltip()!.style.left).toBe('175px')
    expect(tooltip()!.style.top).toBe('180px')
  })

  it('does NOT re-anchor while the pointer is over the (hoverable) tooltip — it would escape the cursor', () => {
    renderViz()
    const node = makeSatellite()
    act(() => forceGraphProps.onNodeHover(node))
    // Cursor moves onto the tooltip to reach its link (PSY-1218) → overTooltipRef = true.
    fireEvent.mouseEnter(tooltip()!)
    node.x = 175
    act(() => forceGraphProps.onEngineTick())
    // Gate held: the tooltip stayed put under the cursor instead of chasing the node.
    expect(tooltip()!.style.left).toBe('100px')
  })

  it('dismisses the tooltip on a filter toggle (activeTypes → graphData reheat)', () => {
    const { rerender } = renderViz(new Set(['similar']))
    act(() => forceGraphProps.onNodeHover(makeSatellite()))
    expect(tooltip()).toBeInTheDocument()
    // Toggling an edge-type pill reheats the layout — dismiss so the tooltip doesn't strand.
    rerender(<ArtistGraphVisualization data={graphData} activeTypes={new Set(['similar', 'shared_label'])} containerWidth={1024} />)
    expect(tooltip()).not.toBeInTheDocument()
  })

  it('dismisses the tooltip on resize (containerWidth change)', () => {
    // Same activeTypes IDENTITY across the rerender so graphData stays stable — this isolates the
    // resize (containerWidth) path from the filter path (a fresh Set would dismiss via graphData).
    const types = new Set(['similar'])
    const { rerender } = renderWithProviders(
      <ArtistGraphVisualization data={graphData} activeTypes={types} containerWidth={1024} />,
    )
    act(() => forceGraphProps.onNodeHover(makeSatellite()))
    expect(tooltip()).toBeInTheDocument()
    rerender(<ArtistGraphVisualization data={graphData} activeTypes={types} containerWidth={760} />)
    expect(tooltip()).not.toBeInTheDocument()
  })
})
