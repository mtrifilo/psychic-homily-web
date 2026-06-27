import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

// PSY-1220: ForceGraphView re-anchors its open tooltip to the hovered node's CURRENT position
// each engine tick (onEngineTick), so the tooltip follows a node that drifts during the settle
// instead of stranding. (ForceGraphView's tooltip is pointer-events-none, so — unlike ArtistGraph
// — there's no over-the-tooltip gate.) Drive the captured canvas callbacks to verify the wiring.

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let forceGraphProps: any = null

vi.mock('next/dynamic', () => ({
  default: () =>
    function ForceGraph2DStub(props: Record<string, unknown>) {
      forceGraphProps = props
      return <div data-testid="force-graph" />
    },
}))

// Reflect the node's CURRENT coords so a moved node yields a NEW anchor (what onEngineTick
// re-anchor relies on); null for hover-out.
vi.mock('./nodeTooltip', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./nodeTooltip')>()
  return {
    ...actual,
    nodeTooltipPlacement: (_graph: unknown, _container: unknown, node: { x?: number; y?: number } | null) =>
      node ? { x: node.x ?? 0, y: node.y ?? 0, flipX: false, flipY: false } : null,
  }
})

import { ForceGraphView, type GraphNode } from './ForceGraphView'

const nodes: GraphNode[] = [{ id: 1, name: 'Alpha', slug: 'alpha', upcoming_show_count: 0 }]
const renderGraph = () =>
  renderWithProviders(
    <ForceGraphView nodes={nodes} links={[]} containerWidth={1024} ariaLabel="test graph" onNodeClick={() => {}} />,
  )

// The tooltip's outer (positioned) div is the parent of the node-name div.
const tooltipBox = () => screen.getByText('Alpha').parentElement as HTMLElement

describe('ForceGraphView tooltip re-anchor (PSY-1220)', () => {
  beforeEach(() => { forceGraphProps = null })

  it('re-anchors the open tooltip to the node’s new position on each engine tick (settle drift)', () => {
    renderGraph()
    const node = { id: 1, name: 'Alpha', slug: 'alpha', upcoming_show_count: 0, cluster_id: 'other', is_isolate: false, x: 100, y: 100 }
    act(() => forceGraphProps.onNodeHover(node))
    expect(tooltipBox().style.left).toBe('100px')

    node.x = 175
    node.y = 180
    act(() => forceGraphProps.onEngineTick())
    expect(tooltipBox().style.left).toBe('175px')
    expect(tooltipBox().style.top).toBe('180px')
  })
})
