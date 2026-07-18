import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

// Degree-tiered label typography on Section-class surfaces. The
// `labelTiers` prop terciles the RENDERED node set by degree and dresses each
// tercile in the locked ladder (screen px, counter-scaled by zoom). These
// tests drive the captured `onRenderFramePost` with a mock 2D context (same
// harness as ForceGraphView.staticLabels.test) and read the font string at
// each fillText, pinning: ladder wiring, zoom counter-scaling, the flat
// legacy clamp when the prop is omitted, curated `nodeLabelStyles`
// precedence, and re-terciling after a cluster filter.

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
          graph2ScreenCoords: (x: number, y: number) => ({ x, y }),
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
import { SECTION_LABEL_TIERS } from './graphLabels'

// Positions far apart so no label is collision-culled: every tier is visible.
// (x/y are d3-force runtime fields, not part of the GraphNode payload type —
// the component spreads them through to its RenderNode.)
const nodes = [
  { id: 1, name: 'Hub', slug: 'hub', upcoming_show_count: 0, x: 0, y: 0 },
  { id: 2, name: 'Mid', slug: 'mid', upcoming_show_count: 0, x: 1000, y: 0 },
  { id: 3, name: 'Leaf', slug: 'leaf', upcoming_show_count: 0, x: 0, y: 800 },
] as unknown as GraphNode[]

// Distinct degrees so each node wears a different tier: Hub 3 (the 1-2 pair
// carries two typed links, both counted by degreeMap), Mid 2, Leaf 1.
const links = [
  { source_id: 1, target_id: 2, type: 'similar' },
  { source_id: 1, target_id: 2, type: 'shared_bills' },
  { source_id: 1, target_id: 3, type: 'similar' },
]

const renderGraph = (extraProps: Partial<ForceGraphViewProps> = {}) =>
  renderWithProviders(
    <ForceGraphView
      nodes={nodes}
      links={links}
      containerWidth={1024}
      ariaLabel="test graph"
      onNodeClick={() => {}}
      {...extraProps}
    />
  )

// Minimal 2d context that records the font active at each fillText — the
// tier assertion surface. measureText derives width from the current font px
// (jsdom canvas returns 0, which would defeat the collision pass).
function paintLabels(globalScale: number) {
  const frame = h.lastProps.onRenderFramePost as (
    ctx: CanvasRenderingContext2D,
    globalScale: number
  ) => void
  expect(typeof frame).toBe('function')
  const drawn: Array<{ text: string; font: string }> = []
  const ctx = {
    font: '',
    textAlign: '',
    textBaseline: '',
    lineJoin: '',
    lineWidth: 0,
    strokeStyle: '',
    fillStyle: '',
    save() {},
    restore() {},
    measureText(text: string) {
      const px = Number(/(\d+(?:\.\d+)?)px/.exec(this.font)?.[1] ?? 10)
      return { width: text.length * px * 0.6 }
    },
    strokeText() {},
    fillText(text: string) {
      drawn.push({ text, font: this.font })
    },
  }
  frame(ctx as unknown as CanvasRenderingContext2D, globalScale)
  return drawn
}

beforeEach(() => {
  h.lastProps = {}
})

describe('ForceGraphView degree-tiered labels (PSY-1456)', () => {
  it('dresses degree terciles in the Section ladder (14/11/9 @ 600/500/400)', () => {
    renderGraph({ labelTiers: SECTION_LABEL_TIERS })
    const byText = Object.fromEntries(paintLabels(1).map(d => [d.text, d.font]))
    expect(byText).toEqual({
      Hub: '600 14px sans-serif',
      Mid: '500 11px sans-serif',
      Leaf: '400 9px sans-serif',
    })
  })

  it('counter-scales tier sizes by zoom — a screen-px contract, not graph px', () => {
    renderGraph({ labelTiers: SECTION_LABEL_TIERS })
    const byText = Object.fromEntries(paintLabels(2).map(d => [d.text, d.font]))
    expect(byText['Hub']).toBe('600 7px sans-serif')
    expect(byText['Leaf']).toBe('400 4.5px sans-serif')
  })

  it('keeps the flat legacy clamp when no ladder is passed', () => {
    renderGraph()
    const fonts = new Set(paintLabels(1).map(d => d.font))
    expect(fonts).toEqual(new Set(['400 11px sans-serif']))
  })

  it('lets curated nodeLabelStyles win over the tier ladder per node', () => {
    renderGraph({
      labelTiers: SECTION_LABEL_TIERS,
      nodeLabelStyles: new Map([[3, { fontSize: 17, fontWeight: 600 }]]),
    })
    const byText = Object.fromEntries(paintLabels(1).map(d => [d.text, d.font]))
    expect(byText['Leaf']).toBe('600 17px sans-serif') // curated override
    expect(byText['Hub']).toBe('600 14px sans-serif') // tier ladder elsewhere
  })

  it('re-tiers over the RENDERED set when an edge type is hidden via the legend', () => {
    renderGraph({ labelTiers: SECTION_LABEL_TIERS, showEdgeLegend: true })
    // Hide the second Hub–Mid link (type shared_bills): Hub drops to degree
    // 2 and Mid ties with Leaf at 1 — the tie shares the mid tier instead of
    // Mid keeping its old solo rank.
    fireEvent.click(screen.getByTitle('Hide Shared Bills connections'))
    const byText = Object.fromEntries(paintLabels(1).map(d => [d.text, d.font]))
    expect(byText).toEqual({
      Hub: '600 14px sans-serif',
      Mid: '500 11px sans-serif',
      Leaf: '500 11px sans-serif',
    })
  })

  it('re-tiers over the RENDERED set when a cluster filter hides nodes (zero-degree floor)', () => {
    renderGraph({
      labelTiers: SECTION_LABEL_TIERS,
      nodes: nodes.map(node =>
        node.id === 1 ? { ...node, cluster_id: 'hidden-cluster' } : node
      ),
      clusters: [
        { id: 'hidden-cluster', label: 'Hidden', size: 1, color_index: 0 },
        { id: 'other', label: 'Other', size: 2, color_index: -1 },
      ],
      hiddenClusterIDs: new Set(['hidden-cluster']),
    })
    const byText = Object.fromEntries(paintLabels(1).map(d => [d.text, d.font]))
    // Hub (and every link — all touch it) is filtered out; Mid + Leaf drop
    // to degree 0 over the rendered set, and zero-degree nodes always wear
    // the bottom tier — never a rank-inflated hub dress.
    expect(byText).toEqual({
      Mid: '400 9px sans-serif',
      Leaf: '400 9px sans-serif',
    })
  })
})
