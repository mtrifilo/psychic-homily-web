import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderWithProviders } from '@/test/utils'

// PSY-1454: the labeled isolate shelf (locked grammar decision 4). Opt-in
// surfaces get a containment band (pre-frame pass) and a "+{N} not yet
// connected artists" caption (post-frame label pass) around the pinned
// shelf. {N} tracks the FILTERED view (post cluster hiding), the treatment
// renders at every zoom level, and surfaces that don't opt in — the homepage
// teaser (isolates already excluded, PSY-1444) and every other existing
// consumer — are byte-identical. jsdom has no real canvas, so we capture the
// frame callbacks at the ForceGraph2D boundary and drive them with a mock
// 2D context (staticLabels-test harness pattern).

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

// Two connected nodes + two isolates split across clusters, so a cluster
// hide can remove exactly one isolate from the rendered set.
const nodes: GraphNode[] = [
  {
    id: 1,
    name: 'Alpha',
    slug: 'alpha',
    upcoming_show_count: 0,
    cluster_id: 'c1',
  },
  {
    id: 2,
    name: 'Beta',
    slug: 'beta',
    upcoming_show_count: 0,
    cluster_id: 'c1',
  },
  // Isolates carry explicit coords (jsdom never runs the simulation) so
  // their labels would NOT collide with anything — any absence is the
  // labeled-shelf suppression, not the collision cull.
  {
    id: 3,
    name: 'Gamma',
    slug: 'gamma',
    upcoming_show_count: 0,
    cluster_id: 'c1',
    is_isolate: true,
    x: 200,
    y: 150,
  } as GraphNode,
  {
    id: 4,
    name: 'Delta',
    slug: 'delta',
    upcoming_show_count: 0,
    cluster_id: 'c2',
    is_isolate: true,
    x: 400,
    y: 150,
  } as GraphNode,
]

const renderGraph = (extraProps: Partial<ForceGraphViewProps> = {}) =>
  renderWithProviders(
    <ForceGraphView
      nodes={nodes}
      links={[{ source_id: 1, target_id: 2, type: 'similar' }]}
      clusters={[
        { id: 'c1', label: 'Cluster One', size: 3, color_index: 0 },
        { id: 'c2', label: 'Cluster Two', size: 1, color_index: 1 },
      ]}
      containerWidth={1024}
      ariaLabel="test graph"
      onNodeClick={() => {}}
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
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
    stroke: vi.fn(),
    font: '',
    textAlign: '',
    textBaseline: '',
    lineJoin: '',
    lineWidth: 0,
    strokeStyle: '',
    fillStyle: '',
  } as unknown as CanvasRenderingContext2D
}

type FrameFn = (ctx: CanvasRenderingContext2D, globalScale: number) => void

function paintFrame(globalScale: number) {
  const pre = h.lastProps.onRenderFramePre as FrameFn
  const post = h.lastProps.onRenderFramePost as FrameFn
  expect(typeof pre).toBe('function')
  expect(typeof post).toBe('function')
  const preCtx = makeCtx()
  const postCtx = makeCtx()
  pre(preCtx, globalScale)
  post(postCtx, globalScale)
  return { preCtx, postCtx }
}

function captionCalls(ctx: CanvasRenderingContext2D): string[] {
  return (ctx.fillText as ReturnType<typeof vi.fn>).mock.calls
    .map(c => c[0] as string)
    .filter(text => text.includes('not yet connected'))
}

beforeEach(() => {
  h.lastProps = {}
})

describe('ForceGraphView labeled isolate shelf (PSY-1454)', () => {
  it('stays off by default — non-opted surfaces draw no band and no caption', () => {
    renderGraph()
    const { preCtx, postCtx } = paintFrame(1)
    expect(preCtx.fillRect).not.toHaveBeenCalled()
    expect(preCtx.stroke).not.toHaveBeenCalled()
    expect(captionCalls(postCtx)).toEqual([])
  })

  it('draws the band + hairline in the pre-frame pass and the count caption in the label pass', () => {
    renderGraph({ showIsolateShelfLabel: true })
    const { preCtx, postCtx } = paintFrame(1)
    expect(preCtx.fillRect).toHaveBeenCalledTimes(1) // containment band
    expect(preCtx.stroke).toHaveBeenCalledTimes(1) // hairline top border
    expect(captionCalls(postCtx)).toEqual(['+2 not yet connected artists'])
  })

  it('counts the FILTERED view: hiding a cluster removes its isolate from the caption', () => {
    renderGraph({
      showIsolateShelfLabel: true,
      hiddenClusterIDs: new Set(['c2']),
    })
    const { postCtx } = paintFrame(1)
    expect(captionCalls(postCtx)).toEqual(['+1 not yet connected artist'])
  })

  it('renders band and caption at all zoom levels, including below the node-label gate', () => {
    renderGraph({ showIsolateShelfLabel: true })
    const { preCtx, postCtx } = paintFrame(LABEL_MIN_SCALE - 0.2)
    expect(preCtx.fillRect).toHaveBeenCalledTimes(1)
    // Node labels are gated out at this zoom, but the group caption is not.
    const drawn = (postCtx.fillText as ReturnType<typeof vi.fn>).mock.calls.map(
      c => c[0]
    )
    expect(drawn).toEqual(['+2 not yet connected artists'])
  })

  it('draws nothing when the rendered view has no isolates', () => {
    renderGraph({
      showIsolateShelfLabel: true,
      nodes: nodes.filter(n => !n.is_isolate),
    })
    const { preCtx, postCtx } = paintFrame(1)
    expect(preCtx.fillRect).not.toHaveBeenCalled()
    expect(captionCalls(postCtx)).toEqual([])
  })

  it('keeps isolate names hover-only on labeled-shelf surfaces (the caption names the group)', () => {
    renderGraph({ showIsolateShelfLabel: true })
    const { postCtx } = paintFrame(1)
    const drawn = (postCtx.fillText as ReturnType<typeof vi.fn>).mock.calls.map(
      c => c[0]
    )
    // Connected node labels draw (collision permitting); isolate labels do
    // not — jsdom leaves connected nodes at (0,0), so only the first of the
    // overlapping pair survives the cull, and no isolate name appears.
    expect(drawn).toContain('Alpha')
    expect(drawn).not.toContain('Gamma')
    expect(drawn).not.toContain('Delta')
  })

  it('labels isolates at rest on surfaces WITHOUT the labeled shelf (unchanged behavior)', () => {
    renderGraph()
    const { postCtx } = paintFrame(1)
    const drawn = (postCtx.fillText as ReturnType<typeof vi.fn>).mock.calls.map(
      c => c[0]
    )
    expect(drawn).toContain('Gamma')
    expect(drawn).toContain('Delta')
  })

  it('still resets the hull-painted frame flag alongside the band draw', () => {
    renderGraph({ showIsolateShelfLabel: true })
    const pre = h.lastProps.onRenderFramePre as FrameFn
    const ctx = makeCtx()
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ;(ctx as any).__forceGraphHullPainted = true
    pre(ctx, 1)
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    expect((ctx as any).__forceGraphHullPainted).toBe(false)
  })
})
