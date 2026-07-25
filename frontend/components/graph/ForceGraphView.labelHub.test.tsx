import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderWithProviders } from '@/test/utils'

// PSY-1530: label hubs are canvas paints, which jsdom can't render (the visual
// is verified by screenshots). This suite guards the glue: that a hub draws as a
// SQUARE while artists draw as arcs, and that a hub joins no cluster — so it
// can't be pulled into a hull or hidden by a cluster-legend toggle. Same
// mocked-canvas prop-capture approach as ForceGraphView.playableMarker.test.tsx.
// The home caption and hub predicate are covered in labelHub.test.ts; the
// caption's draw + collision box in graphLabels.test.ts.

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let forceGraphProps: any = null

vi.mock('next/dynamic', () => ({
  default: () =>
    function ForceGraph2DStub(props: Record<string, unknown>) {
      forceGraphProps = props
      return <div data-testid="force-graph" />
    },
}))

import {
  ForceGraphView,
  type GraphCluster,
  type GraphLink,
  type GraphNode,
} from './ForceGraphView'
import { SECTION_LABEL_TIERS } from './graphLabels'
import { graphClusterIdForNode } from './labelHub'

const HUB_ID = 2_000_000_007

const clusters: GraphCluster[] = [
  { id: 'v_1', label: 'Hotel Vegas', size: 6, color_index: 0 },
  { id: 'other', label: 'Other', size: 3, color_index: -1 },
]

const nodes: GraphNode[] = [
  {
    id: 1,
    entity_type: 'artist',
    name: 'Borzoi',
    slug: 'borzoi',
    upcoming_show_count: 0,
    cluster_id: 'v_1',
  },
  {
    id: HUB_ID,
    entity_type: 'label',
    name: '12XU',
    slug: '12xu',
    city: 'Austin',
    state: 'TX',
    country: 'US',
    upcoming_show_count: 0,
    cluster_id: '',
  },
]

const links: GraphLink[] = [
  { source_id: HUB_ID, target_id: 1, type: 'on_label', score: 1 },
]

const renderNode = (node: GraphNode) => ({
  ...node,
  cluster_id: node.cluster_id ?? '',
  is_isolate: false,
  x: 0,
  y: 0,
})

// Records which primitive drew, so a square (rect/roundRect) can be told from
// the artist circle (arc).
function makeFakeCtx() {
  const calls: string[] = []
  const ctx = {
    globalAlpha: 1,
    lineWidth: 0,
    fillStyle: '',
    strokeStyle: '',
    beginPath() {},
    arc() {
      calls.push('arc')
    },
    rect() {
      calls.push('rect')
    },
    roundRect() {
      calls.push('roundRect')
    },
    fill() {},
    stroke() {},
    calls,
  }
  return ctx
}

const renderGraph = () =>
  renderWithProviders(
    <ForceGraphView
      nodes={nodes}
      links={links}
      clusters={clusters}
      containerWidth={1024}
      ariaLabel="test graph"
      onNodeClick={() => {}}
      labelTiers={SECTION_LABEL_TIERS}
    />,
  )

describe('ForceGraphView — label hub marker (PSY-1530)', () => {
  beforeEach(() => {
    forceGraphProps = null
  })
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('draws a label hub as a rounded square, not a circle', () => {
    renderGraph()
    const ctx = makeFakeCtx()
    forceGraphProps.nodeCanvasObject(
      renderNode(nodes[1]),
      ctx as unknown as CanvasRenderingContext2D,
    )
    expect(ctx.calls.some(c => c === 'roundRect' || c === 'rect')).toBe(true)
    expect(ctx.calls).not.toContain('arc')
  })

  it('draws an artist node as a circle', () => {
    renderGraph()
    const ctx = makeFakeCtx()
    forceGraphProps.nodeCanvasObject(
      renderNode(nodes[0]),
      ctx as unknown as CanvasRenderingContext2D,
    )
    expect(ctx.calls).toContain('arc')
    expect(ctx.calls.some(c => c === 'roundRect' || c === 'rect')).toBe(false)
  })

  // Cluster participation is asserted against the pure resolver rather than
  // the canvas prop: graphData is gated on `canvasReady`, which never flips
  // with a stubbed (canvas-less) ForceGraph2D in jsdom.
  it('resolves a label hub to no cluster, and artists to theirs', () => {
    expect(graphClusterIdForNode({ entity_type: 'label' }, 'other')).toBe('')
    expect(
      graphClusterIdForNode({ entity_type: 'label', cluster_id: 'v_1' }, 'other'),
    ).toBe('')
    expect(
      graphClusterIdForNode({ entity_type: 'artist', cluster_id: 'v_1' }, 'other'),
    ).toBe('v_1')
    // Ungrouped artists still fall back to the "other" bucket.
    expect(graphClusterIdForNode({ entity_type: 'artist' }, 'other')).toBe('other')
  })
})
