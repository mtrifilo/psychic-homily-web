import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { ArtistGraph } from '../types'

// PSY-1453: the ego graph retired its indigo/zinc palette for the shared
// color language — relationship-type chart-token fills (locked Option B),
// ink center/expanded rings, and the shared graphMarkers set. The visuals
// are canvas paints jsdom can't render, so this suite guards the glue the
// same way ForceGraphView.playableMarker.test.tsx does: capture the
// nodeCanvasObject prop and assert which styles it paints with.
//
// jsdom resolves no CSS tokens, so useGraphPalette returns the DARK
// fallback palette — the values asserted below are those fallbacks.

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let forceGraphProps: any = null

vi.mock('next/dynamic', () => ({
  default: () =>
    function ForceGraph2DStub(props: Record<string, unknown>) {
      forceGraphProps = props
      return <div data-testid="force-graph" />
    },
}))

import { ArtistGraphVisualization } from './ArtistGraph'
import {
  PLAYABLE_RING_COLOR,
  UPCOMING_SHOW_DOT_COLOR,
} from '@/components/graph/graphMarkers'

// Dark fallback palette values (see graphPalette.ts).
const CHART_1 = '#e89960' // bills
const CHART_8 = '#6db3a6' // radio
const OTHER = '#94A3B8'
const INK = '#eee7d9' // --foreground (dark)
const PRIMARY = '#e89960' // --primary (dark)
const CENTER_FILL = '#9c8c7c99' // --muted-foreground (dark) @ 60%

const graphData: ArtistGraph = {
  center: {
    id: 1,
    name: 'Diners',
    slug: 'diners',
    upcoming_show_count: 2,
    has_playable_audio: true,
  },
  nodes: [
    { id: 2, name: 'Gatecreeper', slug: 'gatecreeper', upcoming_show_count: 1, has_playable_audio: true },
    { id: 3, name: 'Snailmate', slug: 'snailmate', upcoming_show_count: 0 },
    { id: 4, name: 'Mystery', slug: 'mystery', upcoming_show_count: 0 },
  ],
  links: [
    { source_id: 1, target_id: 2, type: 'shared_bills', score: 0.9, votes_up: 0, votes_down: 0 },
    { source_id: 1, target_id: 3, type: 'radio_cooccurrence', score: 0.4, votes_up: 0, votes_down: 0 },
    { source_id: 1, target_id: 4, type: 'similar', score: 0.5, votes_up: 0, votes_down: 0 },
  ],
  user_votes: {},
}

const node = (id: number, overrides: Record<string, unknown> = {}) => ({
  id,
  name: `n${id}`,
  slug: `n${id}`,
  upcoming_show_count: 0,
  isCenter: false,
  val: 4,
  x: 0,
  y: 0,
  ...overrides,
})

// Fake 2D context recording the fillStyle/strokeStyle in effect at each
// fill()/stroke() call, plus dash state at stroke time.
function makeFakeCtx() {
  const fills: string[] = []
  const strokes: { style: string; lineWidth: number; dashed: boolean }[] = []
  let dash: number[] = []
  const ctx = {
    globalAlpha: 1,
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 0,
    shadowColor: '',
    shadowBlur: 0,
    beginPath() {},
    arc() {},
    moveTo() {},
    lineTo() {},
    save() {},
    restore() {},
    setLineDash(segments: number[]) {
      dash = segments
    },
    fill() {
      fills.push(String(ctx.fillStyle))
    },
    stroke() {
      strokes.push({ style: String(ctx.strokeStyle), lineWidth: ctx.lineWidth, dashed: dash.length > 0 })
    },
    fills,
    strokes,
  }
  return ctx
}

const INDIGO_ZINC = [
  '#6366f1',
  '#818cf8',
  '#a5b4fc',
  '#c7d2fe',
  'rgba(99, 102, 241, 0.3)',
  'rgba(63, 63, 70, 0.6)',
  'rgba(161, 161, 170, 0.5)',
]

function paint(nodeArg: ReturnType<typeof node>, extraProps: Record<string, unknown> = {}, globalScale = 2) {
  renderWithProviders(
    <ArtistGraphVisualization
      data={graphData}
      activeTypes={new Set(['shared_bills', 'radio_cooccurrence', 'similar'])}
      containerWidth={1024}
      {...extraProps}
    />,
  )
  const ctx = makeFakeCtx()
  forceGraphProps.nodeCanvasObject(nodeArg, ctx as unknown as CanvasRenderingContext2D, globalScale)
  return ctx
}

describe('ArtistGraphVisualization — shared palette adoption (PSY-1453)', () => {
  beforeEach(() => {
    forceGraphProps = null
  })
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('fills a bills neighbor with the chart-1 token color', () => {
    const ctx = paint(node(2))
    expect(ctx.fills[0]).toBe(CHART_1)
    expect(ctx.strokes[0]).toMatchObject({ style: CHART_1, lineWidth: 1 })
  })

  it('fills a radio neighbor with the chart-8 token color', () => {
    const ctx = paint(node(3))
    expect(ctx.fills[0]).toBe(CHART_8)
  })

  it('fills an out-of-family (similar) neighbor with the neutral grey', () => {
    const ctx = paint(node(4))
    expect(ctx.fills[0]).toBe(OTHER)
  })

  it('paints the center with a neutral fill and an ink ring — never a hue', () => {
    const ctx = paint(node(1, { isCenter: true, val: 8, upcoming_show_count: 2 }))
    expect(ctx.fills[0]).toBe(CENTER_FILL)
    expect(ctx.strokes[0]).toMatchObject({ style: INK, lineWidth: 2 })
  })

  it('strokes the shared violet playable ring iff has_playable_audio', () => {
    const playable = paint(node(2, { has_playable_audio: true }))
    expect(playable.strokes.map(s => s.style)).toContain(PLAYABLE_RING_COLOR)

    const silent = paint(node(3))
    expect(silent.strokes.map(s => s.style)).not.toContain(PLAYABLE_RING_COLOR)
  })

  it('draws the playable ring on the center too', () => {
    const ctx = paint(node(1, { isCenter: true, val: 8, has_playable_audio: true }))
    expect(ctx.strokes.map(s => s.style)).toContain(PLAYABLE_RING_COLOR)
  })

  it('fills the shared green dot for satellites with upcoming shows, not the center', () => {
    const satellite = paint(node(2, { upcoming_show_count: 3 }))
    expect(satellite.fills).toContain(UPCOMING_SHOW_DOT_COLOR)

    const center = paint(node(1, { isCenter: true, val: 8, upcoming_show_count: 3 }))
    expect(center.fills).not.toContain(UPCOMING_SHOW_DOT_COLOR)
  })

  it('marks an expanded satellite with the ink ring (the "opened hub" reads like the center)', () => {
    const ctx = paint(node(2), { expandedIds: new Set([2]) })
    expect(ctx.strokes[0]).toMatchObject({ style: INK, lineWidth: 2 })
  })

  it('strokes a dashed ink loading ring while a node expansion is in flight', () => {
    const ctx = paint(node(2), { expandingIds: new Set([2]) })
    expect(ctx.strokes).toContainEqual({ style: INK, lineWidth: 1.5, dashed: true })
  })

  it('restyles the suggested-direction hint to a DASHED PRIMARY ring + primary badge', () => {
    const ctx = paint(node(3), { suggestedIds: new Set([3]) })
    expect(ctx.strokes).toContainEqual({ style: PRIMARY, lineWidth: 1.5, dashed: true })
    expect(ctx.fills).toContain(PRIMARY)
  })

  it('paints no indigo/zinc remnants in any node state', () => {
    const states: Array<[ReturnType<typeof node>, Record<string, unknown>]> = [
      [node(1, { isCenter: true, val: 8, has_playable_audio: true, upcoming_show_count: 2 }), {}],
      [node(2, { has_playable_audio: true, upcoming_show_count: 1 }), { expandedIds: new Set([2]) }],
      [node(3), { expandingIds: new Set([3]) }],
      [node(4), { suggestedIds: new Set([4]) }],
    ]
    for (const [n, props] of states) {
      const ctx = paint(n, props)
      const styles = [...ctx.fills, ...ctx.strokes.map(s => s.style)]
      for (const banned of INDIGO_ZINC) {
        expect(styles).not.toContain(banned)
      }
    }
  })

  it('renders the canvas-foot type legend for the families present', () => {
    renderWithProviders(
      <ArtistGraphVisualization
        data={graphData}
        activeTypes={new Set(['shared_bills', 'radio_cooccurrence', 'similar'])}
        containerWidth={1024}
      />,
    )
    const legend = screen.getByTestId('ego-type-legend')
    // bills + radio families, plus the neutral bucket for the similar edge.
    expect(legend.textContent).toBe('billsradioother')
  })
})
