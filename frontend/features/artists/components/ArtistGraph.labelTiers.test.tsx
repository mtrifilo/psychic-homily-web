import { describe, it, expect, vi, afterEach } from 'vitest'
import { act } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { ArtistGraph } from '../types'

// Tool-class tiered labels on the ego graph (artist page dialog +
// /graph Observatory both render ArtistGraphVisualization). The label pass
// dresses DOI/degree terciles in the locked 15/12/10 @ 600/500/400 ladder,
// counter-scaled by zoom; the CENTER is pinned to the top tier so its label
// stays largest. The mocked ForceGraph2D exposes `onRenderFramePost`; a mock
// 2D context records the font active at each fillText.

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let forceGraphProps: any = null

vi.mock('next/dynamic', () => ({
  default: () =>
    function ForceGraph2DStub(props: Record<string, unknown>) {
      forceGraphProps = props
      return <div data-testid="force-graph" />
    },
}))

// jsdom has no settled canvas coords; keep hover cheap and tooltip-free.
vi.mock('@/components/graph/nodeTooltip', async importOriginal => {
  const actual =
    await importOriginal<typeof import('@/components/graph/nodeTooltip')>()
  return {
    ...actual,
    nodeTooltipPlacement: () => null,
  }
})

vi.mock('next/link', () => ({
  default: ({ href, children }: { href: string; children: React.ReactNode }) => (
    <a href={href}>{children}</a>
  ),
}))

import { ArtistGraphVisualization } from './ArtistGraph'
import { TOOL_LABEL_TIERS } from '@/components/graph/graphLabels'

const graphData: ArtistGraph = {
  center: { id: 1, name: 'Center', slug: 'center', upcoming_show_count: 0 },
  nodes: [
    { id: 2, name: 'Second', slug: 'second', upcoming_show_count: 0 },
    { id: 3, name: 'Third', slug: 'third', upcoming_show_count: 0 },
  ],
  links: [
    { source_id: 1, target_id: 2, type: 'similar', score: 0.9, votes_up: 1, votes_down: 0 },
    { source_id: 1, target_id: 3, type: 'similar', score: 0.8, votes_up: 1, votes_down: 0 },
  ],
  user_votes: {},
}

// The satellite node shape the canvas hands to onNodeHover — hovering forces
// its label so it survives the collision cull at jsdom's all-(0,0) layout.
const satellite2 = {
  id: 2,
  name: 'Second',
  slug: 'second',
  upcoming_show_count: 0,
  isCenter: false,
  val: 4,
}

function paintLabels(globalScale: number) {
  const frame = forceGraphProps.onRenderFramePost as (
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

const renderViz = (doiByNodeId?: Map<number, number>, tiered = true) =>
  renderWithProviders(
    <ArtistGraphVisualization
      data={graphData}
      activeTypes={new Set(['similar'])}
      containerWidth={1024}
      doiByNodeId={doiByNodeId}
      labelTiers={tiered ? TOOL_LABEL_TIERS : undefined}
    />
  )

afterEach(() => {
  forceGraphProps = null
})

describe('ArtistGraph Tool-class tiered labels (PSY-1456)', () => {
  it('pins the center to the top tier (15px @ 600 — largest at rest)', () => {
    renderViz()
    const byText = Object.fromEntries(paintLabels(1).map(d => [d.text, d.font]))
    expect(byText['Center']).toBe('600 15px sans-serif')
  })

  it('counter-scales tier sizes by zoom (screen-px contract)', () => {
    renderViz()
    const byText = Object.fromEntries(paintLabels(2).map(d => [d.text, d.font]))
    expect(byText['Center']).toBe('600 7.5px sans-serif')
  })

  it('tiers satellites by degree tercile when no DOI map is supplied', () => {
    renderViz()
    // Force the satellite's label via hover (all nodes overlap at (0,0) in
    // jsdom, so only forced labels survive the cull). Degree rank over the
    // rendered set: center (Infinity) → tier 0; Second + Third tie at
    // degree 1 and share the tier at their median rank → tier 1.
    act(() => forceGraphProps.onNodeHover(satellite2))
    const byText = Object.fromEntries(paintLabels(1).map(d => [d.text, d.font]))
    expect(byText['Second']).toBe('500 12px sans-serif')
  })

  it('ranks tiers by DOI instead of degree when the host supplies it', () => {
    // DOI demotes Second to the bottom of the ranking: Third(0.9) → tier 0,
    // center(0.5) → tier 1 (moot — it is pinned to tier 0), Second(0.1) → tier 2.
    renderViz(new Map([[1, 0.5], [2, 0.1], [3, 0.9]]))
    act(() => forceGraphProps.onNodeHover(satellite2))
    const byText = Object.fromEntries(paintLabels(1).map(d => [d.text, d.font]))
    expect(byText['Second']).toBe('400 10px sans-serif')
    expect(byText['Center']).toBe('600 15px sans-serif')
  })

  it('keeps the legacy flat clamp + bold center when no ladder is passed (bill composition)', () => {
    renderViz(undefined, false)
    act(() => forceGraphProps.onNodeHover(satellite2))
    const byText = Object.fromEntries(paintLabels(1).map(d => [d.text, d.font]))
    expect(byText['Center']).toBe('700 11px sans-serif')
    expect(byText['Second']).toBe('400 11px sans-serif')
  })

  it('keeps the shared zoom gate: no labels at or below 0.7', () => {
    renderViz()
    expect(paintLabels(0.7)).toEqual([])
  })
})
