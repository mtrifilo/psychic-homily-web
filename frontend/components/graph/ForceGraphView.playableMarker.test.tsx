import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderWithProviders } from '@/test/utils'

// PSY-1379: the playable-audio marker is a canvas paint, which jsdom can't render
// (the visual is verified by screenshots). This suite guards the glue: that
// nodeCanvasObject strokes the playable ring iff node.has_playable_audio. Same
// mocked-canvas prop-capture approach as ForceGraphView.hoverFocus.test.tsx.

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let forceGraphProps: any = null

vi.mock('next/dynamic', () => ({
  default: () =>
    function ForceGraph2DStub(props: Record<string, unknown>) {
      forceGraphProps = props
      return <div data-testid="force-graph" />
    },
}))

import { ForceGraphView, type GraphNode, type GraphLink } from './ForceGraphView'
import { PLAYABLE_RING_COLOR } from './graphMarkers'

const nodes: GraphNode[] = [
  { id: 1, name: 'Playable', slug: 'playable', upcoming_show_count: 0, has_playable_audio: true },
  { id: 2, name: 'Silent', slug: 'silent', upcoming_show_count: 0, has_playable_audio: false },
]
const links: GraphLink[] = []

// RenderNode-shaped object for the nodeCanvasObject draw.
const renderNode = (id: number, playable: boolean) => ({
  id,
  name: `n${id}`,
  slug: `n${id}`,
  upcoming_show_count: 0,
  cluster_id: 'other',
  is_isolate: false,
  has_playable_audio: playable,
  x: 0,
  y: 0,
})

// Fake ctx recording the strokeStyle in effect at each stroke() call, so the
// playable ring's violet stroke can be distinguished from the node's cluster-fill stroke.
function makeFakeCtx() {
  const strokes: string[] = []
  const ctx = {
    globalAlpha: 1,
    beginPath() {},
    arc() {},
    fill() {},
    stroke() {
      strokes.push(String(ctx.strokeStyle))
    },
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 0,
    strokes,
  }
  return ctx
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

describe('ForceGraphView — playable-audio marker (PSY-1379)', () => {
  beforeEach(() => {
    forceGraphProps = null
  })
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('strokes the playable ring on nodes with has_playable_audio', () => {
    renderGraph()
    const ctx = makeFakeCtx()
    forceGraphProps.nodeCanvasObject(renderNode(1, true), ctx as unknown as CanvasRenderingContext2D)
    expect(ctx.strokes).toContain(PLAYABLE_RING_COLOR)
  })

  it('does NOT stroke the ring on nodes without playable audio', () => {
    renderGraph()
    const ctx = makeFakeCtx()
    forceGraphProps.nodeCanvasObject(renderNode(2, false), ctx as unknown as CanvasRenderingContext2D)
    expect(ctx.strokes).not.toContain(PLAYABLE_RING_COLOR)
  })
})
